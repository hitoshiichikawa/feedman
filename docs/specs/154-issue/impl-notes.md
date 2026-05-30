# Implementation Notes

本 spec の per-task ループ実装中に得られた知見を、`### Task <id>` 見出し単位で
追記する。先行 task の見出しは改変・削除・並び替えしないこと（前方伝播の規律）。

## Implementation Notes

### Task 1

- 採用方針: 5 レイヤ縦断（`model.ItemSearchHit` → `PostgresItemRepo.SearchByUserAndKeyword` →
  `itemsearch.ItemSearchSummary` → `itemSearchHitResponse` (handler) → `ItemSearchServiceAdapter`）
  すべてに `HatebuFetchedAt *time.Time` を追加し、各レイヤで純粋な pass-through とした。
- 重要な判断:
  - JSON タグは `"hatebu_fetched_at,omitempty"` を採用し、Go 側 nil → JSON から省略する流儀を
    既存 `favicon_url` に揃えた（design.md 行 410 / 414 の方針）。フロント側は次の task 2 以降で
    TypeScript 型を `string | null` で受け、`undefined ?? null` の正規化で `-` 表示判定を統一する。
  - リポジトリ層は `sql.NullTime` で Scan し、`Valid` → `&Time` を保持する既存パターン
    （`FindByID` / `ListNeedingHatebuFetch` 等）に揃えた。SQL の `SELECT` 列追加のみで
    `WHERE` / `ORDER BY` / `LIMIT` / `JOIN` は不変（NFR 3.1 を担保）。
  - 既存テスト `TestSearch_HitsConvertedToSummaries` を拡張し、その上で `HatebuFetchedAt`
    特化テスト `TestSearch_HatebuFetchedAt_NilAndNonNilPassthrough` を別途追加した。
    nil/非 nil の両条件を 1 テスト 1 検証に分けつつ、既存 reflect.DeepEqual の全フィールド
    マッピング検証も `HatebuFetchedAt` を含む新 struct を期待値とした形で更新済み。
  - repo DB 結合テストは既存 `setupItemSearchTestDB` の Skip ガード（DB 接続不可で `t.Skip`）
    を踏襲。CI（DB 起動なし）では Skip され、ローカル DB ありの場合のみ NULL/値あり両分岐を
    実行する。
- 残存課題: なし（task 2 以降の TypeScript 型 / UI 配線は後続 task で対応）。

### Task 2

- 採用方針: `web/src/types/item.ts` の `ItemSearchHit` interface に
  `hatebu_fetched_at: string | null` を必須フィールドとして追加し、既存 `ItemSummary` の
  doc コメント表記（`未取得時はnull`）と整合させた。doc コメントには検索 API が Go 側
  `omitempty` で未取得時にキーを省略する点と、呼び出し側で `undefined ?? null` 正規化する
  方針（design.md Notes for Developers）を併記した。
- 重要な判断:
  - 既存 doc コメントの「省略: `hatebu_fetched_at`（検索 API レスポンスに含まれないため）」は
    削除し、追加フィールドの仕様で置き換えた（design.md の差分指示と整合）。
  - 型を `string | null` の必須プロパティ（optional `?` ではなく）で追加したことで、既存テスト
    fixture（`search-results.test.tsx` / `use-item-search.test.tsx`）の mock object に
    `hatebu_fetched_at: null` 補填が必要になった。CI を壊さないために本 task 内で最小限の
    fixture 修正を加えた。`hatebu_fetched_at` を null にした選択は「既存テスト = 未取得記事の
    分岐挙動」を改変しないことを優先したため。値ありシナリオのテスト追加は Task 6
    （SearchResults 配線時）でカバーされる予定。
  - 既存 `ItemSearchResponse` / 他フィールドの構造・順序・型は不変。新フィールドの配置は
    `hatebu_count` の直後とし、関連フィールドをグルーピングした（design.md の Notes for
    Developers では位置指定なし）。
- 残存課題:
  - tsc baseline で既存の TS エラー（`feed-list.test.tsx` の `onOpenSettings` / `viewMode`、
    `starred-item-list.test.tsx` の `expandedItemId`、`starred-nav-item.test.tsx` の
    `selectedView` / `selectedFeedId`、`rewrites.test.ts` の `NODE_ENV`）が残存している。
    Task 2 と無関係な既存問題のため本 task ではノータッチ。Task 5 / 7 で `ItemList` /
    `ItemDetail` を改修する際にも触れない予定。確認事項として PR レビュワーに共有する必要あり。
  - Task 6（SearchResults 配線）で `hatebu_fetched_at` 値ありケースのテストを追加する想定。

