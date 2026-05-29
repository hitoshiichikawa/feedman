# Design Document

## Overview

**Purpose**: はてなブックマーク数とお気に入り（スター⭐️）を 3 つの記事一覧（通常 / スター横断 / 検索結果）の各行右端に常設し、その場でスター ON/OFF をトグル可能にすることで、利用者が一覧画面から記事を展開せずに重要な指標と状態を把握・操作できるようにする。同時に、詳細ヘッダーから両表示を撤去して責務分離を明確にする。

**Users**: Feedman の記事閲覧者全般。フィード一覧 / スター横断 / 検索結果のいずれの画面遷移でも、はてブ数を確認でき、興味記事を即時にスター登録できるようになる。

**Impact**: 既存の `ItemRow`（通常 / スター横断で共有）と `SearchResultRow`（検索結果固有）の 2 種類の行コンポーネントへの右端メタ表示追加、`ItemDetail` ヘッダーからのメタ撤去、検索 API レスポンス（Go 側 `itemSearchHitResponse` と DB SELECT、TypeScript 側 `ItemSearchHit`）への `hatebu_fetched_at` フィールド追加が変更点となる。スター切替の楽観更新ロジック（`useToggleStar`）は新たに `["item-search"]` キャッシュも対象に含めるよう拡張する。

### Goals

- 3 一覧すべてで「はてブ数（左）＋スター⭐️トグル（右）」を右端に常設し、視覚・操作の挙動を統一する（Req 1 / Req 2 / Req 4）
- スター⭐️トグルは楽観更新（クリック→100ms 以内 UI 反映）し、行クリック展開とイベント独立を確保する（Req 2 / NFR 2）
- 検索 API 応答に `hatebu_fetched_at` を追加し、3 一覧のはてブ数表示ロジック（`-` vs 数値）を同一にする（Req 5）
- 詳細ヘッダーから「はてブ数」「スター⭐️」を撤去し、一覧側に責務を集約する（Req 3）
- 既存の無限スクロール・既読薄表示・フィルタタブ・フィード badge・空/エラー/ローディング状態を非回帰で維持する（Req 6）

### Non-Goals

- はてブ数による並び替え・フィルタ機能の追加
- 一覧 / 詳細レイアウトの全面刷新
- はてブ取得頻度・取得対象のバックエンド変更
- 一括スター操作 UI / コメント閲覧 / 外部はてブページへの遷移
- 詳細ヘッダーのタイトル・著者・元記事リンク・本文・自動既読化挙動の変更（Req 3.3 / 3.4 で維持を明示）

## Architecture

### Existing Architecture Analysis

- **フロント側 3 一覧**: `ItemList`（通常） / `StarredItemList`（スター横断） / `SearchResults`（検索）の 3 コンポーネントが並列に存在する。`ItemList` と `StarredItemList` は **共通 `ItemRow`** を共有する（既存 export 済み）。`SearchResults` は固有レスポンス型 `ItemSearchHit` を持つため独自 `SearchResultRow` を派生させている（既存判断、Issue #117 / #120 由来）
- **既存スター UI 実装**: `ItemDetail` のヘッダー内に `Button + Star` の楽観更新トグルが既に存在する（aria-label / aria-pressed / icon-sm Button / hatebu の `-` 表示分岐を含む）。本実装は **そのままの責務移転先（共通スター行コンポーネント）として再利用可能**
- **`useToggleStar`** は `["items"]` queryKey に対する楽観更新 + invalidate を実装済み。`["items", "starred"]`（スター横断）は前置キー共有で既にカバーされている。`["cross-feed-items"]` は明示 invalidate されている。**`["item-search"]` は現状未カバー**であり、本機能で拡張が必要
- **検索 API**: `internal/handler/item_search_handler.go` の `itemSearchHitResponse` 構造体、`internal/handler/service_adapter.go` の `ItemSearchServiceAdapter.Search`、`internal/itemsearch/service.go` の `ItemSearchSummary`、`internal/repository/postgres_item_repo.go` の `SearchByUserAndKeyword` SQL、`internal/model/item.go` の `ItemSearchHit` の 5 レイヤを縦断する変更が必要

### Architecture Pattern & Boundary Map

```mermaid
flowchart TB
  subgraph Web[web/ (Next.js)]
    ItemList[ItemList]
    StarredItemList[StarredItemList]
    SearchResults[SearchResults]
    ItemRow[ItemRow]
    SearchResultRow[SearchResultRow]
    ItemMetaActions[ItemMetaActions<br/>新規 - 共通スター行 + はてブ表示]
    ItemDetail[ItemDetail<br/>メタ撤去]
    UseToggleStar[useToggleStar<br/>item-search 拡張]
    Types[types/item.ts<br/>ItemSearchHit に hatebu_fetched_at 追加]
  end

  subgraph API[Go API]
    Handler[item_search_handler.go<br/>itemSearchHitResponse]
    Adapter[service_adapter.go<br/>ItemSearchServiceAdapter]
    SvcLayer[itemsearch/service.go<br/>ItemSearchSummary]
    RepoLayer[postgres_item_repo.go<br/>SearchByUserAndKeyword]
    ModelLayer[model/item.go<br/>ItemSearchHit]
    DB[(PostgreSQL items.hatebu_fetched_at)]
  end

  ItemList --> ItemRow
  StarredItemList --> ItemRow
  SearchResults --> SearchResultRow
  ItemRow --> ItemMetaActions
  SearchResultRow --> ItemMetaActions
  ItemRow -.使う.-> UseToggleStar
  SearchResultRow -.使う.-> UseToggleStar
  ItemMetaActions -.callback.-> UseToggleStar

  SearchResults -.型.-> Types
  Handler --> Adapter --> SvcLayer --> RepoLayer --> ModelLayer
  RepoLayer --> DB
```

