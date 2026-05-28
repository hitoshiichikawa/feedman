# Design Document

## Overview

**Purpose**: 本機能は左ペインのフィード一覧から個別フィードの「設定パネル」を開けるようにし、
既存の `SubscriptionSettings`（更新間隔変更・フェッチ再開・購読解除）を Feedman メイン画面の
動線に統合することで、フィード単位の運用操作をユーザーが画面上で完結できるようにする。新規
API・サービス層の追加はなく、既存コンポーネント（`FeedList` / `AppShell` /
`SubscriptionSettings` / `useUnsubscribe` 等）の UI 統合とイベント伝播の整備のみで完結する。

**Users**: ログイン済みの Feedman ユーザーが、左ペインのフィード行にホバーすることで現れる
ギアアイコンをクリックし、画面中央にダイアログ表示される購読設定パネルから購読解除・更新間隔
変更・フェッチ再開を実行する。キーボード操作のユーザーは Tab で行内のギアアイコンにフォーカス
し、Enter / Space で起動する。

**Impact**: 既存の `FeedList` 行レイアウト・選択挙動・未読バッジ・ステータスアイコン・favicon
フォールバックには手を加えず、行末尾に「ホバー or フォーカスで表示されるギアアイコン」のみを
追加する。設定起動のクリックはフィード選択イベントを発火させず、`AppShell` 側で `Dialog` を
開閉する状態を一元管理する。購読解除完了時に「選択中フィードが解除対象だった場合のみ右ペインを
クリアする」分岐を `AppShell` に集約する。バックエンドへの変更は **一切ない**。

### Goals

- 主要目標 1: フィード行ホバー時にギアアイコンを表示し、クリック / キーボード起動で当該フィード
  の `SubscriptionSettings` を `Dialog` で開く（Req 1, NFR 2.1）
- 主要目標 2: 既存 `SubscriptionSettings` を最小改変（`onUnsubscribed` の意味的拡張 / 確認ダイアログ
  の挙動維持）で再利用し、購読解除フロー（確認ダイアログ → DELETE 呼出 → キャッシュ無効化）の
  挙動を変えない（Req 2, 3）
- 主要目標 3: 購読解除成功時に「選択中フィードが解除対象であれば右ペインをクリアし、そうでなければ
  選択状態を維持する」分岐を `AppShell` に集約する（Req 4.2, 4.3, 5.3）
- 主要目標 4: 異常系（DELETE 失敗）でフィード一覧から消さず、再試行可能な状態を保つ（Req 5）
- 成功基準: Req 1〜5 と NFR 1〜3 が全て pass、既存テスト（`feed-list.test.tsx` /
  `subscription-settings.test.tsx` / `app-shell.test.tsx`）が破壊されない

### Non-Goals

- フィード（マスタデータ）の物理削除や、一括解除・undo（Out of Scope）
- バックエンド `DELETE /api/subscriptions/:id` ハンドラ・サービス・リポジトリの挙動変更
- `SubscriptionSettings` 既存設定項目以外の新規設定追加（更新間隔・フェッチ再開・購読解除以外）
- 設定パネルを別ペイン化・スライドオーバー化等のレイアウト改造（Dialog 統合で完結させる）
- 設定起動コントロールのカスタマイズ（種類の切替・コンテキストメニュー化）

## Architecture

### Existing Architecture Analysis

`web/` は Next.js 15 App Router + React 19 + TanStack Query の構成で、UI 状態は
`AppStateProvider`（Context + `useReducer`）が `selectedFeedId` / `expandedItemId` / `filter`
を一元管理する。フィード一覧は `useFeeds()` が `queryKey: ["feeds"]` で取得し、購読変更系の
mutation（`useUpdateFetchInterval` / `useUnsubscribe` / `useResumeFeed`）はいずれも
`onSuccess` で `queryClient.invalidateQueries({ queryKey: ["feeds"] })` を実行して左ペインを
再フェッチする既存パターンが確立している。

尊重すべきドメイン境界:

- **`FeedList`**: フィード行の描画と「行クリック = フィード選択」イベント発火のみを責務とする
  presentational コンポーネント。状態は親（`AppShell`）から props で受ける
- **`AppShell`**: 2 ペインのトップレベル状態（フィード一覧表示・選択フィード・記事展開）を
  一元管理する。本機能の **設定ダイアログの開閉・対象フィード ID** も `AppShell` 側で持つ
