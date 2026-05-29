# Implementation Notes

## Implementation Notes

### Task 1

- 採用方針: design.md「AppState（Modified）」節および tasks.md task 1 の指示通り、`AppAction` ユニオン型に `ClearSelectedFeedAction = { type: "CLEAR_SELECTED_FEED" }` を追加し、`appReducer` に `SELECT_FEED` と同等の副作用パターン（`selectedFeedId` のみ null、`expandedItemId=null` / `filter="all"` / 検索状態リセット）を持つ case を実装した。
- 重要な判断:
  - `selectedView` は `"feed"` に倒した（`SELECT_FEED` と同じ）。理由: 「選択フィードを失った直後にユーザーが見るのは未選択 feed 表示」というシナリオが自然で、`selectedView` を変えないと `"starred"` 状態のまま選択 ID が null になる不整合が起こり得るため（design.md「Components and Interfaces > AppState」の `Postconditions` と整合）。
  - 検索状態（`searchQuery` / `isSearching` / `searchScope` / `searchFeedId`）も同時にリセットした。理由: 既存 `SELECT_FEED` が検索状態をリセットしている事実（reducer 既存実装）に合わせ「選択フィードが消える＝検索コンテキストも消える」のが自然なため。tasks.md 本文には明示されていないが、design.md の「`SELECT_FEED` と同じ副作用パターンを踏襲」という記述に従った解釈。
  - 既存 19 テストには一切手を加えず、新規 5 テストを追加することで挙動変更がないことを担保した（NFR 1.1）。
- 残存課題: なし。task 2 以降で `AppShell` から `dispatch({ type: "CLEAR_SELECTED_FEED" })` を呼ぶ wiring が予定されている（task 5）。本 task の実装はその下流に必要な reducer 機能を提供する。

### Task 2

- 採用方針: design.md「Components and Interfaces > FeedList（Modified）」節および tasks.md task 2 の指示通り、`FeedListProps` に `onOpenSettings: (subscription: Subscription) => void` を追加し、行コンテナを既存 `<button>` から `<div role="button" tabIndex={0}>` に変更してネスト button 問題を回避した。ギアボタン（`<button data-testid="feed-settings-button-<id>">`）を行末尾に配置し、Tailwind の `group-hover` / `group-focus-within` / `focus-visible:opacity-100` で表示制御。
- 重要な判断:
  - 行コンテナを `<div>` 化したが、既存テスト（`fireEvent.click(screen.getByText("Tech Blog"))` 等）は `<div>` 上でも click が発火するため互換性を維持できた（NFR 1.1）。`data-testid="feed-item-<id>"` / `data-selected` 属性も維持。
  - キーボード起動は `onKeyDown(Enter | Space)` で `e.preventDefault()` してから `onSelectFeed` を呼び、`<button>` のネイティブ activate 挙動を再現（AC 1.5 / WAI-ARIA Practice 準拠）。
  - ギアボタンの `onClick` は `e.stopPropagation()` で行 click ハンドラへの伝搬を止め、加えて `onKeyDown(Enter | Space)` でも `stopPropagation` を行い、ネイティブ button click と親 div の onKeyDown 二重発火を防いだ（AC 1.4 拡張）。
  - app-shell.tsx の `<FeedList>` 利用箇所には no-op `onOpenSettings={() => {}}` を渡し、task 5 で正式 wiring が入るまで TypeScript エラーのみを解消（NFR 1.1: 挙動は完全に不変）。インライン JSX に渡す関数リテラルだが props 数が 4 件で軽量なため、再レンダ過敏なコンポーネントではない判断で `useCallback` 化はしていない。
  - 既存 16 件のテストには既存テストの観点（render 後の DOM 表示）を一切壊さずに `onOpenSettings={() => {}}` props のみを追加し、新規 12 件のテストを追加（合計 28 件 pass）。
  - Tailwind class の検証テスト（`expect(className).toContain("opacity-0")` 等）を入れたのは、`group-hover` 等の CSS 擬似クラスは jsdom が評価しないため、`opacity-0` の解除を runtime で検証できないことの代替として class 文字列レベルで担保するため。
