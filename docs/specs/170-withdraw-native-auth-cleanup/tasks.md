# Implementation Plan

- [ ] 1. repository: ユーザー単位削除の追加（AuthCodeRepository 拡張 + DBTX 対応）
  - `internal/repository/interfaces.go`: `AuthCodeRepository` に
    `DeleteByUserID(ctx context.Context, userID string) error` を追加（doc comment は
    `RefreshTokenRepository.DeleteByUserID` と対になる文面。冪等・CASCADE 併存の旨を明記）
  - `internal/repository/postgres_auth_code_repo.go`: `DeleteByUserID`（`r.db` で Exec 変種へ
    委譲）+ `DeleteByUserIDExec(ctx, q DBTX, userID)`（`DELETE FROM auth_codes WHERE user_id = $1`）
    を `PostgresSessionRepo` の 2 段パターンで追加。エラー message に user_id の値を含めない
  - `internal/repository/postgres_refresh_token_repo.go`: `DeleteByUserIDExec` を追加し、
    既存 `DeleteByUserID` を委譲化（SQL・エラー message 不変の挙動等価 refactor。
    `RefreshTokenRepository` interface は変更しない）
  - `internal/repository/postgres_auth_code_repo_db_test.go` /
    `postgres_refresh_token_repo_db_test.go`: 対象ユーザーのみ削除・他ユーザー残存・
    0 件冪等・tx 参加（Rollback で取り消し）のサブテストを追加。既存 `DeleteByUserID`
    サブテストが無変更で green であることを確認
  - _Requirements: 1.1, 1.2, 3.1, NFR 1.2, NFR 1.3_

- [ ] 2. user: 退会トランザクションへの native auth 削除統合
  - `internal/user/service.go`: `TxAuthCodeDeleter` / `TxRefreshTokenDeleter` を追加し、
    `NewServiceWithTx` の引数を 2 つ拡張。`withdrawTx` の sessions 削除直後（users 削除前）に
    nil ガード付きの削除 2 段（認可コード → refresh token）を挿入する。既存手順
    （item_states / subscriptions / sessions / users）の順序・エラー処理・ログは不変とし、
    `Withdraw` の doc comment の削除順序記述を更新する
  - `internal/user/service_test.go`: 呼び出し順（sessions 後・users 前・同一 tx）/
    各 deleter 失敗時の Rollback と Commit 未到達 / nil deleter スキップ（従来どおり成功）の
    各ケースを既存 fake / `txRecorder` パターンで追加
  - _Requirements: 1.1, 1.2, 2.1, 2.2, 2.3_
  - _Depends: 1_

- [ ] 3. app wiring と統合検証
  - `internal/app/withdraw_wiring.go`: `txAuthCodeDeleterAdapter` /
    `txRefreshTokenDeleterAdapter` を追加（`querierFromTx` → `DeleteByUserIDExec` の既存
    アダプタ同型）し、`newTxUserService` の引数と compile-time interface checks を拡張
  - `internal/app/app.go`: `repository.NewPostgresRefreshTokenRepo(db)` を構築（#166 実装が
    先行 merge 済みで構築済みの場合は既存構築を共用し重複させない）し、`newTxUserService`
    に authCodeRepo と合わせて渡す
  - `internal/repository` の DB 結合テストに退会フロー相当の統合ケースを追加: 共有 tx で
    sessions → auth_codes → refresh_token_families → users を削除して Commit した後、
    当該ユーザーの native auth 3 テーブルが 0 件・他ユーザーの行が残存することを検証
  - 既存の退会 API テスト（handler）・既存統合テストが無変更で green であることを確認
  - _Requirements: 2.1, 3.1, NFR 1.1, NFR 1.2, NFR 2.1_
  - _Depends: 2_

## Verify

本 spec の実装後、watcher（stage-a-verify gate）が再実行すべき verify コマンドを以下の
構造化ブロックで宣言する。DB 結合テストは `TEST_DATABASE_URL` 未接続時に `t.Skip` され、
unit テスト + 静的解析のみで green 判定される（CI でも同条件）。

<!-- stage-a-verify -->
```sh
go build ./... && go vet ./... && go test ./...
```
