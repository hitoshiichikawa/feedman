# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-05-26T00:00:00Z -->

## Reviewed Scope

- Branch: claude/issue-100-impl-get-api-subscriptions-favicon-mime-null
- HEAD commit: 3c9e3bd33952c7346e076c72dc14dadbcb1f0603
- Compared to: develop..HEAD

差分構成（`git diff --stat develop..HEAD`）:

- `internal/repository/postgres_subscription_repo.go`（実装修正 1 行）
- `internal/repository/postgres_subscription_repo_db_test.go`（新規回帰テスト 200 行）
- `docs/specs/100-.../requirements.md` / `impl-notes.md`（spec 成果物）

本 Issue は design-less impl（`design.md` / `tasks.md` は不在）であり、`_Boundary:_`
アノテーションは存在しない。Feature Flag Protocol は CLAUDE.md で `opt-out` 宣言のため
flag 観点は適用しない（通常の 3 カテゴリ判定のみ）。

## Verified Requirements

- 1.1 — `TestListByUserIDWithFeedInfo_FaviconMimeNull/favicon_mimeがNULLのフィードのみ...`
  でエラー無し（HTTP 200 相当）を assert。実装は SELECT 句の `COALESCE(f.favicon_mime, '')`
  により NULL Scan 失敗を解消（postgres_subscription_repo.go:187）
- 1.2 — `.../faviconあり.なし混在...` で favicon あり側の正常返却を assert
- 1.3 — `.../favicon_mimeがNULL...` で `results[0].FaviconMime == ""` を assert（空文字返却）
- 1.4 — `.../購読が1件もないとき空の一覧を返す` で `len(results) == 0` を assert
- 2.1 — `.../faviconあり.なし混在...` で `len(results) == 2`（全件返却）を assert
- 2.2 — `.../faviconあり.なし混在...` で favicon ありフィードの `FaviconMime == "image/png"`
  （実際の mime 値）を assert
- 3.1 — `.../favicon_mimeがNULL...` でエラー無しを assert（500 を返さず取得完了）
- 3.2 — `.../favicon_mimeがNULL...` / `.../faviconあり.なし混在...` で当該 NULL フィードが
  購読一覧に含まれることを assert
- 4.1 — 修正は SELECT 句の COALESCE 1 箇所のみ。Scan 先 `SubscriptionWithFeedInfo`
  構造体（interfaces.go:185-195）および JSON レスポンス構造は不変（コード差分で担保）
- 4.2 — `.../faviconあり.なし混在...` で favicon ありの実 mime 値が修正前と同一であることを assert
- NFR 1.1 / 1.2 — テスト用 PostgreSQL を介した結合テスト
  （`postgres_subscription_repo_db_test.go`、`TEST_DATABASE_URL` + `db.Ping()` 失敗時 Skip）で
  検証可能。Skip ガードの慣習は `internal/database/migrate_test.go` と統一されており、
  CLAUDE.md テスト規約「DB 結合テストは実 DB を優先」と整合

## Findings

なし

## Summary

全 numeric AC（Req 1.1–1.4 / 2.1–2.2 / 3.1–3.2 / 4.1–4.2 / NFR 1.1–1.2）に対し実装または
回帰テストのカバレッジを確認。修正は `internal/repository` の SELECT 句 COALESCE 1 箇所のみで
要件 Out of Scope（web 変更なし・favicon_data 不変・フェッチロジック不変）と整合し、境界逸脱は
無い。`go vet ./internal/repository/` / `go test ./internal/repository/` も green（DB 無し環境では
新規結合テストは Skip、既存テスト無破壊）。

RESULT: approve
