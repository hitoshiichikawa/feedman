# Implementation Notes - Issue #105

## Overview
記事一覧および詳細画面のタイトル部分を元記事への外部リンクに変更し、イベントの伝播制御を実装しました。

## Test Coverage Mapping
- **Requirement 1: 記事一覧タイトルからの元記事直リンク**
  - AC 1.1: `web/src/components/item-list.test.tsx` 内の `タイトルが元記事への外部リンクであり、新規タブで開くこと` にて検証。
  - AC 1.2: `web/src/components/item-list.test.tsx` 内の `タイトルが元記事への外部リンクであり、新規タブで開くこと` で `target="_blank"` および `rel="noopener noreferrer"` の付与を検証。
  - AC 1.3: `web/src/components/item-list.test.tsx` 内の `タイトルリンクをクリックした際に親行のクリックイベントが伝搬しないこと` で `e.stopPropagation()` による制御を検証。
  - AC 1.4: `web/src/components/item-list.tsx` の `ItemRow` において `hover:underline cursor-pointer` スタイルを適用。
- **Requirement 2: 記事詳細タイトルからの元記事直リンク**
  - AC 2.1: `web/src/components/item-detail.test.tsx` 内の `タイトルが元記事への外部リンクであり、新規タブで開くこと` にて検証。
  - AC 2.2: `web/src/components/item-detail.tsx` において `hover:underline` スタイルを適用。

## Verification Results
- **Frontend Tests (`vitest run`):** Passed
- **Docker Compose Staging Build & Run:** Succeed

STATUS: complete
