# Implementation Notes

本ノートは Issue #117（フィード横断スター記事一覧）の per-task 実装ループにおける
各 task の learning を記録する。

## Implementation Notes

### Task 1

Repository 層 `ListStarredByUser` メソッドの追加と DB 結合テスト。

#### 採用方針

- `repository.StarredItemRow` 構造体（`model.ItemWithState` を embed + `FeedTitle string`）を
  `interfaces.go` 内に新設。design.md の選択肢のうち「`repository` パッケージ内に新設」を採用。
  理由: `model` パッケージは純粋ドメインモデルに留め、JOIN 由来の派生カラム（feed_title）は
  repository 層のクエリ結果として扱う方が責務分離が綺麗になるため。
- SQL は `items i INNER JOIN item_states s ON i.id = s.item_id INNER JOIN feeds f ON i.feed_id = f.id`
  の 3 段 INNER JOIN を採用。`WHERE s.user_id = $1 AND s.is_starred = true` で部分インデックス
  `idx_item_states_user_starred` を駆動。
- `SELECT` に `true AS is_starred` を明示し、INNER JOIN の結果が常に true であることを SQL
  レベルで宣言する（design.md §Repository SQL 設計方針と同じ）。

#### 重要な判断

- **mockItemRepo へのスタブ追加**: `internal/item/upsert_test.go` の `mockItemRepo` に
  `ListStarredByUser` のスタブを追加（戻り値 `(nil, nil)`）。Service 層テストでは未使用だが、
  `var _ ItemRepository = (*PostgresItemRepo)(nil)` と同じ interface 充足のため
  サービス層テストの mock も interface を満たす必要があるため。Task 2 で実体実装が入る。
- **テスト fixture の helper 関数**: `insertTestItem` / `insertTestItemState` を新設。既存
  `postgres_feed_repo_test.go` の `insertTestUser` / `insertTestFeedWithTitle` /
  `setupListDueTestDB` は package-private で同 package 内から再利用可能だったため、それらを
  再利用しつつ items / item_states 専用 helper のみ追加した。
- **テストケース構成**: 設計指示の (a)〜(g) を以下にマップ:
  - (a) (c) (d) (g): 「自ユーザーの複数フィードのスター記事が降順で返り feed_title が付与される」
    1 ケースに統合（同一 fixture で 4 観点を同時に検証可能なため）
  - (b): 「他ユーザーのスター記事は返らない」（NFR 2.1）
  - (b) 拡張: 「スター解除済み記事は返らない」（is_starred=false の状態行が混入しないこと）
  - (e): 「cursor 指定時に当該時刻より前の記事のみ返る」
  - (f): 「スター記事が 0 件のとき空スライスが返る」
  - 追加: 「limit が指定件数で SQL レベルに反映される」（境界系の補強）

#### EXPLAIN ANALYZE 結果（NFR 1.1 / 1.2 検証）

5000 記事 / 100 ユーザー / 50 フィード / user1 が 100 件スター / 他ユーザー 99 名で合計
396 件スターの状態で `ANALYZE` 済みの状態において、cursor なし先頭ページ取得の実プランは
以下:

