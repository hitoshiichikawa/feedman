# Implementation Notes

Issue #241「パスキー登録した username がどこにも表示されない」の per-task 実装ログ。

## Implementation Notes

### Task 1

- **採用方針**: `RegistrationService.FinishRegistrationNew` 内の `newUser := &model.User{...}` 構築箇所に `Name: normalized` を 1 行追加し、既存の Issue #230 の 1 tx オーケストレーション（`BeginTx → CreateUserOnlyExec → CreateExec → Commit`）境界内で users.name を初期化する。追加の tx 制御・新規パッケージ・新規 interface は導入しない。
- **重要な判断**:
  - Rollback 経路 regression（Req 1.3）は既存の失敗系サブテスト（credential 重複 / infra 障害 / session factory 失敗）が既に `commitCalled=0` / `rollbackCalled=1` を検証済みで、Name 追加のための追加 assert は構造的に不要と判断（該当 3 サブテストに 3 行のコメントで意図を明示するに留めた）。tasks.md / design.md も同判断と整合。
  - DB-backed regression（`postgres_passkey_registration_tx_db_test.go` の `TestPasskeyRegistrationTx_HappyPathCommitsBoth`）は「1 行追加」の指示だったが、既存 fixture の `newUser` が Name を未設定のままだったため、service 層の契約（Name: normalized）を repository 側でも再現する意図で fixture 側にも `Name: normalized` を追加した（Assert 前提を成立させるため 1 行追加）。design.md の意図に反せず、`_Requirements: 1.1_` の DB-backed 二重化を満たす。
  - Google OAuth 経路（`internal/auth/service.go::HandleCallback` / `PostgresUserRepo.CreateWithIdentity`）は今回の変更対象外（Req 4.3 / NFR 3.1）で、diff 上も本 commit で当該ファイルは変更していないことを `git diff --stat` で確認済み。
- **残存課題**: なし（Task 1 スコープ内で完結）。

## 実行結果

- `go vet ./internal/passkey/... ./internal/repository/...`: no findings
- `go test ./internal/passkey/... ./internal/repository/...`: すべて pass（`TestPasskeyRegistrationTx_HappyPathCommitsBoth` は PostgreSQL に接続できないためローカル env では SKIP。CI での実 DB 実行で検証される想定）
- `go test ./...`: すべて pass（既存動線への回帰なし）

## 受入基準 (Task 1 対応分) 追跡

- **Req 1.1**（finish 成功時 `users.name` を保存 username と同値で初期化）:
  - Unit test: `TestRegistrationService_FinishRegistrationNew/成功: ...` の `users.lastCreated.Name == pendingUsername` assert
  - DB-backed regression: `TestPasskeyRegistrationTx_HappyPathCommitsBoth` の `found.Name == normalized` assert
- **Req 1.2**（登録成功時 username / username_normalized を変更しない）:
  - 既存 assert (`users.lastCreated.UsernameNormalized != pendingUsername` 検証) が引き続き pass。本修正は INSERT 値 1 つを追加するのみで既存 username / username_normalized の代入行は無変更（`git diff` で確認）
- **Req 1.3**（finish 失敗時に users 行を永続化しない）:
  - 既存失敗サブテスト 3 種（session factory 失敗 / credential 重複 / infra 障害）の `commitCalled=0` / `rollbackCalled=1` 検証が Name を含む users 行の非永続化を構造的に保証（追記コメントで意図を明示）
- **Req 1.4**（Name 初期化を users 行作成と同一トランザクション内で実施）:
  - `Name: normalized` は `CreateUserOnlyExec(ctx, tx.Querier(), newUser)` の引数 `newUser` に含まれるため、既存の 1 tx オーケストレーション内で INSERT される（tx 制御コードは 1 行も追加していない）
- **Req 4.3**（本修正で Google 由来ユーザーの users 行を書き換えない）:
  - `internal/auth/service.go` は本 commit で変更していない（`git diff --stat` で確認）
- **NFR 2.2**（ログに username 生値を含めない）:
  - `logRejection` / `shortID` の既存契約は本修正で無変更（`registration_service.go` の変更は `newUser` 構築 1 箇所のみ）
- **NFR 3.1**（本修正前に登録済みユーザーの users.name を自動更新しない）:
  - migration script / UPDATE 文を追加していない（INSERT の VALUES 1 つを追加するのみ）

## 確認事項

なし。tasks.md / design.md / requirements.md の task 1 スコープ内には矛盾を発見しなかった。
