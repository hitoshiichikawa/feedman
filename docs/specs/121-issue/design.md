# Design Document

## Overview

**Purpose**: フィードを跨いだ新着記事の横断一覧（Cross-Feed Timeline）を Feedman に追加し、複数フィードを購読するユーザーが個別フィードを巡回せずに新着の有無を一望できるようにする。既存の 2 ペイン UI に「すべての新着記事」仮想エントリを追加し、選択時に全購読フィードの新着記事を `published_at` 降順でマージ表示する。

**Users**: Feedman のログインユーザーが、毎日の新着確認 workflow（複数の購読フィードを順番に開いて新着を消化する操作）に対して、本機能で「左ペインの最上部エントリを 1 クリック→右ペインで全フィード横断の新着リストを参照」という一発操作に置き換えて利用する。

**Impact**: 現在は `selectedFeedId: string | null` を起点に「個別フィード 1 件」のみを右ペインに表示する単一モードのアーキテクチャを、(1) 横断モード／個別モードの 2 系統 ViewMode に拡張し、(2) ユーザー単位で「最後に横断一覧を開いた時刻」をサーバ側 DB（`user_cross_feed_views` テーブル）に永続化することで複数デバイスをまたいで一貫した新着判定を提供する。既存個別フィード閲覧の挙動・レイアウト・パフォーマンスは後退させない（Requirement 5 / NFR 1.2 / NFR 2）。

### Goals

- 左ペイン最上部に常設仮想エントリ「すべての新着記事」を追加し、選択時に全購読フィードの新着記事を `published_at DESC, id DESC` の決定論的順序でマージ表示する（Req 1, 2）
- 各記事カードに発信元フィードのフィード名と favicon バッジを併記し、favicon 未設定時は既存個別フィード一覧と同じ代替アイコン（lucide-react `Rss`）を表示する（Req 3、#122 で実装済の `FeedFavicon` を抽出再利用）
- 「前回横断一覧を開いた時刻」をユーザー単位で DB 永続化し、再ログイン後・複数デバイス間で一貫した新着抽出を提供する（Req 4）
- 既存の個別フィード閲覧導線・既読／スター操作・未読数バッジ・スクロール挙動を一切変更しない（Req 5, NFR 2.1）
- 横断一覧の初期表示開始を 1 秒以内とし、ページング（limit 50 件 / cursor）で大量件数時も UI を無応答にしない（NFR 1.1, 1.3）

### Non-Goals

- フィード別の除外フィルタ（特定フィードを横断一覧から外す）
- 公開日時降順以外のソート切替・横断一覧専用の検索／タグ／ハイライト
- 全フィードの「全記事」マージ表示（本機能は「新着」のみ）
- 横断一覧から発信元フィードの設定編集・購読停止導線
- 「すべての新着記事」エントリ自体への未読数バッジ表示（要件 PM 確認事項 6、本スコープでは未表示で確定。後述 Components 注釈参照）
- 既読記事の「除外」フィルタ実装（PM 確認事項 4、本スコープでは新着抽出後の既読記事も表示し既存と同じ視覚扱い）
- メール・プッシュ通知・外部システム連携

## Architecture

### Existing Architecture Analysis

- **Backend**（`api`）: chi v5 ルータで `/api/feeds/{id}/items`（個別）/ `/api/items/{id}`（詳細）/ `PUT /api/items/{id}/state`（既読・スター）を提供。サービス層は `internal/item.ItemService`（ListItems / GetItem）と `internal/item.ItemStateService`（UpdateState）に分離。リポジトリは `internal/repository/postgres_item_repo.go` の `ListByFeed` がカーソルベース（`published_at < $cursor` で降順ページング）で実装済
- **Frontend**（`web`）: Next.js App Router + React Query。`AppStateContext` が `{ selectedFeedId, expandedItemId, filter }` を `useReducer` で管理。`FeedList` → `ItemList` という単純な props 駆動の連携。記事行コンポーネントは `ItemList` 内の `ItemRow` private function
- **DB**: PostgreSQL（pgcrypto / UUID PK）。マイグレーションは `internal/database/migrations/<YYYYMMDDHHMMSS>_<name>.up.sql` / `.down.sql` 命名規約で golang-migrate
- **#122 favicon 機能**: `subscription.SubscriptionInfo.FaviconURL *string`（data URL 形式）として既に API 経由で配信済。`FeedFavicon` コンポーネントが `feed-list.tsx` 内 private function として存在し、`<img>` の `onError` で `Rss` アイコン fallback を担当

維持すべき統合点:

- カーソルベースページング（`published_at` 単調降順 + `limit+1` で HasMore 判定）の既存 contract
- `model.ItemWithState`（item + 既読／スター LEFT JOIN）のスキャン形式
- `handler.ItemServiceInterface` adapter を介した DTO 変換（domain layer 型を handler 層に漏らさない）
- 既存 `useItems(feedId, filter)` フックの contract（`null` 渡し時に query 無効化）

解消・回避する technical debt:

- `FeedFavicon` が `feed-list.tsx` 内 private のため横断一覧記事カードから再利用できない → 独立コンポーネント化して shared util に昇格させる
- `AppStateContext.selectedFeedId: string | null` の 2 値表現では「未選択」と「横断モード」が区別できない → ViewMode 判別子に拡張（後述 Components）

### Architecture Pattern & Boundary Map

採用パターン: **既存の Layered Architecture（Handler → Service → Repository → DB）の踏襲 + 横断 timeline 専用ドメイン subpackage の追加**。新規境界として `internal/crossfeed`（horizontal aggregation domain）と DB テーブル `user_cross_feed_views`、API endpoint `GET /api/items/cross-feed`、Frontend hook `useCrossFeedItems` / Context ViewMode を導入する。

