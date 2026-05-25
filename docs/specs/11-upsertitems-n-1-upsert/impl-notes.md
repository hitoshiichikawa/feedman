# 実装ノート: Issue #11 UpsertItems の N+1 をバルク UPSERT に変更

## 採用設計

記事 1 件ごとに最大 3 回の SELECT + 1 回の INSERT/UPDATE を逐次実行していた N+1 構造を、
以下の 3 フェーズに再構成して DB 往復を定数オーダー化した。

1. **既存記事の一括取得**: バッチに含まれる `guid_or_id` / `link` / `content_hash` の候補集合を
   収集し、`FindExistingForUpsert` が各キーごとに 1 回ずつ（合計最大 3 回）のバッチ SELECT
   （`WHERE feed_id = $1 AND <col> IN (...)`）で既存記事を引く。
2. **Go 側での 3 段階同一性判定**: 取得した既存記事マップに対し、現状と等価な優先順位
   （`(feed_id, guid_or_id)` > `(feed_id, link)` > `content_hash`）で新規/更新を仕分けする。
   判定アルゴリズムは Go 側に保持したため、判定結果はバルク化前後で不変（NFR 2）。
3. **バルク永続化**: 新規は複数行 VALUES の `INSERT`、既存は `UPDATE ... FROM (VALUES ...)` で
   一括書き込みし、両者を **単一トランザクション**で実行する。

この設計によりスキーマ変更（`link` / `content_hash` へのユニーク制約追加）を回避した。
items テーブルの `(feed_id, guid_or_id)` は部分ユニークインデックスだが、`(feed_id, link)` /
`(feed_id, content_hash)` は通常インデックスのみでユニーク制約が無いため、3 段階判定を
単一 `INSERT ... ON CONFLICT` で表現することは不可能。同一性判定を Go 側に残す方針を採った。
スキーマ変更は requirements の Out of Scope のため行っていない。

## 追加したインターフェースメソッド（`repository.ItemRepository`）

既存メソッドのシグネチャは一切変更していない（他の呼び出し元があるため）。
`UpsertItems` の公開シグネチャも NFR 1 のとおり不変。

- `FindExistingForUpsert(ctx, feedID string, guids, links, hashes []string) (*ExistingItems, error)`
  - 同一性判定に必要な既存記事を guid/link/hash 別に索引した `*ExistingItems` で一括返却。
  - 各キー集合が空のときは当該 SELECT をスキップ（DB アクセスしない）。
- `BulkUpsert(ctx, toCreate, toUpdate []*model.Item) error`
  - 新規一括 INSERT と既存一括 UPDATE を単一トランザクションで実行。途中エラーで全件ロールバック。
- 新規エクスポート型 `repository.ExistingItems`（`ByGUID` / `ByLink` / `ByContentHash` の 3 マップ）。

## オーケストレーター確定事項の反映

1. **エラー時 = 全件ロールバック**: `BulkUpsert` は単一トランザクションで実行し、INSERT/UPDATE
   いずれかでエラーが出れば `tx.Rollback()`（`defer`）で全件ロールバック。`UpsertItems` は
   取得失敗・永続化失敗いずれの場合も `(0, 0, err)` を返し、発生元エラーを `fmt.Errorf("...: %w", ...)`
   で wrap する。エラーは `slog.Error` で構造化ログに記録する（Requirement 3.1〜3.4）。
2. **バッチ内重複 = 後勝ち（事前 dedup）**: `dedupByIdentity` がバッチ内で同一性判定上同一と
   みなされる記事を **最終要素優先（後勝ち）**で 1 件に集約してから書き込む。代表キーは
   優先順位に沿って「guid_or_id があればそれ、無ければ link、無ければ content_hash」を採用する。
   代表キーを持たない記事（3 キーすべて空）は dedup 対象外としてそのまま保持する。

## 同一性判定結果の不変性について（NFR 2 / Requirement 1.4）

- 判定の優先順位ロジック・`computeContentHash` のアルゴリズム（対象フィールド・ハッシュ関数）・
  サニタイズ適用は一切変更していない。`content_hash` は従来どおりサニタイズ後のサマリーを
  用いて計算する。
- 既存の優先順位テスト（GUID > Link、Link > Hash、フォールバック）はバルク化後もそのまま通過。

## テスト戦略

`internal/item/upsert_test.go` の `mockItemRepo` を拡張し、新バルクメソッド
（`FindExistingForUpsert` / `BulkUpsert`）を実装。`findBulkCalls` / `upsertCalls` の呼び出し
回数カウンタと、`findErr` / `upsertErr` のエラー注入フィールドを追加した。`BulkUpsert` は
`upsertErr` 設定時に内部マップを一切変更しないことでロールバック相当を表現する。

既存テスト（`createCalls` / `updateCalls` を検証する 20 件以上のテスト）は、モックの
`BulkUpsert` が `toCreate` / `toUpdate` を走査してこれらのカウンタを更新するため変更なしで通過する。

## 受入基準（AC）とテストの対応