- **`SubscriptionSettings`**: 「対象 subscription」を props で受け、更新間隔・解除・再開の
  mutation 呼出と確認ダイアログを内部で完結させる self-contained 部品。`onUnsubscribed`
  コールバックで親に解除完了を通知する API は既に存在する
- **`useUnsubscribe`**: `DELETE /api/subscriptions/:id` を呼び、成功時に `["feeds"]` キャッシュを
  無効化する mutation フック（変更不要）

維持すべき統合点:

- `useFeeds` の `queryKey: ["feeds"]` を mutation 群の `invalidateQueries` で叩く既存規約
- `AppState` の `SELECT_FEED` action による排他選択挙動（`expandedItemId` / `filter` のリセット）
- `Dialog`（情報表示用）と `AlertDialog`（破壊的確認用）の使い分け（既に shadcn/ui のものが配置済み）

解消・回避する technical debt:

- 現状 `SubscriptionSettings` はメイン画面に到達する動線が無く実質「孤児」コンポーネントだが、
  既存テスト（`subscription-settings.test.tsx`）はそのまま green で温存できる API シグネチャを
  保つ。再設計はしない

### Architecture Pattern & Boundary Map

**採用パターン**: 「状態を親（`AppShell`）に lift up し、子（`FeedList`）はイベント通知に
徹する」既存パターンを踏襲する。`SubscriptionSettings` を `Dialog` でラップする薄い
コンポーネント（`SubscriptionSettingsDialog`）を新規追加し、`AppShell` から open / close と
対象フィード ID を制御する。

```mermaid
flowchart LR
    FL[FeedList<br/>左ペイン行] -- onOpenSettings(feedId) --> AS[AppShell]
    AS -- open=true, subscription --> SSD[SubscriptionSettingsDialog]
    SSD -- contains --> SS[SubscriptionSettings<br/>既存]
    SS -- onUnsubscribed --> SSD
    SSD -- onUnsubscribed(feedId) --> AS
    AS -- SELECT_FEED(null) if<br/>feedId === selectedFeedId --> ASC[AppState]
    SS -- useUnsubscribe.mutate --> RQ[TanStack Query]
    RQ -- invalidate ["feeds"] --> FL
```

**Architecture Integration**:

- ドメイン／機能境界: UI 状態は引き続き `AppShell`、フィード行は `FeedList`、設定 UI は
  `SubscriptionSettings`。新規 `SubscriptionSettingsDialog` は「Dialog の開閉と対象フィード
  ID の解決」のみを担う thin wrapper（境界の追加コストを最小化）
- 既存パターンの維持: `useUnsubscribe` の `["feeds"]` invalidation で `FeedList` が自動更新される
  既存フローを変えず、`AppShell` 側で「選択中フィードが解除対象か」だけ判定して右ペインクリアを
  dispatch する
- 新規コンポーネントの根拠:
  - `SubscriptionSettingsDialog`: `SubscriptionSettings` を Dialog 内でレンダリングし、
    open / onOpenChange の責務を `AppShell` から委譲するため。直接 `AppShell` に `Dialog` 構文を
    展開すると `AppShell` の責務が膨らみ、テスト容易性が落ちる
  - 新規ギアアイコン要素は `FeedList` 内に閉じ込め、別コンポーネントは作らない（行ローカルの
    UI 要素のため境界を増やす価値が薄い）

**代替案と却下理由**:

- 代替 A: ダイアログ状態を `FeedList` に持たせる → 「選択中フィードが解除対象か」の判定に
  `selectedFeedId` を `FeedList` から `AppShell` に通知する逆経路が必要で、Lift Up State の
  React 慣習に反する。却下
- 代替 B: 右ペインに設定パネルをインライン表示する → 「フィード選択イベントを発火しない」
  要件（AC 1.4）と矛盾し、選択中フィードが解除対象だった場合の状態遷移が複雑になる。却下
- 代替 C: グローバル Context に `settingsTargetFeedId` を追加 → AppState の責務が膨らむ。
  ダイアログは AppShell のローカル `useState` で十分。却下

### Technology Stack

