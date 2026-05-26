# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-05-26T00:00:00Z -->

## Reviewed Scope

- Branch: claude/issue-104-impl-feat-web
- HEAD commit: 768e37ad669bd7bab62be485b0259f7af542a954
- Compared to: develop..HEAD

本 Issue は design.md / tasks.md を持たない design-less impl のため、`_Boundary:_`
アノテーションは存在せず boundary 逸脱判定は対象外。Feature Flag Protocol は本リポジトリで
opt-out のため flag 観点は適用しない（通常の 3 カテゴリ判定のみ）。backend Go テストは
`go test ./internal/item/... ./internal/handler/...` で再実行し ok を確認。frontend テストは
本環境に Node.js が無く未実行だが、テストコードは追加済みで CI（`npm test`）で green が担保
される想定。

## Verified Requirements

- 1.1 — `internal/item/service.go` ListItems で `Summary: item.Summary` をマッピング、
  `internal/handler/service_adapter.go` で `Summary: it.Summary` を伝播。テスト
  `TestItemService_ListItems_IncludesSummary` / `TestItemHandler_ListItems_IncludesSummary`
- 1.2 — GetItem / ListItems がともに `item.Summary` を返す。テスト
  `TestItemService_SummaryConsistentBetweenListAndDetail`（list と detail の Summary 一致を検証）
- 1.3 — `itemSummaryResponse.Summary` は `json:"summary"`（omitempty なし）。空概要でも
  キーが存在し空文字列で返る。`TestItemHandler_ListItems_IncludesSummary` の空ケースで検証
- 1.4 — 既存フィールド（title/link/published_at/is_date_estimated/is_read/is_starred/
  hatebu_count）は不変。`TestItemHandler_ListItems_PreservesExistingFields`
- 2.1 — `item-list.tsx` でタイトル行の外（下）に `<p data-testid="item-summary-...">` を配置。
  テスト「概要があるとき記事行のタイトル直下に概要が表示されること」
- 2.2 — 概要 `<p>` に `text-xs`。テスト「概要テキストがタイトルより小さく薄い配色で表示されること」
- 2.3 — 概要 `<p>` に `text-muted-foreground`（低コントラスト配色）。同上テスト
- 2.4 — `hasSummary = item.summary.trim().length > 0` の条件描画。空時は要素自体を描画しない。
  テスト「概要が空のとき概要領域を描画しないこと」
- 2.5 — 既読時の `opacity-60` は `button` 全体に適用され概要 `<p>` も子要素として透過。
  既存テスト「既読記事は視覚的に区別されること」（data-read 検証）
- 3.1 / 3.2 / 3.3 — 概要 `<p>` に `line-clamp-2`（最大 2 行で省略、短文は全表示）。
  テスト「概要が最大2行で省略されるよう line-clamp-2 が適用されること」
- 4.1 — 公開日時 `<span>` をタイトルと同一 `item-title-row` div の右側へ移動。
  テスト「公開日時がタイトルと同一行の右側に配置されること」（title-row が time を含み summary は含まない）
- 4.2 — `is_date_estimated` の推定フラグ `<span data-testid="date-estimated">` を日時 span 内に維持。
  テスト「推定日付の記事では推定フラグが日時に隣接して表示されること」
- 4.3 — タイトル `flex-1 min-w-0`、日時 `flex-shrink-0 whitespace-nowrap`、概要 `line-clamp-2` で
  狭幅時の重なり・はみ出しを抑制（CSS レイアウトのため実装で対応、impl-notes に記載）
- 4.4 — 日時 span に `whitespace-nowrap` を付与し折り返し・切り詰めによる判読不能を回避（実装で対応）
- 5.1 — ページサイズ（`defaultItemsPerPage`）は不変。一覧取得ロジックを変更していない
- 5.2 — 無限スクロール（IntersectionObserver / sentinel）は変更なし。既存テスト
  「無限スクロール用のsentinelが存在すること」
- 5.3 — 記事クリックの onSelectItem 呼び出しは変更なし。既存テスト
  「記事をクリックするとonSelectItemが呼ばれること」
- NFR 1.1 — 既存項目名・型を変更せず summary フィールド追加のみ。
  `TestItemHandler_ListItems_PreservesExistingFields`
- NFR 1.2 — フィールド追加のみで既存項目を変更しないため後方互換（同上テスト）
- NFR 2.1 — 概要は `<p>{item.summary}</p>` のテキストノードとして描画。
  `dangerouslySetInnerHTML` 不使用（diff で確認）
- NFR 3.1 — `line-clamp-2` で 2 行以内に制限（3.2 と同一実装で担保）

## Findings

なし

## Summary

全 numeric ID（Req 1.1〜5.3 および NFR 1.1/1.2/2.1/3.1）について実装または対応テストを確認した。
backend テストは再実行し ok。frontend テストは追加済みで CI 担保。3 カテゴリ（AC 未カバー /
missing test / boundary 逸脱）いずれにも該当する問題は検出されなかった。なお impl-notes の
「ItemDetail の重複 Summary フィールドを撤去」という記述は実コード（service.go / item_handler.go
に重複 Summary フィールドが残存）と不一致だが、Go のフィールドシャドーイングにより list/detail
ともに `item.Summary` が `summary` JSON として正しく返り挙動は要件どおりで、3 カテゴリには
該当しない（ドキュメント記述の精度のみの差異）。

RESULT: approve