```
Limit  (cost=56.79..56.80 rows=5 width=1180) (actual time=0.267..0.271 rows=50 loops=1)
   InitPlan 1 (returns $0)
     ->  Seq Scan on users  (cost=0.00..3.25 rows=1 width=16) (actual time=0.005..0.009 rows=1 loops=1)
           Filter: ((email)::text = 'seed-1@example.com'::text)
   ->  Sort  (cost=53.54..53.55 rows=5 width=1180) (actual time=0.266..0.268 rows=50 loops=1)
         Sort Key: i.published_at DESC
         ->  Nested Loop  (cost=4.61..53.48 rows=5 width=1180) (actual time=0.041..0.228 rows=100 loops=1)
               ->  Nested Loop  (cost=4.47..52.68 rows=5 width=1172) (actual time=0.036..0.168 rows=100 loops=1)
                     ->  Bitmap Heap Scan on item_states s  (cost=4.19..11.18 rows=5 width=17) (actual time=0.026..0.033 rows=100 loops=1)
                           Recheck Cond: ((user_id = $0) AND is_starred)
                           ->  Bitmap Index Scan on idx_item_states_user_starred  (cost=0.00..4.18 rows=5 width=0) (actual time=0.022..0.022 rows=100 loops=1)
                                 Index Cond: (user_id = $0)
                     ->  Index Scan using items_pkey on items i  (cost=0.28..8.30 rows=1 width=1171) (actual time=0.001..0.001 rows=1 loops=100)
                           Index Cond: (id = s.item_id)
               ->  Index Scan using feeds_pkey on feeds f  (cost=0.14..0.16 rows=1 width=23) (actual time=0.000..0.000 rows=1 loops=100)
                     Index Cond: (id = i.feed_id)
 Planning Time: 2.160 ms
 Execution Time: 1.352 ms
```

cursor 指定時（`i.published_at < now() - interval '30 minutes'`）のプラン:

```
Limit  (cost=56.83..56.84 rows=5 width=1180) (actual time=0.230..0.233 rows=50 loops=1)
   ...
   ->  Bitmap Index Scan on idx_item_states_user_starred  (cost=0.00..4.18 rows=5 width=0) (actual time=0.018..0.018 rows=100 loops=1)
   ...
   ->  Index Scan using items_pkey on items i  (cost=0.28..8.31 rows=1 width=1171) (actual time=0.001..0.001 rows=1 loops=100)
         Index Cond: (id = s.item_id)
         Filter: (published_at < (now() - '00:30:00'::interval))
 Planning Time: 0.900 ms
 Execution Time: 0.290 ms
```

確認できた事項:

- **NFR 1.2（部分インデックスを破壊しない）**: `idx_item_states_user_starred` が
  `Bitmap Index Scan` で実際に選択されている。Recheck Cond / Index Cond が想定通り。
- **NFR 1.1（単一フィード API と同等の応答時間）**: 1.352 ms / 0.290 ms はいずれも単一フィード
  クエリの一般的な実行時間（< 数 ms）と同水準であり、有意な悪化はない。
- **N+1 不在**: `feed_title` 取得のための JOIN は `feeds_pkey` を用いた 1 段の Index Scan で、
  ループ内追加クエリは発生していない（design.md §Performance & Scalability 通り）。
- 注意: cursor 適用条件 `published_at < ...` は `Index Scan using items_pkey` の Filter として
  適用されている。今回の fixture では `item_states` 経由で対象が 100 件まで絞り込まれた
  状態で items_pkey にぶつけているため、Filter 適用後の効率は十分。本番でスター件数が
  数万件規模になる場合は `items(id) + published_at` の複合インデックス追加判断が将来的に
  必要になる可能性があるが、本 spec の Non-Goals「追加インデックスの導入判断は design の
  領分」「DB スキーマ追加・新規マイグレーション無し」と整合するため、本タスクでは扱わない。

#### 残存課題

- なし。Task 2（Service 層）は本タスクの `StarredItemRow` を受け取って `StarredItemListResult`
  を組み立てる予定。`StarredItemRow` の構造体定義は安定しており、Task 2 で追加変更しない想定。
- 派生タスク候補: 本番運用でスター件数が極端に多くなった場合の SQL チューニング判断
  （`items` への複合インデックス追加 or マテビュー化）は将来課題として残置可。本 Issue の
  Non-Goals 範囲内。

### Task 2

Service 層 `ListStarredItems` メソッドの追加と単体テスト。

#### 採用方針

- 既存 `ListItems` から `parseItemCursor` / `toItemSummary` / `buildItemListResult` の
  3 ヘルパー関数を抽出し、両メソッドで共有する。`ListItems` 自身も新ヘルパー経由に
  書き換えて、横断 API と単一フィード API のカーソル規約・has_more / next_cursor 算出
  ロジックが恒久的に同一であることを構造的に保証する（NFR 3.1）。
