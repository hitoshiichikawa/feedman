# Implementation Notes — Issue #121 フィード横断新着一覧機能

## Implementation Notes

### Task 1

- 採用方針: `user_cross_feed_views` テーブル（UUID PK + last_seen_at + updated_at）と対応する Go ドメインモデル `UserCrossFeedView` を新規追加。
- 重要な判断: design.md / tasks.md が指定する migration timestamp `20260528120000` が既存 `20260528120000_add_item_search_indexes` と完全衝突するため、同日 1 時間後ろ倒しの `20260528130000` に micro-adjust した（採番のみの差異で意味的変更なし）。
- 残存課題: なし。

## 確認事項

- task 1 で migration timestamp が design.md 指定（`20260528120000`）と既存 migration `20260528120000_add_item_search_indexes` で衝突したため、`20260528130000` に micro-adjust した。後続 task の本文中に同 timestamp を参照する箇所は無いため影響なし（design.md は書き換えていない）。
- `go vet ./...` 全体実行時に `internal/repository/postgres_item_repo_starred_test.go` で `insertTestItem redeclared in this block` の既存事象が出力されたが、本 task で触れていないファイル群であり、本 task の変更とは無関係（変更パッケージ `internal/model` / `internal/database` 単体での vet は pass）。後続 task または別 Issue での対処を要する可能性がある旨を Reviewer / PM に共有する。
