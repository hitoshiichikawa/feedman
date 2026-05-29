# Implementation Plan

- [x] 1. AppState: `CLEAR_SELECTED_FEED` action を追加
  - `web/src/contexts/app-state.tsx` の `AppAction` ユニオン型に `ClearSelectedFeedAction = { type: "CLEAR_SELECTED_FEED" }` を追加する
  - `appReducer` の `switch` に `case "CLEAR_SELECTED_FEED"` を追加し、`selectedFeedId: null`, `expandedItemId: null`, `filter: "all"` に遷移させる（`SELECT_FEED` と同じ副作用パターン）
  - 既存 `SELECT_FEED` / `EXPAND_ITEM` / `SET_FILTER` の挙動は変更しないこと（NFR 1.1）
  - reducer ユニットテストを `web/src/contexts/app-state.test.tsx` に追加する（既存テストファイル不在なら新規作成）。検証項目: (a) `CLEAR_SELECTED_FEED` で `selectedFeedId` が null になる、(b) 同時に `expandedItemId=null` / `filter="all"` にリセットされる、(c) 既存 action（`SELECT_FEED` / `EXPAND_ITEM` / `SET_FILTER`）の挙動が変わらない回帰確認
  - _Requirements: 4.2, NFR 1.1_
  - _Boundary: AppState_

- [x] 2. FeedList: ホバー時ギアアイコン表示と `onOpenSettings` イベント発火
  - `web/src/components/feed-list.tsx` の `FeedListProps` に `onOpenSettings: (subscription: Subscription) => void` を追加する
  - 行コンテナを既存 `<button>` から `<div role="button" tabIndex={0}>` に変更する。`onClick` で `onSelectFeed(feed.feed_id)` を呼び、`onKeyDown` で Enter / Space に対応する（ネスト button 回避のため。既存 `data-testid="feed-item-${id}"` / `data-selected` 属性は維持）
  - 行末尾に `<button type="button" data-testid="feed-settings-button-${id}" aria-label="${feed.feed_title} の設定">` を追加し、`lucide-react` の `Settings` アイコンを表示する
  - ギアボタンの Tailwind class は `opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 focus-visible:opacity-100` を組み合わせ、行コンテナに `group` class を付与する（ホバー時 / 内部 focus 時 / ギア自身に focus-visible 時に表示）
  - ギアボタンの `onClick` は `e.stopPropagation()` してから `onOpenSettings(feed)` を呼ぶ（AC 1.4: 行クリックの `onSelectFeed` を発火させない）
  - `web/src/components/feed-list.test.tsx` を更新:
    - 既存テスト（フィード行 click で `onSelectFeed` 発火、選択ハイライト、未読バッジ、ステータスアイコン、favicon フォールバック）が破壊されないことを確認（NFR 1.1）
    - 新規テストを追加: (a) ギアボタンが各行に存在すること、(b) ギアボタンの `aria-label` が `「<feed_title> の設定」` であること、(c) ギアボタンクリックで `onOpenSettings` が呼ばれること、(d) ギアボタンクリックで `onSelectFeed` が呼ばれないこと（`stopPropagation` 検証）、(e) Tab キーでギアにフォーカス可能 + Enter / Space で `onOpenSettings` 発火
  - `app-shell.tsx` 側の `<FeedList>` 利用箇所も `onOpenSettings={...}` を追加することで TypeScript エラーを解消する（task 5 で正式に wiring するが、ここでは仮の no-op を渡すか、task 5 と同 commit で wiring する）
  - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, 4.5, NFR 1.1, NFR 2.1_
  - _Boundary: FeedList_
  - _Depends: 1_