- `StarredItemSummary` は `ItemSummary` を struct embed し `FeedTitle string` を追加。
  これにより既存 `ItemSummary` を変更せず、フロントエンド向けの追加フィールドを
  純粋に「拡張」として表現できる（既存 API の応答スキーマを汚さない）。
- `StarredItemListResult` は `ItemListResult` と同形（`Items` の型のみ差分）で、
  handler 層が将来 JSON 化する際に既存型と並列扱いできる構造にした。

#### 重要な判断

- **ヘルパー化の粒度**: `buildItemListResult` は ItemListResult を直接組み立てる形にした
  （StarredItemListResult 用の汎用化はせず）。理由: StarredItem 側は ItemSummary に
  FeedTitle を併記する変換が必要で、ジェネリクスや interface 化で抽象化するより、
  `ListStarredItems` 内で truncate / nextCursor 算出を直接書く方が読みやすいため。
  共有しているのは「カーソルパース」「ItemWithState→ItemSummary 変換」の 2 点に絞り、
  「limit+1 → has_more 判定 + nextCursor 算出」のロジックは StarredItemRow から
  StarredItemSummary を生成する必要があるため ListStarredItems 内に複製した
  （仕様上完全に同形のコードであり、両系統テストで挙動同一性を担保）。
- **mockItemRepoForService の拡張**: `listStarredByUserFn` フィールドを追加。Task 1 で
  upsert_test.go の `mockItemRepo` には固定戻り値 `(nil, nil)` のスタブを既に置いて
  あったが、サービス層テストでは fixture を切り替える必要があるため、サービス層
  専用 mock 側でフック関数を持つ形にした。
- **テストケース構成**: task 指示の (a)〜(e) を以下にマップし、追加でカーソル伝搬を補強:
  - (a): `TestItemService_ListStarredItems_EmptyCursor`（空カーソルで先頭ページ、
    limit+1 が repository に渡る、userID が伝搬する、FeedTitle が summary に乗る）
  - (b): `TestItemService_ListStarredItems_InvalidCursor`（パース失敗で INVALID_FILTER、
    repository が呼ばれないことも検証）
  - (c): `TestItemService_ListStarredItems_HasMoreTrue`（limit+1 件で has_more=true、
    summaries が limit 件に切り詰められる、NextCursor が末尾の RFC3339Nano になる）
  - (d): `TestItemService_ListStarredItems_HasMoreFalse`（limit 以下で has_more=false、
    NextCursor が空文字列、複数フィードにまたがる FeedTitle が保持される）
  - (e): `TestItemService_ListStarredItems_NextCursorRFC3339NanoFormat`
    （nanosecond 精度で正確に往復可能なフォーマットであることを精密検証 / NFR 3.1）
  - 追加: `TestItemService_ListStarredItems_CursorPassedToRepo`（cursorStr → time.Time
    へのパース結果が repository 層にそのまま伝わることを検証 / Req 4.5）

#### 残存課題

- なし。Task 3（Handler 層）は本タスクの `StarredItemListResult` を受け取って
  `starredItemListResult`（handler 層 JSON 応答型）に変換する予定。`ItemServiceInterface`
  への `ListStarredItems` 追加は Task 3 の責務範囲。

### Task 3

Handler 層 `ListStarredItems` ハンドラ + アダプタ + ルート登録、および単体テスト。

#### 採用方針

- `starredItemSummaryResponse` は `itemSummaryResponse` を struct embed して
  `FeedTitle string \`json:"feed_title"\`` を追加する形（design.md §要件 2.4 採用案）。
  既存 `itemSummaryResponse` を変更せず、JSON 出力時には embed 元の全フィールド +
  `feed_title` が同列に並ぶ（Go の encoding/json は anonymous embedded struct を
  inline 展開するため）。NFR 3.1（既存応答スキーマと区別不能 = 既存 API のスキーマ
  汚染なし）を構造的に担保する。
