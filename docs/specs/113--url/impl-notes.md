# 実装ノート（Issue #113: フィードタイトルが URL のまま表示される不具合修正）

## 変更概要

左ペインのフィード一覧でフィード名が URL のまま表示される不具合を修正した。原因は
`internal/repository/postgres_feed_repo.go` の `UpdateFetchState` の UPDATE SQL に
`title` / `site_url` カラムが含まれておらず、バックグラウンドフェッチでパースした正しい
タイトル・サイト URL が in-memory（`feed.Title` / `feed.SiteURL`）には設定されるものの
永続化されず、DB 上は初期値（URL）のまま残っていたことにある。

### 変更ファイル

- `internal/repository/postgres_feed_repo.go`
  - `UpdateFetchState` の UPDATE SQL に `title = $2` と `site_url = $3` を追加し、
    `feed.Title`（NOT NULL の通常文字列）と `nullString(feed.SiteURL)`（既存
    Create/Update メソッドのバインド方法に合わせた NullString）をバインドするよう変更。
    プレースホルダ番号は全体を繰り下げた（`fetch_status = $4` …）。
  - doc comment に、呼び出し側（Fetcher）が空タイトル時に `feed.Title` を上書きしない
    ガードを持つため、失敗・未変更パスでも既存値を破壊しない旨を追記。
- `internal/repository/postgres_feed_repo_test.go`
  - `insertTestFeedWithTitle` ヘルパーを追加（title / site_url を任意指定。初期 title=URL の
    再現用。site_url 空時は NULL 挿入）。
  - `TestPostgresFeedRepo_UpdateFetchState` を追加（DB 結合テスト）。
- `internal/worker/fetch/fetcher_test.go`
  - `TestFetcher_Fetch_PersistsTitleAndSiteURL` / `TestFetcher_Fetch_EmptyParsedTitleDoesNotOverwrite`
    を追加。

### Fetcher 側の確認結果

`internal/worker/fetch/fetcher.go` は変更不要。確認した内容:

- 200 成功パス（262-268 行）: `if parsedFeed.Title != "" { feed.Title = parsedFeed.Title }` /
  `if parsedFeed.Link != "" { feed.SiteURL = parsedFeed.Link }` のガードがあり、空値での
  上書きをしない（Requirement 2.1 / 2.2 を担保）。その後 `ApplySuccess` → `UpdateFetchState`
  を呼ぶため、修正後 SQL でタイトル・サイト URL が永続化される（Requirement 1.1 / 1.2 / 1.3）。
- 304 未変更パス（159-179 行）: パースを行わず `feed.Title` / `feed.SiteURL` は DB ロード値
  （`ListDueForFetch` / `FindByID` で読み込んだ既存値）のまま `UpdateFetchState` を呼ぶ。
  修正後 SQL は渡された値をそのまま書くため、既存値を書き戻すだけで破壊しない
  （Requirement 3.2）。
- フェッチ失敗パス（SSRF / HTTP 失敗 / パース失敗 / UPSERT 失敗 / バックオフ / 停止）:
  いずれもパースによるタイトル更新前、または失敗分岐で `feed.Title` / `feed.SiteURL` を
  触らずに `UpdateFetchState` を呼ぶ。これらも既存値の書き戻しに留まり破壊しない
  （Requirement 3.1）。

## 各 AC への対応（担保テスト）

