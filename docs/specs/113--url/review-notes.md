# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-05-27T00:00:00Z -->

## Reviewed Scope

- Branch: claude/issue-113-impl--url
- HEAD commit: bbdffc7d30bccb7bc664cfc92c2e375a6a56cf38
- Compared to: develop..HEAD

本 spec は design.md / tasks.md が不在の design-less impl。boundary 制約は
requirements.md の Out of Scope を正本として照合した。変更差分は
`internal/repository/postgres_feed_repo.go`（実装）+ `postgres_feed_repo_test.go` /
`internal/worker/fetch/fetcher_test.go`（テスト）+ spec ドキュメントのみ。

## Verified Requirements

- 1.1 — `fetcher.go:262-264` の空値ガード後に `feed.Title` を `UpdateFetchState` へ引き渡し、
  `postgres_feed_repo.go:230` で `title = $2` を永続化。`TestFetcher_Fetch_PersistsTitleAndSiteURL`（引き渡し）/
  `TestPostgresFeedRepo_UpdateFetchState`「永続化する」（DB 反映）でカバー
- 1.2 — `fetcher.go:266-267` で `feed.SiteURL` を設定し `postgres_feed_repo.go:231` で
  `site_url = $3`（`nullString`）を永続化。同上テストでカバー
- 1.3 — `UpdateFetchState` の UPDATE SQL に title / site_url を含めパース結果を永続化先へ書き込む。
  `TestPostgresFeedRepo_UpdateFetchState`「永続化する」でカバー
- 1.4 — 同一 UPDATE で fetch_status / consecutive_errors / error_message / next_fetch_at /
  etag / last_modified も従来どおり反映（`postgres_feed_repo.go:232-237`）。
  `TestPostgresFeedRepo_UpdateFetchState` が ETag / LastModified / FetchStatus を assert
- 2.1 — `fetcher.go:263` の `if parsedFeed.Title != ""` ガードで空タイトル時は上書きしない。
  `TestFetcher_Fetch_EmptyParsedTitleDoesNotOverwrite` でカバー
- 2.2 — `fetcher.go:266` の `if parsedFeed.Link != ""` ガードで空サイト URL 時は上書きしない。
  同テストでカバー
- 2.3 — 初期値（URL）を持つ feed に対しパース済みタイトルへ置換。
  `TestFetcher_Fetch_PersistsTitleAndSiteURL`（初期 Title=server.URL → 置換）/
  `TestPostgresFeedRepo_UpdateFetchState`（初期 title=feedURL の前提を assert 後に置換確認）
- 3.1 — 失敗パス（Stop/Backoff/parse 失敗等）は `feed.Title`/`feed.SiteURL` を触らず既存値で
  `UpdateFetchState` を呼ぶ構造。repo が渡された既存値をそのまま書く。
  `TestPostgresFeedRepo_UpdateFetchState`「保持したまま状態のみ更新」でカバー
- 3.2 — 304 未変更パス（`fetcher.go:159-179`）はパースを行わず既存値のまま `UpdateFetchState` を呼ぶ。
  既存値非破壊の repo 層テストで担保
- 4.1 — API スキーマ変更なし。永続化された title を既存 subscription/feed handler
  （`feed_handler.go:241` で `feed.SiteURL` 等を返す）がそのまま返す。永続化を
  `TestPostgresFeedRepo_UpdateFetchState`（DB 反映）+ 既存 API テストで担保
- 4.2 — requirements.md Out of Scope に「左ペイン UI のレイアウト・表示ロジックの改修」が明記され、
  保存タイトルが正しくなれば既存表示で解消される前提。UI 変更なしは妥当
- NFR 1.1 — フェッチ各経路の従来状態項目は更新内容を変えていない（既存 `fetcher_test.go` 群が green）
- NFR 1.2 — API スキーマ / 既存ハンドラ未変更。既存 handler テストが green
- NFR 2.1 — 正常系 / 空入力 / 異常系（非破壊）を上記自動テストでカバー

## Findings

なし

## Summary

全 numeric AC に対応する実装またはテストが確認でき、boundary 逸脱（Out of Scope 抵触）/
missing test も検出されなかった。`go build` / `go test`（repository, worker/fetch）green
（DB 結合テストは未接続のため skip、CI / DB 環境で実行される）。

RESULT: approve
