# 実装ノート: Issue #170 退会時の native auth state cleanup

## 実装サマリ

Issue #170 の「退会フローにおける native auth 認証状態の cleanup」を、design.md / tasks.md の
指針に厳密に従って実装した。退会トランザクション（`user.Service.withdrawTx`）の sessions 削除
直後・users 削除前に、当該ユーザーの auth_codes / refresh_token_families の明示削除 2 段を
挿入し、既存退会フローと同一の原子性境界（単一 tx / 全成功時のみ Commit）で実行する。

設計の主要ポイントは原文どおり踏襲している:

- 削除順序: item_states → subscriptions → sessions → **auth_codes →
  refresh_token_families** → users（新 2 段は sessions 直後・users 前に挿入。
  既存手順の順序・エラー処理・ログは不変）
- refresh_tokens は `refresh_token_families` の DELETE に伴い FK ON DELETE CASCADE で
  連動削除される（family 単位の DELETE 1 文。#164 の保存構造をそのまま利用）
- repository は `PostgresSessionRepo` と同じ **2 段パターン**（`DeleteByUserID` が
  `DeleteByUserIDExec(ctx, q DBTX, userID)` へ委譲）で DBTX 対応。エラー message に
  user_id の値を含めない
- service 層の deleter は **nil ガード付き**（nil なら当該段スキップ。既存 deleter 群と
  同一方針。本番 wiring では常に非 nil を注入）
- レガシー（非 tx）パスには明示削除を追加せず、users 削除時の FK CASCADE による DB 防衛線で
  行残存を防ぐ（design.md どおり。SERVER.md §1.5 の「自動削除でも担保されるが明示推奨」の
  明示側は tx パスで実装）

## タスクとコミット対応表

| Task | 内容 | 実装コミット | 進捗 commit |
|---|---|---|---|
| 1 | repository: ユーザー単位削除の追加（AuthCodeRepository 拡張 + DBTX 対応） | a0b2678 `feat(repository)` | 7c95ed8 `docs(tasks): mark 1` |
| 2 | user: 退会トランザクションへの native auth 削除統合 | 73740ac `feat(user)` | f5f9c95 `docs(tasks): mark 2` |
| 3 | app wiring と統合検証 | 13d2020 `feat(app)` | 2f094e4 `docs(tasks): mark 3` |

実装順序は tasks.md の番号順そのままで、`_Depends:_` の制約（task 2 → 1、task 3 → 2）は
自然に満たされた。

## 受入基準と担保テスト

### Requirement 1: 退会時の native auth 認証状態の削除

| AC ID | 担保テスト |
|---|---|
| 1.1 (認可コードをすべて削除) | `user.TestService_Withdraw_Tx_CommitsOnSuccess`（auth_codes 段の呼び出し順検証）/ `repository.TestWithdrawIntegration_NativeAuthCleanup`（DB 結合: Commit 後 auth_codes 0 件）/ `repository` の `DeleteByUserID_当該userのみ削除し他userに影響しない` |
| 1.2 (refresh token と rotation family をすべて削除) | 同上（refresh_token_families 段）+ `DeleteByUserIDExec_共有txでCommitすると当該userのfamily+tokensが消えて他userは残る`（FK CASCADE で tokens 連動削除を検証） |

### Requirement 2: 退会フローへの統合と原子性

| AC ID | 担保テスト |
|---|---|
| 2.1 (同一の原子性境界内で実行) | `user.TestService_Withdraw_Tx_CommitsOnSuccess`（単一 tx 上で 6 段が順に実行・Commit 1 回）/ `repository.TestWithdrawIntegration_NativeAuthCleanup`（共有 tx で sessions → auth_codes → families → users を削除して Commit） |
| 2.2 (native auth 削除失敗で全体失敗・確定しない) | `user.TestService_Withdraw_Tx_RollsBackOnAuthCodeDeleteError` / `_RollsBackOnRefreshTokenDeleteError`（Rollback 実行・Commit 未到達・後段未呼び出しの fail-fast 検証） |
| 2.3 (後段失敗時に native auth 削除を確定しない) | `repository.TestWithdrawIntegration_NativeAuthRollback`（共有 tx で native auth 削除後に Rollback → 全行残存を検証）/ `user.TestService_Withdraw_Tx_RollsBackOnRefreshTokenDeleteError`（中間失敗で先行削除も未確定） |

### Requirement 3: 他ユーザーへの非影響

