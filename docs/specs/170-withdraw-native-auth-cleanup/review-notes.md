# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-06-11T23:32:28Z -->

## Reviewed Scope

- Branch: claude/issue-170-impl-withdraw-native-auth-cleanup
- HEAD commit: 92220b8
- Compared to: $(git merge-base develop HEAD)..HEAD（merge-base: 9edfce4）
- 対象 commit 群: a0b2678 / 7c95ed8 / 73740ac / f5f9c95 / 13d2020 / 2f094e4 / 92220b8
- 検証実行: `go build ./...` / `go vet ./...` / `go test ./internal/user/ ./internal/repository/ ./internal/app/` すべて pass
- Feature Flag Protocol: `opt-out`（細目チェックは適用外）

## Verified Requirements

- **1.1**（退会時に当該ユーザーの一時認可コードをすべて削除）
  - 実装: `internal/user/service.go` の `withdrawTx` 手順 4（sessions 直後）に
    `s.txAuthCodeDeleter.DeleteByUserIDTx(ctx, tx, userID)` を挿入。
    `PostgresAuthCodeRepo.DeleteByUserID` / `DeleteByUserIDExec` が
    `DELETE FROM auth_codes WHERE user_id = $1` を実行
  - テスト: `TestService_Withdraw_Tx_CommitsOnSuccess`（順序検証で `auth_codes` を含む）/
    `TestPostgresAuthCodeRepo_DB/DeleteByUserID_当該userのみ削除し他userに影響しない` /
    `TestWithdrawIntegration_NativeAuthCleanup`（Commit 後 auth_codes=0 件を COUNT で検証）

- **1.2**（退会時に refresh token と rotation family をすべて削除）
  - 実装: `withdrawTx` 手順 5 に `s.txRefreshTokenDeleter.DeleteByUserIDTx` を挿入。
    `PostgresRefreshTokenRepo.DeleteByUserIDExec` が
    `DELETE FROM refresh_token_families WHERE user_id = $1` を発行し、配下 refresh_tokens は
    FK ON DELETE CASCADE で連動削除
  - テスト: `TestService_Withdraw_Tx_CommitsOnSuccess`（順序検証で `refresh_token_families` を含む）/
    `TestPostgresRefreshTokenRepo_DB/DeleteByUserIDExec_共有txでCommitすると当該userのfamily+tokensが消えて他userは残る` /
    `TestWithdrawIntegration_NativeAuthCleanup`（refresh_token_families と refresh_tokens の双方が 0 件を COUNT で検証）

- **2.1**（既存退会フローと同一の原子性境界内で実行）
  - 実装: 新規 2 段は既存と同じ `tx` を引き回して呼び出される。新規 tx 開始なし。
    `withdraw_wiring.go` の 2 アダプタは `querierFromTx(tx)` 経由で repository の Exec 変種を呼ぶ
  - テスト: `TestService_Withdraw_Tx_CommitsOnSuccess`（単一 tx 上で 6 段 → Commit 1 回）/
    `TestWithdrawIntegration_NativeAuthCleanup`（共有 tx で sessions → auth_codes →
    refresh_token_families → users の順に削除して Commit）

- **2.2**（途中失敗で退会全体を失敗させ、それまでの削除を確定しない）
  - 実装: 既存と同じ `%w` wrap + 即 return → `defer Rollback` が全戻し
  - テスト: `TestService_Withdraw_Tx_RollsBackOnAuthCodeDeleteError`（auth_codes 失敗時に
    Commit 未到達 + refresh_token_families / user 未呼び出しを検証）/
    `TestService_Withdraw_Tx_RollsBackOnRefreshTokenDeleteError`（refresh_token 失敗時に
    user 削除未到達 + Rollback を検証）

- **2.3**（native auth 削除より後の段階で失敗時に native auth 削除を確定しない）
  - 実装: defer Rollback により users 削除 / Commit 失敗時も新規 2 段が未確定に戻る
  - テスト: `TestWithdrawIntegration_NativeAuthRollback`（共有 tx で 3 段削除後に
    Rollback → 3 テーブルすべて残存を COUNT で検証）/
    `TestService_Withdraw_Tx_RollsBackOnRefreshTokenDeleteError`（中間失敗で先行削除も未確定）

- **3.1**（他ユーザーの認証状態を削除も失効もしない）
  - 実装: 削除 SQL の `WHERE user_id = $1` スコープ
  - テスト: `TestPostgresAuthCodeRepo_DB/DeleteByUserID_当該userのみ削除し他userに影響しない` /
    `TestPostgresRefreshTokenRepo_DB/DeleteByUserIDExec_共有txでCommitすると当該userのfamily+tokensが消えて他userは残る` /
    `TestWithdrawIntegration_NativeAuthCleanup`（bystander user の 3 テーブル + sessions が
    残存することを検証）

- **NFR 1.1**（退会 API の応答形式不変）
  - 実装: handler / adapter / `Withdraw` の signature・戻り値型は不変。エラー wrap も既存と
    同形式（`fmt.Errorf("...: %w", err)`）
  - テスト: 既存 handler テストが無変更で green。
    `TestService_Withdraw_Tx_NilNativeAuthDeletersSkip`（nil 注入時の従来動作維持）