- `starredItemListResult` も `itemListResult` と同形のフィールド構成（Items / NextCursor /
  HasMore）に揃え、handler 層レイヤでも横断 API と単一フィード API の応答形状が型レベルで
  並列であることを表現。
- ルート登録は `/api/feeds` 直下の `/{id}` ブロックの**直前**に `r.Get("/starred/items", ...)`
  を置く。chi v5 のトライ木は静的セグメント `starred` を動的パラメータ `{id}` より優先
  するため登録順は問わないが、可読性のためコメント + 物理的な隣接配置で意図を明示。
- `handler` 層単体テストでは router を経由せずハンドラを直接呼ぶため、ルーティング優先順位の
  実 router 経由検証は Task 4（integration_test.go）の責務範囲とする（design.md にも明記）。

#### 重要な判断

- **Items=nil → `"items":[]` の正規化を handler 層に実装**: service 層が `Items: nil` な
  `*starredItemListResult` を返した場合でも、JSON 上 `"items": null` ではなく `"items": []` を
  返すよう handler 内で正規化。NFR 3.1（既存応答スキーマと区別不能 = 配列フィールドは常に
  配列であり null にならない）を確実に守るため。`TestItemHandler_ListStarredItems_EmptyResult_NilItems`
  で当該挙動を JSON 直接 substring マッチで検証している。
  - 既存 `ListItems` ハンドラは service 層の保証に依存して同等の正規化をしていない
    （`buildItemListResult` が常に `make([]ItemSummary, 0)` 以上のスライスを返すため）が、
    `ListStarredItems` 側は handler のテスト時に service 層 mock が `nil` を返すケースを
    扱う必要があるため、handler 層に防御的に正規化を入れた。production 経路では adapter
    層が `make([]starredItemSummaryResponse, len(result.Items))` で常に非 nil スライスを
    作るため、本正規化は no-op になる。
- **mockItemService への `listStarredItemsFn` 追加**: 既存 `mockItemService` は `ItemServiceInterface`
  を満たす実装で、本 task で `ListStarredItems` を interface に追加したため、mock 側にも
  メソッド追加が必要。既存パターン（fn フィールド + nil なら zero value 返却）に合わせた。
  これにより `router_full_test.go` / `router_unauth_ratelimit_test.go` / `integration_test.go` /
  `router_logging_test.go` / `router_metrics_test.go` の各テストで `&mockItemService{}` を
  そのまま渡している既存箇所も、interface 追加の影響を受けずコンパイル可能（実体メソッドが
  zero value を返す）。
- **テストケース構成**: task 指示の (a)〜(e) を以下にマップし、追加で NFR 3.1 補強:
  - (a) `TestItemHandler_ListStarredItems_NoUserID_ReturnsUnauthorized`（401、応答ボディに
    items を含めない）
  - (b) `TestItemHandler_ListStarredItems_Success`（200、Content-Type、items 配列、next_cursor、
    has_more、各 item の feed_title / feed_id / is_starred）
  - (c) `TestItemHandler_ListStarredItems_WithCursor`（?cursor=2026-02-27T10:00:00Z が
    service 層にそのまま伝搬）
  - (d) `TestItemHandler_ListStarredItems_InvalidCursor_ReturnsBadRequest`（service 層が
    `model.NewInvalidFilterError` → 400 + APIError code = `INVALID_FILTER`）
  - (e) `TestItemHandler_ListStarredItems_EmptyResult`（items=[] / has_more=false /
    next_cursor は omit）
  - 追加: `TestItemHandler_ListStarredItems_EmptyResult_NilItems`（service 層 mock が
    Items=nil を返しても JSON 上は items=[] であることを substring マッチで検証 / NFR 3.1）

#### 残存課題

- なし。Task 4（integration_test.go の追加）が次タスクで、router 経由の到達性
  （`/api/feeds/starred/items` が `ListStarredItems` に届くこと、`/api/feeds/{id}/items` と
  衝突しないこと）と認証クッキー付き E2E 形のシナリオを担当する。本 task で実装した
  ハンドラ・アダプタ・ルート登録は Task 4 から再利用可能な状態にしてある。

