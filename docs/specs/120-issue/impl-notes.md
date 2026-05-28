# Implementation Notes

本ファイルは Issue #120「記事の検索機能」の per-task 実装での learning と残存課題を
記録する。各 task ごとに `### Task <id>` 見出しで learning を追記し、先行 task の
learning は改変しない。

## Implementation Notes

### Task 1

- 採用方針: design.md の Physical Data Model / Migration Strategy 節の指定どおり、`pg_trgm`
  拡張の `CREATE EXTENSION IF NOT EXISTS` と `items` テーブル `title` / `content` への
  GIN（`gin_trgm_ops`）インデックスを追加する SQL マイグレーション 2 ファイル
  （`20260528120000_add_item_search_indexes.up.sql` / `.down.sql`）を新規追加した。
- 重要な判断:
  - **down.sql で `DROP EXTENSION pg_trgm` を行わない**。design.md「Physical Data Model」の
    「拡張は他用途で使用されうるため DROP しない方針」決定（tasks.md Task 1 の指示と整合）に
    従い、down はインデックス 2 本の `DROP INDEX IF EXISTS` のみとする。`pg_trgm` は今後他の
    検索機能や類似度比較でも利用され得るため、down で巻き戻すと他機能の indexes が壊れる
    リスクを避ける。
  - **`content` GIN は `WHERE content IS NOT NULL` の partial index**。`items.content` は
    `TEXT NULL` 許容（`initial_schema.up.sql:67`）であり、NULL 行を index から除外することで
    index サイズと更新コストを抑える。tasks.md Task 1 の SQL 文と完全一致。
  - **down の DROP 順は up の CREATE 逆順**（先に `idx_items_content_trgm` → 後に
    `idx_items_title_trgm`）。両 index は独立で依存関係はないが、慣習として逆順を採用。
  - 既存 migration（`20260527080000_add_item_states_timestamps.*.sql`）と同じく、ファイル先頭に
    日本語 1 行の説明コメント、各 SQL 文を改行で区切るスタイルに揃えた。
- 残存課題（次 task に影響する事項）:
  - 本番運用時の `CREATE INDEX CONCURRENTLY` 化（design.md「Migration Strategy」/
    `golang-migrate` の transaction wrap 制約）は別 Issue 扱い。現スケールでは通常
    `CREATE INDEX` で許容範囲と design 判断済み。Task 3.3 以降の DB 結合テストでは
    `TEST_DATABASE_URL` 経由で実 PostgreSQL に migration が適用される前提で本 index が
    検索 SQL から利用される。
  - 本 task は SQL のみで Go コード変更なし。検索本体（service / repository / handler / web）は
    Task 2〜8 で順次実装される。

### Task 2

- 採用方針: 検索ドメイン用の DB レベル射影モデル `ItemSearchHit` と、検索固有の `APIError`
  生成関数 2 種（`NewInvalidSearchQueryError` / `NewFeedNotSubscribedError`）+ HTTP ステータス
  マッピングを追加した。
