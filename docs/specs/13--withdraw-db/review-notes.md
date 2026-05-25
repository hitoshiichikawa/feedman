# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-05-25T10:22:02Z -->

## Reviewed Scope

- Branch: claude/issue-13-impl--withdraw-db
- HEAD commit: 1ee633de1ec32e8f2c46dfc0bdafdf8fbea4a506
- Compared to: 4595bde..HEAD

本 Issue には `tasks.md` が存在しないため、`_Boundary:_` アノテーションの代わりに
`requirements.md` の Out of Scope（feeds/items 削除・復旧・論理削除化・シグネチャ/削除順序/CASCADE 変更）を
境界基準として照合した。Feature Flag Protocol は CLAUDE.md で `opt-out` 宣言のため
flag 観点の確認は行わず、通常の 3 カテゴリ判定のみを適用した。`go test ./...` を実行し全パッケージ pass を確認。

## Verified Requirements

- 1.1 — `internal/user/service.go` `withdrawTx` が item_states/subscriptions/sessions/user を全削除。`TestService_Withdraw_Tx_CommitsOnSuccess`（order 検証）/ `TestService_Withdraw`（legacy）で担保
- 1.2 — `PostgresUserRepo.DeleteByIDExec` が `DELETE FROM users` を実行（identities/user_settings の CASCADE は既存 DB スキーマ責務、本変更で未改変）。user 削除呼び出しを `TestService_Withdraw_Tx_CommitsOnSuccess` で検証
- 1.3 — 削除対象は 4 テーブルのみで feeds/items を含めない。`rec.order` が `[item_states, subscriptions, sessions, user]` のみであることを検証
- 1.4 — `TestService_Withdraw_Tx_CommitsOnSuccess` で `err == nil` を検証
- 2.1 — `withdrawTx` の defer rollback + commit-only-on-success 制御フロー。`TestService_Withdraw_Tx_RollsBackOnDeleteError`（rolledBack==true / committed==false）で検証
- 2.2 — 単一トランザクション + rollback により退会前状態を維持。同テストで失敗後の後続 user 削除が呼ばれないこと + rollback 発生を検証
- 2.3 — `fmt.Errorf("...: %w", err)` で wrap し呼び出し元へ伝播。`TestService_Withdraw_Tx_RollsBackOnDeleteError` / `_CommitError` / `_BeginError` で `errors.Is` 検証
- 3.1 — `withdrawTx` が `user == nil` で `model.NewUserNotFoundError()` を返す。`TestService_Withdraw_Tx_UserNotFound`（ErrCodeUserNotFound）/ `TestService_Withdraw_UserNotFound`（legacy）
- 3.2 — 存在確認をトランザクション開始前に実施。`TestService_Withdraw_Tx_UserNotFound`（beginCalled==false / committed==false / 削除 0 件）
- 4.1 — `TestService_Withdraw_Tx_NoRelatedData`（`err == nil`）。0 件削除でもエラーにならない制御フロー
- 4.2 — 同テストで committed==true / user 削除呼び出しを検証
- NFR 1.1 — `Withdraw(ctx, userID) error` シグネチャ不変。旧 `NewService` を温存し新規 `NewServiceWithTx` を追加。既存 handler テスト群が green
- NFR 1.2 — 削除対象 4 テーブル + CASCADE 集合を不変に維持。`rec.order` で検証
- NFR 1.3 — 成功時の最終削除結果（同一テーブル群削除済み）が不変
- NFR 2.1 — 子→親の削除順序を `TestService_Withdraw_Tx_CommitsOnSuccess` で厳密検証
- NFR 3.1 — `withdrawTx` が FindByID 後に `slog.Info("退会処理を開始します", user_id)` を 1 件出力
- NFR 3.2 — Commit 成功後に `slog.Info("退会処理が完了しました", user_id)` を 1 件出力

## Findings

なし

## Summary

全 AC（1.1〜4.2）および NFR（1.1〜3.2）に対応する実装とテストが存在し、コミット/ロールバック/削除順序/トランザクション未開始（UserNotFound）/関連データ 0 件境界を単体テストで検証している。シグネチャ・削除順序・CASCADE 対象・旧逐次パスはいずれも温存され後方互換性が保たれている。CASCADE と実 DB ロールバックの結合検証は Out of Scope として別 Issue 化が impl-notes に明記されており、本 AC（User Service の制御フロー）のカバレッジ判定には影響しない。`go test ./...` 全 pass。3 カテゴリ（AC 未カバー / missing test / boundary 逸脱）いずれにも該当しない。

RESULT: approve