### Task 3

- 採用方針: `web/src/components/item-meta-actions.tsx` を新規作成し、3 一覧から再利用される
  純粋プレゼンテーショナルコンポーネントとして実装した。Props は design.md 行 211-223 の
  契約（`itemId` / `isStarred` / `hatebuCount` / `hatebuFetchedAt` / `onToggleStar`）に
  そのまま追従。mutation や API 呼び出しは保持せず、責務は (1) はてブ数 / `-` 表示分岐、
  (2) スター⭐️アイコン塗り分け、(3) クリック時 `e.stopPropagation()` + callback 発火 に
  限定（design.md 行 196-200 の責務制約と整合）。
- 重要な判断:
  - 既存 `ItemDetail` のスター UI 実装（aria-label / aria-pressed / `size="icon-sm"` /
    塗り分け）をそのまま移植した。lucide `Star` と `Bookmark` の className 構成、
    `cn()` ユーティリティの利用、`rounded-full` の付与方針も同等とし、外観の差異を最小化。
  - `data-testid` 規約は design.md 行 230-236 の指針どおり `item-meta-actions-${id}` /
    `item-hatebu-count-${id}` / `item-star-toggle-${id}` の 3 つを付与し、既存
    `star-${id}` / `search-result-star-${id}` / `star-toggle` / `hatebu-count` との
    衝突を避けた（NFR 3.2 / 3.3）。Task 5 / 7 で旧 testid を撤去する想定。
  - クリックハンドラは React MouseEvent を分離した named handler として実装し、テスト時の
    `e.stopPropagation()` 検証を「親 div の onClick が呼ばれない」観点で行う設計とした
    （vi.fn の呼出回数で `stopPropagation` の効果を間接検証する慣用パターン）。
  - 0 件 と「未取得」の区別表示は独立した `describe` ブロックで検証し、`hatebuCount=0` +
    `hatebuFetchedAt=null` の境界ケースで `-` を、`hatebuCount=0` + `hatebuFetchedAt=<時刻>`
    で `0` を表示する分岐を AC ごと（Req 1.3 / 1.4 / 5.3 / 5.4）に対応付けた。
  - 32px ヒット領域は Button cva の `icon-sm` variant が `size-8`（= 32px）class を出力する
    ことを `toggle.className.toContain("size-8")` + `data-size="icon-sm"` 属性で 2 重に
    確認した（NFR 1.3）。
  - キーボード操作（NFR 1.4 / Tab + Enter/Space）は Button の native `<button>` 標準挙動
    で担保されるため明示テスト追加は不要と判断（既存 `Button` テスト `src/__tests__/button.test.tsx`
    で標準挙動が担保済み）。
- 残存課題:
  - Task 5（ItemList / StarredItemList 配線）/ Task 6（SearchResults 配線）/ Task 7
    （ItemDetail メタ撤去）で本コンポーネントを呼び出す配線と、既存 `star-${id}` /
    `search-result-star-${id}` / `hatebu-count` / `star-toggle` testid の撤去が必要。
  - tsc baseline の既存 TS エラー（Task 2 で記録した 4 ファイル）は本 task では非影響。
    Task 5 / 7 の改修時にも触れない方針を継続。
  - Task 1 / 2 で記録した「Go 側 `omitempty` で未取得時にキーを省略 → TypeScript 側で
    `undefined ?? null` 正規化」の方針は、本コンポーネントが props として正規化済みの
    `string | null` を受け取るため、本コンポーネント自体には影響しない（正規化は呼び出し側
    Task 6 で担保）。

### Task 4

- 採用方針: 既存 `useToggleStar` の `["items"]` 楽観更新パターン（cancelQueries →
  getQueriesData snapshot → setQueryData で `pages[].items[].is_starred` を反転）を
  `["item-search"]` にもそのまま複製し、3 フェーズ（onMutate / onError / onSettled）の
  すべてで対称的に扱う構造とした。`["cross-feed-items"]` は task 指示に従い既存どおり
  invalidate のみ（楽観更新の対象外）として、本 task のスコープを `["item-search"]`
  追加に限定した。