- 重要な判断:
  - **`ItemSearchHit` は埋め込みではなく個別フィールド構成**にした。tasks.md は「埋め込みまたは
    個別フィールドで再現」のいずれかを許容しているが、`ItemSummary` は `internal/item`
    パッケージに存在し、`internal/model` から `internal/item` へ依存させると現状の依存方向
    （`item → model`）が逆転して循環依存になる。`Feed` 構造体に既に存在する
    `FaviconData []byte` / `FaviconMime string` のフィールド名・型と完全一致させ、既存
    `ItemWithState` と同じくリポジトリ層が直接生成して返す DB レベル射影として扱う。
  - **`Summary` フィールドを含めた**: design.md の `ItemSearchSummary`（Service 層型）が
    `Summary string` を持ち、design.md `## File Structure Plan` の Modified Files でも
    「`ItemSummary` 相当」と書かれている。`ItemSummary` には `Summary` フィールドが存在
    するため `ItemSearchHit` にも追加した（後続 Task 3.2 で SELECT 列に `i.summary` を含める
    前提と整合）。
  - **新規エラーコードの Category 命名**: design.md の Error Handling 節と tasks.md の指示
    どおり、`INVALID_SEARCH_QUERY` を `validation`、`FEED_NOT_SUBSCRIBED` を `authorization`
    に設定した。既存の `auth` Category（USER_NOT_FOUND）は「未認証 / セッション無効」を
    指す用語として運用されているため、認可（権限）系の新区分として `authorization` を採用
    して既存コードと混同しない区別を付けた。
  - **`mapAPIErrorToHTTPStatus` の switch 文末**: 既存の `case` の意味的近隣（バリデーション
    系は `ErrCodeInvalidFilter` / `ErrCodeInvalidFetchInterval` と合流して `400`、認可系は
    新規 `case` を追加して `403 Forbidden`）に挿入した。デフォルト 500 へのフォールバック
    挙動は無変更。
  - **テスト追加方針**: 新規 `internal/model/errors_test.go` で 2 生成関数の Code/Category/
    Message を直接検証し、既存 `feed_handler_test.go` の `TestMapAPIErrorToHTTPStatus_KnownCodes`
    テーブルに 2 ケースを追記した。これにより handler 層の HTTP ステータスマッピングと
    model 層のエラー構造体生成の両方をユニットテストでカバーする。実装-→テスト-→確認の
    順で `go test ./internal/model/... ./internal/handler/...` が green、続いて
    `go test ./...` も全 package 通過を確認済み。
- 残存課題（次 task に影響する事項）:
  - Task 3.1 / 3.2 の Repository 層実装で、本 task で追加した `ItemSearchHit` を SELECT
    結果として組み立てる scanner と、SQL の SELECT 列（`i.id, i.feed_id, i.title, i.link,
    i.summary, i.published_at, i.is_date_estimated, i.hatebu_count, f.title AS feed_title,
    f.favicon_data, f.favicon_mime, COALESCE(st.is_read), COALESCE(st.is_starred)`）を
    `ItemSearchHit` の各フィールドへマッピングする実装が必要。
  - Task 4 のサービス層（`itemsearch.SearchService`）で、`ItemSearchHit` をアプリケーション
    向け表現 `ItemSearchSummary`（design.md 行 295-309 / `FaviconURL *string` 形式）へ変換
    する責務が発生する。本 task の `FaviconData []byte` / `FaviconMime string` を、既存
    `subscription.Service.ListSubscriptions` と同じく `data:<mime>;base64,...` 形式の
    data URL に整形してから API レスポンスへ渡す変換を、Task 5.3 の `ItemSearchServiceAdapter`
    が担うのが既存パターンと整合する。
  - Task 4 / 5 / 5.4 が、本 task で追加した `model.NewInvalidSearchQueryError` /
    `model.NewFeedNotSubscribedError` をサービス層・ハンドラ層で使用する前提で実装される。

### Task 3

- 採用方針: `ItemSearchRepository` インターフェースを `interfaces.go` に新規定義し、
  `PostgresItemRepo.SearchByUserAndKeyword` で design.md 参照 SQL（`items JOIN
  subscriptions JOIN feeds LEFT JOIN item_states` + `$3::uuid IS NULL` ガード +
  タプル比較ページング）どおりに 1 クエリで横断 / フィード内検索を表現した。
  Skip ガード付き DB 結合テスト 10 ケースで AC 2.2/2.3/2.4/2.5/2.6/3.1/3.2/3.4/4.1/5.3
  を網羅する。
