# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-05-25T00:00:00Z -->

## Reviewed Scope

- Branch: claude/issue-12-impl--api
- HEAD commit: 0263c3fb54b1abe55d72233f6b1f28a28c1e968e
- Compared to: develop..HEAD

## Verified Requirements

- 1.1 — `io.LimitReader(resp.Body, maxResponseBodySize+1)`（client.go:98）でボディ読み込み量を上限以内に制限。`ExactlyMaxSize_ParsesSuccessfully` で上限内処理を確認
- 1.2 — `maxResponseBodySize = 1 * 1024 * 1024`（client.go:22 = 1,048,576 バイト）
- 1.3 — 上限以内では `len(body) <= maxResponseBodySize` で全量を読み込みパースへ進む（client.go:98-113）。`ExactlyMaxSize_ParsesSuccessfully`
- 2.1 — 各 URL のブックマーク数マップを返す。既存 `TestClient_GetBookmarkCounts_SingleURL` / `MultipleURLs`（パースロジック client.go:115-132 無改変）
- 2.2 — 含まれない URL を 0 件補完（client.go:124-132）。既存 `ZeroBookmarks_MissingFromResponse`
- 2.3 — 空 URL リストで API 非呼び出し・空マップ返却（client.go:47-49）。既存 `EmptyURLList` / `NilURLList`
- 2.4 — 戻り値型 `map[string]int` 不変、既存全テストが無改変で pass
- 3.1 — 上限超過時に `nil, error` を返す（client.go:107-113）。`OneByteOverMaxSize_ReturnsError` / `OversizedBody_ReturnsErrorAndLogs`（counts==nil を assert）
- 3.2 — `c.logger.Error("レスポンスボディが読み込み上限サイズを超過しました", ...)`（client.go:108-111）。`OversizedBody_ReturnsErrorAndLogs`（ERROR ログ + "上限" 文言を assert）
- 3.3 — 上限超過時は JSON パースに到達せず nil 返却（client.go:107-113、パースは 115 以降）。`OneByteOverMaxSize` / `OversizedBody`（counts==nil）
- 4.1 — 上限ちょうど（1,048,576 バイト）はエラーにせずパース（`len(body) > maxResponseBodySize` が false）。`ExactlyMaxSize_ParsesSuccessfully`
- 4.2 — 1 バイト超過でエラー（+1 読込により `len(body) == maxResponseBodySize+1`）。`OneByteOverMaxSize_ReturnsError`
- 5.1 — 上限処理（client.go:95-113）は非 200 ステータスチェック（client.go:87-93）より後段に配置され、構造的に先行保証。既存 `HTTPError` / `LogsError`
- 5.2 — 上限処理は通信失敗チェック（client.go:76-83）より後段。既存 `ContextCancelled`
- NFR 1 — `LimitReader` により読込量を上限 +1 バイトに抑制（client.go:98）。`OversizedBody` で間接担保
- NFR 2 — 上限値を定数 `maxResponseBodySize` 化（client.go:20-22）、リテラル散在なし
- NFR 3 — 公開シグネチャ `GetBookmarkCounts(ctx, urls) (map[string]int, error)` 不変、既存全テスト無改変 pass

## Findings

なし

## Summary

全 numeric requirement ID（1.1〜5.2 および NFR 1〜3）に対応する実装とテストを確認した。上限処理はボディ読込箇所のみに限定され、非 200 / 通信失敗の既存エラー処理経路や JSON パース・0 件補完ロジックは無改変で、Out of Scope（エンドポイント / スキーマ / URL 上限 / 外部設定化）への逸脱もない。新規境界値・異常系テスト 3 件を含む対象パッケージのテストは green（再実行で確認済み）。Feature Flag Protocol は opt-out のため flag 観点は適用しない。

RESULT: approve