| Layer | Choice / Version | Role in Feature | Notes |
|-------|------------------|-----------------|-------|
| Frontend | Next.js 15 + React 19 + TypeScript 5 | UI 全体 | 既存採用。変更なし |
| State (UI) | React Context + `useReducer`（`AppStateProvider`） | 選択中フィード ID | 既存。本機能では `SELECT_FEED` action のみ利用 |
| Local State | `useState` in `AppShell` | 設定ダイアログ開閉・対象 subscription | 新規。グローバル化しない |
| Data Fetching | TanStack React Query 5 | `useFeeds` / `useUnsubscribe` 等 | 既存。`queryKey: ["feeds"]` の invalidation 規約を踏襲 |
| UI Primitives | shadcn/ui（radix-ui base）`Dialog` / `AlertDialog` / `Button` | ダイアログ・確認 / ボタン | 既に repo に配置済み（`web/src/components/ui/dialog.tsx` / `alert-dialog.tsx`） |
| Icons | lucide-react（既存） | `Settings`（ギアアイコン） | 既存依存。追加インストール不要 |
| Styling | Tailwind CSS 4 | ホバー時表示制御（`group-hover` / `focus-within` / `peer-focus`） | a11y のためフォーカス時にも表示 |
| Tests | Vitest + Testing Library（jsdom） | 単体・統合 | 既存 `*.test.tsx` 同居パターン |

## File Structure Plan

### Directory Structure

```
web/src/
├── components/
│   ├── feed-list.tsx                      # [Modified] ギアアイコン追加・onOpenSettings props 追加
│   ├── feed-list.test.tsx                 # [Modified] ホバー表示・キーボード起動・stopPropagation テスト追加
│   ├── app-shell.tsx                      # [Modified] 設定ダイアログ状態・解除後の右ペインクリア分岐
│   ├── app-shell.test.tsx                 # [Modified] ギア → ダイアログ → 解除 → 右ペインクリアのフローテスト
│   ├── subscription-settings.tsx          # [Modified-minor] onUnsubscribed の意味的拡張（コメント追記）
│   ├── subscription-settings.test.tsx     # [Unchanged] 既存テストはそのまま green
│   ├── subscription-settings-dialog.tsx   # [New] Dialog + SubscriptionSettings のラッパ
│   ├── subscription-settings-dialog.test.tsx # [New] open/close・onUnsubscribed コールバック検証
│   └── ui/
│       ├── dialog.tsx                     # [Unchanged] 既存利用
│       └── alert-dialog.tsx               # [Unchanged] 既存利用（SubscriptionSettings 内）
├── contexts/
│   └── app-state.tsx                      # [Unchanged] SELECT_FEED の既存挙動のみ利用
└── hooks/
    └── use-subscriptions.ts               # [Unchanged] useUnsubscribe / useUpdateFetchInterval をそのまま利用
```

### Modified Files

- `web/src/components/feed-list.tsx`
  - `FeedListProps` に `onOpenSettings: (subscription: Subscription) => void` を追加
  - 各 `<button data-testid="feed-item-...">` 内の右端に `<button data-testid="feed-settings-button-${id}">` を新設し、`Settings` アイコンを表示。`onClick` で `e.stopPropagation()` してから `onOpenSettings(feed)` を呼ぶ
  - フィード行を `<button>` から **`<div role="button">` 兼ね合いではなく**、既存の `<button>` を維持しつつネスト `<button>` 回避のため、行コンテナを `<div>` 化（クリックは `onClick` / Enter / Space ハンドラで再現）するか、もしくは行を `<div>` でラップし行クリック領域（記事選択）とギアボタン領域を兄弟関係に配置する。**採用案**: 行コンテナを `<div role="row" tabIndex={0}>` にし、ハンドラを `onClick` / `onKeyDown(Enter|Space)` で実装する。既存テスト互換のため `data-testid="feed-item-${id}"` / `data-selected` 属性は維持
  - ホバー / フォーカス時表示は Tailwind の `group` ユーティリティで実装（`group hover:` + `group focus-within:` + 設定ボタン自体の `focus-visible:opacity-100`）
