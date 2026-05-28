# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-05-25T00:00:00Z -->

## Reviewed Scope

- Branch: claude/issue-13-impl--withdraw-db
- HEAD commit: e0177a3f70a882a5688ba27d7ffb23c97e9f3f6d
- Compared to: develop..HEAD

本 Issue には `tasks.md` が存在しないため（Architect 非経由）、`_Boundary:_` アノテーションの
代わりに `requirements.md` の Out of Scope（feeds/items 削除・復旧・論理削除化・シグネチャ/
削除順序/CASCADE 変更・退会 API レスポンス変更）を境界基準として照合した。Feature Flag Protocol は
CLAUDE.md で `opt-out` 宣言のため flag 観点の確認は行わず、通常の 3 カテゴリ判定のみを適用した。
`go test`（user/repository/app）/ `go build ./...` / `go vet` を実行し pass を確認。

## Verified Requirements

- 1.1 — `internal/user/service.go` `withdrawTx` が単一 tx 上で item_states → subscriptions → sessions → user を全削除し成功時 Commit / `TestService_Withdraw_Tx_CommitsOnSuccess`（order 検証）, `TestService_Withdraw`（legacy）
- 1.2 — `PostgresUserRepo.DeleteByIDExec` が `DELETE FROM users` を実行（identities/user_settings の CASCADE は DB スキーマ責務で本変更では不変）/ `TestService_Withdraw_Tx_CommitsOnSuccess`（user 削除呼び出しを検証）
- 1.3 — `withdrawTx` の削除対象が 4 テーブルのみで feeds/items 削除呼び出しを持たない / `rec.order == [item_states, subscriptions, sessions, user]` で残存を担保
- 1.4 — `TestService_Withdraw_Tx_CommitsOnSuccess`（`err == nil` を検証）
- 2.1 — `withdrawTx` の `defer` ロールバック + 途中失敗で early return / `TestService_Withdraw_Tx_RollsBackOnDeleteError`（rolledBack==true / committed==false）, `TestService_Withdraw_Tx_BeginError`
- 2.2 — 単一トランザクション + Rollback により退会前状態を維持 / `TestService_Withdraw_Tx_RollsBackOnDeleteError`（失敗後に user 削除が呼ばれないこと + ロールバックを検証）
- 2.3 — `fmt.Errorf("...: %w", err)` でエラーを wrap し返却 / `TestService_Withdraw_Tx_RollsBackOnDeleteError` / `_CommitError` / `_BeginError`（いずれも `errors.Is` で wrap 検証）
- 3.1 — `withdrawTx` が `FindByID` で nil を検出し `model.NewUserNotFoundError()` を返す / `TestService_Withdraw_Tx_UserNotFound`（`ErrCodeUserNotFound`）, `TestService_Withdraw_UserNotFound`（legacy）
- 3.2 — 存在確認をトランザクション開始前に実施し未検出時は `BeginTx` を呼ばない / `TestService_Withdraw_Tx_UserNotFound`（beginCalled==false / committed==false / 削除 0 件）
- 4.1 — `TestService_Withdraw_Tx_NoRelatedData`（関連 0 件でも `err == nil`）
- 4.2 — `TestService_Withdraw_Tx_NoRelatedData`（committed==true / user 削除呼び出しを検証）
- NFR 1.1 — `Withdraw(ctx, userID) error` シグネチャ不変（旧 `NewService` 温存 + 新規 `NewServiceWithTx` 追加。`go build ./...` 成功、既存テスト群 green）
- NFR 1.2 / 1.3 — 削除対象 4 テーブル + CASCADE 集合を維持 / `rec.order` で検証
- NFR 2.1 — 子→親の削除順序を `TestService_Withdraw_Tx_CommitsOnSuccess` で厳密検証
- NFR 3.1 — `withdrawTx` で `slog.Info("退会処理を開始します", user_id)` を出力（実装上で観測可能）
- NFR 3.2 — `withdrawTx` で Commit 成功後に `slog.Info("退会処理が完了しました", user_id)` を出力（実装上で観測可能）

## Findings

なし

## Summary

全 AC（Req 1〜4 / NFR 1〜3）に対応する `withdrawTx` の単一トランザクション原子化実装と単体テスト
（commit / rollback / begin error / commit error / UserNotFound / 関連データ 0 件 / 子→親削除順序）が
確認できた。repository 層の `*Exec` バリアントは旧メソッドに `r.db` を委譲する等価リファクタで
legacy パスも温存され、後方互換（NFR 1.1〜1.3）を満たす。Out of Scope（feeds/items 削除・シグネチャ・
エンドポイント変更・論理削除化）への逸脱はなく、boundary 逸脱なし。`go test`（user/repository/app）/
`go build ./...` / `go vet` も pass。CASCADE と実 DB ロールバックの結合検証は Out of Scope として
別 Issue 化が impl-notes に明記済みで、本 AC（User Service の制御フロー）カバレッジ判定には影響しない。
3 カテゴリ（AC 未カバー / missing test / boundary 逸脱）いずれにも該当しない。

RESULT: approve
