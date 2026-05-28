# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-05-26T00:00:00Z -->

## Reviewed Scope

- Branch: claude/issue-98-impl-worker-select-distinct-for-update
- HEAD commit: 63258cc3d01702f24c44b3f46d317561961889bd
- Compared to: develop..HEAD

備考: 本 Issue はバグ修正の design-less impl で、`docs/specs/98-.../` 配下に `design.md` /
`tasks.md` は存在しない。よって `_Boundary:_` アノテーションは無く、境界判定は変更ファイルが
impl-notes.md 記載のスコープ（`internal/repository/postgres_feed_repo.go` とその近傍テスト）に
閉じているかで確認した。CLAUDE.md の Feature Flag Protocol は `opt-out` のため flag 観点は適用しない。

## Verified Requirements

- 1.1 — `postgres_feed_repo.go:170-217` EXISTS 方式で 0A000 を解消。全 4 サブテストが `ListDueForFetch` のエラー無しを assert
- 1.2 — `postgres_feed_repo_test.go` `選別条件...期限到来済み_active`（include=true）/ 複数購読者テストで due な購読済みフィードが返ることを検証
- 1.3 — `購読フィードが存在しない空のデータ状態のときエラーなく空の結果が返る`（err==nil, len==0）
- 2.1 — `購読者が複数存在するフィードのとき結果に1回だけ含まれる`（`countFeedID == 1`）
- 2.2 — `購読者が0人のフィードのとき結果から除外される`（`withSubFeedID` の出現回数 == 1）+ `EXISTS (SELECT 1 FROM subscriptions ...)`
- 2.3 — 同テストで `noSubFeedID` の出現回数 == 0
- 2.4 — `選別条件...` `期限到来済み_active` / `境界_期限ちょうど現在時刻以下_active`（include=true）+ クエリ `WHERE f.next_fetch_at <= now()`
- 2.5 — `選別条件...` `期限未到来_active`（include=false）
- 2.6 — `選別条件...` `期限到来済み_active`（include=true）+ クエリ `AND f.fetch_status = 'active'`
- 2.7 — `選別条件...` `期限到来済み_stopped` / `期限到来済み_error`（include=false）
- 3.1 — クエリ `FOR UPDATE OF f SKIP LOCKED` を修正前後で不変に維持（`postgres_feed_repo.go:180`）
- 3.2 — 同上。SKIP LOCKED 句で他トランザクションがロック中の行をスキップする挙動を保持
- 4.1 — `購読者が複数存在するフィードのとき結果に1回だけ含まれる` が重複しないことを検証
- 4.2 — `選別条件...` の `期限未到来` / `stopped` / `error` 除外ケースが境界・異常系を検証
- 4.3 — `購読者が0人のフィードのとき結果から除外される`
- 4.4 — `購読フィードが存在しない空のデータ状態...`
- NFR 1 — SELECT カラム順 / Scan 順 / シグネチャ / 返却型 `[]*model.Feed` 不変。`internal/worker/`・`internal/repository/interfaces.go` は無変更（`git diff` で空）
- NFR 2 — scheduler のエラーログ経路は本 Issue スコープ外で無変更（既存実装維持）

## Findings

なし

## Summary

`ListDueForFetch` を `DISTINCT + INNER JOIN` から `EXISTS` サブクエリへ書き換え、`FOR UPDATE OF f
SKIP LOCKED` を維持したまま 0A000 失敗を解消。requirements.md の全 numeric ID（1.1〜4.4, NFR）に
対応する実装と回帰テストを確認した。境界逸脱なし、AC 未カバーなし、AC が要求するテストは全て追加済み。
Req 3.2 の並行ロック専用テストは要件 4 が要求しておらず、missing test には当たらない。

RESULT: approve