- `web/src/components/app-shell.tsx`
  - `useState<Subscription | null>(null)` で `settingsTarget` を保持
  - `handleOpenSettings(feed)` を `FeedList` の `onOpenSettings` に渡す
  - `<SubscriptionSettingsDialog open={settingsTarget !== null} subscription={settingsTarget} onOpenChange={...} onUnsubscribed={handleUnsubscribed} />` を `<aside>` 外で配置（Portal なので位置は副次的）
  - `handleUnsubscribed(unsubscribedFeedId)`: `if (unsubscribedFeedId === state.selectedFeedId) dispatch({ type: "SELECT_FEED_NONE" })` 相当の遷移を行う。**注**: 既存 `SELECT_FEED` は string を要求するため、`AppState` に **`CLEAR_SELECTED_FEED` action** を新設するか、`selectedFeedId` を `null` に戻す手段を導入する必要がある。後述「Components and Interfaces」参照
- `web/src/components/subscription-settings.tsx`
  - シグネチャは現行維持（`subscription` / `onUnsubscribed`）。`onUnsubscribed` は「解除に成功した直後、ダイアログ閉鎖を親に依頼する通知」として **既存挙動のまま** 利用する
  - **追加変更**: `onUnsubscribed` の引数に **解除された feed_id を渡す**（現状 `onUnsubscribed: () => void` → 新 `onUnsubscribed: (unsubscribedFeedId: string) => void`）。これにより親 `AppShell` が「選択中フィードが解除対象か」を判定可能になる。既存テストは引数を無視できる callback で呼ばれているので破壊しない
- `web/src/contexts/app-state.tsx`
  - 新規 action `CLEAR_SELECTED_FEED` を追加（`type: "CLEAR_SELECTED_FEED"`）、reducer で
    `selectedFeedId: null, expandedItemId: null, filter: "all"` に遷移
  - 既存 `SELECT_FEED` 挙動は変更しない（NFR 1.1）

### New Files

- `web/src/components/subscription-settings-dialog.tsx`
  - `Dialog` でラップし、`SubscriptionSettings` を中身として描画する thin wrapper
  - props: `{ open: boolean; subscription: Subscription | null; onOpenChange: (open: boolean) => void; onUnsubscribed: (unsubscribedFeedId: string) => void }`
- `web/src/components/subscription-settings-dialog.test.tsx`
  - open / close 遷移、`subscription === null` で nothing render、`onUnsubscribed` の伝搬を検証

## Requirements Traceability

| Requirement | Summary | Components | Interfaces | Flows |
|-------------|---------|------------|------------|-------|
| 1.1 / 1.2 | ホバー時のみギア表示 | FeedList | `group-hover`, `focus-within` | CSS のみ。JS state なし |
| 1.3 | ギアクリックで設定起動 | FeedList → AppShell → SubscriptionSettingsDialog | `onOpenSettings(feed)` | クリック → state.settingsTarget = feed |
| 1.4 | ギアクリックで選択発火しない | FeedList | `e.stopPropagation()` on gear button | 親の onClick を発火させない |
| 1.5 | キーボード起動 | FeedList | `tabIndex=0`, `aria-label`, Enter/Space handler | tab focus → Enter/Space |
| 2.1〜2.5 | 設定パネル既存挙動 | SubscriptionSettings | `subscription` prop | 既存 mutation 群で完結（変更なし） |
| 3.1〜3.5 | 解除確認ダイアログ | SubscriptionSettings | 既存 `AlertDialog` + `useUnsubscribe` | 既存挙動を温存 |
| 4.1 / 4.5 | 一覧から消える | useUnsubscribe → useFeeds | `invalidateQueries(["feeds"])` | 既存（変更なし） |
| 4.2 | 選択中だった場合の右ペインクリア | AppShell + AppState `CLEAR_SELECTED_FEED` | `onUnsubscribed(feedId)` → dispatch | 解除完了 → 比較 → dispatch |
| 4.3 | 選択中でなければ維持 | AppShell | 比較分岐 | feedId !== selectedFeedId なら no-op |
| 4.4 | 解除成功でダイアログ閉じる | SubscriptionSettings + SubscriptionSettingsDialog | `onUnsubscribed` で `onOpenChange(false)` | 既存 SubscriptionSettings 内の `setShowUnsubscribeDialog(false)` を温存 + Dialog 自体も閉じる |
| 5.1〜5.2 | 失敗時の再試行 | SubscriptionSettings | 既存 mutation エラー扱い | mutation の error 時は state 不変 |
| 5.3 | 失敗時に右ペイン不変 | AppShell | `onUnsubscribed` は成功時のみ呼ばれる既存挙動 | mutation の `onSuccess` 内のみで callback 発火 |
| NFR 1.1 | 既存 FeedList 挙動不変 | FeedList | 行クリックの SELECT_FEED 発火を維持 | 既存テストが green |
| NFR 1.2 | 既存 SubscriptionSettings 挙動不変 | SubscriptionSettings | API シグネチャ温存 | 既存テストが green |
| NFR 2.1 | ギアのアクセシブル | FeedList | `aria-label="<feed_title> の設定"`, `type="button"` | スクリーンリーダ互換 |
| NFR 2.2 | ダイアログのフォーカストラップ | Dialog / AlertDialog（radix-ui） | radix-ui の既定挙動 | 追加実装不要 |
| NFR 3.1 | 1 秒以内の進行表示 | SubscriptionSettings | `unsubscribe.isPending` でボタン非活性 + ラベル変更（既存） | 既存挙動を維持 |