- 重要な判断:
  - **context shape の拡張方針**: design.md 行 373-377 の `ToggleStarContext`
    （`previousItems` / `previousSearch` / `previousCrossFeed` 3 グループ）を踏襲しつつ、
    実装側は `previousCrossFeed` を含めない（cross-feed は invalidate のみで snapshot 不要)。
    また既存 `previousData` フィールドを `previousItems` の alias として残置する後方互換
    措置を採った（外部からの参照可能性は低いが、明示的に context type を `useMutation`
    に注入する都合上、コードレベルで shape diff を最小化する判断）。alias の cleanup は
    Issue 完結後の cleanup PR で実施する想定。
  - **`["cross-feed-items"]` を楽観更新の対象外としたこと**: tasks.md 4 行目 / design.md
    行 340-342 は `["items"]` と `["item-search"]` のみを楽観更新対象として明示しており、
    `["cross-feed-items"]` は invalidate のみで設計上は十分（onSettled で必ず refetch される
    ため、UI ラグは数百 ms 範囲）。design.md 行 376 の `previousCrossFeed` は将来の拡張
    余地として残された設計記述だが、本 task ではスコープ外として扱う。
  - **テストの cache seeding パターン**: 既存テストは `createWrapper()` が render ごとに
    新しい QueryClient を作る設計だったため、テスト本体から `setQueryData` で seed したり
    `getQueryData` で観測したりすることができなかった。本 task では追加テスト用に
    `createSharedClientWrapper()` ヘルパーを新設し、`queryClient` と `wrapper` を同一
    インスタンスから返すことで cache 操作と hook 呼び出しを共有 QueryClient 上で行えるよう
    にした。これにより onMutate 直後の楽観更新と onError 後のロールバックを
    `queryClient.getQueryData(searchKey)` で直接検証できる。
  - **エラー mock の方針**: `apiClient.put` は non-ok response で `ApiError` を throw する
    実装のため、`mockFetch.mockResolvedValue({ ok: false, status: 500, text: async () => ... })`
    の形で 500 を返すと mutation が isError 状態に遷移する。これは既存 `use-cross-feed-items`
    / `use-items` のテストで使われている定番パターンと整合。
  - **テストファイル拡張子の齟齬**: tasks.md 行 40 は `web/src/hooks/use-item-state.test.ts`
    と指定しているが、既存実装ファイルが `.test.tsx` 拡張（QueryClientProvider の JSX を
    使う都合）であり、テストを別ファイルに分離するとモック共有や fixture 重複が発生する。
    本 task では既存 `use-item-state.test.tsx` にそのまま追記する判断とした。これは
    spec の細部齟齬であり PR 確認事項にも記載予定。
  - **テスト件数**: AC (a)(b)(c) の 3 件 + cache 不在時の robustness 1 件の合計 4 件を
    追加。(c) は既存挙動非回帰を「失敗系で `["items"]` もロールバックされる」観点で 1
    ケース直接検証する形にした（既存テスト 3 件の pass で間接的に担保されている既存挙動を、
    1 ケースだけ明示的に固定する規律）。
- 残存課題:
  - `previousData` alias の cleanup は Issue 完結後の cleanup PR で実施予定。現状で
    外部から参照されていないが、context shape の 1st-class フィールドとして残置中。
  - tsc baseline の既存 TS エラー（Task 2 で記録した 4 ファイル）は本 task では非影響。
    Task 5 / 6 / 7 でも触れない方針を継続。
  - `["cross-feed-items"]` への楽観更新が将来必要になった場合（例: cross-feed 一覧で
    スター操作直後に表示反映の遅延が UX 上問題になるなど）は、design.md 行 376 の
    `previousCrossFeed` グループを参考に追加実装する。本 task ではスコープ外。

### Task 5

- 採用方針: `ItemRow` の props に `onToggleStar: (itemId, nextStarred) => void` を
  必須追加し、タイトル行内の `<time>` 表示の右隣（既存の読み取り専用 `Star` アイコン
  があった位置）に Task 3 で実装済みの `ItemMetaActions` を配置した。`ItemList` /
  `StarredItemList` / `CrossFeedItemList` の 3 呼び出し側で既存の `handleToggleStar`
  を pass-through する形で配線し、3 一覧すべてが共通の `ItemMetaActions` を経由して
  `useToggleStar` mutation を発火する経路に統一した（Req 1.1〜1.7 / 2.3 / 4.1 / 4.2）。