```mermaid
flowchart LR
  subgraph Web[web/ Next.js]
    AS[AppStateContext<br/>viewMode + lastCrossFeedSeenAt]
    FL[FeedList<br/>+ AllNewItemsEntry]
    IL[ItemList<br/>routing by viewMode]
    CFI[CrossFeedItemList<br/>+ FeedBadge]
    FF[FeedFavicon<br/>shared]
    HCFI[useCrossFeedItems<br/>useInfiniteQuery]
    AS --> FL
    AS --> IL
    IL -- viewMode=cross --> CFI
    IL -- viewMode=feed --> existingItemList[(既存 ItemList 振る舞い)]
    CFI --> HCFI
    CFI --> FF
    FL --> FF
  end

  subgraph API[api/ Go chi]
    H[CrossFeedHandler<br/>GET /api/items/cross-feed<br/>PUT /api/users/me/cross-feed-last-seen]
    S[crossfeed.Service<br/>ListNewItems / TouchLastSeen]
    R1[ItemRepository<br/>+ ListNewAcrossFeeds]
    R2[UserCrossFeedViewRepository<br/>Get / Upsert]
    H --> S
    S --> R1
    S --> R2
  end

  HCFI -- HTTP --> H

  subgraph DB[(PostgreSQL)]
    T1[(items)]
    T2[(subscriptions)]
    T3[(feeds)]
    T4[(item_states)]
    T5[(user_cross_feed_views<br/>NEW)]
  end

  R1 -- JOIN items×subscriptions×feeds×item_states --> T1
  R1 --> T2
  R1 --> T3
  R1 --> T4
  R2 --> T5
```

**Architecture Integration**:
- 採用パターン: 既存の `internal/<domain>/service.go` + `internal/handler/<domain>_handler.go` + `internal/repository/postgres_<domain>_repo.go` パターンを **そのまま** 踏襲（新規ドメインを subscription / item と同位の `crossfeed` として追加）
- ドメイン／機能境界:
  - `crossfeed` 集約: 「横断新着取得」+「最後に開いた時刻の管理」を 1 ドメインに閉じ込める（DB 表 `user_cross_feed_views` の所有も `crossfeed` package が責任）
  - 既存 `item` / `subscription` / `feed` ドメインは **書き換えない**（`ItemRepository` には新メソッド `ListNewAcrossFeeds` のみ追加。`ListByFeed` 等の既存メソッド signature 不変）
- 既存パターンの維持: chi ルータの `SetupItemRoutes` 相当として `SetupCrossFeedRoutes` を追加 / `RouterDeps` に `CrossFeedService` フィールドを追加 / `service_adapter.go` に `CrossFeedServiceAdapter` を追加（DTO 変換）/ 既存 `handler.handleServiceError` を再利用
- 新規コンポーネントの根拠:
  - `crossfeed.Service`: 「最後に開いた時刻取得→新着抽出→新着結果の cursor 計算→TouchLastSeen の分離」がドメインルール（既存 `ItemService` には無関係なので分離）
  - `UserCrossFeedViewRepository`: `users` テーブルへ列追加すると `User` model 全体に影響するため、別表に切り出すことで Req 4 のスコープを局所化（Open/Closed 原則）

### Technology Stack

| Layer | Choice / Version | Role in Feature | Notes |
|-------|------------------|-----------------|-------|
| Frontend / CLI | Next.js 15 (App Router) + React 19 + TypeScript 5 | 左ペイン仮想エントリ・横断一覧 UI・ViewMode 切替 | 既存 `web/` 配下に追加。新規依存ライブラリなし |
| Frontend Data | TanStack React Query | `useCrossFeedItems`（useInfiniteQuery）/ `useTouchCrossFeedLastSeen`（useMutation） | 既存 `use-items.ts` と同パターン |
| Frontend UI | Tailwind CSS 4 + shadcn/ui + lucide-react | 仮想エントリ button / フィード badge / `Rss` 代替アイコン | 既存 `feed-list.tsx` / `item-list.tsx` のスタイル規約を踏襲 |
| Backend / Services | Go 1.25 + chi/v5 | `GET /api/items/cross-feed` / `PUT /api/users/me/cross-feed-last-seen` ハンドラ・サービス | 既存 chi ルータ `NewRouter` の認証必須グループに登録 |
| Data / Storage | PostgreSQL 16 + `lib/pq` + golang-migrate | `user_cross_feed_views` 新テーブル / JOIN クエリ | up/down マイグレーションを追加 |
| Messaging / Events | （N/A） | — | 横断一覧は同期 GET / PUT で完結。バックグラウンドジョブ追加なし |
| Infrastructure / Runtime | Docker / docker-compose（既存構成踏襲） | — | 構成変更なし |
| Test | Go `testing` + `testdata/` / Vitest + Testing Library | repository / service / handler / hook / component / context | 既存テスト配置規約を踏襲 |

## File Structure Plan

### Directory Structure

