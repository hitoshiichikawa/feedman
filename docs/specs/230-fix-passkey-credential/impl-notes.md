# 実装ノート #230 — 新規パスキー登録 finish の users / passkey_credentials INSERT を単一 tx 化する

## サマリー

`internal/passkey/registration_service.go` の `FinishRegistrationNew` が users
INSERT と passkey_credentials INSERT を別段階で確定していた実装バグを、既存
`internal/user/service.go` の `withdrawTx` パターンに揃えて単一トランザクションで
実行するよう修正した。credential 側の失敗（`ErrCredentialAlreadyRegistered`・
任意のインフラ障害）が発生した場合は tx rollback により users 行が永続化されず、
逆に user 側の失敗（`ErrUsernameTaken` race）でも credential 側 INSERT を発行せず
rollback する。#216 design.md L797〜802 の「登録 finish: users INSERT と
passkey_credentials INSERT の合成成功を保証する（1 tx）」トランザクション境界を
実装バグとして修復した形（新規の設計判断は含まない）。

## 変更ファイル一覧

### プロダクションコード

- `internal/repository/postgres_user_repo.go` — `CreateUserOnlyExec(ctx, q DBTX, u)`
  を追加し、既存 `CreateUserOnly` は `CreateUserOnlyExec(ctx, r.db, u)` に委譲する
  形へ集約。INSERT SQL / `ErrUsernameTaken` マッピング / COALESCE デフォルトの
  ロジックは `*Exec` 側に単一で持ち、コピペ重複を作らない
- `internal/repository/postgres_passkey_credential_repo.go` — `CreateExec(ctx, q, c)`
  を追加し、既存 `Create` は `CreateExec(ctx, r.db, c)` に委譲。
  `ErrCredentialAlreadyRegistered` マッピングは `*Exec` 側に集約
- `internal/passkey/registration_service.go` —
  - `UserWriter` に `CreateUserOnlyExec(ctx, q repository.DBTX, u *model.User) error` を追加
  - `PasskeyCredentialWriter` に `CreateExec(ctx, q repository.DBTX, c *model.PasskeyCredential) error` を追加
  - 新規 interface `RegistrationTx` (`Querier() / Commit() / Rollback()`) と
    `RegistrationTxBeginner` (`BeginTx(ctx) (RegistrationTx, error)`) を追加
  - `RegistrationService` struct に `txBeginner RegistrationTxBeginner` を追加、
    `NewRegistrationService` シグネチャに tx beginner 依存を注入（旧引数の後・`now` の前）
  - `FinishRegistrationNew` を tx オーケストレーションへ書き換え
    （`withdrawTx` と同じ `committed = false` / `defer rollback` パターン）
  - `BeginRegistrationNew` / `BeginAddCredential` / `FinishAddCredential` は無変更
    （追加登録は Out of Scope）
- `internal/app/withdraw_wiring.go` — `passkeyRegistrationTxBeginnerAdapter`
  （`*repository.SQLTxBeginner` → `passkey.RegistrationTxBeginner` 適合）と
  `newPasskeyRegistrationTxBeginner` ヘルパを追加。既存 `txBeginnerAdapter` は
  `user.Tx`（Querier を持たない）用のため流用不可
- `internal/app/app.go` — `passkey.NewRegistrationService` 呼び出しに
  `newPasskeyRegistrationTxBeginner(txBeginner)` を注入（退会 tx と同じ
  `*repository.SQLTxBeginner` を共有）

### テスト

- `internal/passkey/registration_service_test.go`
  - `stubUserWriter` に `CreateUserOnlyExec` フックと `createUserOnlyExecCalled` / `lastCreateExecQuerier` を追加
  - `stubCredentialWriter` に `CreateExec` フックと `createExecCalled` / `lastCreateExecQuerier` を追加
  - `stubRegistrationTx` / `stubRegistrationTxBeginner` / `stubDBTX` を追加（NFR 4.1）
  - `newRegistrationServiceFixture` の戻り値を 6 タプルに拡張し、tx beginner stub を露出
  - `FinishRegistrationNew` の既存ケースを Exec 呼び出しに移し替え、tx.Commit / tx.Rollback
    の呼び出し回数と `tx.Querier()` と同一の DBTX が渡ることを新規に検証
  - 追加テストケース: credential インフラ障害での rollback（Req 1.4）、BeginTx 自体の失敗、
    challenge 早期拒否時に BeginTx が呼ばれないこと
  - `FinishAddCredential` / `BeginRegistrationNew` / `BeginAddCredential` の既存
    テスト観点は変更なし（fixture の戻り値タプル更新のみ）