- 重要な判断:
  - **`onToggleStar` を必須 prop にした判断**: optional + default no-op にする選択肢
    もあったが、ItemRow の責務（一覧上スター⭐️操作の発火）を型レベルで強制する
    方針に倒した（呼び出し側が必ず配線する規律。NFR 3.2 の整合性確保）。代償として
    既存呼び出し側 `cross-feed-item-list.tsx` も同時に prop 配線が必要となったため、
    本 task の単一 commit にまとめて含めた。
  - **`cross-feed-item-list.tsx` への波及**: Task 5 の `_Boundary:_` は ItemList /
    StarredItemList / ItemRow を列挙しているが、`ItemRow` の prop が breaking
    change（required prop 追加）になるため、同じ `ItemRow` を import している
    `CrossFeedItemList` も同時配線が必要だった（既存 `handleToggleStar` を渡す
    1 行追加のみ）。これは Boundary 逸脱というより props 互換性を保つための必然の
    pass-through で、`useToggleStar` の `["cross-feed-items"]` invalidate は Task 4
    で既に対応済み（楽観更新ではなく refetch 経由の UX）。確認事項として PR 本文に
    残す必要あり。
  - **既存読み取り専用 Star testid（`star-${id}`）の撤去**: tasks.md の指示通り
    `data-testid={"star-" + id}` を完全に削除した。これは NFR 3.2 の「等価後継
    識別子で置き換える」要件を `item-star-toggle-${id}`（toggle 操作可能な後継）
    で満たす形（design.md 行 234-235）。旧 testid を参照していた既存テストは存在
    しなかったため移行コストはなかった。
  - **HTML 構造の `<button>` ネスト警告**: `ItemRow` 自体が `<button data-testid=
    "item-row-${id}">` で行全体をクリック可能にしている既存設計のため、内側に
    `<Button data-testid="item-star-toggle-${id}">` を配置すると `<button>` cannot
    contain a nested `<button>` の hydration warning が React DevTools から出力さ
    れる。これは本 task の design.md / tasks.md が暗黙的に要求する構造（既存
    `ItemRow` button のまま + `ItemMetaActions` 内 Button）の結果で、テストは全件
    pass する。本来は `ItemRow` を `<div role="button">` へ書き換えるか、行クリック
    を別途 click handler で処理する設計修正が望ましい。design.md にこの点の指針が
    無いため本 task では既存構造を踏襲し、PR 本文「確認事項」に残して別 Issue 化を
    提案する。
  - **テスト件数**: item-list.test.tsx に Task 5 関連 4 件（testid 出現 / `-`
    vs 数値分岐 / 塗り分け / 伝播抑止 + mutation）、starred-item-list.test.tsx に
    4 件（testid 出現 / hatebu 分岐 / 伝播抑止 + mutation / aria 状態分岐）を追加。
    既存 18 + 6 件は不変で非回帰を担保。
  - **CrossFeedItemList のテスト**: 本 task では cross-feed-item-list.tsx の 1 行
    pass-through 追加のみで既存テストは全件 pass。明示的な per-task 追加テストは
    避けた（cross-feed は Task 5 の `_Requirements:_` 範囲外であり、Boundary
    `CrossFeedItemList` は本タスクの責務に含まれない）。
- 残存課題:
  - **HTML ネスト構造の整理**: `<button>` 内 `<button>` の警告は別 Issue で
    `ItemRow` を `<div role="button">` 化するか、行クリック展開を別 div の onClick
    へ分離する形で解消するのが望ましい。PR 本文「確認事項」に記載予定。
  - Task 6（SearchResults 配線）/ Task 7（ItemDetail ヘッダーメタ撤去）が後続。
    既存 `item-detail.tsx` の `star-toggle` / `hatebu-count` testid は本 task で
    撤去していない（Task 7 の責務）。新規 testid `item-star-toggle-${id}` /
    `item-hatebu-count-${id}` は一意化により衝突しない（NFR 3.3 を担保）。
  - tsc baseline の既存 TS エラー（Task 2 で記録した 4 ファイル）は本 task では
    非影響。
  - 本 task 完了時点で `cd web && npm test -- --run`（398 件）/ `npm run lint`
    （0 errors）は全 green。Go 側は本 task では変更なし。

### Task 6

- 採用方針: `SearchResultRowProps` に `onToggleStar: (itemId, nextStarred) => void` を
  必須追加し、タイトル行内の `<span>` 日時表示の右隣（既存の読み取り専用 `Star`
  アイコンがあった位置）に Task 3 で実装済みの `ItemMetaActions` を配置した。
  `SearchResults` 本体で既存の `handleToggleStar` を `SearchResultRow` に
  pass-through する形で配線し、Task 5 の `ItemList` / `StarredItemList` /
  `CrossFeedItemList` と合わせて 3 一覧（通常 / スター横断 / 検索結果）すべてが
  共通の `ItemMetaActions` を経由して `useToggleStar` mutation を発火する経路に
  統一した（Req 1.1〜1.7 / 2.3 / 4.3 / 5.3 / 5.4）。