```
internal/
├── crossfeed/                                  # 新規ドメイン: 横断新着集約
│   ├── service.go                              # ListNewItems / TouchLastSeen ビジネスロジック
│   └── service_test.go                         # サービス層単体テスト（境界値・タイブレーク・初回 fallback）
├── model/
│   └── crossfeed.go                            # NEW: UserCrossFeedView 構造体（user_id, last_seen_at）
├── repository/
│   ├── interfaces.go                           # MODIFIED: ItemRepository に ListNewAcrossFeeds 追加 / UserCrossFeedViewRepository 追加 / CrossFeedItem 型追加
│   ├── postgres_item_repo.go                   # MODIFIED: ListNewAcrossFeeds 実装（JOIN クエリ + cursor）
│   ├── postgres_item_repo_test.go              # MODIFIED: ListNewAcrossFeeds の integration test
│   ├── postgres_user_cross_feed_view_repo.go   # NEW: Get / Upsert 実装
│   └── postgres_user_cross_feed_view_repo_test.go # NEW
├── handler/
│   ├── crossfeed_handler.go                    # NEW: GET /cross-feed と PUT /cross-feed-last-seen
│   ├── crossfeed_handler_test.go               # NEW
│   ├── service_adapter.go                      # MODIFIED: CrossFeedServiceAdapter 追加 / DTO 変換
│   └── router.go                               # MODIFIED: RouterDeps に CrossFeedService 追加 / 認証必須グループにルート登録
├── database/migrations/
│   ├── 20260528120000_add_user_cross_feed_views.up.sql   # NEW
│   └── 20260528120000_add_user_cross_feed_views.down.sql # NEW
└── app/
    └── app.go                                  # MODIFIED: runServe で crossfeed.Service と Repository を初期化・配線

web/src/
├── types/
│   └── crossfeed.ts                            # NEW: CrossFeedItem / CrossFeedListResponse 型
├── hooks/
│   ├── use-cross-feed-items.ts                 # NEW: useInfiniteQuery + useMutation（touch）
│   └── use-cross-feed-items.test.tsx           # NEW
├── components/
│   ├── feed-favicon.tsx                        # NEW: feed-list.tsx から抽出（shared）
│   ├── feed-favicon.test.tsx                   # NEW
│   ├── feed-list.tsx                           # MODIFIED: AllNewItemsEntry を最上部に常設 / FeedFavicon import 化
│   ├── feed-list.test.tsx                      # MODIFIED: 新エントリの click・選択状態テストを追加
│   ├── cross-feed-item-list.tsx                # NEW: 横断一覧表示（FeedBadge を含む）
│   ├── cross-feed-item-list.test.tsx           # NEW
│   ├── app-shell.tsx                           # MODIFIED: viewMode による ItemList / CrossFeedItemList 切替
│   └── app-shell.test.tsx                      # MODIFIED
└── contexts/
    ├── app-state.tsx                           # MODIFIED: viewMode discriminator 追加 / SELECT_ALL_NEW_ITEMS action
    └── app-state.test.tsx                      # MODIFIED
```

### Modified Files

- `internal/repository/interfaces.go` — `ItemRepository` に `ListNewAcrossFeeds` を追加 / `CrossFeedItem` row 型と `UserCrossFeedViewRepository` interface を新設
- `internal/repository/postgres_item_repo.go` — `ListNewAcrossFeeds` を実装。`items i JOIN subscriptions s ON s.feed_id=i.feed_id AND s.user_id=$1 JOIN feeds f ON f.id=i.feed_id LEFT JOIN item_states st ON st.item_id=i.id AND st.user_id=$1 WHERE i.published_at > $2 [AND (i.published_at, i.id) < ($3, $4)] ORDER BY i.published_at DESC, i.id DESC LIMIT $5`（cursor は `(published_at, id)` 複合キー）
- `internal/handler/service_adapter.go` — `CrossFeedServiceAdapter` を追加し domain `crossfeed.NewItemsResult` を handler DTO に変換
- `internal/handler/router.go` — `RouterDeps.CrossFeedService` 追加 / 認証必須グループ内に `r.Route("/api/items/cross-feed", ...)` と `r.Put("/api/users/me/cross-feed-last-seen", ...)` を登録
- `internal/app/app.go` — `runServe` に `userCrossFeedViewRepo := repository.NewPostgresUserCrossFeedViewRepo(db)` / `crossFeedService := crossfeed.NewService(itemRepo, userCrossFeedViewRepo)` / `deps.CrossFeedService = handler.NewCrossFeedServiceAdapter(crossFeedService)` を追加
- `web/src/contexts/app-state.tsx` — `AppState` を `{ viewMode: 'feed' | 'cross-feed' | 'none', selectedFeedId: string | null, expandedItemId, filter }` に拡張し、`SELECT_ALL_NEW_ITEMS` / `SELECT_FEED` action を追加
- `web/src/components/app-shell.tsx` — `viewMode === 'cross-feed'` のとき `<CrossFeedItemList />` を、それ以外は既存 `<ItemList />` を描画
- `web/src/components/feed-list.tsx` — 最上部に `<AllNewItemsEntry>` 常設 button を追加。`FeedFavicon` を新規 `feed-favicon.tsx` から import 化
- `internal/repository/postgres_item_repo_test.go` — `ListNewAcrossFeeds` の integration test を追加（タイブレーク・cursor 動作・初回 fallback 不在検証）

## Requirements Traceability