**Architecture Integration**:
- **採用パターン**: 既存 `ItemRow` と `SearchResultRow` に共通の小コンポーネント `ItemMetaActions`（はてブ数表示 + スター⭐️トグル）を切り出し、3 一覧から再利用する。`ItemDetail` 由来のスター UI 実装（`aria-label` / `aria-pressed` / 32px ヒット領域 / 楽観更新ハンドラ）を `ItemMetaActions` に集約する
- **代替案（不採用）**: `ItemRow` と `SearchResultRow` を統合して 1 つの行コンポーネントに集約する案も検討したが、(1) `SearchResultRow` は `ItemSearchHit` の published_at が nullable、(2) フィード badge 表示位置の差、の 2 点で構造が異なるため、行コンポーネント自体は併存させ、右端メタ部分のみを共通化する判断を採用した（既存 Issue #117 の責務分離判断を尊重）
- **ドメイン／機能境界**: フロント側は `ItemMetaActions` の責務（提示 + コールバック発火）と `useToggleStar`（mutation + キャッシュ更新）の責務を分離する。API 側は handler / adapter / service / repository / model の 5 レイヤをまたぐが、各層の責務は変わらず `hatebu_fetched_at` フィールド追加のみが波及する
- **既存パターンの維持**: `useToggleStar` の楽観更新パターン、`ItemDetailArea` の展開エリア責務、無限スクロール (`IntersectionObserver`)、3 一覧の `expandedItemId` 排他展開
- **新規コンポーネントの根拠**: `ItemMetaActions` は 3 一覧から呼ばれる共通 UI ブロックであり、`ItemRow` / `SearchResultRow` の差分（型・badge）に依存しないため独立化が妥当。`item.id` + `is_starred` + `hatebu_count` + `hatebu_fetched_at` の 4 値のみで再描画可能

### Technology Stack

| Layer | Choice / Version | Role in Feature | Notes |
|-------|------------------|-----------------|-------|
| Frontend / CLI | Next.js 15 / React 19 / TypeScript 5 | `ItemMetaActions` 新規、`ItemRow` / `SearchResultRow` / `ItemDetail` 改修、`useToggleStar` 拡張、`ItemSearchHit` 型拡張 | 既存スタック再利用 |
| Frontend UI | Tailwind CSS 4 + shadcn/ui (`Button variant="ghost" size="icon-sm"`) + lucide-react (`Star`, `Bookmark`) | 32px ヒット領域 / 黄塗り vs アウトライン アイコン | 既存 `ItemDetail` のスター実装をそのまま移植 |
| Frontend Test | Vitest + Testing Library (jsdom) | 3 一覧の表示 / トグル / 伝播抑止 / ロールバックの単体・統合テスト | 既存 `item-list.test.tsx` / `starred-item-list.test.tsx` / `search-results.test.tsx` / `item-detail.test.tsx` を改修・追加 |
| Backend / Services | Go 1.25 + chi/v5 | `itemSearchHitResponse` / `ItemSearchSummary` / `ItemSearchHit` に `HatebuFetchedAt *time.Time` 追加、SELECT 列追加 | JSON タグ `hatebu_fetched_at,omitempty` |
| Data / Storage | PostgreSQL 16 (`items.hatebu_fetched_at TIMESTAMPTZ`) | 検索 SQL の SELECT 列追加（既存列、スキーマ変更なし） | マイグレーション不要 |
| Backend Test | 標準 `testing` パッケージ | handler / adapter / service_test の `hatebu_fetched_at` 反映確認 | 既存 `item_search_handler_test.go` / `itemsearch/service_test.go` / `postgres_item_repo_search_test.go` を改修 |

## File Structure Plan

### Directory Structure