- 残存課題: なし。task 5 で AppShell の `handleOpenSettings` 実装と wiring を行い、no-op を置き換える予定。

### Task 3

- 採用方針: tasks.md task 3 の指示通り、`SubscriptionSettingsProps` の `onUnsubscribed` シグネチャを `() => void` から `(unsubscribedFeedId: string) => void` に拡張し、`handleUnsubscribe` 内の mutation `onSuccess` で `onUnsubscribed(subscription.feed_id)` を渡すように変更した（AC 4.4）。その他の挙動（更新間隔 Select / フェッチ再開 / 警告表示 / 確認 AlertDialog / 進行中ラベル / キャンセル挙動 / 文言）は一切変更していない（NFR 1.2 / NFR 3.1）。
- 重要な判断:
  - `Subscription` 型は既に import 済みで、`subscription.feed_id` は型上 `string` として保証されているので追加 nullish check は不要と判断した。
  - 既存テスト 9 件は `onUnsubscribed={() => {}}` のように引数を無視する callback で呼ばれており、TypeScript の関数型互換性（少ない引数を受け取る関数は多い引数を受け取る関数の型に代入可能）により破壊されない。実際に既存 9 件は全て pass し続けることを確認済み。
  - 新規テスト 1 件「購読解除が成功したとき onUnsubscribed が subscription.feed_id を引数として呼ばれること」を追加し、`vi.fn()` で `onUnsubscribed` を spy し、`mockFetch` で `DELETE /api/subscriptions/sub-1` を成功させた後に `expect(onUnsubscribed).toHaveBeenCalledWith("feed-1")` で AC 4.4 を直接検証した。
  - mutation の `onSuccess` 内で `setShowUnsubscribeDialog(false)` の後に `onUnsubscribed(subscription.feed_id)` を呼ぶ既存順序を維持。task 5 で wire される `AppShell.handleUnsubscribed` 側で `CLEAR_SELECTED_FEED` dispatch と `setSettingsTarget(null)` が行われるが、ダイアログ自体の `open` は SubscriptionSettings 内 state なので先に false にしておく必要がある。
  - `app-shell.tsx` 側で `SubscriptionSettings` を直接利用している箇所は存在せず（Grep で確認済み）、task 4 で新規追加される `SubscriptionSettingsDialog` 経由で利用される予定のため、本 task では他ファイルへの波及修正は不要。`npm run build` も task 3 段階では未実施でよく、`npm test` / `npm run lint` のみで pass を確認した。
- 残存課題: なし。task 4 で `SubscriptionSettingsDialog` ラッパが新規作成され、本 task で拡張した `onUnsubscribed(feedId)` シグネチャを親に伝播する。

### Task 4