| Requirement | Summary | Components | Interfaces | Flows |
|-------------|---------|------------|------------|-------|
| 1.1 | 左ペイン先頭に「すべての新着記事」常設 | AllNewItemsEntry / FeedList | FeedList props（viewMode 渡し） | FeedList render 時に常時 1 件追加 |
| 1.2 | 選択時に右ペインを横断一覧に切替 | AppStateContext / AppShell | dispatch SELECT_ALL_NEW_ITEMS | viewMode='cross-feed' に遷移 |
| 1.3 | 横断選択中→個別フィード選択で個別表示に戻す | AppStateContext / FeedList | dispatch SELECT_FEED | viewMode='feed' & selectedFeedId 更新 |
| 1.4 | 選択中エントリの視覚強調 | AllNewItemsEntry | viewMode === 'cross-feed' で `bg-accent` | 既存 FeedList と同 className 規約 |
| 1.5 | レイアウト整合 | AllNewItemsEntry / FeedList | 既存 button スタイル踏襲 | favicon 領域に `Rss` 代替アイコン配置 |
| 2.1 | 全購読フィード集約 | crossfeed.Service / ItemRepository.ListNewAcrossFeeds | SQL: subscriptions JOIN items WHERE user_id=$1 | userID から購読フィードを限定 |
| 2.2 | published_at 降順マージ | ItemRepository.ListNewAcrossFeeds | ORDER BY published_at DESC, id DESC | DB レベルでマージ |
| 2.3 | 同一日時の決定論順序 | ItemRepository.ListNewAcrossFeeds | (published_at, id) 複合 tiebreak | PM 確認事項 5 に対する設計確定: 記事 ID 降順 |
| 2.4 | 既存個別フィードと同等情報表示 | CrossFeedItemList / ItemRow 再利用 | CrossFeedItem に既存 ItemSummary フィールド + feed メタ | 同レイアウトを共有 |
| 2.5 | 既読化／スター操作の同一性 | useMarkAsRead / useToggleStar（既存再利用） | PUT /api/items/:id/state（既存） | mutation 成功時に `["cross-feed-items"]` キャッシュも invalidate |
| 3.1 | 記事カードにフィード名表示 | CrossFeedItemList の FeedBadge | DTO の feed_title フィールド | ItemRow 拡張 props |
| 3.2 | 記事カードに favicon バッジ表示 | FeedFavicon（shared） | DTO の feed_favicon_url フィールド | 既存 FeedFavicon 抽出再利用 |
| 3.3 | favicon 未設定・読込失敗時の代替アイコン | FeedFavicon | `Rss` lucide icon | 既存 #122 の onError fallback ロジック踏襲 |
| 3.4 | レイアウト視認性阻害なし | CrossFeedItemList / ItemRow 拡張 | バッジは右上 or 左 16px 領域 | Tailwind `flex-shrink-0` で固定幅 |
| 4.1 | ユーザーごとに最後に開いた時刻を記録 | UserCrossFeedViewRepository / crossfeed.Service | `user_cross_feed_views(user_id PK, last_seen_at)` | Upsert で 1 行/ユーザー |
| 4.2 | 当該時刻より後の記事を新着抽出 | ItemRepository.ListNewAcrossFeeds | `WHERE i.published_at > $sinceTime` | sinceTime は前回保存値 or fallback |
| 4.3 | 表示処理完了時に時刻を更新 | crossfeed.Service / TouchLastSeen | PUT /api/users/me/cross-feed-last-seen | 別エンドポイント分離（PM 確認事項 2） |
| 4.4 | 初回は既定窓 fallback | crossfeed.Service | `if lastSeen IS NULL: sinceTime = now - 24h` | PM 確認事項 2: 24 時間を design 確定 |
| 4.5 | 再ログイン後も保持 | user_cross_feed_views テーブル | DB 永続化（PM 確認事項 1: サーバ側採用） | session lifecycle と独立 |
| 4.6 | 新着 0 件時の空状態表示 | CrossFeedItemList | empty state UI | 既存「記事がありません」相当のメッセージ |
| 5.1 | 個別フィード閲覧の挙動不変 | AppStateContext / ItemList（既存） | viewMode='feed' 時は既存 ItemList を一切変更せず描画 | reducer の SELECT_FEED は既存と同等 reset 挙動 |
| 5.2 | 個別フィード並び順・スタイル不変 | FeedList（最上部追加のみ） | 既存 feeds.map 部分は無変更 | FeedFavicon の抽出は同一の見た目 |
| 5.3 | 既読・スター操作の同期 | useMarkAsRead / useToggleStar / queryClient | mutation success 時に `["items", ...]` と `["cross-feed-items"]` 両方 invalidate | Frontend キャッシュ整合 |
| 5.4 | 横断→個別→横断戻り時の最新状態反映 | useCrossFeedItems | useInfiniteQuery の refetchOnMount + staleTime 設定 | useFeeds と同パターン |
| NFR 1.1 | 初期表示 1 秒以内 | ItemRepository.ListNewAcrossFeeds + index 設計 | `idx_subscriptions_user_id` + `idx_items_feed_published_at`（既存）の組み合わせで JOIN | limit=50 で初回 fetch |
| NFR 1.2 | 個別フィード初期表示の悪化なし | （既存 ListByFeed に変更を加えない） | 新規パスのみ追加 | 既存クエリのプランは不変 |
| NFR 1.3 | 大量件数時のページング | crossfeed.Service / useCrossFeedItems | cursor-based pagination（limit=50 + has_more） | 既存 ListByFeed と同方式 |
| NFR 2.1 | 既存左ペイン挙動不変 | FeedList | 既存 feeds.map と onSelectFeed は不変 | dispatch path のみ拡張 |
| NFR 2.2 | 既読化・スター付与の同一 API/契約 | useMarkAsRead / useToggleStar（既存再利用） | PUT /api/items/:id/state（既存） | 新規 API なし |
| NFR 3.1 | キーボード操作対応 | AllNewItemsEntry | `<button>` ネイティブ tab/Enter 対応 | aria-current 等を付与 |
| NFR 3.2 | テキストコントラスト | CrossFeedItemList / FeedBadge | 既存 `text-muted-foreground` 規約 | 既存記事一覧と同基準 |

## Components and Interfaces

### Backend Domain Layer

#### crossfeed.Service

| Field | Detail |
|-------|--------|
| Intent | 横断新着一覧の取得と「最後に横断一覧を開いた時刻」の管理を担うドメインサービス |
| Requirements | 2.1, 2.2, 2.3, 4.1, 4.2, 4.3, 4.4, 4.5 |

**Responsibilities & Constraints**
- 主責務: ユーザーの最終アクセス時刻を読み出し→新着記事の cursor ベース取得→TouchLastSeen で時刻更新（更新は別エンドポイント呼び出しで実施、List 自体は副作用なし）
- ドメイン境界: 「横断 timeline view 状態」と「item 集合の絞り込みルール」を所有。`item` ドメイン（個別フィード）の責務は侵さない
- 不変条件: TouchLastSeen は冪等（同時刻書き込みは安全）。ListNewItems は同一引数で常に同一結果（DB 状態が変わらない限り）

**Dependencies**
- Inbound: `handler.CrossFeedServiceAdapter` — DTO 変換 (Critical)
- Outbound: `repository.ItemRepository.ListNewAcrossFeeds` — 新着集約 (Critical) / `repository.UserCrossFeedViewRepository` — last_seen Get/Upsert (Critical)
- External: なし

**Contracts**: Service [x] / API [ ] / Event [ ] / Batch [ ] / State [ ]