## Components and Interfaces

### Left Pane / Feed List

#### FeedList（Modified）

| Field | Detail |
|-------|--------|
| Intent | フィード行を表示し、行クリックで選択イベント、行末ギアアイコンクリックで設定起動イベントを発火する |
| Requirements | 1.1, 1.2, 1.3, 1.4, 1.5, NFR 1.1, NFR 2.1 |

**Responsibilities & Constraints**

- フィード一覧を表示（既存挙動を温存。テスト互換のため `data-testid="feed-item-${id}"` / `data-selected` 属性を維持）
- 行ホバー時 / 行内フォーカス時にギアアイコンを表示
- ギアアイコンクリックで `onOpenSettings(feed)` を呼び、行クリックの `onSelectFeed(feedId)` を **発火させない**（`e.stopPropagation()`）
- キーボードユーザーが Tab で行内ギアにフォーカスでき、Enter / Space で起動できる
- 既存の未読バッジ・ステータスアイコン・favicon フォールバックは変更しない

**Dependencies**

- Inbound: AppShell — `feeds` / `selectedFeedId` / `onSelectFeed` / `onOpenSettings` (Critical)
- Outbound: なし（presentational）
- External: lucide-react（既存。新規 import: `Settings`）

**Contracts**: Service [ ] / API [ ] / Event [x] / Batch [ ] / State [ ]

##### Component Interface

```typescript
interface FeedListProps {
  feeds: Subscription[];
  selectedFeedId: string | null;
  onSelectFeed: (feedId: string) => void;
  /** ギアアイコンクリック時に対象 subscription を通知する。
   *  AC 1.3: クリックで設定パネルが開く。
   *  AC 1.4: フィード選択イベントは発火しない（onSelectFeed は呼ばれない）。 */
  onOpenSettings: (subscription: Subscription) => void;
}
```

- Preconditions: `feeds` は最新の `useFeeds()` 結果。`selectedFeedId` は `state.selectedFeedId`。
- Postconditions: `onSelectFeed` と `onOpenSettings` は排他的（同一クリックで両方発火しない）。
- Invariants: 行コンテナ内に `<button>` をネストしない（HTML 仕様違反回避）。

**実装メモ（疑似コード）**:

```tsx
<div
  role="button"
  tabIndex={0}
  data-testid={`feed-item-${feed.id}`}
  data-selected={isSelected ? "true" : "false"}
  className="group flex items-center gap-2 ..."
  onClick={() => onSelectFeed(feed.feed_id)}
  onKeyDown={(e) => {
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      onSelectFeed(feed.feed_id);
    }
  }}
>
  <FeedFavicon ... />
  <span className="flex-1 truncate">{feed.feed_title}</span>
  {/* ステータスアイコン / 未読バッジ（既存） */}
  ...
  <button
    type="button"
    aria-label={`${feed.feed_title} の設定`}
    data-testid={`feed-settings-button-${feed.id}`}
    className={cn(
      "flex-shrink-0 rounded p-1 text-muted-foreground",
      "opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 focus-visible:opacity-100",
      "hover:bg-accent-foreground/10 focus-visible:ring-2"
    )}
    onClick={(e) => {
      e.stopPropagation();
      onOpenSettings(feed);
    }}
    onKeyDown={(e) => {
      // Enter/Space は button のデフォルト挙動でクリック発火するが、
      // 親 onKeyDown の伝搬を止める
      if (e.key === "Enter" || e.key === " ") e.stopPropagation();
    }}
  >
    <Settings className="w-4 h-4" />
  </button>
</div>
```