- 採用方針: tasks.md task 4 / design.md「SubscriptionSettingsDialog（New）」節の指示通り、`web/src/components/subscription-settings-dialog.tsx` を新規作成。shadcn/ui `Dialog` + `DialogContent` + `DialogHeader` + `DialogTitle="フィードの設定"` で `SubscriptionSettings` をラップし、`onUnsubscribed` を `(feedId) => { onUnsubscribed(feedId); onOpenChange(false); }` で wire する thin wrapper として実装した（AC 2.5 / 4.4）。`subscription === null` のとき `SubscriptionSettings` 自体を render せず、防御的ガードを実装した（task 5 で `settingsTarget=null` を初期値とする AppShell との契約を踏まえた挙動）。
- 重要な判断:
  - `DialogDescription` を `sr-only` で追加した。理由: radix-ui Dialog は `aria-describedby` が未指定だと開発モードで warning を出す既定挙動があり、視覚的にはタイトル直下の領域を取りたくないため `sr-only` でスクリーンリーダ向けの説明文のみ提供。これにより NFR 2.2 のアクセシビリティ要件も補強される。
  - `subscription === null` のとき "DialogContent 内を空にする" 方式を選択し、`return null` 方式は採らなかった。理由: `Dialog` 自体は `open` で制御されており、親（task 5 で `settingsTarget` を `null` にすると `open=false` になる）の責務として閉鎖されるが、レンダリングサイクルで一瞬 `open=true && subscription=null` 状態が起こりうる（state 更新の順序による）。`return null` だと Dialog 自体の closing animation が走らない可能性があるため、Dialog 構造は保ったまま中身だけガードする方式が安全。
  - `onUnsubscribed` の wiring 順序は「親の `onUnsubscribed(feedId)` を先に呼んでから `onOpenChange(false)`」とした。理由: AppShell 側で `handleUnsubscribed` 内に `SELECT_FEED` dispatch が入る予定（task 5）で、右ペインクリア → ダイアログ閉鎖の順序がユーザー体感的に自然（ダイアログが先に消えて右ペインが残ったままになると視覚的に不整合）。tasks.md 本文の `(feedId) => { onUnsubscribed(feedId); onOpenChange(false); }` 順序記述に厳密に従った。
  - テストでは `userEvent.keyboard("{Escape}")` で Esc 経由の `onOpenChange(false)` 発火も追加検証した（NFR 2.2 の radix-ui 既定挙動を実環境で確認するため）。tasks.md の (a)〜(d) の 4 ケースに加え、Esc 検証を 5 ケース目として追加。これは task 7（deferrable a11y 追加検証）の一部前倒しだが、Dialog ラッパの責務として基本中の基本のため task 4 で担保した方が文脈が近い。
  - `open=false` で内容が render されないことのテストは、Dialog 自体が portal に render するため通常時 DOM に存在しないことを `queryByText` / `queryByTestId` で `not.toBeInTheDocument()` で検証。
- 残存課題: なし。task 5 で `AppShell` が `settingsTarget` state を保持し本コンポーネントを wire する予定。本 task の Dialog wrapper は AppShell 側の state 設計（`Subscription | null`）と完全に整合している。

### Task 5

- 採用方針: design.md「AppShell（Modified）」節および tasks.md task 5 の指示通り、`web/src/components/app-shell.tsx` に `useState<Subscription | null>(null)` で `settingsTarget` ローカル state を追加し、`handleOpenSettings` / `handleUnsubscribed` ハンドラを実装。`<FeedList>` の `onOpenSettings` を task 2 の no-op から `handleOpenSettings` に差し替え、`<SubscriptionSettingsDialog>` を 2 ペイン領域の外（`<div data-testid="app-shell">` 直下、`</div>` の直前）に配置した。
- 重要な判断:
  - `settingsTarget` を `Subscription | null` で保持し、`open` 制御は `settingsTarget !== null` で導出する設計（design.md「Architecture Pattern & Boundary Map」と完全一致）。`subscription` も同 state から渡すことで、open と対象 subscription の状態整合性を 1 つの state で担保する（zombie state を作らない）。
  - `handleUnsubscribed` 内で `dispatch({ type: "CLEAR_SELECTED_FEED" })` の後に `setSettingsTarget(null)` を呼ぶ順序とした。React の state 更新は同 turn でバッチされるため順序は視覚的影響を与えないが、ロジックの読みやすさとして「先に右ペインクリア判定 → 後にダイアログ閉鎖」というユーザーシナリオ順序に合わせた。
  - AC 5.3 の構造的保証についてのコメントを `handleUnsubscribed` の JSDoc 内に明記した。SubscriptionSettings → SubscriptionSettingsDialog → AppShell の callback チェーン全体で「mutation `onSuccess` 内でのみ発火」が保たれているため、AppShell 側で明示的なエラー分岐は不要であることを将来の保守者向けに残した。
  - `onOpenChange` の処理を `(open) => { if (!open) setSettingsTarget(null); }` とし、Esc / 外側クリックでの閉鎖でも `settingsTarget` を null に戻すことで購読解除を経由しない「単にダイアログを閉じる」経路も正しく動作するようにした（AC 2.5）。task 4 で実装した SubscriptionSettingsDialog 側の `onUnsubscribed → onOpenChange(false)` 順序とも整合する。
  - 既存ハンドラ・2 ペインレイアウト・`ThemeToggle` 等は一切変更せず、追記のみで実装した（NFR 1.1）。既存 16 テスト（app-shell.test.tsx）が破壊されないことを `npm test` で確認済み。
  - 統合テストは task 6 でカバーするため、本 task では追加テスト不要と判断（tasks.md task 5 の指示通り）。既存テストが全て pass し、AC は task 6 の統合テストで担保される。