##### Service Interface

```go
package crossfeed

// Service は横断新着 timeline のビジネスロジック。
type Service struct { /* itemRepo, userCrossFeedViewRepo */ }

// NewItemsResult は ListNewItems の戻り値。
type NewItemsResult struct {
    Items      []CrossFeedItemSummary // public domain DTO
    NextCursor string                  // 空文字なら更なるページ無し
    HasMore    bool
    SinceTime  time.Time               // 抽出に用いた閾値（観測用）
}

// CrossFeedItemSummary は ItemSummary + 発信元フィードメタ
type CrossFeedItemSummary struct {
    item.ItemSummary
    FeedID         string
    FeedTitle      string
    FeedFaviconURL *string // data URL（subscription.SubscriptionInfo と同形式）。未設定なら nil
}

// ListNewItems はユーザーの全購読フィードから lastSeen 以降の新着を public 降順で取得する。
// lastSeen 記録が無いユーザーは 24 時間 fallback を採用する（Req 4.4）。
// cursorStr は (RFC3339Nano, itemID) を区切り文字 ":" で連結した複合カーソル。
func (s *Service) ListNewItems(ctx context.Context, userID, cursorStr string, limit int) (*NewItemsResult, error)
// Preconditions: userID != ""、limit > 0
// Postconditions: 戻り値 Items は published_at DESC, id DESC の決定論順序

// TouchLastSeen は「最後に横断一覧を開いた時刻」を now() で UPSERT する。
// 単独で呼び出され、ListNewItems からは呼ばない（リトライ・冪等性のため分離）。
func (s *Service) TouchLastSeen(ctx context.Context, userID string) error
// Postconditions: user_cross_feed_views(user_id=userID).last_seen_at = now()
```

### Backend Repository Layer

#### ItemRepository.ListNewAcrossFeeds（既存 interface への追加メソッド）

| Field | Detail |
|-------|--------|
| Intent | 全購読フィードを横断した新着記事集合を 1 クエリで取得し、N+1 を回避 |
| Requirements | 2.1, 2.2, 2.3, 4.2, NFR 1.1 |

**Responsibilities & Constraints**
- 主責務: `items × subscriptions × feeds × item_states (LEFT)` を JOIN し、`published_at > sinceTime` の条件で `published_at DESC, id DESC` 降順 limit 取得
- 制約: 既存 `idx_subscriptions_user_id` + `idx_items_feed_published_at` を活用（subscription を user_id で絞った後 feed_id で items を引く）。新規 index は **不要**（後述 Data Models で根拠を明示）
- 戻り値の row 型 `CrossFeedItem` は `model.ItemWithState` を embed + feed メタを追加

**Dependencies**
- Inbound: `crossfeed.Service.ListNewItems`
- Outbound: PostgreSQL（既存 `db *sql.DB`）

**Contracts**: Service [x]

##### Repository Interface（差分のみ）

```go
package repository

// ItemRepository に追加
type ItemRepository interface {
    // ... existing methods ...

    // ListNewAcrossFeeds は user の全購読フィードから sinceTime より後の記事を取得する。
    // cursor は (publishedAt, itemID) の複合キー。cursorPublishedAt がゼロ値の場合は先頭から取得。
    // 戻り値は published_at DESC, id DESC で決定論的に並ぶ。limit+1 件取得して呼び出し側が HasMore を判定する。
    ListNewAcrossFeeds(
        ctx context.Context,
        userID string,
        sinceTime time.Time,
        cursorPublishedAt time.Time,
        cursorItemID string,
        limit int,
    ) ([]CrossFeedItem, error)
}

// CrossFeedItem は横断一覧で利用する row 型。
type CrossFeedItem struct {
    model.ItemWithState
    FeedTitle       string
    FaviconData     []byte
    FaviconMime     string
}

// UserCrossFeedViewRepository は「最後に横断一覧を開いた時刻」の永続化。
type UserCrossFeedViewRepository interface {
    // Get は当該ユーザーの記録を取得する。未登録の場合は (nil, nil)。
    Get(ctx context.Context, userID string) (*model.UserCrossFeedView, error)
    // Upsert は user_id をキーに last_seen_at を上書き保存する。
    Upsert(ctx context.Context, userID string, lastSeenAt time.Time) error
}
```

##### SQL（参考、実装で確定）

```sql
-- ListNewAcrossFeeds（cursor あり / なしを分岐）
SELECT i.id, i.feed_id, i.guid_or_id, i.title, i.link, i.summary, i.author,
       i.published_at, i.is_date_estimated, i.fetched_at,
       i.hatebu_count, i.created_at, i.updated_at,
       COALESCE(st.is_read, false) AS is_read,
       COALESCE(st.is_starred, false) AS is_starred,
       f.title AS feed_title,
       f.favicon_data, COALESCE(f.favicon_mime, '') AS favicon_mime
FROM items i
JOIN subscriptions s ON s.feed_id = i.feed_id AND s.user_id = $1
JOIN feeds f ON f.id = i.feed_id
LEFT JOIN item_states st ON st.item_id = i.id AND st.user_id = $1
WHERE i.published_at > $2                                  -- sinceTime
  -- cursor あり時のみ:
  AND (i.published_at, i.id) < ($3, $4)
ORDER BY i.published_at DESC, i.id DESC
LIMIT $5;
```

### Backend Handler Layer

#### CrossFeedHandler

| Field | Detail |
|-------|--------|
| Intent | 横断一覧 GET と最終アクセス時刻 PUT の HTTP エンドポイント |
| Requirements | 1.2, 2.1, 4.3 |

**Responsibilities & Constraints**
- 認証必須グループ配下に登録（既存 `r.Group` 内で middleware.UserIDFromContext を使用）
- GET と PUT を **分離**（PM 確認事項 2: リトライ・冪等性に強い）
- レスポンス DTO に `feed_id` / `feed_title` / `feed_favicon_url` を含め N+1 を回避