**ネスト button 回避の根拠**: 既存実装はフィード行を `<button>` で表していたが、HTML 仕様上
`<button>` 内に `<button>` を置けないため、行を `<div role="button" tabIndex={0}>` 化する。
WAI-ARIA Practice では「カスタム button は Enter/Space で activate」が必要なため `onKeyDown`
で再現する。既存テスト（`feed-list.test.tsx`）は `fireEvent.click(screen.getByText("Tech Blog"))`
を行っており、`<div>` 上でも click は発火するため互換性は保たれる。

### Application Shell

#### AppShell（Modified）

| Field | Detail |
|-------|--------|
| Intent | 設定ダイアログの開閉状態と対象 subscription を保持し、購読解除完了時に選択中フィードの場合のみ右ペインをクリアする |
| Requirements | 1.3, 4.2, 4.3, 5.3 |

**Responsibilities & Constraints**

- `useState<Subscription | null>` で `settingsTarget` を保持
- `FeedList` の `onOpenSettings(feed)` で `setSettingsTarget(feed)` を呼ぶ
- `SubscriptionSettingsDialog` を render し、`open=settingsTarget !== null` で制御
- `handleUnsubscribed(unsubscribedFeedId)`: 解除された feed_id が `state.selectedFeedId` と一致する場合のみ `CLEAR_SELECTED_FEED` を dispatch
- ダイアログを閉じる責務（`setSettingsTarget(null)`）は `onUnsubscribed` の中で実行し、また `onOpenChange(false)` でも実行する（ユーザーが手動で閉じた場合）

**Dependencies**

- Inbound: なし（top-level）
- Outbound: AppState `useAppDispatch` / `useAppState` (Critical), useFeeds (Critical), FeedList (Critical), SubscriptionSettingsDialog (Critical), ItemList (Critical)
- External: なし

**Contracts**: Service [ ] / API [ ] / Event [x] / Batch [ ] / State [x]

##### State Transitions

```
[idle]
  ─ user hovers feed row ─ → [gear visible]
  ─ user clicks gear      ─ → [dialog open: settingsTarget = feed]
[dialog open]
  ─ user closes dialog (Esc / Cancel / outside click) ─ → [idle: settingsTarget = null]
  ─ user confirms unsubscribe & API success            ─ → [idle: settingsTarget = null,
                                                              if feedId === selectedFeedId then clear selection]
  ─ user confirms unsubscribe & API failure            ─ → [dialog open]（state 不変、SubscriptionSettings 内のエラー UI で再試行）
```

#### SubscriptionSettingsDialog（New）

| Field | Detail |
|-------|--------|
| Intent | `SubscriptionSettings` を `Dialog` でラップし、open / close を親から制御できるようにする thin wrapper |
| Requirements | 2.1, 2.5, 4.4 |

**Responsibilities & Constraints**

- `subscription === null` のときは何も render しない（`open` は親が `settingsTarget !== null` で制御するため二重ガード）
- `SubscriptionSettings` の `onUnsubscribed(feedId)` を受けて、親の `onUnsubscribed(feedId)` を呼ぶ
- Dialog のタイトル「フィードの設定」を `DialogTitle` で明示
- フォーカストラップ・Esc 閉鎖は radix-ui Dialog の既定挙動に委譲

**Dependencies**

- Inbound: AppShell — `open` / `subscription` / `onOpenChange` / `onUnsubscribed` (Critical)
- Outbound: SubscriptionSettings (Critical)
- External: shadcn/ui `Dialog`

**Contracts**: Service [ ] / API [ ] / Event [x] / Batch [ ] / State [ ]

##### Component Interface

```typescript
interface SubscriptionSettingsDialogProps {
  open: boolean;
  subscription: Subscription | null;
  onOpenChange: (open: boolean) => void;
  /** 購読解除成功時。対象 subscription の feed_id を引数で渡す（AppShell が「選択中フィードか」を判定するため）。 */
  onUnsubscribed: (unsubscribedFeedId: string) => void;
}
```

- Preconditions: `subscription` が non-null のとき `open` も true（親が制御）。
- Postconditions: `onUnsubscribed` の発火後は `onOpenChange(false)` も呼ばれる（自動閉鎖）。
- Invariants: `subscription === null` で `<DialogContent>` を render しない。

### Subscription Settings（既存・微改修）

