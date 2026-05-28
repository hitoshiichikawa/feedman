# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-05-28T14:55:00Z -->

## Reviewed Scope

- Branch: claude/issue-122-impl-rss-favicon
- HEAD commit: 53bf082405708d59d5f27f2127b749621e71a88f
- Compared to: develop..HEAD
- spec ディレクトリには `requirements.md` と `impl-notes.md` のみが存在し、`tasks.md` /
  `design.md` は存在しない（design-less impl）。Boundary 判定は requirements.md の
  Feed Service / Feed List UI スコープと CLAUDE.md のコンポーネント境界
  （`internal/feed/` バックエンド・`web/src/components/` フロントエンド）に照らして実施した。
- CLAUDE.md `## Feature Flag Protocol` の `**採否**:` 行は `opt-out` を確認。flag 観点の
  細目判定は適用しない。

## Verified Requirements

- 1.1 — `TestFetchFaviconWithFallback_StageC_SiteOriginICOSucceeds` /
  `TestFetchFaviconWithFallback_StageD_SiteOriginHTMLSucceeds`（記事リンクから推測した
  サイト本体オリジンで favicon 再取得が試行されることを確認）
- 1.2 — `internal/feed/service.go:278` `feedRepo.UpdateFavicon(ctx, feedID, data, mimeType)`
  により、いずれの段階で取得した favicon も同じ経路で永続化される。`StageC_SiteOriginICOSucceeds`
  が data 返却を、既存 `service_test.go` が永続化呼び出しを検証
- 1.3 — `TestFetchFaviconWithFallback_NoArticles_StopsAfterStageB`（記事リンク 0 件の
  RSS で `siteHit.Load()` が false であることを assert）
- 1.4 — `TestFetchFaviconWithFallback_AllStagesFail_ReturnsNil`（全段階 NotFound 時に
  `data == nil` を assert）+ 既存 `TestRegisterFeed_SucceedsWhenFaviconNotFound` で
  null 永続化経路を担保
- 1.5 — 既存 `TestRegisterFeed_ReturnsBeforeFaviconCompletes`（service_async_test.go）が
  バックグラウンド処理であることを担保。本 PR で `mockFaviconFetcher` /
  `controllableFaviconFetcher` / `deadlineObservingFetcher` に `FetchFaviconWithFallback`
  メソッドが追加され、既存非同期テストが引き続き green
- 2.1 — `TestFetchFaviconWithFallback_StageA_FeedOriginICOSucceeds` /
  `StageC_SiteOriginICOSucceeds`（feed origin → site origin の順序を hit フラグで確認）
- 2.2 — `StageA_FeedOriginICOSucceeds` / `StageB_FeedOriginHTMLSucceeds` /
  `StageC_SiteOriginICOSucceeds` で後続段階の hit フラグが false であることを assert
- 2.3 — `internal/feed/favicon.go:223,226,238,243,248,251,189,206,368` 等で
  `slog.Info("favicon取得: 成功", "stage", ...)` / `slog.Info("favicon取得: 段階失敗", ...)`
  により段階・URL・成功可否が構造化ログに出力される
- 2.4 — `TestParseFaviconURLFromHTML_*` 8 件（IconAbsolute / RelativeResolved /
  ShortcutIcon / AppleTouchIcon / Priority* / NoMatch / IgnoreOutsideHead / EmptyHref /
  InvalidBaseURL）+ `StageB_FeedOriginHTMLSucceeds` / `StageD_SiteOriginHTMLSucceeds`
- 3.1 — `web/src/components/feed-list.test.tsx`
  「faviconがnullの場合に代替アイコン（fallback）を表示すること」
- 3.2 — `web/src/components/feed-list.test.tsx`
  「favicon画像の読み込みに失敗した場合に代替アイコンに切り替わること」
  （`fireEvent.error(img)` で onError 発火 → fallback 表示を assert）
- 3.3 — `web/src/components/feed-list.tsx:122,136,138` で fallback 表示要素の外側 div /
  内側 span / Rss アイコンすべてに `w-4 h-4` クラスを付与。実 favicon の `<img>` も
  `w-4 h-4`。同一サイズ・配置領域を CSS class で担保
- 3.4 — `web/src/components/feed-list.test.tsx`
  「代替アイコン表示時もフィードタイトル・未読数バッジ・ステータスアイコンのレイアウトを
  維持すること」（sub-2 行で fallback / タイトル / ステータス が同行に並ぶことを assert）
- 4.1 — `TestFetchFaviconWithFallback_StageA_FeedOriginICOSucceeds`（従来経路で取得できれば
  後続段階を試行しない）+ `TestFetchFaviconWithFallback_SameHostArticleLink_NoStageCDExtraFetch`
  （配信ドメインと記事リンクオリジンが同一なら段階 c/d を skip）
- 4.2 — `internal/feed/service.go:142` の `startFaviconFetch` 呼び出しは新規登録経路
  （`RegisterFeed` 内のみ）であり、既存永続化フィードへの再取得経路は導入されていない
- 4.3 — `web/src/components/feed-list.test.tsx` の既存テスト群（favicon あり時の `<img>`
  描画、選択状態、未読数、ステータス）は impl-notes 記載のとおり 218 passed で全て green
- NFR 1.1 — `TestFetchFaviconWithFallback_SSRFGuardBlocksAll`（`blockAll=true` で
  外部 HTTP リクエスト 0 件を assert）。実装では favicon 取得 (`FetchFavicon`)、
  HTML 取得 (`fetchHTML`)、フィード再取得 (`deriveSiteOriginFromFeed`) いずれも
  `ssrfGuard.ValidateURL` を経由
- NFR 1.2 — `faviconTimeout = 5 * time.Second` / `maxFaviconSize = 2MB` /
  `maxHTMLFetchSize = 2MB` / `maxFeedFetchSizeForFavicon = 5MB`。呼び出し側で
  `io.LimitReader` と長さ検査により厳密に上限を強制
- NFR 1.3 — `isImageMime` で画像 MIME 判定を統一。HTML は href 抽出のみで画面に出さない
- NFR 2.1 — `slog.Info("favicon取得: 成功", "stage", "feed_origin_ico" 等, "url", "mime")`
  を含む構造化ログ
- NFR 3.1 — `model.Feed` / `repository.FeedRepository.UpdateFavicon` のシグネチャ・
  スキーマは変更されていない（diff 未検出）
- NFR 3.2 — 既存 favicon ありフィードの表示は `showImage = hasURL && !imgFailed` が
  true のとき従来通り `<img>` を描画するため挙動不変

## Findings

なし

## Summary

要件 1〜4 および NFR 1〜3 に紐づくすべての AC について、`internal/feed/favicon.go` /
`internal/feed/service.go` / `web/src/components/feed-list.tsx` の実装と、新規追加された
`internal/feed/favicon_fallback_test.go`（637 行・段階 a〜d 統合テスト + HTML パーサ単体）
/ `web/src/components/feed-list.test.tsx`（要件 3.1〜3.4 をカバーする 4 ケース追加）
で必要な観測可能挙動が確認できた。design-less impl のため tasks.md `_Boundary:_` は
存在しないが、requirements.md スコープ（Feed Service / Feed List UI）と CLAUDE.md の
コンポーネント境界（`internal/feed/` / `web/src/components/`）の範囲内に変更が閉じており
boundary 逸脱なし。Feature Flag Protocol は opt-out 宣言のため flag 観点の細目判定は適用外。

RESULT: approve