**Contracts**: Service [ ] / API [x] / Event [ ] / Batch [ ] / State [ ]

##### API Contract

| Method | Endpoint | Request | Response | Errors |
|--------|----------|---------|----------|--------|
| GET | `/api/items/cross-feed?cursor=<RFC3339Nano:itemID>&limit=<N>` | query parameters のみ | `CrossFeedListResponse` | 401 / 500 |
| PUT | `/api/users/me/cross-feed-last-seen` | （body 無し） | 204 No Content | 401 / 500 |

クエリパラメータ:
- `cursor`: 省略時は先頭から取得。`<published_at(RFC3339Nano)>:<item_id(UUID)>` の文字列（コロン区切り）
- `limit`: 省略時 50。最大 200（NFR 1.3 / PM 確認事項 3: design 確定値）

レスポンス DTO（JSON）:

```json
{
  "items": [
    {
      "id": "uuid",
      "feed_id": "uuid",
      "feed_title": "string",
      "feed_favicon_url": "data:image/png;base64,..." /* または null */,
      "title": "string",
      "link": "string",
      "summary": "string",
      "published_at": "RFC3339",
      "is_date_estimated": false,
      "is_read": false,
      "is_starred": false,
      "hatebu_count": 0
    }
  ],
  "next_cursor": "2026-05-27T12:34:56.789Z:550e8400-e29b-41d4-a716-446655440000",
  "has_more": true,
  "since_time": "2026-05-27T00:00:00Z"  // 観測用
}
```

エラー: 既存 `handleServiceError` を再利用。`401 UNAUTHORIZED` は middleware が早期返却。

### Frontend Layer

#### AppStateContext（拡張）

| Field | Detail |
|-------|--------|
| Intent | viewMode 判別子を導入し、横断モード／個別モード／未選択を厳密区別 |
| Requirements | 1.2, 1.3, 5.1 |

**Responsibilities & Constraints**
- `AppState` 拡張: `viewMode: 'none' | 'feed' | 'cross-feed'` を追加。`selectedFeedId` は viewMode='feed' でのみ意味を持つ
- 新 action: `SELECT_ALL_NEW_ITEMS`（viewMode='cross-feed' に遷移、`selectedFeedId=null`, `expandedItemId=null`）/ 既存 `SELECT_FEED` は viewMode='feed' + selectedFeedId 設定に拡張
- 後方互換: `useAppState()` の戻り値型に `viewMode` を追加するのみ。既存呼び出し側は selectedFeedId だけを参照するなら破壊なし
- 初期 state は `viewMode: 'none'`（既存挙動と同等：何も選択されていない状態）

##### State Transition

```typescript
type ViewMode = 'none' | 'feed' | 'cross-feed';

interface AppState {
  viewMode: ViewMode;
  selectedFeedId: string | null;   // viewMode === 'feed' でのみ非 null
  expandedItemId: string | null;
  filter: ItemFilter;
}

type AppAction =
  | { type: 'SELECT_FEED'; feedId: string }            // viewMode='feed', selectedFeedId=feedId, reset expanded/filter
  | { type: 'SELECT_ALL_NEW_ITEMS' }                   // viewMode='cross-feed', selectedFeedId=null, reset expanded/filter
  | { type: 'EXPAND_ITEM'; itemId: string }            // 既存
  | { type: 'SET_FILTER'; filter: ItemFilter };        // 既存
```

#### AllNewItemsEntry（FeedList 内の private function）

| Field | Detail |
|-------|--------|
| Intent | 左ペイン最上部に常設する仮想エントリ button |
| Requirements | 1.1, 1.4, 1.5, NFR 3.1 |

**Responsibilities & Constraints**
- `<button>` 要素で実装し、既存 `feeds.map((feed) => ...)` の **直前** に固定描画
- 選択中（viewMode='cross-feed'）の場合は既存個別フィード選択時と同じ `bg-accent text-accent-foreground font-medium` を適用
- favicon 領域は `FeedFavicon` 抽出後コンポーネントに `faviconURL={null}` を渡し既定の `Rss` アイコン表示
- 「未読数バッジ」は本スコープでは付与しない（Non-Goals / PM 確認事項 6）
- `data-testid="all-new-items-entry"` / `data-selected` / `aria-current="page"`（選択中時）

#### CrossFeedItemList（新規 component）

| Field | Detail |
|-------|--------|
| Intent | 横断一覧の右ペイン描画。記事行に FeedBadge を併記 |
| Requirements | 2.1, 2.2, 2.4, 2.5, 3.1, 3.2, 3.3, 3.4, 4.6, NFR 1.3 |

**Responsibilities & Constraints**
- 内部 state: `expandedItemId` は AppStateContext 経由（既存 ItemList と同等）。フィルタ tabs は横断一覧では **表示しない**（Non-Goals に従い filter=all 固定）
- 無限スクロールは既存 ItemList と同じ IntersectionObserver パターン
- マウント時に `useTouchCrossFeedLastSeen()` を **初回データ受信完了後 1 回だけ** 呼び出し（Req 4.3 の「表示処理完了」を初回ページ受信完了で定義）
- 既読化・スター付与は既存 `useMarkAsRead` / `useToggleStar` を再利用。mutation `onSuccess` で `queryClient.invalidateQueries({ queryKey: ['cross-feed-items'] })` も追加（既存 hook を拡張）
- 空状態: `data?.pages[0]?.items.length === 0` のとき「新着記事はありません」を表示（Req 4.6）

##### Component Interface

```typescript
export function CrossFeedItemList(): JSX.Element;
// AppStateContext から viewMode='cross-feed' 時のみ呼び出される前提（AppShell で条件分岐）
```

#### useCrossFeedItems / useTouchCrossFeedLastSeen（hooks）

