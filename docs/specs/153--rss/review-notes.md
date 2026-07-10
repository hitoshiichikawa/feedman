# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-05-30T00:10:00Z -->

## Reviewed Scope

- Branch: claude/issue-153-impl--rss
- HEAD commit: 0e0d2213e848b48e3ac273b7654f7c34cab22fdf
- Compared to: develop..HEAD
- Mode: design-less impl（`tasks.md` / `design.md` 不在のため、AC カバレッジと missing test の 2 軸で評価。`_Boundary:_` 注釈は存在しないため、boundary 逸脱は CLAUDE.md の禁止事項のみで判定）

## Verified Requirements

- 1.1 — `internal/feed/detector.go:333-343` で `resp.StatusCode` を判定対象として扱う分岐を追加。`TestDetectFeedURL_HTTPError_4xx/5xx/3xxFinalResponse` が判定通過を検証
- 1.2 — 上記分岐は 2xx で false 評価され既存フローを継続。`TestDetectFeedURL_2xxStillWorks` および既存 `TestDetectFeedURL_DirectRSSFeed` / `TestDetectFeedURL_HTMLWithFeedLink` 等で回帰確認
- 1.3 — 4xx / 5xx で `io.ReadAll` を呼ばず `model.NewFeedHTTPError` を返す（`detector.go:340-342`）。`TestDetectFeedURL_HTTPError_4xx`（404/429/403）/ `TestDetectFeedURL_HTTPError_5xx`（500/503）でカバー
- 1.4 — 同一分岐条件 `< 200 || >= 300` で 3xx 最終応答も同経路。`TestDetectFeedURL_HTTPError_3xxFinalResponse`（Location なし 302）で検証
- 1.5 — `ErrCodeFeedHTTPError = "FEED_HTTP_ERROR"` を `internal/model/errors.go:42` に新設し、`FEED_NOT_DETECTED` と別 code。各テストで `Code != ErrCodeFeedNotDetected` を assert
- 2.1 — `NewFeedHTTPError` の Message が「ブロックしている / URL が間違っている / 一時的にサーバが応答していない」3 原因を含み、Action が「別のURLを試す / 時間を置いて再試行」を含む（`internal/model/errors.go:236-243`）。`TestNewFeedHTTPError` が Action 非空を検証
- 2.2 — 2xx 経路の `FEED_NOT_DETECTED` 返却ロジックは未変更。既存 `TestDetectFeedURL_HTMLNoFeedLink` 等が引き続き pass
- 2.3 — Message は `fmt.Sprintf` で受信ステータスコードのみを差し込む固定テンプレ。レスポンスボディや stacktrace を含めない（`errors.go:237`）
- 2.4 — Message に `HTTP %d` を埋め込み、`Details["status_code"]` に int で添付。`TestNewFeedHTTPError` で Message に "HTTP" 含有、`Details["status_code"].(int)` 型・値を検証
- 3.1 — 200 + RSS Content-Type の検出パスを変更せず。`TestDetectFeedURL_2xxStillWorks` + 既存 `TestDetectFeedURL_DirectRSSFeed` で担保
- 3.2 — HTML 内 feed link 優先順位ロジックは未変更。既存 `TestDetectFeedURL_HTMLWithFeedLink` / `TestDetectFeedURL_HTMLWithMultipleFeedLinks_PrioritySelection` で担保
- 3.3 — `client.Do(req)` の err 分岐（`detector.go:316-328`）は未変更で、レスポンス取得前の失敗は引き続き `FETCH_FAILED`。本変更の HTTP ステータス判定は `client.Do` 成功後にのみ実行される
- 3.4 — `go.mod` に変更なし。新規 import は標準ライブラリ `log/slog` のみで外部依存追加なし
- 4.1 — 200 + 空ボディの分岐は未変更。既存の `ParseFeedLinksFromHTML` 経路で `FEED_NOT_DETECTED` を返す挙動を温存
- 4.2 — 200 + Content-Type 欠落 + XML 有効の判定（`IsDirectFeed` の body sniff 経路）は未変更
- 4.3 — 200 + 非 HTML/XML の経路は未変更
- 4.4 — `TestDetectFeedURL_HTTPError_4xx_DoesNotParseHTMLFeedLink`: 429 応答ボディに `<link rel="alternate" type="application/rss+xml">` を含む HTML を返しても HTML パース実行されず `FEED_HTTP_ERROR` 返却を assert
- NFR 1.1 — 既存 200 系テスト群が無修正で全件 pass（`go test ./internal/feed/... ./internal/model/... ./internal/handler/...` green を確認）
- NFR 1.2 — `io.LimitReader(resp.Body, detectorMaxResponseSize)`（5MB）の呼び出しは温存。非 2xx 時は LimitReader 自体も実行されない
- NFR 1.3 — `detectorTimeout`（10 秒）は変更なし
- NFR 2.1 — `slog.Warn("フィード検出: HTTPステータス異常", "url", inputURL, "status", resp.StatusCode)` を非 2xx 分岐で出力（`detector.go:336-339`）。`favicon.go` の既存パターンと整合
- NFR 3.1 — User-Agent / Cookie 永続化 / JS 評価などボット保護回避コードは一切追加されていない

## handler 層の整合

- `internal/handler/feed_handler.go:277-283` で `ErrCodeFeedHTTPError` → `http.StatusBadGateway` (502) にマップ。Req 1〜4 には HTTP ステータスのマップ自体を直接規定する AC は無いが、impl-notes.md 設計判断 2 に基づき `FETCH_FAILED` と同じ 502 を選択。FEED_NOT_DETECTED が 422 のままなので UI 側はエラーコードで区別可能（Req 1.5, 2.1 のスコープと整合）

## design-less impl の取り扱い

- `tasks.md` / `design.md` が不在のため `_Boundary:_` / `_Requirements:_` アノテーションによる境界判定は行えない。代わりに CLAUDE.md「禁止事項」に照らして検査:
  - base ブランチ（develop）への直接 push: 無し
  - 機密情報のコミット: 無し
  - 外部 API キー埋め込み: 無し
  - SSRF/サニタイズ機構の迂回: `safeurl` / SSRF ガードは経由されたまま、新規分岐は判定後の処理のみ追加
- 変更ファイルはすべて Issue スコープ（feed 検出 + handler マッピング + model エラー定義 + テスト + impl-notes）に閉じており、無関係領域への波及なし

## Findings

なし

## Summary

全 numeric AC（Req 1.1〜4.4 / NFR 1.1〜3.1）に対応する実装またはテストが diff または既存コードのいずれかに確認できた。Req 1.3 / 1.4 / 1.5 / 2.4 / 4.4 はそれぞれ専用テストが追加され、2xx 系の互換性も `TestDetectFeedURL_2xxStillWorks` および既存テスト群で担保されている。`go test ./internal/feed/... ./internal/model/... ./internal/handler/...` を独立再実行し全件 green を確認。新規外部依存は無く、ボット保護回避コードも追加されていない。

RESULT: approve