```
web/src/
├── components/
│   ├── item-meta-actions.tsx        # 新規: はてブ数 + スター⭐️トグルの共通行コンポーネント
│   ├── item-meta-actions.test.tsx   # 新規: ItemMetaActions の単体テスト
│   ├── item-list.tsx                # 改修: ItemRow に ItemMetaActions を挿入、スター callback 配線
│   ├── item-list.test.tsx           # 改修: 一覧上トグル / 伝播抑止 / 表示の検証を追加
│   ├── starred-item-list.tsx        # 改修: ItemRow への onToggleStar prop 配線（共通コンポーネント側で対応）
│   ├── starred-item-list.test.tsx   # 改修: 横断一覧でのトグル / hatebu 表示の検証を追加
│   ├── search-results.tsx           # 改修: SearchResultRow に ItemMetaActions を挿入、hatebu_fetched_at 表示分岐
│   ├── search-results.test.tsx      # 改修: 検索結果での hatebu_fetched_at 反映 / トグル検証（存在しない場合は新規作成）
│   ├── item-detail.tsx              # 改修: ヘッダーの hatebu / star トグル UI を撤去（タイトル・著者・リンクは維持）
│   └── item-detail.test.tsx         # 改修: 撤去後の構造を検証（hatebu-count / star-toggle が存在しないこと）
├── hooks/
│   ├── use-item-state.ts            # 改修: useToggleStar の楽観更新 / invalidate 対象に ["item-search"] を追加
│   └── use-item-state.test.ts       # 改修: item-search キャッシュへの楽観反映 / ロールバック検証（存在しない場合は新規作成）
└── types/
    └── item.ts                      # 改修: ItemSearchHit に hatebu_fetched_at: string | null を追加

internal/
├── model/
│   └── item.go                      # 改修: ItemSearchHit に HatebuFetchedAt *time.Time 追加
├── itemsearch/
│   ├── service.go                   # 改修: ItemSearchSummary に HatebuFetchedAt *time.Time 追加、pass-through
│   └── service_test.go              # 改修: HatebuFetchedAt の伝搬テストを追加
├── repository/
│   ├── postgres_item_repo.go        # 改修: SearchByUserAndKeyword の SELECT 列に i.hatebu_fetched_at 追加、Scan 拡張
│   └── postgres_item_repo_search_test.go  # 改修: SELECT 結果の HatebuFetchedAt 検証を追加
└── handler/
    ├── item_search_handler.go       # 改修: itemSearchHitResponse に HatebuFetchedAt *time.Time + json タグ追加
    ├── service_adapter.go           # 改修: ItemSearchServiceAdapter.Search の field copy に HatebuFetchedAt を追加
    └── item_search_handler_test.go  # 改修: レスポンス JSON に hatebu_fetched_at が含まれることを検証
```

### Modified Files

- `web/src/components/item-detail.tsx` — タイトル右側の meta グループ（`item-detail-meta-group`, `hatebu-count`, `star-toggle`）を削除。タイトル行は title リンクのみとなる。`item.is_read` 自動既読化、本文サニタイズ、折りたたみ機能は不変
- `web/src/components/item-list.tsx` — `ItemRow` のタイトル行に `ItemMetaActions` を挿入。既存の `is_starred ? <Star /> : null`（読み取り専用 star 表示）は削除し、`ItemMetaActions` のトグル UI に置き換える。`ItemList` 本体は `onToggleStar` を `ItemRow` に渡す（現在は detail のみに渡している）
- `web/src/components/starred-item-list.tsx` — `ItemRow` 経由で `onToggleStar` を伝達。`feed_title` 表示は維持
- `web/src/components/search-results.tsx` — `SearchResultRow` のタイトル行に `ItemMetaActions` を挿入。既存の `hit.is_starred ? <Star /> : null` を削除。`onToggleStar` callback を `SearchResultRow` に渡す
- `web/src/hooks/use-item-state.ts` — `useToggleStar` の `onMutate` で `["item-search"]` queryKey も対象に追加（`InfiniteData<ItemSearchResponse>` 形状で `items[].is_starred` を反転）、`onSettled` の `invalidateQueries` にも追加
- `web/src/types/item.ts` — `ItemSearchHit` に `hatebu_fetched_at: string | null` を追加
- `internal/model/item.go` — `ItemSearchHit` 構造体に `HatebuFetchedAt *time.Time` を追加
- `internal/itemsearch/service.go` — `ItemSearchSummary` に `HatebuFetchedAt *time.Time` を追加、`Search` メソッド内の summary 組み立てで pass-through
- `internal/handler/item_search_handler.go` — `itemSearchHitResponse` に `HatebuFetchedAt *time.Time` json `"hatebu_fetched_at,omitempty"` を追加
- `internal/handler/service_adapter.go` — `ItemSearchServiceAdapter.Search` のレスポンス組み立てで `HatebuFetchedAt` をコピー
- `internal/repository/postgres_item_repo.go` — `SearchByUserAndKeyword` の SELECT に `i.hatebu_fetched_at` を追加、`sql.NullTime` で Scan、`hit.HatebuFetchedAt` に代入

## Requirements Traceability

