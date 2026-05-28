# 実装ノート: Issue #100 GET /api/subscriptions favicon_mime NULL 500

## 不具合の概要

フィード登録は成功し DB に購読も作成されるのに、購読一覧（左ペイン）が表示されない。
原因は `GET /api/subscriptions` が呼ぶ `ListByUserIDWithFeedInfo`
（`internal/repository/postgres_subscription_repo.go`）の SELECT 句で `f.favicon_mime` が
COALESCE されておらず、Scan 先 `SubscriptionWithFeedInfo.FaviconMime`（非 NULL `string`、
`internal/repository/interfaces.go:191`）へ NULL を読み取ろうとして
`sql: Scan error on column index 9, name "favicon_mime": converting NULL to string is unsupported`
で Scan に失敗し、一覧 API 全体が 500 を返していた。favicon を持たない（= favicon 未取得の）
フィードを 1 件でも購読していると一覧全体が表示不能になる。

## 採用した修正案とその理由

- **最小・既存パターン準拠の修正**: 同クエリ内に既存する `COALESCE(f.error_message, '')` と
  同様に、`f.favicon_mime` を `COALESCE(f.favicon_mime, '')` に変更した（1 箇所のみ）。
  - NULL のとき空文字で返るため Scan が成功し、200 で一覧が返る（Req 1.1 / 1.2 / 1.3 / 3.1 / 3.2）。
  - JSON レスポンス構造（フィールド名・型・階層）は変えず、favicon ありフィードの mime 値は
    従来どおり実際の値を返す（Req 2.2 / Req 4.1 / Req 4.2）。
  - `favicon_data`（`BYTEA` → `[]byte`）は NULL でも Scan 可能なため変更不要（要件 Out of Scope と整合）。
- 修正ファイル: `internal/repository/postgres_subscription_repo.go`（187 行付近の SELECT 句のみ）。

## テスト方針と CI での skip 挙動

- このリポジトリの `internal/repository/*_test.go` は純粋ユニットテストのみで、go.mod に
  sqlmock / dockertest 等の DB モック依存は無い。CI（`.github/workflows/ci.yml`）は PostgreSQL を
  起動しない。そのため DB 結合テストは `internal/database/migrate_test.go` の慣習に厳密に従い、
  環境変数 `TEST_DATABASE_URL`（未設定時は docker-compose 想定のデフォルト URL）を読み、
  `db.Ping()` 失敗時に `t.Skip(...)` する形にした。
  - 新規依存（sqlmock 等）は go.mod に**追加していない**（既存慣習＝実 DB + Skip ガードで統一）。
  - 新規テストファイル: `internal/repository/postgres_subscription_repo_db_test.go`。
    setup ヘルパ（クリーンアップ → `database.RunMigrations`）／ユーザー・フィード・購読挿入
    ヘルパは migrate_test.go のパターンを踏襲。
- **CI での挙動**: CI には DB が無いため `db.Ping()` が失敗し新規結合テストは `SKIP` される。
  既存テストは壊れない（`go test ./...` 全 package green を確認済み）。
- **Red → Green 検証**: ローカルで使い捨ての PostgreSQL 16 コンテナを起動し
  `TEST_DATABASE_URL` 経由で検証した。
  - 修正前（COALESCE 無し）: NULL favicon_mime ケースが
    `converting NULL to string is unsupported` で FAIL（Red を確認）。
  - 修正後: 全サブテスト PASS（Green を確認）。検証後コンテナは削除済み。

## 受入基準とテストの対応

| Req ID | 内容 | 担保テスト |
|---|---|---|
| 1.1 | favicon mime 未保持フィードを含む一覧で 200 を返す | `TestListByUserIDWithFeedInfo_FaviconMimeNull/favicon_mimeがNULL...`（エラー無しを assert） |
| 1.2 | 全フィードが favicon mime 保持時に 200 を返す | `.../faviconあり.なし混在...`（favicon あり側の正常返却を assert） |
| 1.3 | favicon mime 未保持フィードの mime を空文字で返す | `.../favicon_mimeがNULL...`（`FaviconMime == ""` を assert） |
| 1.4 | 購読 0 件のとき空の一覧を返す | `.../購読が1件もないとき空の一覧を返す` |
| 2.1 | favicon あり/なし混在で全件返却 | `.../faviconあり.なし混在...`（`len(results) == 2` を assert） |
| 2.2 | favicon ありは実際の mime 値で返す | `.../faviconあり.なし混在...`（`FaviconMime == "image/png"` を assert） |
| 3.1 | favicon mime NULL でも 500 にせず取得完了 | `.../favicon_mimeがNULL...`（エラー無しを assert） |
| 3.2 | favicon なしフィード購読直後の一覧に当該フィードを含む | `.../favicon_mimeがNULL...` / `.../faviconあり.なし混在...`（該当フィードの返却を assert） |
| 4.1 | レスポンス構造（フィールド名・型・階層）を維持 | 修正は SELECT 句の COALESCE のみで Scan 先構造体・JSON は不変（コード差分で担保）。混在テストで既存フィールドの値も検証 |
| 4.2 | favicon あり時の mime 値を修正前と同一で返す | `.../faviconあり.なし混在...`（実際の mime 値を assert） |
| NFR 1.1 / 1.2 | テスト用 PostgreSQL を介した結合テストで検証可能 | 本ファイルの結合テスト（`TEST_DATABASE_URL` + Skip ガード）で担保 |

## 確認事項（レビュワー判断ポイント）

- 本リポジトリは Architect 不在の design-less impl であり、`design.md` / `tasks.md` は存在しない。
  そのため tasks.md 進捗追跡（`IMPL_RESUME_PROGRESS_TRACKING`）は適用外。
- `gofmt -l .` を全体に掛けると本変更と無関係な既存ファイル群が複数列挙されるが、これは
  ブランチ作成時点の worktree 状態に起因する既存の整形差分であり、本 Issue の変更ファイル
  （`internal/repository/postgres_subscription_repo.go` /
  `internal/repository/postgres_subscription_repo_db_test.go`）は `gofmt -l` で差分なし（クリーン）。
  本 Issue のスコープ外のため整形には手を入れていない。
- CI は DB を起動しないため新規結合テストは CI 上では常に SKIP される。回帰の常時自動検証を
  望む場合は CI への PostgreSQL サービス追加が必要だが、それは本 Issue のスコープ外（別 Issue 候補）。

## 検証結果サマリ（ローカル）

- `gofmt -l <変更2ファイル>`: 差分なし
- `go vet ./...`: pass（exit 0）
- `go build ./...`: pass
- `go test ./...`（DB 無し / CI 同等）: 全 package ok（新規結合テストは SKIP、既存テスト無破壊）
- `go test ./...`（`TEST_DATABASE_URL` 指定 / DB あり）: 全 package ok（新規結合テスト含め PASS）

STATUS: complete
