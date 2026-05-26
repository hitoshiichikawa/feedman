# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-05-26T00:00:00Z -->

## Reviewed Scope

- Branch: claude/issue-47-impl--handler-feed-subscription
- HEAD commit: 2b50ffb76a192bf55a064bd07a785c8f11bbf3ba
- Compared to: develop..HEAD

本 Issue は design-less impl（design.md / tasks.md なし）。requirements.md の numeric ID と
実装コード・追加テストを突き合わせて判定した。CLAUDE.md の Feature Flag Protocol は opt-out の
ため flag 観点は適用しない。diff は test ファイル 4 本（internal/handler / internal/feed ×2 /
internal/subscription）と spec markdown 2 本のみで、実装コード（非テスト）の変更は 0 件。

## Verified Requirements

- 1.1 — `internal/handler/feed_handler_test.go`: `TestMapAPIErrorToHTTPStatus_UnknownCode_ReturnsInternalServerError` / `TestMapAPIErrorToHTTPStatus_EmptyCode_ReturnsInternalServerError` が未マップ/空コードの default 分岐で HTTP 500 を検証（実装 feed_handler.go:291 default 分岐に対応）
- 1.2 — 同ファイル `TestMapAPIErrorToHTTPStatus_KnownCodes` が既知 14 コードの個別マッピングを table-driven で検証し、default 系（1.1）と別関数で区別
- 2.1 — `internal/feed/detector_test.go`: `TestNewFeedDetector_SSRFGuardEnabled_UsesSafeClient` がガード有効時 `NewSafeClient` 1 回呼出を検証（detector.go:71-72 の `ssrfGuard != nil` 分岐に対応）
- 2.2 — 同ファイル `TestNewFeedDetector_SSRFGuardDisabled_UsesPlainClient` がガード nil 時に素クライアント（`detectorTimeout`）が選択される経路を検証
- 2.3 — `internal/feed/favicon_test.go`: `TestNewFaviconFetcher_SSRFGuardEnabled_UsesSafeClient` がガード有効時 `NewSafeClient` 1 回呼出を検証（favicon.go:53-54 分岐に対応）
- 2.4 — 同ファイル `TestNewFaviconFetcher_SSRFGuardDisabled_UsesPlainClient` がガード nil 時に素クライアント（`faviconTimeout`）が選択される経路を検証
- 3.1 — `internal/subscription/service_test.go`: `TestService_Unsubscribe_NilItemStateRepo_SkipsItemStateDelete` が itemStateRepo=nil 時に記事状態削除をスキップし購読削除が成功することを検証（service.go:166 の nil ガードに対応）
- 3.2 — 同ファイル `TestService_Unsubscribe_ItemStateDeleteError_PropagatesError` が DeleteByUserAndFeed のエラーを `errors.Is` で wrap 伝播確認し、後続の購読 Delete が呼ばれないことも検証（service.go:167-169）
- 4.1 — 同ファイル `TestService_ResumeFetch_NotStopped_ReturnsFeedNotStoppedAndDoesNotUpdate` が active フィードに対し `FEED_NOT_STOPPED` 専用エラー（*model.APIError）を返し UpdateFetchState を呼ばないことを検証（service.go:202-203）
- NFR 1.1 — diff stat 上、実装コード（非テストファイル）の変更は 0 件。テストファイルと spec markdown のみ
- NFR 1.2 — `go test ./internal/handler/ ./internal/feed/ ./internal/subscription/` 全 ok（reviewer 再実行で green 確認）。追加 9 関数すべて -v で PASS を確認
- NFR 2.1 — 追加テストはいずれも期待挙動が読み取れる関数名 + Arrange/Act/Assert コメント構造
- NFR 2.2 — HTTP 経路は httptest + mockSSRFGuard、nil 分岐・純粋ロジックは実物で検証

## Findings

なし

## Summary

requirements.md の全 numeric AC（1.1, 1.2, 2.1〜2.4, 3.1, 3.2, 4.1）に対応する追加テストが存在し、
いずれも実装の該当分岐と整合して PASS。実装コード変更なしで NFR 1.1 を満たし、boundary 逸脱なし。

RESULT: approve