| Requirement | Summary | Components | Interfaces | Flows |
|-------------|---------|------------|------------|-------|
| 1.1, 1.2 | タイトル右端に hatebu + star を同一行配置、hatebu が star 左 | `ItemMetaActions`, `ItemRow`, `SearchResultRow` | `<div className="flex items-center gap-1">` 内に hatebu→star 順 | UI 表示 |
| 1.3, 1.4 | `hatebu_fetched_at` が null なら `-`、ありなら整数値 | `ItemMetaActions` | `hatebuFetchedAt === null ? "-" : String(hatebuCount)` | 表示分岐 |
| 1.5, 1.6 | 塗りつぶし / アウトライン Star 切替 | `ItemMetaActions` | `isStarred ? "fill-yellow-400 text-yellow-400" : "text-muted-foreground"` | UI 状態 |
| 1.7 | 既存「公開日時」「(推定)」「概要」維持 | `ItemRow`, `SearchResultRow` | 改修対象は meta グループのみ、他要素は不変 | 非回帰 |
| 2.1 | クリックで状態反転リクエスト送信 | `ItemMetaActions`, `useToggleStar` | `onClick={(e) => { e.stopPropagation(); onToggle(item.id, !item.is_starred); }}` | mutation 発火 |
| 2.2 | 楽観更新（応答待たず UI 反映） | `useToggleStar` | `onMutate` で `["items"]` / `["item-search"]` を即時更新 | 楽観更新 |
| 2.3 | 行クリック伝播抑止 | `ItemMetaActions` | `e.stopPropagation()` をクリックハンドラで呼ぶ | イベント独立 |
| 2.4 | 失敗時ロールバック | `useToggleStar` | `onError` で `previousData` を復元 | エラー復元 |
| 2.5 | 一覧間スター状態一貫性 | `useToggleStar` | `["items"]` 前置キー共有 + `["item-search"]` / `["cross-feed-items"]` の明示 invalidate | キャッシュ整合 |
| 3.1, 3.2 | 詳細ヘッダーから hatebu / star 撤去 | `ItemDetail` | `item-detail-meta-group` div を削除 | UI 削除 |
| 3.3 | タイトル・著者・元リンク・本文維持 | `ItemDetail` | meta グループ以外は不変 | 非回帰 |
| 3.4 | 展開時の自動既読化維持 | `ItemDetail` | `useEffect(() => { if (!is_read) onMarkAsRead(...) }, [item.id])` を保持 | 非回帰 |
| 4.1, 4.2, 4.3 | 3 一覧で Req 1 / Req 2 を満たす | `ItemList`, `StarredItemList`, `SearchResults` | いずれも `ItemMetaActions` を組み込み、`onToggleStar` を配線 | 統一適用 |
| 4.4 | 検索結果のフィード badge / 日時維持 | `SearchResultRow` | favicon + feed_title バッジ部、日時 `<time>` 要素は不変 | 非回帰 |
| 4.5 | スター横断の feed_title 行内表示維持 | `StarredItemList` | 既存 `starred-item-feed-title-${id}` div 不変 | 非回帰 |
| 5.1, 5.2 | 検索 API 応答に `hatebu_fetched_at` 追加、既存フィールド保持 | `itemSearchHitResponse`, `ItemSearchSummary`, `ItemSearchHit`, `SearchByUserAndKeyword` SQL | JSON `"hatebu_fetched_at,omitempty"` + Go `*time.Time` + SELECT `i.hatebu_fetched_at` | API スキーマ拡張 |
| 5.3, 5.4 | 検索結果でも未取得 `-` / 取得済み数値 | `SearchResultRow` → `ItemMetaActions` | `hit.hatebu_fetched_at` を pass | 表示分岐 |
| 6.1 | 既読の薄表示維持 | `ItemRow`, `SearchResultRow` | `item.is_read && "opacity-60"` 不変 | 非回帰 |
| 6.2 | 無限スクロール維持 | `ItemList`, `StarredItemList`, `SearchResults` | IntersectionObserver / sentinel 不変 | 非回帰 |
| 6.3 | フィルタタブ維持 | `AppShell` / `FeedPaneHeader`（既存） | 改修対象外 | 非回帰 |
| 6.4 | タイトルリンクの伝播抑止維持 | `ItemRow`, `SearchResultRow` | `<a onClick={(e) => e.stopPropagation()}>` 不変 | 非回帰 |
| 6.5 | (推定) バッジ維持 | `ItemRow`, `SearchResultRow` | `is_date_estimated && <span>(推定)</span>` 不変 | 非回帰 |
| 6.6 | スター横断ヘッダ「お気に入り」維持 | `StarredItemList` | 既存 h2 不変 | 非回帰 |
| 6.7 | 検索結果のバッジ / 空 / エラー / loading 状態維持 | `SearchResults` | 出し分けロジック不変 | 非回帰 |
| NFR 1.1, 1.2 | aria-label / aria-pressed | `ItemMetaActions` | `aria-label={isStarred ? "スターを解除する" : "スターを付ける"}`, `aria-pressed={isStarred}` | アクセシビリティ |
| NFR 1.3 | 32px ヒット領域 | `ItemMetaActions` | `Button size="icon-sm"` (h-8 w-8) | サイズ確保 |
| NFR 1.4 | キーボード操作 | `ItemMetaActions` | `<button>` のため Enter / Space で発火 | 標準挙動 |
| NFR 2.1 | 行クリックを発火させない | `ItemMetaActions` | `e.stopPropagation()` | 独立性 |
| NFR 2.2 | 100ms 以内 UI 反映 | `useToggleStar` | `onMutate` 同期実行で setQueryData | 性能 |
| NFR 2.3 | 失敗 200ms 以内に元に戻す | `useToggleStar` | `onError` 同期実行で previousData 復元 | 性能 |
| NFR 3.1 | 既存一覧 API スキーマ維持 | `/api/feeds/:id/items`, `/api/feeds/starred/items` | 改修対象外 | 後方互換 |
| NFR 3.2 | テスト識別子の維持 / 等価後継 | `ItemRow`, `SearchResultRow`, `ItemMetaActions` | 既存 `star-${id}`, `search-result-star-${id}` を `ItemMetaActions` の `data-testid` で置き換える | テスト整合 |
| NFR 3.3 | 詳細側撤去識別子と一覧側新規識別子の衝突回避 | `ItemDetail`, `ItemMetaActions` | 詳細側 `star-toggle` / `hatebu-count` を撤去、一覧側は `item-star-toggle-${id}` / `item-hatebu-count-${id}` のように一意化 | 命名分離 |

## Components and Interfaces

### Frontend Layer

#### ItemMetaActions（新規）

| Field | Detail |
|-------|--------|
| Intent | 一覧行右端の「はてブ数表示 + スター⭐️トグル」を表現する共通プレゼンテーショナルコンポーネント |
| Requirements | 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 2.1, 2.3, NFR 1.1, NFR 1.2, NFR 1.3, NFR 1.4, NFR 2.1 |