- `internal/repository/postgres_passkey_registration_tx_db_test.go`（新規）
  - `TestPasskeyRegistrationTx_CredentialDuplicateRollsBackUser`（AC 1.3 / NFR 4.1）：
    別 user 所有の credential_id を新規登録相当 tx で衝突させ、Rollback 後に新規 user 行が
    users 表に永続化されないことを検証
  - `TestPasskeyRegistrationTx_UserRaceRollsBackCredential`（AC 1.5 / NFR 4.3）：
    先行 user と同一 username_normalized で `CreateUserOnlyExec` が `ErrUsernameTaken`
    を返す状況で、後続 tx を Rollback しても先行 user 側に想定外の credential 行が
    残らないことを検証
  - `TestPasskeyRegistrationTx_HappyPathCommitsBoth`（AC 1.1 / NFR 4.4）：
    tx 上で user → credential → Commit の順で両行が永続化されることを検証
  - DB URL 未設定・接続不可時は `t.Skip` する。既存
    `postgres_withdraw_integration_db_test.go` の `setupWithdrawTestDB` / `countByUserID`
    / `insertTestUserForWithdraw` を再利用（共有ヘルパの重複を作らない）
- `internal/handler/passkey_e2e_db_test.go` — E2E DB テストの
  `passkey.NewRegistrationService` 呼び出しに real DB 由来 `*repository.SQLTxBeginner` を
  `e2ePasskeyRegTxBeginner` 経由で注入（app 側 wiring を handler パッケージ内から
  循環せず組む）

## AC → 実装 / テストの対応

### Requirement 1（原子性）

| AC | 実装場所 | 対応テスト |
|---|---|---|
| 1.1（成功時: 両方永続化） | `FinishRegistrationNew` の `CreateUserOnlyExec` → `CreateExec` → `tx.Commit`、`committed = true` | `internal/passkey/registration_service_test.go` — 「成功: user 行と credential 行を単一 tx で作成し Commit / userID を返す」／ DB: `TestPasskeyRegistrationTx_HappyPathCommitsBoth` |
| 1.2（後続認証で解決可能） | Commit で users / passkey_credentials の合成永続化を保証 | DB: `TestPasskeyRegistrationTx_HappyPathCommitsBoth`（credential_id 逆引きで対応 user 解決を確認） |
| 1.3（credential 重複 → user 未永続化） | `CreateExec` が `ErrCredentialAlreadyRegistered` を返すと defer rollback → `ErrRegistrationFailed` | Unit: 「credential 重複 (ErrCredentialAlreadyRegistered) は tx rollback + ErrRegistrationFailed」／ DB: `TestPasskeyRegistrationTx_CredentialDuplicateRollsBackUser` |
| 1.4（credential インフラ障害 → user 未永続化） | `CreateExec` が任意 error を返すと wrap + defer rollback | Unit: 「credential インフラ障害 (任意の error) は tx rollback + wrap して返す」 |
| 1.5（user race → credential 未永続化） | `CreateUserOnlyExec` が `ErrUsernameTaken` を返した時点で credential INSERT を発行せず defer rollback | Unit: 「CreateUserOnlyExec の UNIQUE 衝突 (username race) は tx rollback + ErrRegistrationFailed / credential は永続化されない」／ DB: `TestPasskeyRegistrationTx_UserRaceRollsBackCredential` |
| 1.6（部分永続化を残さない包括宣言） | 全失敗経路で defer rollback + uniform 拒否 | 上記 1.3〜1.5 の rollback アサーション、および malformed challenge / attestation 失敗ケースで `BeginTx` 自体が呼ばれないことの確認 |

### Requirement 2（API 契約維持）

- `NewRegistrationService` の外部シグネチャは変更したが、呼び出し側は wiring 3 箇所
  （app.go / handler の E2E test / unit test）のみで、外部 HTTP 契約
  （リクエスト body / レスポンス JSON / ステータス）は無変更
- `ErrRegistrationFailed` への uniform 化（credential 重複・username race・attestation
  失敗）は既存 `handler` 層の応答マッピングと変わらず、拒否時 HTTP status / error code
  は #216 で公開済みのまま
- **担保テスト**: `internal/handler/passkey_e2e_db_test.go` の
  `TestE2E_PasskeyFullFlow_DBBacked` が本修正後も同シグネチャの
  `NewRegistrationService` 経由で登録 → 認証 → 保護 API 到達までを緑で通す
  （AC 2.1 / 2.4）。Unit test 群で ErrRegistrationFailed の uniform 化を継続的に
  検証（AC 2.2 / 2.3）

### NFR

- **NFR 1.1**: `FinishRegistrationNew` は全経路で「ユーザーと credential のいずれか
  一方だけが永続化された状態」を作らない。tx 未開始（早期拒否）→ 何も INSERT しない、
  tx 開始後失敗 → defer rollback、成功 → Commit
- **NFR 1.2**: 事前チェック（begin 段の pre-check）だけでなく finish 段の永続化
  本体でも tx 境界により部分 commit を作らない
- **NFR 2.1**: 拒否ログは既存 `logRejection` を維持（NFR 3.1 準拠）
- **NFR 2.2**: 追加した error wrap（`failed to begin registration transaction: %w`
  / `failed to commit registration transaction: %w`）は username / credential_id 生値を
  含まない