#### SubscriptionSettings（Modified-minor）

| Field | Detail |
|-------|--------|
| Intent | フィード単位の設定 UI。更新間隔・フェッチ再開・購読解除（確認ダイアログ込み）を提供 |
| Requirements | 2.1, 2.2, 2.3, 2.4, 2.5, 3.1, 3.2, 3.3, 3.4, 3.5, 5.1, 5.2, NFR 1.2, NFR 3.1 |

**Responsibilities & Constraints**

- 既存挙動を温存（更新間隔・再開・解除確認・isPending 制御）
- **変更点 1（必須）**: `onUnsubscribed` を `() => void` から `(feedId: string) => void` に拡張する。
  内部 `unsubscribe.mutate(subscription.id, { onSuccess: () => { setShowUnsubscribeDialog(false); onUnsubscribed(subscription.feed_id); } })`
- 確認 `AlertDialog` の挙動・ボタン disabled・ラベルは既存のまま

**API シグネチャの後方互換性**: `onUnsubscribed: (feedId: string) => void` への変更は、既存呼出側
（`subscription-settings.test.tsx` 内の `onUnsubscribed = vi.fn()` / `() => {}`）が引数を無視できる
ため非破壊（TypeScript の型上は `() => void` から `(arg: string) => void` への狭め化だが、
**実引数が増えた callback は既存の `() => void` 受け取り側でも問題ない**ことを vitest テストが
ランタイムで確認する）。

**Dependencies**

- Inbound: SubscriptionSettingsDialog — `subscription` / `onUnsubscribed` (Critical)
- Outbound: useUnsubscribe / useUpdateFetchInterval / useResumeFeed (Critical)
- External: shadcn/ui `AlertDialog` / `Select` / `Button`

**Contracts**: Service [ ] / API [ ] / Event [x] / Batch [ ] / State [x]

### Global UI State

#### AppState（Modified）

| Field | Detail |
|-------|--------|
| Intent | UI 状態の中央管理。本機能では `selectedFeedId` を null に戻す action を新設する |
| Requirements | 4.2 |

**Responsibilities & Constraints**

- 新規 action `CLEAR_SELECTED_FEED` を追加。reducer で `selectedFeedId: null`, `expandedItemId: null`, `filter: "all"` に遷移（`SELECT_FEED` と同じ副作用パターンを踏襲）
- 既存 `SELECT_FEED` / `EXPAND_ITEM` / `SET_FILTER` の挙動は変更しない（NFR 1.1）

**Dependencies**

- Inbound: AppShell — `dispatch({ type: "CLEAR_SELECTED_FEED" })` (Critical)
- Outbound: なし
- External: なし

**Contracts**: Service [ ] / API [ ] / Event [ ] / Batch [ ] / State [x]

##### Action Interface

```typescript
type ClearSelectedFeedAction = { type: "CLEAR_SELECTED_FEED" };
export type AppAction =
  | SelectFeedAction
  | ExpandItemAction
  | SetFilterAction
  | ClearSelectedFeedAction; // new
```

- Preconditions: なし（任意の state から呼べる）。
- Postconditions: `selectedFeedId === null` かつ `expandedItemId === null` かつ `filter === "all"`。
- Invariants: 他のフィールドに副作用を持たない。

## Data Models

### Domain Model

新規ドメインモデルは追加しない。本機能は既存 `Subscription`（`web/src/types/feed.ts`）の
`id` / `feed_id` / `feed_title` 等のフィールドを参照するのみ。

### UI State Model

- `AppState`: `{ selectedFeedId: string | null; expandedItemId: string | null; filter: ItemFilter }` — 既存。new action `CLEAR_SELECTED_FEED` で `selectedFeedId` を null に戻す。
- `AppShell` local state: `settingsTarget: Subscription | null` — 新規。ダイアログの開閉 / 対象を保持。

## Error Handling

### Error Strategy

購読解除の異常系は、既存 `useUnsubscribe` の mutation エラー処理パスに乗せる。`onSuccess` が
呼ばれなければ `onUnsubscribed` も呼ばれず、`AppShell` 側の選択クリア分岐にも到達しない
（AC 5.3 を構造的に満たす）。

### Error Categories and Responses

