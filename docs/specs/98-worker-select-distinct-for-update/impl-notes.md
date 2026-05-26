# 実装ノート: Issue #98 worker のフェッチ対象取得が `SELECT DISTINCT` + `FOR UPDATE` で全失敗

## 変更概要

worker のフェッチ対象取得処理 `PostgresFeedRepo.ListDueForFetch`
（`internal/repository/postgres_feed_repo.go`）のクエリを修正した。

修正前は以下の通り `SELECT DISTINCT ... INNER JOIN subscriptions ... FOR UPDATE OF f
SKIP LOCKED` となっており、PostgreSQL が `DISTINCT` と `FOR UPDATE` の併用を許可しない
ため、フェッチサイクルが毎回 `pq: FOR UPDATE is not allowed with DISTINCT clause (0A000)`
で失敗していた（空 DB でもクエリ自体が不正で失敗）。

修正方針（Issue 本文で示された期待挙動どおり）:

- `DISTINCT` + `INNER JOIN subscriptions` をやめ、購読者の存在を
  `EXISTS (SELECT 1 FROM subscriptions s WHERE s.feed_id = f.id)` で判定する形に書き換えた。
- これにより feeds は 1 行/フィードを保証しつつ `FOR UPDATE OF f SKIP LOCKED` を維持できる。
- `WHERE f.next_fetch_at <= now() AND f.fetch_status = 'active'` と
  `ORDER BY f.next_fetch_at ASC` は現行どおり維持。
- SELECT するカラム・Scan 順序・返却型（`[]*model.Feed`）・呼び出しインターフェースは
  一切変更していない（NFR 1 後方互換）。`scheduler` 等の呼び出し側は無変更。

## 変更ファイル

- `internal/repository/postgres_feed_repo.go` — `ListDueForFetch` のクエリを EXISTS 方式に修正、
  併用不可の理由を doc comment に追記
- `internal/repository/postgres_feed_repo_test.go` — `ListDueForFetch` の回帰テストと
  DB セットアップ／挿入ヘルパーを追加（テスト用 PostgreSQL 使用）

## AC との対応（どのテストで担保したか）

| Requirement (AC) | 担保しているテスト |
|---|---|
| 1.1 DB エラーを発生させず結果を返す | 全 4 サブテストが `ListDueForFetch` のエラー無しを検証 |
| 1.2 due な購読済みフィードを取得して引き渡す | `購読者が複数存在するフィードのとき結果に1回だけ含まれる` / `選別条件のとき期限到来済みかつactiveなフィードのみ返る`（期限到来済み active が返ること） |
| 1.3 空のデータ状態でエラーなく空の結果 | `購読フィードが存在しない空のデータ状態のときエラーなく空の結果が返る` |
| 2.1 購読者複数でも 1 回だけ含める | `購読者が複数存在するフィードのとき結果に1回だけ含まれる`（出現回数 == 1） |
| 2.2 購読者 1 人以上のフィードのみ含める | `購読者が0人のフィードのとき結果から除外される`（購読者ありが返ること） |
| 2.3 購読者 0 人を除外 | `購読者が0人のフィードのとき結果から除外される`（出現回数 == 0） |
| 2.4 next_fetch_at <= now() のみ含める | `選別条件...`（`期限到来済み_active` / `境界_期限ちょうど現在時刻以下_active` が含まれる） |
| 2.5 next_fetch_at が未来のものを除外 | `選別条件...`（`期限未到来_active` が除外される） |
| 2.6 fetch_status='active' のみ含める | `選別条件...`（`期限到来済み_active` が含まれる） |
| 2.7 active 以外を除外 | `選別条件...`（`期限到来済み_stopped` / `期限到来済み_error` が除外される） |
| 3.1 取得行に対しロックを獲得 / 3.2 ロック済み行をスキップ | クエリ上 `FOR UPDATE OF f SKIP LOCKED` を維持（修正前後で不変）。本 AC は実装で保持され、回帰テストで対象行が正しく取得できることを通じて間接的に担保。並行ロック衝突の専用テストは Out of Scope（取得対象選別の復旧が本 Issue の主眼）であり、未追加（後述「確認事項」参照） |
| 4.1〜4.4 回帰テスト | `TestPostgresFeedRepo_ListDueForFetch` の 4 サブテスト（重複なし / 境界・異常系 / 購読者0除外 / 空データ）が要件 4 の 4 ケースを直接カバー |
| NFR 1 後方互換 | SELECT カラム・Scan 順序・シグネチャ・返却型を不変に保ち、呼び出し側無変更（`go build ./...` 全パッケージ通過で確認） |
| NFR 2 可観測性 | 既存の `scheduler` のエラーログ経路は変更なし（本 Issue のスコープ外、現行実装を維持） |