### Task 4

Handler 層の結合テスト追加（既存挙動の非干渉確認込み）。

#### 採用方針

- 既存 `createIntegrationRouter` ヘルパーを **拡張する** 方針を採用（独立ヘルパー
  `createIntegrationRouterForStarred` の新設はしない）。理由: (a) 既存テストへの影響が
  state.items / state.itemStates が空の場合は zero-length 一覧を返すだけで非破壊的、
  (b) integrationState の `lastStarred*` フィールドで router → service 層への
  伝搬を結合的にトレースできるため検証粒度を細かく取れる、(c) 別ヘルパー新設はテスト
  間の fixture 構築差を生んで保守コストが高い。
- `mockItemService.listStarredItemsFn` を inline で state-scanning な実装にし、
  RFC3339Nano → RFC3339 のフォールバックパース、user_id + is_starred フィルタ、
  published_at DESC ソート、limit+1 風の has_more 判定、feed_title 解決を
  ステートフルに行う。これにより**実 router 経由で**「user 境界の漏洩なし」
  「published_at 降順」「不正カーソルの 400 マッピング」「cursor 伝搬」を
  単一フィクスチャで横断的に検証できる。
- 401 検証は `TestIntegration_ProtectedEndpoints_RequireAuth` への
  `/api/feeds/starred/items` 追加（横断的担保）と、個別の
  `TestIntegration_ListStarredItems_Unauthorized_Returns401`（応答ボディに
  items を含めない / service 層が呼ばれない、を strict 検証）の **両方** を入れた。
- 既存 `GET /api/feeds/{id}/items` の非干渉確認は新規テスト
  `TestIntegration_ListItems_ByFeedID_StillWorksAfterStarredRouteAdded` を独立
  関数として追加（既存 `TestIntegration_FeedRegistrationFlow/step5` の検証強化案より
  責務が明確）。chi v5 のトライ木で static segment が dynamic param を優先する事実に
  **依存せず**、実 router 経由でハンドラ到達を確認している。

#### 重要な判断

- **integrationState の拡張**: トレーシング用フィールド `lastStarredUserID` /
  `lastStarredCursor` / `lastStarredLimit` / `starredCallCount` を追加した。これにより
  「ListStarredItems ハンドラに正しくディスパッチされたか」「cursor がそのまま service
  層に伝搬したか」「401 で service 層に到達しないか」を mock の record として直接
  assertion できる。これは task 指示 4 つ目「実 router 経由で `/api/feeds/starred/items`
  が `ListStarredItems` ハンドラに到達することを統合的に検証」の直接的担保。
- **不正 cursor の 400 マッピング**: handler 層単体テスト（item_handler_test.go の
  TestItemHandler_ListStarredItems_InvalidCursor_ReturnsBadRequest）では service mock が
  `model.NewInvalidFilterError` を直接返していた。integration 層では mock 内に RFC3339Nano
  → RFC3339 のフォールバックパースを忠実に再現することで、実際の cursor 文字列
  `"not-a-valid-time"` が `model.NewInvalidFilterError` 経由で `handleServiceError` の
  400 マッピングに到達する一連のフローを通せるようにした。
- **401 の応答ボディ検証**: session middleware (`internal/middleware/session.go`) の
  401 は `http.Error(w, "unauthorized", ...)` で plain text を返す実装のため、JSON
  パースを期待せず `bytes.Contains(body, []byte(\`"items"\`))` で「items 文字列を
  含まない」ことを担保した。`"items"` 完全一致部分文字列での否定なので false positive
  を避けつつ「事実として items を含まない」という要件 4.6 の strict 担保になる。
- **既存挙動の非干渉確認の網羅性**: Req 5.1（既存単一フィード API 不変）は
  `TestIntegration_ListItems_ByFeedID_StillWorksAfterStarredRouteAdded` で、Req 5.2
  （既存スター更新 API 不変）は task 指示の通り `TestIntegration_ItemStateManagement` が
  そのまま green であることで担保（追加テスト不要）、Req 5.3（既存 UI 動線不変）は
  Web 側 task 7 で別途担保される。