- 重要な判断:
  - **null 引数は動的 WHERE 文字列再構築ではなく SQL ガード（`$3::uuid IS NULL`,
    `$4::timestamptz IS NULL`）で処理する**。`ListByFeed` は WHERE を文字列連結で
    動的に組み立てる方式（cursor 有無で `argIndex` を変える）だが、本 method は
    design.md がガード式での表現を明示しており、PostgreSQL のプランナが `IS NULL`
    ガードを定数畳み込みするため横断検索時の index 利用に追加コストが生じない
    という設計判断に従った。Go 側は `interface{}` 変数 1 つを `nil` / 値で
    分岐するだけで済み、SQL 本体は静的文字列にできる。
  - **scanner は inline 実装で `scanItem` を再利用しない**。本 method の SELECT 列
    （`i.id, i.feed_id, i.title, i.link, i.summary, i.published_at,
    i.is_date_estimated, i.hatebu_count, f.title, f.favicon_data, f.favicon_mime,
    is_read, is_starred`）は `itemSelectColumns`（16 列 / Item 全体）とは
    異なる射影（13 列 / `ItemSearchHit`）であり、`scanItem` を流用しても恩恵が
    なく、むしろ別構造体（`Item` vs `ItemSearchHit`）への詰め替えコードが増える。
    検索専用の射影モデルとして inline scanner を選択した（Task 2 の決定と整合）。
  - **`PublishedAt` の NULL マッピング**は `time.Time{}` ゼロ値に倒した
    （`model.ItemSearchHit.PublishedAt` が非ポインタ `time.Time` で定義されている
    Task 2 の決定に従う）。ORDER BY 側は `NULLS LAST` で NULL 行が末尾に来る
    ため、ゼロ値が他の値と混ざってもページネーション順序の不変性は保たれる。
  - **クロスユーザー隔離は SQL レベルで強制する**。design.md 参照 SQL の
    `JOIN subscriptions s ON s.feed_id = i.feed_id AND s.user_id = $1` により、
    呼び出し側で追加チェックなしに「未購読フィード」「他ユーザーの購読」を
    結果集合から除外できる（Req 3.1 / 3.2 / 3.4 を SQL の JOIN 述語のみで担保）。
  - **テストの `cleanupSQL` を再利用するが helper 名は collision 回避のため新規**。
    既存 `postgres_subscription_repo_db_test.go` の `setupSubscriptionTestDB` /
    `insertTestUserForSub` を直接 export せず、本ファイル専用の
    `setupItemSearchTestDB` / `insertTestUserForSearch` 等として複製した。
    将来両ファイルで共通化したい場合は別 task で `_test_helpers.go` 等の
    名前で集約する余地を残す。
  - **不変条件テストは `item_states` と `items` の両方をスナップショット**する
    (Req 5.3)。検索前後で `is_read` / `is_starred` / `updated_at` / `hatebu_count`
    すべてが等価であることを assert する。`updated_at` の `Equal` 比較は
    timezone を考慮するため `time.Time.Equal` を使用する。
- 残存課題（次 task に影響する事項）:
  - **cursor 形式 `<RFC3339Nano>|<uuid>` のパースは Service 層の責務**。Task 4 の
    `itemsearch.SearchService` で `cursorStr` を `(time.Time, string)` に分解し、
    本 method の `cursorPublishedAt` / `cursorID` 引数に渡す形になる。形式不正は
    `model.NewInvalidSearchQueryError` で `400` を返す（Task 2 で追加済み）。
  - **`feed_id` の購読確認も Service 層の責務**。本 repository method は
    `feed_id` の有無に関わらず SQL の JOIN で隔離するため、未購読 feed_id を
    渡されても空配列が返るだけで `403 FEED_NOT_SUBSCRIBED` にはならない。Task 4 で
    `SubscriptionRepository.Exists`（または `FindByUserAndFeed` の nil 判定）を
    呼び出してから本 method を呼ぶ実装が必要。
  - **`ItemSearchHit.PublishedAt` が非ポインタ `time.Time`** であるため、Service 層
    で `ItemSearchSummary` へ変換する際は、ゼロ値判定（`IsZero()`）が必要なら
    そのまま行う。design.md 行 295-309 の `ItemSearchSummary` 仕様で
    `PublishedAt` をポインタ型として扱うか非ポインタとするかは Task 4 の判断
    範囲。本 repository 層は zero `time.Time{}` を返すことだけ保証する。
  - **Task 4 / 5 で `HasMore` 判定**には `limit+1` を本 method に渡し、
    `len(hits) > limit` で判定して `hits = hits[:limit]` に切り詰める設計
    （design.md と tasks.md Task 4 で明示）。本 method 自体は LIMIT を
    そのまま適用するだけで HasMore ロジックを持たない。

### Task 4