- **NFR 1.2**（退会以外の native auth 操作の挙動を変更しない）
  - 実装: `AuthCodeRepository` interface に `DeleteByUserID` を追加（Create / FindByHash /
    MarkUsed は不変）。`RefreshTokenRepository` interface は不変。
    `PostgresRefreshTokenRepo.DeleteByUserID` は `DeleteByUserIDExec(ctx, r.db, userID)` への
    委譲化のみ（SQL・エラー message 不変の挙動等価 refactor）
  - 既存 repository テスト・user テストの未変更箇所が green

- **NFR 1.3**（データ移行・保存構造の変更なしに導入）
  - 実装: migration 追加なし（File Structure Plan に migration ファイルが含まれない）。
    FK ON DELETE CASCADE は #164 で導入済みの保存構造を温存

- **NFR 2.1**（退会完了後に当該ユーザーの認可コード・refresh token・rotation family が
  残存しないことを明示的に検証）
  - テスト: `TestWithdrawIntegration_NativeAuthCleanup` が Commit 後に
    `SELECT COUNT(*) FROM auth_codes / refresh_token_families / refresh_tokens WHERE user_id = $1`
    の 3 件すべてを 0 で assert（refresh_tokens は family CASCADE 経路の期待挙動を明文化）

## Findings

なし。確認した観点は以下のとおり。

### 確認した観点

1. **削除順序の規約遵守**: design.md「削除順序の決定」と service.go の挿入位置が完全一致
   （手順 4: auth_codes / 手順 5: refresh_token_families / 手順 6: users）。
   `TestService_Withdraw_Tx_CommitsOnSuccess` の `want` 配列が 6 段を順に検証している
2. **トランザクション境界**: `withdrawTx` 内で `s.txBeginner.BeginTx` の戻り値 `tx` を 4〜5 段に
   そのまま渡しており、新規 tx 開始や別 tx 利用は無い。`withdraw_wiring.go` のアダプタも
   `querierFromTx(tx)` 経由で同一 `*sql.Tx` を再利用
3. **nil ガード方針**: 既存 `txStateDeleter` 等と同型の `if s.txAuthCodeDeleter != nil` /
   `if s.txRefreshTokenDeleter != nil` のガードで段スキップ。本番 wiring（`app.go`）では
   `&txAuthCodeDeleterAdapter{repo: authCodeRepo}` / `&txRefreshTokenDeleterAdapter{repo: refreshTokenRepo}`
   を常に注入するため、本番では skip されない
4. **boundary 逸脱**: design.md File Structure Plan に列挙されたファイル
   （`internal/repository/interfaces.go` / `postgres_auth_code_repo.go` /
   `postgres_refresh_token_repo.go` / `postgres_auth_code_repo_db_test.go` /
   `postgres_refresh_token_repo_db_test.go` / `internal/user/service.go` /
   `internal/user/service_test.go` / `internal/app/withdraw_wiring.go` / `internal/app/app.go`）
   と diff 内変更ファイル群が一致。task 3 の追加分として明示された
   `internal/repository/postgres_withdraw_integration_db_test.go` の新設・
   `internal/app/withdraw_wiring_test.go` の compile-time interface check 拡張も
   impl-notes.md「design からの逸脱」節で事前に開示済みで、tasks.md task 3 の
   `_Boundary:_` 内（repository / app）に閉じている
5. **handler / adapter 変更なし**: `internal/handler/` 配下に diff は無く、退会 API の
   応答形式は不変（NFR 1.1）
6. **既存メソッドの挙動等価性**: `PostgresRefreshTokenRepo.DeleteByUserID` は SQL 文・
   エラー message を保ったまま `DeleteByUserIDExec` に委譲化されており、既存 DB 結合テスト
   （`DeleteByUserID_...` サブテスト群）が無変更で green
7. **インターフェース分離の維持**: `AuthCodeRepository` interface に `DeleteByUserID` を
   追加したのみで、`auth.AuthCodeCreator`（#165）/ `auth.AuthCodeConsumer`（#166 設計）の
   狭い IF を持つ既存利用側は影響を受けない
8. **テスト規約**: Arrange / Act / Assert の 3 パート分離、`<対象>: <条件>のとき<期待結果>`
   形式の命名、table-driven ではなく `t.Run` サブテストでの 1 テスト = 1 検証対象、
   いずれも CLAUDE.md の Go テスト規約に従っている

## Summary

design.md / tasks.md の指針に厳密に従い、退会 tx に native auth 削除 2 段を sessions 直後・
users 前に挿入する実装が完了している。全 numeric ID（Req 1.1〜1.2 / 2.1〜2.3 / 3.1 /
NFR 1.1〜1.3 / NFR 2.1）に対し、対応する実装と単体テスト・DB 結合テストが確認できた。
boundary 逸脱なし、`go build` / `go vet` / `go test ./internal/{user,repository,app}/` すべて pass。

RESULT: approve