- [x] 3. SubscriptionSettings: `onUnsubscribed` シグネチャ拡張と既存挙動温存の確認
  - `web/src/components/subscription-settings.tsx` の `SubscriptionSettingsProps` の `onUnsubscribed` を `() => void` から `(unsubscribedFeedId: string) => void` に変更する
  - `handleUnsubscribe` 内の `unsubscribe.mutate(subscription.id, { onSuccess: () => { setShowUnsubscribeDialog(false); onUnsubscribed(subscription.feed_id); } })` のように、解除された subscription の `feed_id` を引数で渡す（AC 4.4）
  - 確認 `AlertDialog` の表示・ボタン非活性化（`unsubscribe.isPending`）・進行中ラベル・キャンセル挙動・確定挙動・タイトル / 本文文言は全て既存挙動を維持する（AC 3.1, 3.2, 3.3, 3.4, 3.5, NFR 3.1, NFR 1.2）
  - 更新間隔 Select（AC 2.2）・フェッチ再開ボタン（AC 2.3, 2.4）・停止中フィードの警告表示（AC 2.1）も既存実装をそのまま温存する（NFR 1.2）
  - 既存 `web/src/components/subscription-settings.test.tsx` は引数を無視する callback で呼ばれており破壊されないことを確認。必要なら spy で引数を assert するテストを 1 件追加（解除成功時に `subscription.feed_id` が渡される、AC 4.4）
  - _Requirements: 2.1, 2.2, 2.3, 2.4, 3.1, 3.2, 3.3, 3.4, 3.5, 4.4, NFR 1.2, NFR 3.1_
  - _Boundary: SubscriptionSettings_

- [x] 4. SubscriptionSettingsDialog: `Dialog` ラッパコンポーネント新設 (P)
  - `web/src/components/subscription-settings-dialog.tsx` を新規作成する
  - props: `{ open: boolean; subscription: Subscription | null; onOpenChange: (open: boolean) => void; onUnsubscribed: (unsubscribedFeedId: string) => void }`
  - 中身は shadcn/ui `Dialog` + `DialogContent` + `DialogHeader`（`DialogTitle="フィードの設定"`）+ `<SubscriptionSettings subscription={subscription} onUnsubscribed={(feedId) => { onUnsubscribed(feedId); onOpenChange(false); }} />`
  - `subscription === null` のとき `<DialogContent>` 内を空にする（または return null）。`open` 制御は親に委譲
  - Dialog のフォーカストラップ・Esc 閉鎖は radix-ui の既定挙動に依拠（NFR 2.2）。追加実装不要
  - 閉じる操作（Esc / Cancel / 外側クリック）で `onOpenChange(false)` が呼ばれ、左ペイン表示状態に戻ること（AC 2.5）
  - `web/src/components/subscription-settings-dialog.test.tsx` を新規作成し、以下を検証:
    - (a) `open=true` + `subscription=<mock>` で `SubscriptionSettings` が render されること（`unsubscribe-button` data-testid 等が画面に存在）
    - (b) `open=false` で内容が render されないこと
    - (c) `subscription === null` で `open=true` でも render しないこと（防御的ガード）
    - (d) `SubscriptionSettings` 内の `onUnsubscribed` 発火で親の `onUnsubscribed(feedId)` が呼ばれ、`onOpenChange(false)` も呼ばれること（AC 4.4）
  - _Requirements: 2.5, 4.4, NFR 2.2_
  - _Boundary: SubscriptionSettingsDialog_
  - _Depends: 3_