- 採用方針: `internal/itemsearch.SearchService` を `service.go` / `service_test.go`
  の 2 ファイルで新規実装。design.md の `Search(ctx, userID, rawQuery, feedID, cursorStr, limit)`
  シグネチャに従い、(1) クエリ正規化（前後空白 trim + 空クエリ判定）、(2) LIKE メタ文字
  エスケープ、(3) feed_id 指定時の購読確認、(4) cursor `<RFC3339Nano>|<uuid>` 解析、
  (5) limit クランプ、(6) `limit+1` 取得 → HasMore 判定 → NextCursor 生成、(7)
  `ItemSearchHit` → `ItemSearchSummary` 変換の各責務を 1 メソッドで完結させる。
  テストは 30+ ケースのテーブル駆動で AC 1.5 / 2.4 / 2.6 / 3.5 / 4.1 / 4.4 を網羅する。
- 重要な判断:
  - **`SubscriptionRepository.Exists` を新規追加せず、既存 `FindByUserAndFeed` を再利用する**。
    tasks.md は「未存在なら追加」も許容するが、`FindByUserAndFeed` は既に
    `(*model.Subscription, error)` を返し nil で未購読を判定可能なため、interface 拡張は
    不要と判断した。新規メソッド追加は本 task のスコープを膨らませ、他コンシューマへの
    影響もあるため、最小侵襲を優先する。Service 層が `sub == nil → NewFeedNotSubscribedError`
    で判定する形にし、`subRepo.FindByUserAndFeed` のエラーは
    `fmt.Errorf("check subscription: %w", err)` で wrap する。
  - **`ItemSearchSummary` に `FaviconData []byte` / `FaviconMime string` を追加した**
    （design.md 行 295-309 の field 一覧は `FaviconURL *string` のみだが拡張）。理由は
    impl-notes Task 2 で「favicon の data URL 化は Task 5.3 の `ItemSearchServiceAdapter`
    が担う」と責務分離が確定しているため、Service 層が data URL を生成すると Adapter 層
    と二重責務になる。Service 層は `ItemSearchHit.FaviconData` / `FaviconMime` を
    そのまま pass-through し、`FaviconURL` は常に nil としておく形を採用した。
    Adapter 層（Task 5.3）が `data:<mime>;base64,...` 形式に整形して `FaviconURL` を
    populate する責務を持つ。design.md の field 一覧との差分はあるが、impl-notes
    Task 2 の責務分離判断を優先する。
  - **NextCursor を末尾項目の `PublishedAt` がゼロ値の場合は空文字とする**。
    `repository` 層は NULL `published_at` をゼロ値（`time.Time{}`）にマッピングする
    （impl-notes Task 3）。ゼロ値で cursor を組み立てると `(published_at, id) <
    (cursor)` のタプル比較が壊れて並び順が不安定になるため、安全側に倒して NextCursor
    を発行しない。HasMore は実体ベースで保持し、UI 側で「次ページあり」だけは表示できる。
  - **limit クランプを Service 層でも実施**（0 以下 → 50、200 超 → 200）。
    design.md の handler 層仕様で同じクランプを行う前提だが、handler を経由しない
    直接呼び出しや handler 側のバグへの防御として Service 層でも適用する。クランプ後の
    `effectiveLimit` に対して `+1` した値を repository に渡し、`len(hits) > effectiveLimit`
    で HasMore を判定する。
  - **cursor parse は Service 層の責務**（impl-notes Task 3 既述）。
    `strings.SplitN(cursorStr, "|", 2)` で分割し、length != 2 / parse 失敗 / id 空 /
    `|` が複数 のいずれも `NewInvalidSearchQueryError` で 400 に倒す。UUID 自体の
    厳密 parse は repository 層の `$5::uuid` cast に任せ、Service 層は「空でない文字列」
    程度の sanity check に留めた（過剰な依存追加を避ける）。
  - **`escapeLikePattern` の順序**: `\` → `%` → `_` の順で `strings.ReplaceAll` を
    適用する（順序逆転すると `%` のエスケープに使った `\` 自体が再エスケープされて
    `\\%` のように二重に化けるため）。PostgreSQL ILIKE の標準 escape 文字は `\` で、
    本実装の出力（例: `50\%off`）はそのまま LIKE / ILIKE の述語に渡せる。
  - **テストモックの設計**: `mockItemSearchRepo` は呼び出し引数を
    `recordedSearchCall` スライスに記録する callable assertion パターンを採用。
    `mockSubRepo` は `repository.SubscriptionRepository` interface 全メソッドを実装する
    必要があるが、本テストで触れない他メソッドは `panic` で fail-fast にした
    （誤って呼ばれたら即座にテスト失敗）。`mockSubRepo` は
    `var _ repository.SubscriptionRepository = (*mockSubRepo)(nil)` で compile-time
    check を入れている。
- 残存課題（次 task に影響する事項）:
  - **Task 5.3 の `ItemSearchServiceAdapter` 実装で、`ItemSearchSummary.FaviconData`
    /  `FaviconMime` を data URL に整形する責務**が確定。`subscription.Service.
    ListSubscriptions` と同じ `data:<mime>;base64,...` 形式のロジックを再利用する
    形（既存パターンとの整合）。Adapter 側で `FaviconURL` を populate し、Service 層
    では `FaviconURL=nil` のままにする。
  - **Task 5.1 の handler 実装で、`limit` クエリパラメータの解析・クランプ**は
    本 Service 層でも防御的に行うが、handler 側も同じクランプ（design.md「API
    Contract」節の `defaultItemsPerPage = 50` / 上限 200）を実装する必要がある。
    handler が `?limit=0` のような不正値を渡しても Service 層が defaultSearchLimit
    に倒して安全動作するが、handler 側で明示的に解析するのが UX 上望ましい。
  - **Task 5.4 の handler テストで、Service モックを差し替える**実装が必要。
    本 task では `SearchService` を struct で公開しているため、handler 側で
    `ItemSearchServiceInterface`（design.md 行 70-79 / tasks.md Task 5.1）を定義し、
    Adapter 経由で interface を満たす設計が想定される（既存
    `ItemServiceInterface` / `ItemServiceAdapter` パターンと整合）。
  - **Task 5.5 の `app.go` wiring で、`itemsearch.NewSearchService(itemRepo, subRepo)`
    を呼び出す**形になる。`itemRepo` は `repository.ItemSearchRepository`（Task 3.1
    で追加済み）、`subRepo` は既存 `repository.SubscriptionRepository`（変更不要）。

### Task 5

- 採用方針: `GET /api/items/search` のハンドラ層 5 ファイル
  （`item_search_handler.go` / `item_search_handler_test.go` / `router.go` /
  `service_adapter.go` / `app.go`）を子タスク 5.1〜5.5 で順次実装し、
  既存 `item_handler.go` の `UserIDFromContext` パターン・`subscription_handler.go` の
  `data:<mime>;base64,...` data URL 化パターン・`service_adapter.go` の
  Adapter + compile-time check パターンを全て踏襲した。
- 重要な判断:
  - **`ItemSearchServiceInterface` の戻り値型は `*itemSearchResponse`**（HTTP レスポンス型）
    を採用した。tasks.md Task 5.1 の指示に従い、Service 層のドメイン型（`*itemsearch.SearchResult`）
    を返すのではなく Adapter 層で API レスポンス型へ変換する。これにより favicon の
    data URL 化責務（impl-notes Task 2 / Task 4 の決定）を Adapter 層に閉じ込め、
    handler 層は HTTP 境界の責務（パース・認証・ログ）のみに集中できる。既存
    `ItemServiceInterface` も同じパターン（`*itemListResult` を返す）で実装されており、
    既存設計と整合する。
  - **UUID 形式バリデーションは `github.com/google/uuid` の `uuid.Parse` を使用**。
    既存コードベースで `uuid.Parse`/`uuid.MustParse` の利用例は無かったが、`go.mod` で
    既に `github.com/google/uuid v1.6.0` が direct require されているため新規依存追加は
    不要。なお `uuid.Parse` は dashes 無しの 32 hex 形式（`urn:uuid:` を除いた中身）も
    妥当として受け付ける挙動があるため、handler テストの「UUID 不正」ケースには
    `not-a-uuid` / `1234` / `zzzzzzzz-...` のような明確に妥当でない値のみを使用した
    （実環境でも DB の `$N::uuid` cast が最終的なバリデーションを担うため、handler 層は
    library に任せる方針で過剰検証しない）。
  - **`/api/items/search` を `/api/items/{id}` より前に登録**。chi v5 は static segment
    （`/search`）を `{id}` パターンよりも優先するため明示的な順序付けがなくても動作するが、
    design.md と tasks.md の指示通り「保険的に明示順序」を採用した。テストでも
    `withChiURLParam(id, "search")` のような誤発火パターンが起きないことを実装で保証する。
  - **`limit` クエリのバリデーションを handler 層でも実施**（0 以下 / 非数値で 400）。
    Service 層の `clampLimit` が 0 以下を `defaultSearchLimit` に倒すため、handler 層で
    パースエラーを 400 に変換しなくても動作はするが、ユーザーに「明らかな入力ミス」を
    silent fallback で隠す UX は望ましくないため明示エラーに倒した。`limit > maxSearchLimit`
    の場合は handler 側で `maxSearchLimit` にクランプ（design.md「API Contract」節と
    既存 `ListItems` の慣習に整合）。
  - **空クエリ時の JSON レスポンス**: Service 層は `Items: nil` の `*SearchResult` を返すが、
    JSON エンコード時に `nil` は `null` になってしまうため、handler 層で `result.Items == nil`
    のときに `[]itemSearchHitResponse{}` を代入してから encode する。これで JSON は
    `"items":[]` を返し、UI 側の「空配列分岐」が機械的に動作する（Req 1.5 / Req 4.3 の
    UX 一貫性）。
  - **`mockItemSearchService` 設計**: `internal/itemsearch/service_test.go` の
    `mockItemSearchRepo` と同じ「呼び出し引数を `recordedSearchCall` スライスに記録」
    パターンを採用。`searchFn` 未指定時は `&itemSearchResponse{Items: []itemSearchHitResponse{}}`
    を返すデフォルト挙動とし、各テストで必要に応じて挙動を差し替える。これにより
    「service が呼ばれない」アサート（401 / 400 / feed_id 不正 / limit 不正）が
    `svc.callCount == 0` で機械的にチェックできる。
  - **`go test ./...` 全 21 パッケージ green** で確認済み。`go build ./...` / `go vet ./...`
    も clean。DB 結合テスト（Task 3.3）は `TEST_DATABASE_URL` 未設定で skip された
    （ローカル環境に CI 用 Postgres が無いため）が、本 Task は mock service ベースで
    DB 不要なため影響なし。
- 残存課題（次 task に影響する事項）:
  - **Task 6〜8 は Web フロントエンド実装**（`web/src/contexts/app-state.tsx` /
    `web/src/components/header-search-bar.tsx` / `feed-search-bar.tsx` /
    `search-results.tsx` / `app-shell.tsx` / `item-list.tsx` /
    `web/src/hooks/use-item-search.ts`）。本 Task で確定した API レスポンス形状
    （`items[].feed_title` / `items[].favicon_url`（data URL 形式 `*string` /
    `omitempty`）/ `next_cursor` / `has_more`）と一致する型を `web/src/types/item.ts` に
    定義する必要がある（Task 7.1）。
  - 本 Task のクエリパラメータ仕様（`q` 必須に近い / `feed_id` 任意 / `cursor` 任意 /
    `limit` 任意・既定 50・上限 200）に対し、`use-item-search.ts`（Task 7.2）が
    `useInfiniteQuery` で `getNextPageParam: (lastPage) => lastPage.has_more ? lastPage.next_cursor : undefined`
    を実装する前提（design.md ですでに記述）。`next_cursor` が空文字でも `has_more=true` の
    ケース（末尾項目の `PublishedAt` がゼロ値）がありうるため、UI 側で空文字
    `next_cursor` を「次ページなし」として扱うガードが必要（Service 層の判断と整合）。
  - 本 Task の構造化ログ（`slog.Info("item search request", ...)`）は NFR 3.1 を満たすが、
    クエリ本文は PII / ログ汚染回避のため出していない（長さのみ）。運用観察時に「特定
    キーワードがヒットしないユーザー」のトラブルシュートを行う場合、別経路（DB 側の
    `pg_stat_statements` 等）から `pattern` を引く必要がある点を覚えておく。
