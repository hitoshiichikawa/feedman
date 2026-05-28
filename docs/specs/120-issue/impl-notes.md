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