- 残存課題: なし。task 6 で AppShell の統合テスト（ホバー → ダイアログ → 解除 → 右ペインクリア / 非クリア / 失敗時の 5 シナリオ）を追加することで AC 1.3 / 4.2 / 4.3 / 5.3 のランタイム動作が直接検証される予定。

### Task 6

- 採用方針: tasks.md task 6 の指示通り、`web/src/components/app-shell.test.tsx` の既存 `describe("AppShell コンポーネント", ...)` 配下に内側 `describe("購読解除フロー（task 6）", ...)` を追加し、(a)〜(e) の 5 シナリオをそれぞれ 1 `it` で実装した。既存 11 件のテストには一切手を加えず、合計 16 件全て pass する状態を維持した（NFR 1.1）。
- 重要な判断:
  - `setupUnsubscribeMockFetch(deleteOk: boolean)` という共通ヘルパを `describe` 内に定義し、`DELETE /api/subscriptions/:id` の成否を引数で切り替える設計とした。成功時は `deletedIds` Set に id を蓄積し、以降の `GET /api/subscriptions`（`useFeeds` の refetch）が解除済みを除いた一覧を返すよう動的に挙動を切替える。これにより (c)(d) シナリオで `["feeds"]` invalidate → refetch → 一覧 UI 更新（AC 4.1）まで含めて end-to-end に検証できる。
  - **設定 Dialog 表示中の accessibility tree 隠蔽問題**: radix-ui の `Dialog` はモーダル表示中に AppShell 内のメインコンテンツ（右ペインの `<Tabs>` 含む）に `aria-hidden="true"` を付与し、accessibility tree から隠す。このため (e) シナリオで `screen.getByRole("tab", { name: "全て" })` が dialog 開放中は **取得不可** となる（最初のテスト実行で hit したエッジケース）。対応として、右ペインが Tech Blog のままであることの検証を「左ペインのフィード行 `data-testid="feed-item-sub-1"` の `data-selected="true"` 属性確認」+ 「`「フィードを選択してください」` テキストが描画されていない（= ItemList feedId=null 描画ではない）」の組み合わせで代替した。DOM 属性 / textContent ベースのクエリは aria-hidden に影響されないため、selectedFeedId の維持を確実に検証できる。
  - **ホバー検証の jsdom 制約**: tasks.md (a) は「フィード行ホバー → ギアアイコン表示 → クリック」と指示するが、jsdom は CSS `:hover` 擬似クラスを評価しないため `userEvent.hover()` で表示変化を直接観測できない。task 2 で `feed-list.test.tsx` 側に class 文字列レベル（`opacity-0` / `group-hover:opacity-100`）の検証は既に存在するため、本 task では `userEvent.hover()` でイベントを発火させた上で `feed-settings-button-sub-1` ボタンが DOM 上に存在することを確認 → クリックして「フィードの設定」ダイアログが描画されるまでを統合経路として検証した。
  - **失敗時のダイアログ残存検証 (e)**: `subscription-settings.tsx` の `handleUnsubscribe` 内で `unsubscribe.mutate(...)` の `onSuccess` 内でのみ `setShowUnsubscribeDialog(false)` と `onUnsubscribed(subscription.feed_id)` が呼ばれる構造。500 失敗時は `onSuccess` が発火せず、よって AppShell の `handleUnsubscribed` も呼ばれず、`settingsTarget` も `null` に戻らない（→ 親 Dialog が開きっぱなし）ことを構造的に保証している。テストでは `expect(screen.getByText("フィードの設定")).toBeInTheDocument()` でこの残存を観測している（AC 5.2）。なお radix-ui `AlertDialogAction` クリックでは確認 AlertDialog 自体は閉じる（コンポーネント仕様）ため、(e) は「確認ダイアログ→閉、設定ダイアログ→残存」という現実的な挙動を反映した検証になっている。
  - **`unhandled` promise rejection の扱い**: 500 を返す `apiClient.delete` は `ApiError` を throw し `mutation.onError` パスに行くが、`useUnsubscribe` は `onError` を定義していないので unhandled rejection 相当になる。`QueryClient` を `mutations: { retry: false }` で生成しているため retry 連鎖はせず、テスト pass に影響しないことを確認済み（既存 `subscription-settings.test.tsx` も同パターンを採用）。
  - **複数ダイアログのフォーカス管理**: (c)(d)(e) の "確認 AlertDialog → 親設定 Dialog" の 2 段モーダル構造で radix-ui のフォーカストラップが正しく動作することを `await user.click(screen.getByRole("button", { name: "購読解除" }))` の遷移で間接的に検証している（user-event の click はフォーカス可能要素を自動でフォーカスする）。