| AC ID | 担保テスト |
|---|---|
| 3.1 (他ユーザーの認証状態を削除も失効もしない) | `repository` DB 結合テストの他ユーザー残存検証（auth_codes / families+tokens の双方）+ `TestWithdrawIntegration_NativeAuthCleanup` の他ユーザー行残存検証 |

### Non-Functional Requirements

| NFR | 担保テスト・実装 |
|---|---|
| NFR 1.1 (退会 API の応答形式不変) | `Withdraw` のシグネチャ・エラー wrap 形式は不変（追加 2 段も既存と同じ `fmt.Errorf("...: %w")`）。既存 handler テストが無変更で green / `user.TestService_Withdraw_Tx_NilNativeAuthDeletersSkip`（nil 注入でも従来どおり成功） |
| NFR 1.2 (退会以外の native auth 操作を変更しない) | `AuthCodeRepository` への追加は `DeleteByUserID` のみ。既存メソッド・既存 `DeleteByUserID`（refresh 側）は委譲化のみで SQL・エラー message 不変。既存テスト無変更 green |
| NFR 1.3 (データ移行・保存構造の変更なし) | migration 追加なし（FK CASCADE は #164 で定義済みの保存構造を利用） |
| NFR 2.1 (残存しないことを明示的に検証) | `repository.TestWithdrawIntegration_NativeAuthCleanup`（退会フロー相当の削除を Commit 後、native auth 3 テーブルが 0 件であることを直接 COUNT 検証） |

## 検証結果

- `go build ./...`: 成功
- `go vet ./...`: 警告なし
- `go test ./...`: 全パッケージ pass（2026-06-12 引き継ぎセッションで再確認済み）
- `gofmt -l <変更ファイル群>`: 出力なし（フォーマット差分なし）

DB 結合テスト（`postgres_auth_code_repo_db_test.go` / `postgres_refresh_token_repo_db_test.go` /
`postgres_withdraw_integration_db_test.go` の追加分）は `TEST_DATABASE_URL` 未設定時に skip
する既存設計で、CI では postgres サービス上で実行される。

## design からの逸脱

無し。design.md の以下は **完全にそのまま** 実装した。

- File Structure Plan に列挙された全ファイル（変更 7 ファイル。これに加え task 3 指定の
  退会フロー統合 DB テストを `internal/repository/postgres_withdraw_integration_db_test.go`
  として新設、compile-time interface checks の拡張で `withdraw_wiring_test.go` を更新）
- `TxAuthCodeDeleter` / `TxRefreshTokenDeleter` の最小 IF（`DeleteByUserIDTx` 1 メソッド）と
  adapter 同型パターン（`querierFromTx` → `DeleteByUserIDExec`）
- Requirements Traceability の手順 4〜5 挿入位置（sessions 直後・users 前）
- 新規 migration なし・レガシーパス無変更・`NewService`（非 tx コンストラクタ）無変更

## 実装上の判断

### 既存テストへの nil 注入

`NewServiceWithTx` の引数拡張に伴い、既存の tx パステスト 5 件には native auth deleter を
`nil` で注入した（各テストの検証対象が native auth 段と無関係なため。コメントで理由を明記）。
nil ガード経路自体は専用の `TestService_Withdraw_Tx_NilNativeAuthDeletersSkip` が検証する。

### 削除順序: auth_codes → refresh_token_families

design.md の手順番号どおり認可コード → refresh token の順とした。両テーブル間に FK は無く
順序の機能的制約は無いが、トレーサビリティのため design の記述順を維持した。

## 追加した依存

無し（go.mod / go.sum 変更なし）。

## 引き継ぎメモ

- task 3（app wiring / 13d2020）は前任セッションが developer エージェント停止後に
  build / vet / test green を確認してコミットしたもの。本引き継ぎセッションで全 diff を
  再検証し、Reviewer も通常どおり全 diff をレビューしている。
- **Issue #168 (revoke)**: `POST /api/auth/revoke` は本 spec と独立（退会は物理削除、revoke
  は failure 失効）。本 spec の `DeleteByUserIDExec` 2 段パターンは #168 では使用しない。

## 確認事項（PR 本文転載用）

- 設計矛盾は無し。
- レガシー（非 tx）パスは design.md どおり明示削除を追加していない（本番 wiring は常に
  tx パス。レガシーパスは FK CASCADE が防衛線）。
- 退会フロー統合 DB テストは service 層を経由せず repository レイヤの共有 tx 操作で
  退会相当の削除列を再現している（tasks.md task 3 の指定どおり）。

STATUS: complete