- **NFR 3.1**: マイグレーション追加なし・既存テーブル構造変更なし。データ移行不要
- **NFR 3.2**: 既存 Native Auth Contract Tests / #216 テストスイートは無変更で green
  （`go test ./...` の handler / app / passkey 各パッケージが引き続き pass）
- **NFR 4.1 / 4.2 / 4.3 / 4.4**: 上記 AC 1.3〜1.5 / 1.1 の対応テストで担保。DB 統合
  テストは `TEST_DATABASE_URL` 未設定時に `t.Skip`

## 設計判断

### 1. 「新規 interface / adapter を追加」対「既存 interface を拡張」

既存 `user.Tx` / `user.TxBeginner` は `Commit / Rollback` のみを持ち `Querier()` が
公開されていないため、passkey 側では独自の `RegistrationTx` / `RegistrationTxBeginner`
を新設した。理由:

- `user.Tx` に `Querier()` を後付けすると `withdraw_wiring.go` の
  `querierFromTx(tx)` の hidden downcast が不要になる一方、user パッケージが
  repository の `DBTX` を interface に露出させることになる。責務境界的に passkey 側の
  ローカル concern を user 側に持ち込むのは避けたい
- 別 interface を切ることで、将来 passkey 側の tx 要件が変わっても user 側の tx 契約に
  影響を与えない（interface segregation / CLAUDE.md §5）

### 2. `*Exec` 実装は既存 `Create` / `CreateUserOnly` に委譲

`postgres_user_repo.go` / `postgres_passkey_credential_repo.go` の既存 `Create` /
`CreateUserOnly` を `*Exec` 版に委譲する形にしたのは、既存呼び出し側
（`internal/handler/passkey_e2e_db_test.go` / 追加登録経路 `FinishAddCredential` /
既存の非 tx test 群）が引き続き `Create` / `CreateUserOnly` を使い続けられるため
（シグネチャ後方互換）。INSERT SQL と 23505 → sentinel マッピングは `*Exec` 側に
単一で持ち、コピペ重複は生まない。

### 3. tx beginner adapter を app パッケージに置く

`repository` パッケージは `passkey` を import できない（passkey → repository の
依存関係のため）ことから、両者を結ぶアダプタは既存 `withdraw_wiring.go` と同じく
`internal/app/` 側に置いた。E2E test（`internal/handler/passkey_e2e_db_test.go`）は
handler パッケージ内から app パッケージを import すると循環が発生するため、
handler test 側に同構造の最小アダプタ `e2ePasskeyRegTxBeginner` を配置した
（本番配線は app 側、テストは handler test 側で minimal 化）。

### 4. `BeginTx` 失敗時のエラーポリシー

`BeginTx` 自体の失敗はインフラ障害（DB 到達不能等）に相当するため、`ErrRegistrationFailed`
への uniform 化ではなく wrap して返す設計にした（handler 側で 500）。理由:
tx が開始できない状況は user race / credential 重複とは性質が異なる（クライアントが
再試行しても即座には解消しない）ため、既存 `FinishRegistrationNew` の infra error 経路
（例: `failed to consume registration challenge`）と同じ扱いに揃えた。

### 5. `FinishAddCredential` は tx 不要（Out of Scope 明記）

追加登録 finish は credential 単体 INSERT で、対応する user 行は既存の permanent
アカウントの再利用であるため、Issue #230 の「孤立ユーザーを作らない」問題は発生
しない。requirements.md の Out of Scope 節に明記された通り、本 Issue では
`FinishAddCredential` に変更を加えていない。

## 確認事項（Architect / PM への差し戻し提案なし）

- なし。#216 の既存 requirements / design に矛盾する解釈は行っておらず、
  design.md L797〜802 の「登録 finish: users INSERT と passkey_credentials INSERT の
  合成成功を保証する（1 tx）」に対する実装バグ修復に閉じたスコープで実装した

## 検証結果

- `go build ./...`: pass
- `go vet ./...`: pass
- `gofmt -l ./internal/passkey ./internal/repository ./internal/app internal/handler/passkey_e2e_db_test.go`:
  対象ファイルはすべて gofmt 準拠（既存の他パッケージに残るフォーマット差分は本
  修正の対象外）
- `go test ./...`: 全パッケージ green（DB 統合テスト 3 件は DB 未接続環境で t.Skip
  として PASS 扱い）
- 新規 `TestRegistrationService_FinishRegistrationNew` 9 サブテストすべて pass
  （成功時 Commit / challenge 期限切れ Skip / attestation 失敗 Skip / user race
  Rollback / credential 重複 Rollback / credential インフラ障害 Rollback /
  BeginTx 失敗 wrap / malformed challenge / malformed session）

## STATUS 行（orchestrator 抽出用 / 末尾行）

STATUS: complete