- 残存課題: なし。本 task で AC 1.3 / 2.5 / 4.1 / 4.2 / 4.3 / 4.4 / 4.5 / 5.1 / 5.2 / 5.3 の統合動作を E2E に近い形で検証できた。task 7 は deferrable な追加 a11y 検証で、必要時のみ実装する。

## 受入基準カバレッジ（task 1 分のみ）

| Requirement | テスト |
|---|---|
| 4.2（部分）: 解除されたフィードが選択中だった場合に右ペインをクリアするために必要な reducer 機能 | `app-state.test.tsx` の `CLEAR_SELECTED_FEED アクションで selectedFeedId が null になり、expandedItemId と filter がリセットされること` / `CLEAR_SELECTED_FEED アクションで検索状態（searchQuery / isSearching / searchScope / searchFeedId）もリセットされること` / `CLEAR_SELECTED_FEED アクションは初期状態に対しても安全に動作すること（冪等性）` |
| NFR 1.1: 既存 action 挙動の非変更 | `app-state.test.tsx` の `CLEAR_SELECTED_FEED アクション導入後も既存 SELECT_FEED の挙動が変わらないこと（NFR 1.1 回帰）` / `... 既存 EXPAND_ITEM のトグル挙動 ...` / `... 既存 SET_FILTER の挙動 ...` および既存 17 テストが全て green |

## verify 実行結果（task 1 分のみ）

- `web/src/contexts/app-state.test.tsx`: 22 件 pass（既存 17 件 + 新規 5 件）
- `npm test`（web 全体）: 322 件 pass / 34 ファイル全て green（既存テストの破壊なし、NFR 1.1 担保）
- 実行ノードは Node 24.11.1 を利用（`web/package.json` の依存 `whatwg-url@16.0.1` が Node `^20.19.0 || ^22.12.0 || >=24.0.0` を要求するため。`PATH` 上の Node 22.11.0 はバージョン不整合で vitest が起動しない既存環境問題があり、Node 24.x で代替実行した）。

## 確認事項

- なし（task 1 単体では requirements.md / design.md / tasks.md と矛盾なく実装可能だった）