- [x] 5. AppShell: 設定ダイアログ状態管理と購読解除後の右ペインクリア統合
  - `web/src/components/app-shell.tsx` に `const [settingsTarget, setSettingsTarget] = useState<Subscription | null>(null)` を追加する
  - `handleOpenSettings = (feed: Subscription) => setSettingsTarget(feed)` を定義し、`<FeedList onOpenSettings={handleOpenSettings} />` に渡す（AC 1.3）
  - `<SubscriptionSettingsDialog open={settingsTarget !== null} subscription={settingsTarget} onOpenChange={(open) => { if (!open) setSettingsTarget(null) }} onUnsubscribed={handleUnsubscribed} />` を 2 ペインの外（`<div data-testid="app-shell">` 直下）に配置する
  - `handleUnsubscribed = (unsubscribedFeedId: string) => { if (unsubscribedFeedId === state.selectedFeedId) { dispatch({ type: "CLEAR_SELECTED_FEED" }); } setSettingsTarget(null); }` を実装する（AC 4.2 / 4.3 の分岐をここに集約）
  - AC 5.3: `onUnsubscribed` は `SubscriptionSettings` の mutation `onSuccess` 内でのみ発火するため、エラー時は本ハンドラに到達せず右ペインが触られないことが構造的に保証されることをコメントで明示する
  - 既存ハンドラ（`handleSelectFeed` / `handleSelectItem` / `handleFeedRegistered`）と 2 ペインのレイアウトは変更しない（NFR 1.1）
  - _Requirements: 1.3, 4.2, 4.3, 5.3_
  - _Boundary: AppShell_
  - _Depends: 2, 4_

- [ ] 6. AppShell: 統合テスト（ホバー → ダイアログ → 解除 → 右ペインクリア / 非クリア / 失敗時）
  - `web/src/components/app-shell.test.tsx` に以下のシナリオを追加する（既存テストパターン: `QueryClientProvider` + `AppStateProvider` + `ThemeProvider` を踏襲、`mockFetch` で HTTP を制御）:
    - (a) フィード行ホバー（`pointerEnter` / `mouseEnter`）→ ギアアイコン表示 → クリック → 「フィードの設定」ダイアログが表示される（AC 1.3, 2.1）
    - (b) ダイアログのキャンセル / Esc で閉じる（AC 2.5）
    - (c) **選択中フィードを購読解除**: 事前にフィードを選択 → ギア → 解除確認 → DELETE 成功（mockFetch で `ok: true`）→ ダイアログ閉鎖 + 一覧から該当フィードが消える + 右ペインが初期表示（`<ItemList feedId={null}>`）に戻ること（AC 4.1, 4.2, 4.4, 4.5）
    - (d) **非選択フィードを購読解除**: フィード A 選択中に **別フィード B** のギア → 解除確定 → DELETE 成功 → 右ペインが A のままで変化しない、B が一覧から消える（AC 4.1, 4.3）
    - (e) **DELETE 失敗時**: `mockFetch` を 500 にして解除確定 → ダイアログが残る、`["feeds"]` 一覧が invalidate されず B がリストに残る、右ペインが変化しない（AC 5.1, 5.2, 5.3）
  - 既存 `app-shell.test.tsx` のテスト（フィード選択・記事展開等）が壊れていないこと（NFR 1.1）
  - _Requirements: 1.3, 2.5, 4.1, 4.2, 4.3, 4.4, 4.5, 5.1, 5.2, 5.3, NFR 1.1_
  - _Boundary: AppShell, FeedList, SubscriptionSettingsDialog_
  - _Depends: 5_

- [ ]* 7. アクセシビリティ追加検証（NFR 2.1 / 2.2）
  - `web/src/components/feed-list.test.tsx` に Tab キーでギアにフォーカスでき、`aria-label` がスクリーンリーダー上で読み上げられる形式（`「<feed_title> の設定」`）であることの追加 assertion を入れる
  - `web/src/components/subscription-settings-dialog.test.tsx` に Esc キーでダイアログが閉じる（radix-ui の既定挙動）統合テストを追加する
  - 本タスクは deferrable（既存統合テストでカバレッジは十分。a11y 観点の重点強化用）
  - _Requirements: NFR 2.1, NFR 2.2_
  - _Boundary: FeedList, SubscriptionSettingsDialog_

## Verify

本 spec の実装後、watcher（stage-a-verify gate）が再実行すべき verify コマンドを構造化ブロックで宣言する。本機能はフロントエンドのみの変更（バックエンド変更なし）であり、`web/package.json` の scripts に従い `npm test` / `npm run lint` / `npm run build` を実行する。

<!-- stage-a-verify -->
```sh
cd web && npm test && npm run lint && npm run build
```