**Responsibilities & Constraints**
- はてブ数の表示（取得済み → 数値 / 未取得 → `-`）
- スター⭐️トグルボタンの提示（aria-label / aria-pressed / 32px ヒット領域）
- クリック時の `e.stopPropagation()` 実行 + `onToggle(itemId, nextStarred)` callback 発火
- mutation や API 呼び出しは行わない（callback のみ。`useToggleStar` の責務）
- `feed_title` バッジや日時表示は責務外（行コンポーネント側で配置）

**Dependencies**
- Inbound: `ItemRow` (`item-list.tsx`), `SearchResultRow` (`search-results.tsx`) — 一覧行から差し込まれる
- Outbound: `Button` (`@/components/ui/button`), `Star` / `Bookmark` (`lucide-react`), `cn` (`@/lib/utils`)
- External: なし

**Contracts**: Service [ ] / API [ ] / Event [ ] / Batch [ ] / State [ ]

##### Component Props

```typescript
interface ItemMetaActionsProps {
  /** 記事 ID（onToggle のキー） */
  itemId: string;
  /** 現在のスター状態 */
  isStarred: boolean;
  /** はてブ数（hatebuFetchedAt が null のときは表示されない） */
  hatebuCount: number;
  /** はてブ取得日時。null のとき数値ではなく "-" を表示する */
  hatebuFetchedAt: string | null;
  /** スター⭐️切替コールバック（楽観更新を呼び出し側 mutation で行う） */
  onToggleStar: (itemId: string, nextStarred: boolean) => void;
}
```

- Preconditions: `itemId` は空文字でない / `hatebuCount` は 0 以上
- Postconditions: クリック時に `e.stopPropagation()` が呼ばれた後、`onToggleStar(itemId, !isStarred)` が同期発火する
- Invariants: 描画結果は props のみに依存（内部状態なし）

##### data-testid 規約（NFR 3.2 / 3.3）

- `item-meta-actions-${itemId}` — メタグループ全体
- `item-hatebu-count-${itemId}` — はてブ数表示要素
- `item-star-toggle-${itemId}` — スター⭐️トグルボタン
- 既存 `star-${itemId}` (item-list.tsx 内の read-only star) は削除し、上記新規 testid に置き換える
- 既存 `search-result-star-${itemId}` (search-results.tsx 内の read-only star) は削除し、上記新規 testid に置き換える
- 既存 `star-toggle` / `hatebu-count` (item-detail.tsx) は削除（衝突しない命名）

#### ItemRow（改修）

| Field | Detail |
|-------|--------|
| Intent | 通常 / スター横断一覧の記事行レイアウト（既存）。タイトル行右端に `ItemMetaActions` を組み込む |
| Requirements | 1.7, 2.3, 4.1, 4.2, 6.1, 6.4, 6.5 |

**Responsibilities & Constraints**
- 既存責務: タイトル / 概要 / 公開日時 / (推定) バッジ / 既読薄表示 / 行クリック展開
- 追加責務: タイトル行の右端（日時の右隣）に `ItemMetaActions` を配置し、`onToggleStar` を伝搬する
- 既存の `is_starred && <Star />`（読み取り専用 star アイコン）は削除し、`ItemMetaActions` 内の Toggle Star に置き換える

**Dependencies**
- Inbound: `ItemList`, `StarredItemList` — 共通行として呼ばれる
- Outbound: `ItemMetaActions`（新規依存）
- External: なし

##### Props 変更

```typescript
interface ItemRowProps {
  item: ItemSummary;
  isExpanded: boolean;
  onClick: () => void;
  /** 新規: スター切替 callback */
  onToggleStar: (itemId: string, nextStarred: boolean) => void;
}
```

- 注意: `ItemSummary` には既に `hatebu_count` / `hatebu_fetched_at` が含まれている（通常一覧 / スター横断 API は既存対応済み）。`ItemRow` 内で `item.hatebu_count` / `item.hatebu_fetched_at` を `ItemMetaActions` に渡す

#### SearchResultRow（改修）

| Field | Detail |
|-------|--------|
| Intent | 検索結果の記事行（既存）。タイトル行右端に `ItemMetaActions` を組み込む |
| Requirements | 1.7, 4.3, 4.4, 5.3, 5.4, 6.1, 6.4, 6.5, 6.7 |

**Responsibilities & Constraints**
- 既存責務: フィード badge（横断検索時の favicon + feed_title） / タイトル / 概要 / 公開日時 / (推定) / 既読薄表示
- 追加責務: タイトル行の右端に `ItemMetaActions` を配置し、`onToggleStar` を伝搬する
- 既存の `hit.is_starred && <Star />`（読み取り専用）を削除し、`ItemMetaActions` 経由に置き換える
- `hit.hatebu_fetched_at`（型拡張後）を `ItemMetaActions` に渡す

##### Props 変更

```typescript
interface SearchResultRowProps {
  hit: ItemSearchHit;
  isExpanded: boolean;
  showFeedBadge: boolean;
  onClick: () => void;
  /** 新規: スター切替 callback */
  onToggleStar: (itemId: string, nextStarred: boolean) => void;
}
```

#### ItemDetail（改修）

| Field | Detail |
|-------|--------|
| Intent | 記事展開表示（既存）。ヘッダー meta グループ（hatebu / star）を撤去する |
| Requirements | 3.1, 3.2, 3.3, 3.4 |