- **User Errors（4xx）**:
  - 401 認証切れ等は既存 `apiClient` のエラー処理に従う。本機能ではダイアログを閉じず、
    `SubscriptionSettings` 内のエラーメッセージ表示は既存実装をそのまま利用
- **System Errors（5xx / ネットワークエラー）**:
  - mutation の `error` 状態を UI に反映（既存パターン）。ユーザーは確認ダイアログから再試行可能（AC 5.2）
  - フィード一覧は invalidate されないため、対象フィードは一覧に残る（AC 5.1）
  - 右ペインは触らない（AC 5.3）
- **Business Logic Errors（422 / 404）**:
  - 既に解除済みのフィードに対する DELETE で 404 が返る場合、サーバー側の挙動に従う。
    本仕様では特別な分岐は持たず、汎用エラー表示で扱う

### Error UX

- `SubscriptionSettings` 内の購読解除ボタンは `unsubscribe.isPending` で disabled（既存）
- エラー時は確認ダイアログを閉じず、`AlertDialogAction` を再度押せる（既存挙動。AC 5.2）

## Testing Strategy

### Unit Tests

1. `feed-list.test.tsx`: ホバー前はギアが見えないこと（`opacity-0` クラス検証 / もしくは `getByTestId` で要素は存在しつつ非可視）
2. `feed-list.test.tsx`: ギアクリックで `onOpenSettings` が呼ばれ、**`onSelectFeed` が呼ばれない**こと（AC 1.4 / `stopPropagation` 検証）
3. `feed-list.test.tsx`: ギアの `aria-label` が `「<feed_title> の設定」` であること（NFR 2.1）
4. `feed-list.test.tsx`: Tab でギアにフォーカスでき、Enter / Space で `onOpenSettings` が発火すること（AC 1.5）
5. `subscription-settings-dialog.test.tsx`: `open=true` / `subscription=<sub>` で `SubscriptionSettings` が render され、`open=false` で消えること
6. `subscription-settings-dialog.test.tsx`: 内部 `SubscriptionSettings` の `onUnsubscribed` 呼出で親 `onUnsubscribed(unsubscribedFeedId)` が伝搬すること
7. `app-state.test.tsx`（既存 or 新規 if 無ければ追加）: `CLEAR_SELECTED_FEED` action で `selectedFeedId=null`, `expandedItemId=null`, `filter="all"` になること

### Integration Tests

1. `app-shell.test.tsx`: フィード行ホバー → ギアクリック → ダイアログ表示 → キャンセルで閉じる、のフロー（AC 1.3, 2.5）
2. `app-shell.test.tsx`: 選択中フィードに対する購読解除 → 解除成功 → ダイアログ閉鎖 + 右ペイン初期化（`feedId === selectedFeedId` パス、AC 4.2）
3. `app-shell.test.tsx`: 非選択フィードに対する購読解除 → 解除成功 → 右ペイン状態が維持される（AC 4.3）
4. `app-shell.test.tsx`: 購読解除 API が 500 を返す → ダイアログが残り、`["feeds"]` が invalidate されず一覧から消えない（AC 5.1, 5.2, 5.3）
5. `subscription-settings.test.tsx`（既存）: 既存テストが全て green であること（NFR 1.2 担保。本タスクでテスト本体は基本変更しないが、`onUnsubscribed` のシグネチャ拡張に追従するため引数を受ける spy に差し替える）

### E2E / UI Tests

本リポジトリには Playwright 等の E2E 機構は無いため対象外。Vitest + Testing Library の統合テストで担保する。

### Performance / Load

本機能は新規 API 呼出を増やさない（既存 `useUnsubscribe` の 1 呼出のみ）。NFR 3.1
（1 秒以内の進行表示）は既存 `unsubscribe.isPending` でのボタン非活性化により満たす。

## Security Considerations

- 認証は既存 `apiClient` のセッション Cookie に依拠（変更なし）
- ユーザーは自身の `subscription.id` に対してのみ DELETE を発行可能（バックエンドが session の
  user_id で認可する既存挙動。フロントエンドからの操作で他ユーザーの subscription を解除する
  経路は無い）
- ダイアログ表示時に対象フィードタイトルを表示するが、これは既に一覧で公開済みの情報なので
  情報漏洩リスクは無い

## Migration Strategy

スキーマ・データ変更なし。フロントエンドのみのデプロイで完結。Feature Flag は本リポジトリで
opt-out のため不要（main マージで即時有効）。
