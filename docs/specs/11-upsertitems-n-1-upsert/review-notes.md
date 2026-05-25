# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-05-25T10:02:14Z -->

## Reviewed Scope

- Branch: claude/issue-11-impl-upsertitems-n-1-upsert
- HEAD commit: ab54a55fdf117b762ac19e128797a348c5fc0158
- Compared to: develop..HEAD

差分対象ファイル: `internal/item/upsert.go`（ItemUpsertService）/ `internal/repository/interfaces.go`
（ItemRepository インターフェース拡張 + `ExistingItems` 型）/ `internal/repository/postgres_item_repo.go`
（PostgresItemRepo バルク SQL）/ `internal/item/upsert_test.go`（AC 網羅テスト）。
`CLAUDE.md` の Feature Flag Protocol は `opt-out` のため flag 観点の確認は行わず、通常の 3 カテゴリ判定を実施。
`go build ./...` / `go vet` / `go test ./internal/item/... ./internal/repository/... ./internal/worker/...` いずれも green を確認。

## Verified Requirements

- 1.1 — `TestUpsertItems_Mixed50_Counts`, `TestUpsertItems_MultipleItems`（新規=inserted / 既存=updated に分離）
- 1.2 — `TestUpsertItems_Mixed50_Counts`（新規 30・既存 20 の混在 50 件で inserted=30/updated=20）
- 1.3 — `TestUpsertItems_Update_PreservesID`（既存 id を保持、`buildUpdatedItem` が `*existing` をコピーし id 不変）
- 1.4 — `TestUpsertItems_IdentityPriority_GUIDOverLink` / `..._LinkOverHash` / `..._GUIDNotFound_FallbackToLink` / `..._GUIDAndLinkNotFound_FallbackToHash`。`matchExisting`（upsert.go:200-217）の優先順位は旧 `findExistingItem` と等価
- 1.5 — `TestUpsertItems_Update_SanitizedContentAndHash`, `..._Update_ContentIsSanitized`, `..._Update_ContentHashUpdated`（サニタイズ後コンテンツ・サマリーと再計算 hash を保存）
- 2.1 — `TestUpsertItems_RoundTripsConstant`（1/10/50 件で findBulkCalls=1, upsertCalls=1）+ postgres_item_repo.go の IN 句バッチ SELECT / 複数行 VALUES
- 2.2 — `TestUpsertItems_RoundTripsConstant`（件数増加でも往復回数固定）
- 3.1 — `TestUpsertItems_DBError_UpsertRollsBackAndReturnsZero`（永続化記事数 0 を検証）+ `BulkUpsert` 単一 tx + `defer tx.Rollback()`
- 3.2 — `TestUpsertItems_DBError_FindReturnsZeroAndWrappedError`, `..._UpsertRollsBackAndReturnsZero`（(0,0,err) 返却）
- 3.3 — 上記 2 テストで `errors.Is(err, sentinel)`。upsert.go:79/102 が `fmt.Errorf("...: %w", ...)` で wrap
- 3.4 — upsert.go:75-79 / 96-102 の `slog.Error`（取得失敗・永続化失敗の両経路で構造化ログ発行）。ログ出力は外部副作用のため実装側で担保（テスト規約のモック方針と整合）
- 4.1 — `TestUpsertItems_EmptyItems_NoDBAccess`, `..._EmptyItems`（空スライスで DB 非アクセス・(0,0,nil)）
- 4.2 — `TestUpsertItems_NilItems_NoDBAccess`, `..._NilItems`（nil で DB 非アクセス・(0,0,nil)）
- 4.3 — `TestUpsertItems_SingleNewItem`, `..._NewItem_Insert`（1 件新規で (1,0,nil)）
- 4.4 — `TestUpsertItems_SingleExistingItem`, `..._IdentityByGUID`（1 件既存で (0,1,nil)）
- NFR 1.1 — `internal/worker/fetch/fetcher.go` の呼び出し元に diff なし。`UpsertItems` の公開シグネチャ不変・build green
- NFR 1.2 — 件数系テスト全般で inserted=挿入数 / updated=更新数 の意味を維持
- NFR 2.1 / 2.2 — 1.4 のテスト群 + `computeContentHash` / サニタイズ / 優先順位ロジックを Go 側に温存（アルゴリズム不変）
- NFR 3.1 — `TestUpsertItems_RoundTripsConstant`（50 件でも往復定数オーダー）

## Findings

なし

## Summary

requirements.md の全 numeric ID（R1.1〜4.4 / NFR 1〜3）に対応する実装・テストを確認した。バルク化に伴う
同一性判定・サニタイズ・hash 計算は Go 側に温存され、旧逐次実装と等価。既存の優先順位テストも保持され
バルク経路で通過する。3 カテゴリ（AC 未カバー / missing test / boundary 逸脱）いずれにも該当なし。テストは
全 green。impl-notes 記載のエラー時挙動変更・バッチ内 dedup・ライブ DB テスト不在は requirements の Open
Questions / 確定事項で合意済みであり、AC 観点での reject 事由には当たらない。

RESULT: approve
