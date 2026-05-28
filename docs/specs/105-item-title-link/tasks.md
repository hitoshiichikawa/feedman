# Implementation Plan

- [x] 1. Frontend: 記事一覧タイトル（ItemRow）の直リンク化
  - [x] 1.1 `web/src/components/item-list.tsx` 内の `ItemRow` にて、タイトル表示部を `<a>` リンク（`target="_blank"`, `rel="noopener noreferrer"` 付き）に変更する。
    - _Requirements: 1.1, 1.2_
    - _Boundary: ItemRow_
  - [x] 1.2 タイトルクリック時に `e.stopPropagation()` を実行するようにし、親の `button` のクリックイベントをブロックする。
    - _Requirements: 1.3_
    - _Boundary: ItemRow_
  - [x] 1.3 `web/src/components/item-list.test.tsx` にて、タイトルリンクの存在、属性値、およびクリック時のイベント伝播ブロックを検証するテストを追加する。
    - _Requirements: 1.1, 1.2, 1.3_

- [x] 2. Frontend: 記事詳細タイトル（ItemDetail）の直リンク化
  - [x] 2.1 `web/src/components/item-detail.tsx` 内の `ItemDetail` にて、`h3` のタイトル表示部分を `<a>` リンク（`target="_blank"`, `rel="noopener noreferrer"` 付き）に変更する。
    - _Requirements: 2.1_
    - _Boundary: ItemDetail_
  - [x] 2.2 `web/src/components/item-detail.test.tsx` にて、詳細タイトルの直リンク化を検証するテストを追加する。
    - _Requirements: 2.1, 2.2_

## Verify

<!-- stage-a-verify -->
```sh
go test ./...
```