- 重要な判断:
  - **`hit.hatebu_fetched_at` の `?? null` 正規化**: Task 1 で Go 側 JSON タグを
    `omitempty` で確定したため、検索 API レスポンスは未取得時にキーを省略する
    （JSON 上は `undefined`）。Task 2 で TypeScript 型を `string | null` の必須
    プロパティで定義したため厳密には `undefined` が来ないはずだが、design.md
    Notes for Developers の方針に従い `hit.hatebu_fetched_at ?? null` で
    防御的に正規化した。これは Task 5 の `ItemList` 配線（`item.hatebu_fetched_at`
    をそのまま渡す）と差異があるが、`ItemSummary` 側は API の `omitempty` が
    無い前提で `null` がそのまま来るのに対し、`ItemSearchHit` は将来の API
    変更で `undefined` 化されても影響を受けない設計を優先した判断。
  - **`onToggleStar` を必須 prop にした判断**: Task 5 の `ItemRow` と同じく
    optional + default no-op ではなく必須プロパティとし、`SearchResults`
    呼び出し側で必ず配線する規律を型レベルで強制した（NFR 3.2 の整合性確保）。
  - **既存 `search-result-star-${id}` testid の撤去**: tasks.md の指示通り
    完全削除した。既存テストは「既読記事と未読記事でスタイル状態が data-read
    属性で区別されること」で `search-result-star-item-2` の存在 / 非存在を
    検証していたため、`item-star-toggle-${id}` の aria-pressed 検証に置換した
    （NFR 3.2 の「等価後継識別子で置き換える」要件）。
  - **`<button>` ネスト警告**: Task 5 で記録した通り、`SearchResultRow` 自体が
    `<button data-testid="search-result-row-${id}">` で行全体をクリック可能に
    している既存設計のため、内側に `<Button data-testid="item-star-toggle-${id}">`
    を配置すると `<button>` cannot contain a nested `<button>` の hydration
    warning が React DevTools から出力される（テストでも stderr 出力されるが
    13 件全 pass）。これは本 task の design.md / tasks.md が暗黙的に要求する
    構造の結果で、本 task では既存構造を踏襲する判断を継続。PR 本文「確認事項」に
    引き続き残し別 Issue 化を提案する。
  - **テスト件数**: search-results.test.tsx に Task 6 関連 4 件（hatebu null →
    `-` 分岐 / hatebu 取得済み数値分岐 [0 件・多数の境界 2 ケース] / 伝播抑止 +
    mutation / testid 出現と旧 testid 撤去）を追加。既存 9 件は data-read
    区別テストの testid 置換を除いて不変で非回帰を担保。test 全件 13 件 pass。
  - **mock fetch のパターン**: 既存 `setupSearchFetch` / `setupErrorFetch`
    ヘルパーは検索 API のみを ok 応答する設計だったため、mutation PUT テスト
    では mockFetch を直接 `.mockImplementation` で上書きし、検索 API と
    `/api/items/item-1/state` PUT の両方をルーティングする形にした。これは
    item-list.test.tsx の `setupMockFetchWithDetail` と類似のパターン。
- 残存課題:
  - **HTML ネスト構造の整理**: Task 5 で記録した通り `<button>` 内 `<button>`
    の警告は別 Issue で `SearchResultRow` / `ItemRow` を `<div role="button">`
    化するか、行クリック展開を別 div の onClick へ分離する形で解消するのが
    望ましい。本 task でも継続。PR 本文「確認事項」に記載予定。
  - Task 7（ItemDetail ヘッダーメタ撤去）が後続。本 task で `item-detail.tsx` の
    `star-toggle` / `hatebu-count` testid は撤去していない（Task 7 の責務）。
    新規 testid `item-star-toggle-${id}` / `item-hatebu-count-${id}` は一意化
    により衝突しない（NFR 3.3 を担保）。
  - tsc baseline の既存 TS エラー（Task 2 で記録した 4 ファイル）は本 task では
    非影響。
  - 本 task 完了時点で `cd web && npm test -- --run`（402 件）/ `npm run lint`
    （0 errors / 5 既存 warnings）/ `go test ./...` / `go vet ./...` は全 green。
