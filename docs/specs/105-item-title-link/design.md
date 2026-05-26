# Design Document

## Overview

記事一覧および詳細画面のタイトル部分を元記事への外部リンク（`target="_blank"`）に変更し、クリック時のイベント伝播を適切にコントロールすることで、利便性と操作の一貫性を両立します。

**Purpose**: この機能は タイトルをクリックして元記事を開く直感的な導線 を提供する ことにより、ユーザー に スムーズな遷移体験 を提供する。
**Users**: 興味のある記事を元のサイトで読みたい全ユーザー が 利用する。
**Impact**: 元記事を開くのに詳細展開を経由しなければいけなかった状態 を、一覧や詳細のタイトルから一発で開ける状態 に変える。

### Goals
- `ItemRow` のタイトルテキストを `<a>` リンク（`target="_blank"`, `rel="noopener noreferrer"`）にする。
- タイトルをクリックした際、`e.stopPropagation()` を実行して、背後にあるリスト行の `onClick`（詳細パネル展開処理）が誤発火するのを防ぐ。
- `ItemDetail` の `h3` タイトルテキストも `<a>` リンク化する。

### Non-Goals
- 全体のレイアウト設計の変更。

## File Structure Plan

### Directory Structure
```
web/src/
└── components/
    ├── item-list.tsx    # ItemRow のタイトルを <a> リンク化、stopPropagation の追加
    └── item-detail.tsx  # ItemDetail のタイトルを <a> リンク化
```

### Modified Files
- `web/src/components/item-list.tsx`
- `web/src/components/item-detail.tsx`

## Components and Interfaces

### Frontend Layer

#### ItemRow (in item-list.tsx)
- タイトル `span` タグを `a` タグに変更:
  ```tsx
  <a
    href={item.link}
    target="_blank"
    rel="noopener noreferrer"
    onClick={(e) => e.stopPropagation()}
    className="hover:underline flex-1 text-sm line-clamp-2"
  >
    {item.title}
  </a>
  ```
- `e.stopPropagation()` により、アンカー要素のクリックは親の `button.onClick` に伝達されず、新しいタブでリンク先を開く動作だけが実行されます。

#### ItemDetail (in item-detail.tsx)
- `h3` 内のテキストを `a` タグで囲みます:
  ```tsx
  <h3 className="text-lg font-semibold leading-tight">
    <a
      href={item.link}
      target="_blank"
      rel="noopener noreferrer"
      className="hover:underline"
    >
      {item.title}
    </a>
  </h3>
  ```

## Testing Strategy

- **Unit Tests**:
  - `web/src/components/item-list.test.tsx` でタイトルリンクが `href={item.link}` かつ `target="_blank"` であることを検証。
  - `web/src/components/item-list.test.tsx` でタイトルをクリックした際、親の `onSelectItem` が呼び出されない（伝播停止）ことを検証。
  - `web/src/components/item-detail.test.tsx` で詳細タイトルが `href={item.link}` であることを検証。