| AC | 内容 | 担保テスト |
|---|---|---|
| 1.1 | パース済みタイトルを保存タイトルに反映 | `TestFetcher_Fetch_PersistsTitleAndSiteURL`（永続化処理へ引き渡し確認）/ `TestPostgresFeedRepo_UpdateFetchState`「永続化する」サブテスト（DB 反映） |
| 1.2 | パース済みサイト URL を保存サイト URL に反映 | 同上 |
| 1.3 | 状態更新時にタイトル・サイト URL を永続化先へ書き込む | `TestPostgresFeedRepo_UpdateFetchState`「永続化する」サブテスト |
| 1.4 | 従来のフェッチ状態項目も同一更新で反映 | `TestPostgresFeedRepo_UpdateFetchState`「永続化する」サブテスト（ETag / LastModified / FetchStatus を assert）/ 既存 `TestFetcher_Fetch_*`（fetch_status・consecutive_errors 等の回帰） |
| 2.1 | パース済みタイトルが空のとき空値で上書きしない | `TestFetcher_Fetch_EmptyParsedTitleDoesNotOverwrite` |
| 2.2 | パース済みサイト URL が空のとき空値で上書きしない | `TestFetcher_Fetch_EmptyParsedTitleDoesNotOverwrite` |
| 2.3 | 初期値（URL）をパース済みタイトルへ置き換える | `TestFetcher_Fetch_PersistsTitleAndSiteURL`（初期 Title=URL → 置換）/ `TestPostgresFeedRepo_UpdateFetchState`「永続化する」（初期 title=URL の前提を assert 後に置換確認） |
| 3.1 | 失敗時に既存タイトル・サイト URL を破壊しない | `TestPostgresFeedRepo_UpdateFetchState`「保持したまま状態のみ更新」サブテスト（バックオフ相当の状態更新で既存値維持） |
| 3.2 | 未変更（304）時に既存タイトル・サイト URL を維持 | 同上（304 パスも fetcher が title/site_url を触らず既存値で `UpdateFetchState` を呼ぶ構造のため、repo 層の「既存値を破壊しない」テストで担保。fetcher.go の 304 分岐は変更なし） |
| 4.1 | 永続化後に購読一覧取得でパース済みタイトルを返す | スコープ外の API 変更なし。永続化された title を既存 API がそのまま返すため、`TestPostgresFeedRepo_UpdateFetchState`（DB 反映）+ 既存 subscription API テストで担保 |
| 4.2 | 一覧表示で保存サイトタイトルを表示 | Out of Scope（左ペイン UI 改修なし。保存タイトルが正しくなれば既存表示で解消される前提。requirements の Out of Scope 記載どおり） |
| NFR 1.1 | フェッチ各経路の従来状態項目の更新結果を変えない | 既存 `internal/worker/fetch/fetcher_test.go` 群が全 green（回帰なし） |
| NFR 1.2 | タイトル以外のレスポンス項目を変えない | API スキーマ・既存ハンドラ未変更。既存 handler テスト全 green |
| NFR 2.1 | 正常系・空入力・異常系を再現可能な自動テストで検証 | 上記 3 テストでカバー |

## テスト方針・Red→Green

- リポジトリ層 DB 結合テスト（`TestPostgresFeedRepo_UpdateFetchState`）が永続化バグの
  正本となる検証。既存 DB テスト機構（`TEST_DATABASE_URL` 利用、未接続時 `t.Skip`、
  マイグレーション適用）に準拠して追加した。
- 既存 `TestFetcher_Fetch_UpdatesFeedTitle` は mock repo で in-memory の `feed.Title` のみを
  検証しており永続化バグを検出できないため、`TestFetcher_Fetch_PersistsTitleAndSiteURL` で
  「`UpdateFetchState` に正しい値が引き渡される」観点（呼び出し境界）を補った。
- `go test ./...` は全 green。DB 結合テストはローカルに PostgreSQL（`localhost:5432`）が
  起動していないため `t.Skip` でスキップされる（CI / DB 接続環境では実行される）。
- Red→Green について: 修正前の SQL（`title` / `site_url` 列なし）に対しては、DB 接続のある
  環境で `TestPostgresFeedRepo_UpdateFetchState`「永続化する」サブテストの
  `reloaded.Title` / `reloaded.SiteURL` の assert が失敗する（DB は初期値のまま）。本環境は
  DB 未接続のためスキップ状態となり Red を直接観測できていない点を明記する。fetcher 層の
  2 テストは fetcher.go の既存ガード（修正前から存在）を検証するもので、修正前後で green。

## 確認事項

- なし（根本原因・修正方針が明確で、要件 / Out of Scope と矛盾なし）。
  - Requirement 4.2（左ペイン表示）は requirements の Out of Scope に「左ペイン UI の
    レイアウト・表示ロジックの改修」が明記されており、保存タイトルの修正で既存表示が
    解消される前提のため UI 変更は行っていない。
  - 既存保存済みの誤ったタイトル（URL のまま）の一括バックフィルも Out of Scope のため
    実施していない（次回フェッチ成功時に自然に更新される）。

STATUS: complete