```typescript
export function useCrossFeedItems(): UseInfiniteQueryResult<CrossFeedListResponse>;
// queryKey: ['cross-feed-items']
// queryFn: apiClient.get<CrossFeedListResponse>('/api/items/cross-feed?cursor=...&limit=50')
// getNextPageParam: (lastPage) => lastPage.has_more ? lastPage.next_cursor : undefined
// staleTime: 0（横断戻り時に最新状態を反映, Req 5.4）

export function useTouchCrossFeedLastSeen(): UseMutationResult<void, Error, void>;
// mutationFn: () => apiClient.put('/api/users/me/cross-feed-last-seen')
// 失敗しても UI には影響しない（次回起動時 lastSeen が同じ値のまま）
```

#### FeedFavicon（既存 feed-list.tsx 内 private → 共有 component に昇格）

| Field | Detail |
|-------|--------|
| Intent | favicon URL / 未設定 / 読込失敗のいずれにも対応した favicon 表示 |
| Requirements | 3.2, 3.3, 5.2 |

**Responsibilities & Constraints**
- 既存 `feed-list.tsx` の `FeedFavicon` を `web/src/components/feed-favicon.tsx` に **そのまま** 切り出す（挙動不変、import パスのみ変更）
- props: `{ feedId: string; faviconURL: string | null; feedTitle: string }`（変更なし）
- 既存 #122 で実装済の `<img onError>` → `Rss` fallback ロジックを保持
- 横断一覧記事カードでは `feedId` には記事の `feed_id`、`faviconURL` には `feed_favicon_url`、`feedTitle` には `feed_title` を渡す

## Data Models

### Domain Model

**Aggregate**: `UserCrossFeedView`（user_id を集約 root）
- `user_id` (UUID, PK) — `users.id` への外部キー（ON DELETE CASCADE）
- `last_seen_at` (TIMESTAMPTZ) — 当該ユーザーが最後に横断一覧を開いた時刻

**Domain Invariants**:
- `user_id` 単位で **高々 1 行**（PK 制約で物理的に保証）
- `last_seen_at` は単調増加（TouchLastSeen は常に `now()` で上書き、過去時刻書き込みは想定しない）

### Physical Data Model

#### 新規テーブル: `user_cross_feed_views`