#### 残存課題

- なし。Task 5（Web 型定義 + useStarredItems フック）が次の独立 task として続く。
  本 task で追加した statefulness（state.items / state.itemStates / state.feeds の
  fixture 構築パターン）は Web 側 task では再利用されない（フロントエンドは fetch mock
  ベースで完結する想定）。

### Task 5

Web フロントエンド: `StarredItemSummary` / `StarredItemListResponse` 型定義と
`useStarredItems` フックの新規追加。

#### 採用方針

- `StarredItemSummary` は `ItemSummary` を `extends` で拡張し `feed_title: string` を
  追加。`ItemListResponse` と `StarredItemListResponse` は **並列型** として共存させ、
  既存の `ItemSummary` / `ItemListResponse` には一切手を加えない（NFR 3.1 を型レベルで
  担保 = 既存 API スキーマを汚染しない）。Backend 側 Task 3 で採用した
  `starredItemSummaryResponse` の Go 構造体 embed パターンと意図が並列している。
- `useStarredItems` は `useItems` の構造をほぼそのまま流用し、相違点は
  (a) queryKey が `["items", "starred"]` 固定（引数なし）、(b) URL が
  `/api/feeds/starred/items` 固定、(c) `filter` 引数がない、(d) `enabled` 制御がない
  （フックを呼ぶ条件はコンポーネント側で判定する design.md §useStarredItems 方針）の 4 点。
- queryKey の前置キーを `"items"` で共有することで、既存 `useToggleStar` の
  `onSettled` 内 `queryClient.invalidateQueries({ queryKey: ["items"] })` が prefix match
  により横断キャッシュを **自動的に invalidate** する。Req 3.2 / 3.3 / 3.4 を
  追加コードなしで構造的に充足する設計（design.md §useStarredItems 実装方針通り）。

#### 重要な判断

- **queryKey 前置キー共有による invalidate 追従**: TanStack Query の `invalidateQueries`
  は queryKey を **prefix で部分一致** するため、`["items"]` を渡すと `["items", feedId, filter]`
  も `["items", "starred"]` も両方 invalidate される。これにより `useToggleStar` 側に
  横断キャッシュ専用の invalidate 呼び出しを追加する必要がなく、Task 5 の変更で
  既存 hook ファイル (`use-item-state.ts`) を一切触らずに Req 3 系を満たせる。
- **`useToggleStar` 楽観的更新の互換性**: `useToggleStar.onMutate` は
  `getQueriesData<InfiniteData<ItemListResponse>>({ queryKey: ["items"] })` で
  横断キャッシュも拾うが、TypeScript 型は `ItemListResponse` を期待する。runtime では
  `StarredItemListResponse` も同形（`items` / `next_cursor` / `has_more` の同じ
  フィールド構成 + items 要素も `ItemSummary` の super-set である `StarredItemSummary`）
  であり、楽観的更新で `item.id === itemId ? { ...item, is_starred: isStarred } : item`
  の spread copy を行っても `feed_title` プロパティは保持される（spread が source の
  全プロパティを inherit するため）。Task 7 の `StarredItemList` 側で
  `feed_title` を表示する際にも、楽観的更新後の cache 上で `feed_title` が消えていない
  ことを期待できる。
- **テストの URL マッチング戦略**: 2 回目リクエスト URL に `cursor=2026-02-26T10:00:00Z` が
  含まれることを検証する際、`URLSearchParams.set("cursor", ":")` は `:` を `%3A` に
  encode するため、生・encoded の両方を OR 条件で許容する形にした。実装側の encoding
  挙動 (`URLSearchParams.toString()`) に依存しない、より頑健な assertion。