| AC | 担保するテスト |
|---|---|
| R1.1 新規/既存件数の分離 | `TestUpsertItems_Mixed50_Counts`, `TestUpsertItems_MultipleItems` |
| R1.2 混在 50 件で inserted=N/updated=M | `TestUpsertItems_Mixed50_Counts` |
| R1.3 更新時 id 保持 | `TestUpsertItems_Update_PreservesID`, `TestUpsertItems_Update_OverwritesContent` |
| R1.4 判定結果不変 | `TestUpsertItems_IdentityPriority_GUIDOverLink`, `..._LinkOverHash`, `TestUpsertItems_GUIDNotFound_FallbackToLink`, `..._FallbackToHash`, `TestUpsertItems_BatchInternalDuplicate_ExistingLastWins` |
| R1.5 サニタイズ後コンテンツ・再計算 hash 保存 | `TestUpsertItems_Update_SanitizedContentAndHash`, `TestUpsertItems_Update_ContentIsSanitized`, `..._ContentHashUpdated` |
| R2.1 / R2.2 / NFR3.1 往復定数オーダー | `TestUpsertItems_RoundTripsConstant`（1/10/50 件で findBulkCalls=1, upsertCalls=1） |
| R3.1 全件ロールバック | `TestUpsertItems_DBError_UpsertRollsBackAndReturnsZero`（永続化記事数 0 を検証） |
| R3.2 件数 0 + エラー | `TestUpsertItems_DBError_FindReturnsZeroAndWrappedError`, `..._UpsertRollsBackAndReturnsZero` |
| R3.3 発生元エラーの wrap | 上記 2 テストで `errors.Is(err, sentinel)` を検証 |
| R3.4 構造化ログ記録 | 実装で `slog.Error` を発行（取得失敗・永続化失敗の両経路）。ログ検証は副作用のため slog 出力のアサートは行わず実装側で担保 |
| R4.1 0 件で DB 非アクセス | `TestUpsertItems_EmptyItems_NoDBAccess`, `TestUpsertItems_EmptyItems` |
| R4.2 nil で DB 非アクセス | `TestUpsertItems_NilItems_NoDBAccess`, `TestUpsertItems_NilItems` |
| R4.3 1 件新規 (1,0,nil) | `TestUpsertItems_SingleNewItem`, `TestUpsertItems_NewItem_Insert` |
| R4.4 1 件既存 (0,1,nil) | `TestUpsertItems_SingleExistingItem`, `TestUpsertItems_IdentityByGUID` |
| NFR1.1 公開シグネチャ不変 | `internal/worker/fetch` の `ItemUpserter` インターフェース経由呼び出しが変更なしで通過（コンパイル + 既存テスト green） |
| NFR1.2 戻り値の意味不変 | 件数系テスト全般 |
| NFR2.1 / NFR2.2 判定結果不変 | R1.4 のテスト群 |
| バッチ内重複の後勝ち（確定事項 2） | `TestUpsertItems_BatchInternalDuplicate_LastWins`, `..._ExistingLastWins` |

## ライブ DB 未カバー範囲（手動検証方針）

- `internal/repository/postgres_item_repo_test.go` は既存慣習どおり**インターフェース適合の
  コンパイル時チェックのみ**で、ライブ DB 結合テストは存在しない。本実装でも同慣習に従い、
  追加したバルク SQL（`FindExistingForUpsert` の `IN (...)` SELECT、`bulkInsertItems` の
  複数行 VALUES INSERT、`bulkUpdateItems` の `UPDATE ... FROM (VALUES ...)`）に対する
  ライブ DB 統合テストは追加していない。
- バルク UPDATE の SQL は、`VALUES` 由来の派生テーブルでプレースホルダ型推論が NULL 値で
  失敗するのを避けるため、内側 SELECT で各カラムに明示キャスト（`::uuid` / `::timestamptz` /
  `::boolean` / `::text`）を付与している。**この SQL の実 DB 動作はローカルの PostgreSQL 16 に
  対して手動で `docker-compose` 起動 + マイグレーション適用後に検証することを推奨**する
  （CI には DB 結合テストの仕組みが無いため）。
- 手動検証観点: (a) guid/link/hash 混在バッチで件数が一致するか、(b) NULL published_at を
  含む新規/更新が型エラーなく書き込めるか、(c) 途中で意図的にエラー（重複 PK 等）を起こした
  際にトランザクションが全件ロールバックされるか。

## 確認事項（レビュワー / PM・Architect 判断ポイント）

- **requirements.md の Open Questions（エラー時挙動の変更）**: Requirement 3 は現状の
  「部分成功カウントを返す」挙動から「全件ロールバック・件数 0」への**意図的な変更**である。
  オーケストレーター確定事項どおり実装したが、これは呼び出し元（`worker/fetch/fetcher.go`）から
  見える挙動変更を含む（従来はエラー時も途中までの inserted/updated が返っていた）。運用上
  部分成功カウントの温存が必要なら PM へ差し戻しが必要。
- **バッチ内重複の dedup 仕様**: 確定事項どおり「事前 dedup（最終要素優先）」を採用した。
  これにより同一 guid の新規重複 2 件は `inserted=1, updated=0` となる（逐次処理での
  `inserted=1, updated=1` とは件数が異なる）。これは requirements R1.2 が前提とする
  「重複なし混在バッチ」では差異を生まないが、重複ありバッチでの件数定義を確定事項で
  固定した点を明記する。
- **ライブ DB テスト不在**: 上記「ライブ DB 未カバー範囲」のとおり、バルク SQL の実 DB 動作は
  本リポジトリの結合テスト慣習（インターフェース適合のみ）に従い自動テスト化していない。
  DB 結合テスト基盤の整備は別 Issue として切り出す候補。

## 次の Issue 候補（派生タスク）

- items テーブルのバルク SQL に対するライブ DB 結合テスト基盤の整備（テスト用 PostgreSQL
  コンテナの導入）。
- 大量バッチ（PostgreSQL のプレースホルダ上限 65535 に対し 1 記事 16 列 = 約 4000 件で上限）
  に対する分割書き込み。現状フィードは最大 50 件想定のため未対応だが、将来的な上限到達への備え。

STATUS: complete