```sql
-- 20260528120000_add_user_cross_feed_views.up.sql
CREATE TABLE user_cross_feed_views (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    last_seen_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

```sql
-- 20260528120000_add_user_cross_feed_views.down.sql
DROP TABLE IF EXISTS user_cross_feed_views;
```

#### 設計判断: 新規 index は追加しない

`ListNewAcrossFeeds` の JOIN プランで利用する index は **既存** で揃う:
- `subscriptions(user_id)` → `idx_subscriptions_user_id`（既存）
- `items(feed_id, published_at DESC)` → `idx_items_feed_published_at`（既存）
- `item_states(user_id, item_id)` → `uq_item_states_user_item`（既存）

PostgreSQL planner は `subscriptions` を user_id で nested-loop → 各 `feed_id` に対して `items` を `idx_items_feed_published_at` で `published_at > sinceTime` で絞り込む計画を選択するため、**新規 composite index は不要**。仮に planner が逸れた場合は ANALYZE 後に EXPLAIN で再評価する（運用調整）。

#### 既存テーブル変更

なし。`users` テーブルへの列追加は意図的に避け（影響範囲拡大を回避）、`crossfeed` ドメイン専用テーブルに切り出す。

### Cursor Encoding（contract）

`next_cursor`（およびクライアントから送られる `cursor` query parameter）の形式:

```
<RFC3339Nano(published_at)>:<UUID(item_id)>
```

例: `2026-05-27T12:34:56.789012345Z:550e8400-e29b-41d4-a716-446655440000`

- handler 層で `strings.SplitN(s, ":", 2)` ではなく **末尾の `:` から split**（RFC3339Nano にも `:` が含まれるため、`strings.LastIndex(s, ":")` で分割）
- 不正値は `model.NewInvalidFilterError` を返し 400 BadRequest

## Error Handling

### Error Strategy

既存 `internal/model/errors.go` の `APIError` パターンを踏襲し、`handleServiceError` で HTTP status にマッピングする。新規ドメインエラーコードは導入しない（既存 `INVALID_FILTER` / `UNAUTHORIZED` / `INTERNAL_ERROR` で十分カバー可能）。

### Error Categories and Responses

- **User Errors (4xx)**:
  - 401 UNAUTHORIZED — middleware が session 未取得時に返却（既存挙動）
  - 400 INVALID_REQUEST — 不正 cursor 形式 / `limit` 範囲外（1〜200 外）。既存 `WriteErrorResponse` を再利用
- **System Errors (5xx)**:
  - 500 INTERNAL_ERROR — DB 接続断 / クエリ失敗。既存 `handleServiceError` のデフォルトケース
  - Frontend 側: `useCrossFeedItems` が `isError` のとき「記事の読み込みに失敗しました」を表示（既存 ItemList と同 UX）
- **Business Logic Errors (422)**: 本機能では存在しない（横断一覧は副作用の少ない取得操作のため）

### Favicon Failure Handling（Req 3.3）

- `feed_favicon_url` が `null`（feeds テーブルの favicon_data が未保存）→ `FeedFavicon` の `faviconURL=null` 経路で `Rss` 代替アイコン表示
- `<img>` の読込失敗 → 既存 `onError` → `setImgFailed(true)` → `Rss` 代替アイコン表示
- どちらも **DB 取得失敗 / API エラーとしては扱わない**（既存 #122 と同方針）

### TouchLastSeen 失敗時の挙動（Req 4.3）

- `useTouchCrossFeedLastSeen` の mutation 失敗時は UI に通知しない（silent fail）。次回横断一覧表示時に **前回と同じ sinceTime** で再抽出されるため、ユーザーは「新着が増えただけ」に見え、機能的退行は起きない
- バックエンドログには `slog.Warn` で記録（既存 `handleServiceError` の slog 経路）

## Testing Strategy

### Unit Tests

- **`crossfeed.Service.ListNewItems`** — lastSeen 記録あり時に `sinceTime = lastSeen` で repo を呼ぶこと
- **`crossfeed.Service.ListNewItems`** — lastSeen 記録なし（初回）時に `sinceTime = now - 24h` fallback で repo を呼ぶこと（Req 4.4）
- **`crossfeed.Service.ListNewItems`** — cursorStr が不正形式時に `INVALID_REQUEST` 相当のエラーを返すこと
- **`crossfeed.Service.TouchLastSeen`** — `UserCrossFeedViewRepository.Upsert` が呼ばれること
- **`appReducer` (web)** — `SELECT_ALL_NEW_ITEMS` action で viewMode='cross-feed', selectedFeedId=null, expandedItemId=null, filter='all' になること

### Integration Tests

- **`PostgresItemRepo.ListNewAcrossFeeds`** — 2 フィード購読 + 各フィードに 3 件記事の環境で、`sinceTime` 以後の記事のみが `published_at DESC, id DESC` で取得されること
- **`PostgresItemRepo.ListNewAcrossFeeds`** — cursor 指定時に複合キー `(published_at, id) <` で正しくページングされること（同一 published_at の境界含む）
- **`PostgresUserCrossFeedViewRepo.Upsert` → `Get`** — 同一 user_id への二度目の Upsert で last_seen_at が更新されること
- **`POST /api/items/cross-feed` → 認証なし** → 401（既存 middleware の動作確認）
- **`GET /api/items/cross-feed` → 認証あり** → items 配列 + next_cursor + has_more が返ること

### E2E/UI Tests

- **`FeedList` test** — 購読 0 件でも「すべての新着記事」エントリが描画されること
- **`FeedList` test** — `AllNewItemsEntry` クリックで `dispatch({ type: 'SELECT_ALL_NEW_ITEMS' })` が呼ばれること（Req 1.2）
- **`AppShell` test** — viewMode='cross-feed' のとき `<CrossFeedItemList />` が描画され、viewMode='feed' のとき既存 `<ItemList />` が描画されること（Req 1.2, 1.3, 5.1）
- **`CrossFeedItemList` test** — API モックで 0 件返却時に「新着記事はありません」が表示されること（Req 4.6）
- **`CrossFeedItemList` test** — マウント時に `useTouchCrossFeedLastSeen` の mutate が初回データ受信後に 1 回だけ呼ばれること（Req 4.3）

### Performance/Load

- **`PostgresItemRepo.ListNewAcrossFeeds`** — ユーザー 1 人が 50 購読 / 各 1000 件 items の状態で limit=50 取得が EXPLAIN ANALYZE で 100ms 以内に収まること（NFR 1.1 の design margin）
- **既存 `PostgresItemRepo.ListByFeed`** — ベンチマーク（既存テスト）が本変更前後で 5% 以上劣化しないこと（NFR 1.2）

## Security Considerations

- 既存 `middleware.NewSessionMiddleware` の認証必須グループ配下に登録するため、未認証アクセスは middleware が 401 で早期返却
- すべての SQL クエリは `user_id = $1` を含む（`subscriptions.user_id = $1`）ため、他ユーザーの購読フィードに属する記事は構造的に取得できない
- `user_cross_feed_views.user_id` に `REFERENCES users(id) ON DELETE CASCADE` を付与し、退会時に自動削除（既存 `internal/app/withdraw_wiring.go` の責務範囲に自然に含まれる）
- 新規エンドポイントは内部 API のみ参照（外部 URL fetch を行わないため SSRF / sanitize の追加対策不要）

## Performance & Scalability

- limit 上限を 200 に設定（query parameter で受け取った値を handler 層で `min(200, requested)` にクランプ）
- `useCrossFeedItems` の `staleTime: 0` は記事既読化後の同期性を優先（Req 5.4）。React Query の `gcTime` は default（5 分）のまま
- 初期表示時のデフォルト limit=50 + 無限スクロール（既存 `ItemList` と同 IntersectionObserver パターン）で「無応答にならない」（NFR 1.3）を担保

## Architect 確認事項

設計上の判断は本ドキュメント内で **すべて確定**しました（PM の `## 確認事項` 全 6 件への design 回答を下表に明示）。実装段階で発生する細部の調整は通常の PR コメント / レビューで対応可能であり、Developer 着手前に追加の人間判断は不要です。

| PM 確認事項 | Design 確定値 | 確定根拠 |
|---|---|---|
| 1. 永続化粒度（サーバ／クライアント） | サーバ側 DB（新規 `user_cross_feed_views` 表） | 複数デバイス一貫性 + 再ログイン保持の要件 4.5 を素直に満たす |
| 2. 初回 fallback 窓 | 直近 24 時間 | 「日次で新着確認する」典型 workflow に合致。実装は単純な定数 |
| 3. 件数上限 | デフォルト 50 件 / 上限 200 件 + cursor 追加読み込み | 既存 `ItemService.defaultItemsPerPage=50` と整合。上限 200 で UI 無応答リスク回避 |
| 4. 既読除外 | 除外しない（既読記事も新着として表示し、既存と同じ視覚扱い） | Non-Goals 化。実装単純化と「新着 = 公開時刻ベース」の semantics 明確化 |
| 5. タイブレーク | `(published_at DESC, id DESC)` 複合キー | UUID は時系列性が無いが ULID と異なり決定論順序として十分。cursor 実装の自然性 |
| 6. 「すべての新着記事」エントリの未読数バッジ | 本スコープでは表示しない | Non-Goals 化。実装すると未読カウントクエリが追加で必要になりスコープ拡大 |

> Note: 上記確定値はいずれも Issue #121 の AC 範囲内で design.md として閉じています。後続 spec で再検討が必要な場合は別 Issue 化（例: 「すべての新着記事の未読数バッジ表示」「既読除外フィルタ」等）してください。