**Responsibilities & Constraints**
- 削除: タイトル右側の `<div data-testid="item-detail-meta-group">` 全体（`hatebu-count`, `star-toggle` を含む）
- 維持: タイトル `<h3>`、タイトルリンク、著者表示、元記事リンク、本文サニタイズ + 折りたたみ + 「続きを読む」、`useEffect` での自動既読化
- props `onToggleStar` は引き続き型上は残す（`item-list.tsx` / `starred-item-list.tsx` / `search-results.tsx` が `ItemDetailArea` 経由で `ItemDetail` に渡すが、本コンポーネント内で使用しない不要 prop となる）。本 Issue では削除も可だが、`ItemDetailArea` の prop chain 変更を最小化するため **prop シグネチャは維持し本体での参照のみ削除**する判断とする
  - 代替案: `ItemDetail` から `onToggleStar` prop を完全削除すると `ItemDetailArea` / 3 呼び出し側コンポーネントから順次削除する波及があるため、本 Issue では維持を採用（cleanup は別 Issue）

##### Props（変更なし、本体実装のみ縮減）

```typescript
interface ItemDetailProps {
  item: ItemDetailType;
  onMarkAsRead: (itemId: string) => void;
  /** 詳細側からは使用しない（一覧側のみで使用）。型は互換維持のため残置 */
  onToggleStar: (itemId: string, isStarred: boolean) => void;
}
```

#### ItemList / StarredItemList / SearchResults（改修）

| Field | Detail |
|-------|--------|
| Intent | 各一覧コンポーネントが `onToggleStar` を `ItemRow` / `SearchResultRow` に prop drilling する |
| Requirements | 4.1, 4.2, 4.3 |

- `ItemList`: 既存 `handleToggleStar` を `<ItemRow ... onToggleStar={handleToggleStar} />` に渡す
- `StarredItemList`: 同上
- `SearchResults`: 既存 `handleToggleStar` を `<SearchResultRow ... onToggleStar={handleToggleStar} />` に渡す

### Frontend State Layer

#### useToggleStar（改修）

| Field | Detail |
|-------|--------|
| Intent | スター切替の mutation + 楽観更新 + 失敗時ロールバック（既存）。`["item-search"]` キャッシュも対象に拡張する |
| Requirements | 2.1, 2.2, 2.4, 2.5, 4.1, 4.2, 4.3, NFR 2.2, NFR 2.3 |

**Responsibilities & Constraints**
- 既存: `["items"]` 系の `InfiniteData<ItemListResponse>` を `onMutate` で書き換え、`onError` でロールバック、`onSettled` で invalidate
- 追加: `["item-search"]` の `InfiniteData<ItemSearchResponse>` も同様に `onMutate` で書き換え、`onError` で復元、`onSettled` で invalidate
- 1 つの mutation の onMutate / onError / onSettled 内で両 queryKey を扱う（mutation 並列化はしない）

**Dependencies**
- Inbound: `ItemList`, `StarredItemList`, `SearchResults`, `CrossFeedItemList`, `ItemDetail`(後方互換のため残置)
- Outbound: `apiClient.put("/api/items/:id/state", { is_starred })`, `queryClient.setQueryData` / `cancelQueries` / `invalidateQueries`
- External: TanStack Query `useMutation`

##### State Transitions

```typescript
// onMutate
//  1. ["items"] / ["item-search"] / ["cross-feed-items"] の進行中 refetch をキャンセル
//  2. queryClient.getQueriesData で current snapshots を取得（rollback 用）
//  3. setQueryData で各 InfiniteData の pages[].items[item.id === itemId].is_starred を更新
//  4. previousData = { items: [...], itemSearch: [...], crossFeed: [...] } を context として return
//
// onError (context, err)
//  - context.previousData をすべて setQueryData で復元
//
// onSettled
//  - ["items"] / ["item-search"] / ["cross-feed-items"] を invalidateQueries
```

##### Service Interface（疑似）

```typescript
interface ToggleStarParams {
  itemId: string;
  isStarred: boolean;
}

interface ToggleStarContext {
  previousItems: Array<[QueryKey, InfiniteData<ItemListResponse> | undefined]>;
  previousSearch: Array<[QueryKey, InfiniteData<ItemSearchResponse> | undefined]>;
  previousCrossFeed: Array<[QueryKey, InfiniteData<CrossFeedListResponse> | undefined]>;
}
```

- Preconditions: `itemId` が空でない
- Postconditions: クリックから 100ms 以内に UI 反映（`onMutate` 同期実行）、失敗時 200ms 以内に復元
- Invariants: ロールバック後のキャッシュ状態は mutation 前と同一

### Backend Layer

#### itemSearchHitResponse（改修）

| Field | Detail |
|-------|--------|
| Intent | 検索 API レスポンス 1 件の JSON 構造（既存）。`hatebu_fetched_at` フィールドを追加する |
| Requirements | 5.1, 5.2 |

##### Struct 変更

```go
type itemSearchHitResponse struct {
    ID              string     `json:"id"`
    FeedID          string     `json:"feed_id"`
    FeedTitle       string     `json:"feed_title"`
    FaviconURL      *string    `json:"favicon_url,omitempty"`
    Title           string     `json:"title"`
    Link            string     `json:"link"`
    Summary         string     `json:"summary"`
    PublishedAt     time.Time  `json:"published_at"`
    IsDateEstimated bool       `json:"is_date_estimated"`
    IsRead          bool       `json:"is_read"`
    IsStarred       bool       `json:"is_starred"`
    HatebuCount     int        `json:"hatebu_count"`
    // 新規: 既存 ItemSummary レスポンスと同じく、未取得時は nil → JSON omit
    HatebuFetchedAt *time.Time `json:"hatebu_fetched_at,omitempty"`
}
```