- **node 環境のバージョン依存**: 開発 worktree の標準 PATH には node 22.11.0
  (`.local/node/bin/node`) しか出ておらず vite/vitest の engine 要件
  (`>=22.12.0` / `>=24.0.0`) を満たさず ERR_REQUIRE_ESM で起動失敗。`/tmp/node24-bin/node`
  (v24.11.1) を PATH に追加することで build/test/lint いずれも成功。CI (GitHub Actions
  `.github/workflows/ci.yml`) は node 24 を使う設定であることが既存挙動と整合。

#### 残存課題

- なし。Task 6 (`StarredNavItem` + AppState 拡張) は本タスクと依存関係なし (P) で並列実装
  可能。Task 7 (`StarredItemList` + AppShell 統合) は本タスクの `useStarredItems` を
  消費する。Task 7 が `useToggleStar` の楽観的更新を経由した invalidate / 楽観的反映の
  end-to-end 挙動を確認することになるため、本タスクではフック単体 (`useInfiniteQuery`
  の挙動) のみ責任を持つ。

### Task 6

Web フロントエンド: `AppState` 拡張と `StarredNavItem` コンポーネントの新規追加。

#### 採用方針

- `AppState` に `selectedView: "feed" | "starred"` を導入し、`selectedFeedId: string | null`
  と並列に管理する設計（design.md §AppShell / AppState 採用案）。`selectedView` を別
  フィールド化することで、既存 `selectedFeedId === null` の意味（「未選択」）と
  「お気に入り選択中（selectedView==='starred' かつ selectedFeedId===null）」の両方を
  型レベルで区別できる。代替案の sentinel 化（`selectedFeedId: string | "starred" | null`）
  は採用しなかった（既存 `feed-list.tsx` の `feed.feed_id === selectedFeedId` 比較に
  特殊 sentinel 値が紛れ込むと bug 源になるため）。
- `SELECT_STARRED` reducer 遷移は `selectedView="starred"`, `selectedFeedId=null`,
  `expandedItemId=null`, `filter="all"` を 1 アクションで全て遷移させる。
  既存 `SELECT_FEED` も `selectedView="feed"` を併せて更新するため、Task 7 で AppShell が
  `state.selectedView` をピボットとして右ペインを切り替える際に他フィールドとの整合性を
  reducer が保証する形にできる（Component 側で派生条件を組み立てる必要なし）。
- `StarredNavItem` のスタイルは `feed-list.tsx` の `<button>` 行の class 構成を
  base/hover/active で 3 段に分解した形をそのまま流用（要件 1.2: 既存表示規約と同じ）。
  ベースクラス `flex items-center gap-2 rounded-md px-3 py-2 text-left text-sm transition-colors` +
  hover クラス `hover:bg-accent hover:text-accent-foreground` + active クラス
  `bg-accent text-accent-foreground font-medium` の 3 段は `feed-list.tsx` と完全一致。
- アイコンは `lucide-react` の `Star`（既存 `item-list.tsx` / `item-detail.tsx` がスター
  アイコンとして使っているのと同じシンボル）。サイズ `w-4 h-4` も feed-list.tsx の
  favicon と同じ視覚言語に揃え、左端のアイコン枠を `w-4 h-4` の固定幅 wrapper で囲む
  ことで、購読フィード行と縦方向のラインアップが揃う。

#### 重要な判断

- **既存テストの初期状態検証を破壊しない拡張方針**: `app-state.test.tsx` の最初の
  テスト「初期状態が正しく設定されていること」に `expect(result.current.selectedView).toBe("feed")`
  を 1 行追加し、既存 3 アサート（selectedFeedId/expandedItemId/filter）は不変のまま。
  既存テスト 9 件は全て green を維持。追加テストは「SELECT_STARRED で他フィールドリセット」
  と「SELECT_STARRED → SELECT_FEED で selectedView が feed に戻る」の 2 件。