## テスト実行結果

本環境にはアプリ常駐の PostgreSQL は無かったため、検証用に使い捨ての PostgreSQL 16 コンテナを
起動し、`TEST_DATABASE_URL` を与えて実 DB に対して Red→Green を観測した。

### Red（修正前の buggy クエリを一時的に復元して実行）

修正前の `SELECT DISTINCT ... INNER JOIN ... FOR UPDATE` を一時的に戻して実行すると、
4 サブテスト全てが以下のエラーで失敗（Issue 記載の現象を再現）:

```
ListDueForFetch returned error: フェッチ対象フィードの取得に失敗しました:
pq: FOR UPDATE is not allowed with DISTINCT clause (0A000)
--- FAIL: TestPostgresFeedRepo_ListDueForFetch (4 サブテスト全て FAIL)
```

### Green（修正後のクエリで実行）

```
--- PASS: TestPostgresFeedRepo_ListDueForFetch (0.14s)
    --- PASS: .../購読者が複数存在するフィードのとき結果に1回だけ含まれる
    --- PASS: .../選別条件のとき期限到来済みかつactiveなフィードのみ返る
    --- PASS: .../購読者が0人のフィードのとき結果から除外される
    --- PASS: .../購読フィードが存在しない空のデータ状態のときエラーなく空の結果が返る
ok  github.com/hitoshi/feedman/internal/repository
```

### 全体検証

- `gofmt -l`（対象 2 ファイル）: 差分なし
- `go vet ./internal/repository/...`: OK
- `go build ./...`: OK
- `go test ./...`（実 PostgreSQL 接続あり）: 全パッケージ `ok`（既存テストの破壊なし）

DB に接続できない環境では、`setupListDueTestDB` が `db.Ping()` 失敗時に `t.Skip` するため、
回帰テストはスキップされる（`internal/database` の既存 `setupTestDB` パターンに準拠。
コメントアウトや assert 緩和による誤魔化しは行っていない）。CI（テスト用 PostgreSQL あり）
では実行される。

## 設計・実装上の判断

- テストは既存の `internal/database/migrate_test.go` の `testDatabaseURL` / `setupTestDB`
  パターン（環境変数 `TEST_DATABASE_URL`、未設定時は docker-compose 既定 URL、`db.Ping`
  失敗で `t.Skip`、`RunMigrations` でスキーマ適用）を `repository` パッケージに踏襲した。
  新たなモック方式は導入せず、実物のテスト用 PostgreSQL を使用する既存スタイルを維持。
- feeds.id は UUID 主キー（`gen_random_uuid()`）のため、挿入ヘルパーは `RETURNING id` で
  生成 ID を取得する方式とした（既存テストと同様）。subscriptions は users への FK を持つ
  ため、購読挿入時に users も合わせて挿入している。
- 重複検査は「同一 feedID の出現回数 == 1」を直接アサートする `countFeedID` で行い、
  単に件数を見るより重複バグに対する観点を明確化した。

## 確認事項（レビュワー判断ポイント）

- Requirement 3（排他取得の維持）について、修正は `FOR UPDATE OF f SKIP LOCKED` を不変で
  保持しているため挙動は維持されるが、「別トランザクションがロック中の行をスキップして
  他行を返す」(AC 3.2) の並行衝突を直接再現する専用テストは追加していない。理由は (a) 要件 4
  の回帰テスト要求 4 ケースに並行ロックテストが含まれないこと、(b) 本 Issue の主眼が
  「DISTINCT + FOR UPDATE 併用不可による全失敗の復旧」であり Out of Scope に
  「取得対象選別以外のスケジューラ挙動の変更」が挙げられていること、による。並行ロックの
  明示的な回帰テストが必要と判断される場合は別 Issue 化を提案する。
- 本環境にはアプリ用 PostgreSQL が常駐していなかったため、検証は使い捨てコンテナで実施した。
  CI の `TEST_DATABASE_URL`（または docker-compose 既定）でテスト用 PostgreSQL が
  提供される前提で、本回帰テストは CI 上で実行される想定。

STATUS: complete
