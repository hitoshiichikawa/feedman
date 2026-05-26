# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-05-26T18:42:00Z -->

## Reviewed Scope

- Branch: claude/issue-18-impl-registerfeed-favicon-writetimeout
- HEAD commit: 02cc6c6088664d7b0227fa102d68e37cb48c4e1e
- Compared to: develop..HEAD

注: 本 Issue は design-less impl（`tasks.md` / `design.md` 不在）。`_Boundary:_` アノテーションが
存在しないため、boundary 判定は変更が要件の対象コンポーネント（Feed Service 層）に閉じているかで
確認した。CLAUDE.md の Feature Flag Protocol 採否は `opt-out` のため flag 観点は適用しない。

## Verified Requirements

- 1.1 — `internal/feed/service.go:141` で `startFaviconFetch` 起動後すぐ `return feed, sub, nil`。`TestRegisterFeed_ReturnsBeforeFaviconCompletes`（favicon を block したまま即時返却を検証）
- 1.2 — 同上。favicon 取得時間を応答に加算しない。`TestRegisterFeed_ReturnsBeforeFaviconCompletes`（応答 1 秒未満を assert）
- 1.3 — favicon は独立 goroutine 実行。レスポンス送出をブロックしない。`TestRegisterFeed_ReturnsBeforeFaviconCompletes`
- 1.4 / NFR 1.1, 1.2 — 即時返却により WriteTimeout(15s) と無関係。`TestRegisterFeed_ReturnsBeforeFaviconCompletes`
- 2.1, 2.4 — 取得エラー時も登録成功・null 保持（DB 更新せず）。`TestRegisterFeed_SucceedsWhenFaviconFetchErrors` / 既存 `TestFeedService_RegisterFeed_FaviconFetchFailure`
- 2.2 — タイムアウト時も登録成功。`backgroundFaviconTimeout` 機構 + 即時返却（`TestRegisterFeed_ReturnsBeforeFaviconCompletes` / `TestStartFaviconFetch_AppliesTimeoutDeadline`）
- 2.3 — 未検出（data==nil）時 null 保持。`TestRegisterFeed_SucceedsWhenFaviconNotFound`
- 2.5 / NFR 2.1 — `internal/feed/service.go:259-270` の `fetchAndSaveFavicon` 内 `slog.Warn`/`slog.Info`（feedID/siteURL 付き）。テスト実行ログで出力確認
- 3.1, 3.2 — 成功時 `UpdateFavicon` 経由で DB 保存（後続 `GetFeed` で参照可）。`TestRegisterFeed_FaviconSavedAsynchronously` / 既存 `TestFeedService_RegisterFeed_FaviconSavedOnSuccess`
- 3.3 — `context.WithoutCancel(ctx)` で独立 context 化（`service.go:157`）。`TestRegisterFeed_FaviconContinuesAfterRequestCtxCanceled`（リクエスト ctx キャンセル後も完了・保存、呼び出し時 ctx 未キャンセルを検証）
- 4.1 — `backgroundFaviconTimeout = 30 * time.Second`（`service.go:23`）。`TestBackgroundFaviconTimeout_IsBounded`（≤30s）/ `TestStartFaviconFetch_AppliesTimeoutDeadline`（context デッドライン設定）
- 4.2 — `context.WithTimeout` で fetcher に渡す ctx にデッドライン付与（上限到達で ctx キャンセル）。`TestStartFaviconFetch_AppliesTimeoutDeadline`
- 4.3 — `defer cancel()`（`service.go:165`）でリソース解放。`go test ./internal/feed/... -race` clean（goroutine リーク・競合なし）
- 5.1, 5.3 / NFR 3.1 — `RegisterFeed` シグネチャ・戻り値不変、handler 層（`internal/handler/`）未変更。レスポンス形式 feed/subscription 維持
- 5.2 — handler 層未変更により 201 Created 維持。既存 handler テストで担保

## Findings

なし

## Summary

全 numeric requirement ID（1.x / 2.x / 3.x / 4.x / 5.x / NFR）について、Feed Service 層の非同期化実装と
対応テストでカバレッジを確認した。変更は `internal/feed/service.go` および同パッケージのテストに閉じており、
handler 層・WriteTimeout 設定は不変で後方互換を保つ。`go test ./internal/feed/... -race -count=1` 全 PASS を
独立に再実行して確認した。

RESULT: approve