- 注意: フロント側で「omit (= 未取得)」と「null」を等価扱いするため、TypeScript 型は `string | null` とし、レスポンス JSON にフィールドが存在しないケースは TypeScript で `undefined` となるが、`undefined ?? null` の正規化で `-` 表示に統一する。代替として Go 側で `omitempty` を外し常に `null` を出力する案もあるが、既存 `favicon_url` と同じ流儀を採用

##### Contract（API レスポンス JSON 例）

```json
{
  "items": [
    {
      "id": "uuid-1",
      "hatebu_count": 0,
      "hatebu_fetched_at": "2026-05-29T10:00:00Z"
    },
    {
      "id": "uuid-2",
      "hatebu_count": 0
    }
  ],
  "next_cursor": "...",
  "has_more": false
}
```

- 1 件目: 取得済み・はてブ 0 件 → フロントで `0` を表示
- 2 件目: 未取得（`hatebu_fetched_at` フィールド省略） → フロントで `-` を表示

#### ItemSearchSummary（改修）

| Field | Detail |
|-------|--------|
| Intent | サービス層の検索結果 1 件表現（既存）。`HatebuFetchedAt` を追加し、リポジトリから handler レスポンスまで pass-through |
| Requirements | 5.1, 5.2 |

##### Struct 変更

```go
type ItemSearchSummary struct {
    // ...既存フィールド...
    HatebuCount     int
    HatebuFetchedAt *time.Time  // 新規: 未取得時は nil
}
```

#### ItemSearchHit（改修）

| Field | Detail |
|-------|--------|
| Intent | リポジトリ層の検索結果 1 件モデル（既存）。`HatebuFetchedAt` を追加し、SQL SELECT 結果を保持する |
| Requirements | 5.1, 5.2 |

##### Struct 変更

```go
type ItemSearchHit struct {
    // ...既存フィールド...
    HatebuCount     int
    HatebuFetchedAt *time.Time  // 新規: items.hatebu_fetched_at が NULL なら nil
}
```

#### SearchByUserAndKeyword（改修）

| Field | Detail |
|-------|--------|
| Intent | 検索 SQL（既存）。SELECT 列に `i.hatebu_fetched_at` を追加し、`sql.NullTime` で Scan する |
| Requirements | 5.1, 5.2 |

##### SQL 変更

```sql
SELECT
    i.id, i.feed_id, i.title, i.link, i.summary,
    i.published_at, i.is_date_estimated, i.hatebu_count,
    i.hatebu_fetched_at,                       -- 新規追加
    f.title AS feed_title,
    f.favicon_data, f.favicon_mime,
    COALESCE(st.is_read, false)   AS is_read,
    COALESCE(st.is_starred, false) AS is_starred
FROM items i
JOIN subscriptions s ...
```

- Scan: 既存パターン（`sql.NullTime` → `if .Valid { hit.HatebuFetchedAt = &.Time }`）に従う
- WHERE 句 / ORDER BY / LIMIT は無変更

#### ItemSearchServiceAdapter.Search（改修）

| Field | Detail |
|-------|--------|
| Intent | サービス層 → handler レスポンスへの変換（既存）。`HatebuFetchedAt` のコピーを追加 |
| Requirements | 5.1, 5.2 |

##### 変更箇所

```go
hit := itemSearchHitResponse{
    // ...既存フィールド...
    HatebuCount:     it.HatebuCount,
    HatebuFetchedAt: it.HatebuFetchedAt,  // 新規: pointer をそのまま pass
}
```

## Data Models

### Domain Model

- アグリゲート: 記事 (`Item`) — `id`, `hatebu_count`, `hatebu_fetched_at` は既存属性
- 値オブジェクト: `HatebuStatus = { count: int, fetchedAt: time.Time? }` を概念的に持つ（実装上は分離された 2 フィールド）
- ドメインイベント: なし（はてブ取得は worker 側で発生し、本 Issue では既存挙動を変更しない）

### Physical Data Model

- `items.hatebu_fetched_at TIMESTAMPTZ NULL`（既存列、スキーマ変更なし）
- マイグレーション: 不要

### Frontend / Backend 型同期マトリクス

| TypeScript フィールド | JSON キー | Go struct | DB 列 |
|---|---|---|---|
| `hatebu_count: number` | `hatebu_count` | `HatebuCount int` | `items.hatebu_count` |
| `hatebu_fetched_at: string \| null` | `hatebu_fetched_at` (omitempty) | `HatebuFetchedAt *time.Time` | `items.hatebu_fetched_at` |

- 通常一覧 (`/api/feeds/:id/items`) / スター横断 (`/api/feeds/starred/items`) / 検索 (`/api/items/search`) の 3 API すべてが同じ 2 フィールドを返すようになる

## Error Handling

### Error Strategy

- スター切替 mutation の失敗は **楽観更新ロールバック + ユーザー知覚最小化** の戦略を採る
- 詳細ヘッダーからスター UI を撤去するため、エラー時の visual feedback（赤い border 等）は **一覧行のスター⭐️トグル**に集約する
- 既存挙動と同様、トースト UI は本 Issue では追加しない（Out of Scope）。`onError` は previousData の復元のみを行う

### Error Categories and Responses

