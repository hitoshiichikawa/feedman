# Implementation Notes

本 spec の per-task ループ実装中に得られた知見を、`### Task <id>` 見出し単位で
追記する。先行 task の見出しは改変・削除・並び替えしないこと（前方伝播の規律）。

## Implementation Notes

### Task 1

- 採用方針: 5 レイヤ縦断（`model.ItemSearchHit` → `PostgresItemRepo.SearchByUserAndKeyword` →
  `itemsearch.ItemSearchSummary` → `itemSearchHitResponse` (handler) → `ItemSearchServiceAdapter`）
  すべてに `HatebuFetchedAt *time.Time` を追加し、各レイヤで純粋な pass-through とした。
- 重要な判断:
  - JSON タグは `"hatebu_fetched_at,omitempty"` を採用し、Go 側 nil → JSON から省略する流儀を
    既存 `favicon_url` に揃えた（design.md 行 410 / 414 の方針）。フロント側は次の task 2 以降で
    TypeScript 型を `string | null` で受け、`undefined ?? null` の正規化で `-` 表示判定を統一する。
  - リポジトリ層は `sql.NullTime` で Scan し、`Valid` → `&Time` を保持する既存パターン
    （`FindByID` / `ListNeedingHatebuFetch` 等）に揃えた。SQL の `SELECT` 列追加のみで
    `WHERE` / `ORDER BY` / `LIMIT` / `JOIN` は不変（NFR 3.1 を担保）。
  - 既存テスト `TestSearch_HitsConvertedToSummaries` を拡張し、その上で `HatebuFetchedAt`
    特化テスト `TestSearch_HatebuFetchedAt_NilAndNonNilPassthrough` を別途追加した。
    nil/非 nil の両条件を 1 テスト 1 検証に分けつつ、既存 reflect.DeepEqual の全フィールド
    マッピング検証も `HatebuFetchedAt` を含む新 struct を期待値とした形で更新済み。
  - repo DB 結合テストは既存 `setupItemSearchTestDB` の Skip ガード（DB 接続不可で `t.Skip`）
    を踏襲。CI（DB 起動なし）では Skip され、ローカル DB ありの場合のみ NULL/値あり両分岐を
    実行する。
- 残存課題: なし（task 2 以降の TypeScript 型 / UI 配線は後続 task で対応）。