- **StarredNavItem テストでの dispatch 取り回し**: `useAppDispatch` を子コンポーネント
  外から呼び出せないため、`StateProbe` ヘルパーを setup 内で別レンダリングして dispatch
  と state の参照を `useEffect` で外部スコープにキャプチャする方式を採用。`renderHook` で
  別 Provider が立つと StarredNavItem 側の Provider と分離されるためテストにならない。
  AppStateProvider 配下に StarredNavItem と StateProbe を兄弟として配置することで、
  dispatch 経由の状態遷移と DOM 反映を同一 Provider 内で同期検証できる。
- **アクティブクラスの完全一致確認**: spec 要件 1.4 で「既存 `feed-list.tsx` と同じ
  アクティブクラス」を明示要求されているため、テストでは `bg-accent` / `text-accent-foreground`
  / `font-medium` を個別 regex match で検証（substring 順序に依存しないため tailwind の
  クラス順変更耐性あり）。アクティブ解除側のテストは結合文字列の不一致を直接検証して
  「class が消える」ことを担保。
- **node 環境の継続課題**: Task 5 learning と同様、worktree の標準 PATH 上の node 22.11.0
  では vitest / next build が ERR_REQUIRE_ESM で起動不可。`/tmp/node24-bin/node` (v24.11.1)
  を PATH 先頭に追加して `npm ci` → `npm test -- --run` → `npm run lint` → `npm run build`
  の順で実行。node_modules が未インストール状態だったため `npm ci` も追加実行が必要だった
  （Task 5 の検証実行時には既に installed だった環境差分。本 task では fresh worktree slot
  だったため install からやり直し）。

#### 残存課題

- なし。Task 7（`StarredItemList` + `AppShell` 統合）が次タスクで、本 task で追加した
  `selectedView` / `SELECT_STARRED` / `StarredNavItem` を AppShell に組み込み、Task 5 で
  追加した `useStarredItems` を経由した右ペイン描画を担当する。本 task では AppShell への
  組み込みは行っていない（spec の `_Boundary: AppState, StarredNavItem_` に従って境界外）。

## 確認事項

なし（design.md / requirements.md と本実装に矛盾は確認されていない）。

## Requirements Coverage (Task 1 範囲)

| Requirement | テスト |
|---|---|
| 2.2（公開日時降順で全フィード横断スター記事を表示） | `TestPostgresItemRepo_ListStarredByUser/自ユーザーの複数フィードのスター記事が降順で返り_feed_title_が付与される` |
| 2.4（フィードタイトルの併記） | 同上（`FeedTitle` フィールドが "Feed A" / "Feed B" として返ることを検証） |
| 4.1（リクエストユーザー自身のスター記事のみ） | 同上 + `他ユーザーのスター記事は返らない` |
| 4.2（published_at 降順） | 同上 |
| 4.9（別ユーザーのスター記事を一切含めない） | `他ユーザーのスター記事は返らない` |
| 4.10（feed_id を含む） | `StarredItemRow` が `model.ItemWithState` を embed しているため `FeedID` も含まれる（実装上の自明な担保） |
| NFR 1.1（応答時間） | EXPLAIN ANALYZE 結果（1.352 ms / 0.290 ms、上記）で検証 |
| NFR 1.2（部分インデックスを利用可能な状態） | EXPLAIN ANALYZE で `Bitmap Index Scan on idx_item_states_user_starred` が選択されることを確認 |
| NFR 2.1（クロスユーザー漏洩なし） | `他ユーザーのスター記事は返らない` |
| 追加: cursor 境界 | `cursor 指定時に当該時刻より前の記事のみ返る` |
| 追加: 空状態 | `スター記事が 0 件のとき空スライスが返る` |
| 追加: limit 境界 | `limit が指定件数で SQL レベルに反映される` |

## テスト実行コマンド

```sh
# テスト用 PostgreSQL に接続可能な環境で
TEST_DATABASE_URL="postgres://<user>:<pass>@<host>:5432/feedman_test?sslmode=disable" \
  go test ./internal/repository/... -run TestPostgresItemRepo_ListStarredByUser

# 接続できない場合は t.Skip で自動スキップ（既存テストと同じパターン）
go test ./internal/repository/... -run TestPostgresItemRepo_ListStarredByUser
```