- **User Errors (4xx)**:
  - `400 INVALID_SEARCH_QUERY` — 検索クエリ形式不正（既存挙動維持）
  - `401 UNAUTHORIZED` — セッション切れ（既存挙動維持）
  - `403 FEED_NOT_SUBSCRIBED` — フィード内検索のスコープ未購読（既存挙動維持）
- **System Errors (5xx)**:
  - `500 INTERNAL_ERROR` — DB エラー時、検索リクエストは既存どおり 500 を返す
  - スター切替 PUT が 5xx の場合、フロントは `onError` で楽観更新をロールバックする
- **Business Logic Errors**:
  - はてブ未取得状態（`hatebu_fetched_at === null`）はエラーではなく `-` 表示の正常系として扱う（Req 1.3, 5.3）

### 楽観更新の失敗時挙動（具体例）

```
ユーザー操作:    クリック → onClick(stopPropagation) → useToggleStar.mutate({ itemId, isStarred: true })
                         ↓                            ↓
時刻 t+0ms:    onMutate: cancelQueries + setQueryData → UI に塗りつぶし Star を即時表示
時刻 t+150ms:  fetch PUT /api/items/:id/state ... 5xx エラー
時刻 t+200ms:  onError: previousData を setQueryData で復元 → UI がアウトライン Star に戻る
時刻 t+250ms:  onSettled: invalidateQueries で server-side truth を再取得
```

## Testing Strategy

- **Unit Tests**:
  1. `ItemMetaActions`: `hatebu_fetched_at === null` のとき `-` を表示する / `hatebu_count = 0` で `0` を表示する
  2. `ItemMetaActions`: `is_starred=true` で `fill-yellow-400` の Star、`false` でアウトライン Star を表示する
  3. `ItemMetaActions`: クリック時に `e.stopPropagation()` が呼ばれ、`onToggleStar(itemId, !isStarred)` が発火する（mock callback で検証）
  4. `ItemMetaActions`: `aria-label` / `aria-pressed` が現状態と整合する
  5. `useToggleStar`: `["item-search"]` キャッシュへの楽観更新と `onError` ロールバックが動作する

- **Integration Tests**:
  1. `ItemList`: 一覧行のスター⭐️トグルクリック後、行クリック展開（`expandedItemId` 設定）が発火しない
  2. `StarredItemList`: 一覧行のスター⭐️トグルクリック後、`useToggleStar` mutation が発火し UI 反映される
  3. `SearchResults`: 検索結果行の `hatebu_fetched_at === null` で `-`、値ありで数値が表示される
  4. `SearchResults`: 検索結果行のスター⭐️トグルクリックで他一覧キャッシュ（`["items"]`）への波及が起こる
  5. `ItemDetail`: ヘッダー領域に `hatebu-count` / `star-toggle` testid が存在しない（撤去確認）

- **Backend Tests**（Go）:
  1. `item_search_handler_test.go`: レスポンス JSON に `hatebu_fetched_at` が含まれる（取得済み記事）/ omit される（未取得記事）
  2. `itemsearch/service_test.go`: `ItemSearchSummary.HatebuFetchedAt` がリポジトリ結果から pass-through される
  3. `postgres_item_repo_search_test.go`: SELECT 結果の `HatebuFetchedAt` が `items.hatebu_fetched_at` カラム値と一致する（NULL → nil / 値あり → 同一時刻）

- **Non-Regression Tests**:
  1. `ItemList` / `StarredItemList` / `SearchResults` の既存無限スクロール / 既読薄表示 / 空・エラー状態テストが全 pass する
  2. `ItemDetail` の本文サニタイズ / 折りたたみ / 自動既読化テストが全 pass する
  3. `item_search_handler_test.go` の既存 JSON フィールド（feed_title, favicon_url, hatebu_count 等）の検証が全 pass する

## Security Considerations

- スター切替 API（`PUT /api/items/:id/state`）は既存実装で認証必須・ユーザー自身の状態のみ更新可能（既存挙動維持）
- `hatebu_fetched_at` フィールドはユーザー固有ではなく記事固有の情報のため、サブスクリプション境界以外の権限チェックは不要
- 検索 SQL の SELECT 列追加は `items.hatebu_fetched_at` を購読中フィードの記事のみに対して返す（既存 JOIN 構造が境界を担保）

## Performance & Scalability

- `ItemMetaActions` は props のみで描画決定するメモ化対象（必要なら `React.memo` で包む。本 Issue では未適用とし、tasks.md 完了後の体感パフォーマンスを観察してから判断）
- 検索 SELECT 列追加 1 つは既存クエリプランに影響を与えない（インデックスは title / content の ILIKE と (published_at, id) の order がドミナント）
- `useToggleStar` の `["item-search"]` キャッシュ書き換えは O(検索キャッシュ件数 × ページ数) で実行され、典型ケース（数十件 × 数ページ）では UI フレーム内に完了する

## Notes for Developers

- `ItemDetail` の `onToggleStar` prop は型のみ残置し本体実装で使用しないが、`ItemDetailArea` の prop chain を最小変更にとどめるための過渡的措置。将来的に `ItemDetailArea` / 3 一覧から不要 prop を削除する cleanup は別 Issue で対応する判断（本 Issue のスコープ外）
- 検索結果の `hatebu_fetched_at` フィールドは Go 側 `omitempty` のため未取得時は JSON に含まれない。TypeScript 側は `string | null | undefined` のいずれかを受け、`?? null` で正規化する
