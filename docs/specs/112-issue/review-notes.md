# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-05-27T16:53:00Z -->

## Reviewed Scope

- Branch: claude/issue-112-impl-
- HEAD commit: c28de4d6140319dea2c52403a537d248c6677585
- Compared to: develop..HEAD

## Verified Requirements

- 1.1 — `item-detail.tsx`: `COLLAPSED_MAX_HEIGHT_PX = 300` を閾値に `isCollapsed` 時のみ `max-h-[300px] overflow-hidden` を付与。テスト「本文の高さが300pxを超えるとき折りたたまれ…」で `max-h-[300px]` 付与を確認
- 1.2 — 同上。`overflow-hidden` で 300px 超部分をクリップ（同テストで検証）
- 1.3 — `content-fade`（`bg-gradient-to-t from-background`）を `isCollapsed` 時に absolute で重畳。テストで `content-fade` 表示を確認
- 1.4 — `dangerouslySetInnerHTML` + CSS クリップで DOM ツリーを維持（文字列切り取りなし）。実装・impl-notes で確認
- 2.1 — `isOverflowing` 時に `content-toggle` を「続きを読む」文言で表示。テストで文言確認
- 2.2 — `content-toggle` 押下で `isExpanded` 反転し `max-h-[300px]` 非付与。テスト「『続きを読む』ボタンを押下すると全文表示に…」で確認
- 2.3 — 展開時 `content-fade` 消失。同テストで `queryByTestId("content-fade")` 不在を確認
- 2.4 — 展開時に文言「折りたたむ」へ変更。同テストで確認
- 3.1 — 全文表示中も `isOverflowing` のため `content-toggle` を「折りたたむ」で継続表示。テストで確認
- 3.2 — 「折りたたむ」押下で `max-h-[300px]` 再付与。テスト「全文表示中に『折りたたむ』ボタンを押下すると…」で確認
- 3.3 — 同上で `content-fade` 再表示を確認
- 3.4 — 同上で文言「続きを読む」復帰を確認
- 4.1 — `scrollHeight <= 300` のとき高さ制限クラス非付与。テスト「本文の高さが300px未満のとき…」「ちょうど300pxのとき…」で確認
- 4.2 — 同テストで `content-toggle` 不在を確認
- 4.3 — 同テストで `content-fade` 不在を確認
- NFR 1.1 — 文字列切り取りせず scrollHeight ベースの CSS クリップ。実装で確認
- NFR 1.2 — 既存 `sanitizeContentHtml` + `dangerouslySetInnerHTML` を維持。既存サニタイズテスト（script/onerror/空文字列）が非回帰で pass
- NFR 2.1 — タイトル・著者・元記事リンク・はてブ数・スター切替の既存テストが非回帰で pass
- NFR 2.2 — 展開時自動既読化の既存テストが非回帰で pass

## Findings

なし

## Summary

Req 1〜4 の全 numeric ID に対応する実装と Vitest テストが存在し、ローカルで `item-detail.test.tsx` の 19 ケースが全 pass（境界値「ちょうど300px」「300px未満」を含む）。NFR 1/2 は既存テスト群の非回帰維持で担保。design-less impl のため `_Boundary:_` アノテーションは無く、変更は `web/src/components/item-detail.{tsx,test.tsx}` と spec のみで Out of Scope への逸脱なし。Feature Flag Protocol は opt-out のため flag 観点は適用しない。

RESULT: approve
