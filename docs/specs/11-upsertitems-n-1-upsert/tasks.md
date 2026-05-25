# Implementation Tasks

本フローに Architect は入っていないため、Developer が `.claude/rules/tasks-generation.md` 準拠で
作成した簡潔なタスク分割。

- [ ] 1. ItemRepository にバルク用メソッドを追加する
  - 既存メソッドのシグネチャは変更しない（他の呼び出し元がある）
  - バッチ化 find（guid/link/hash 群を一括取得）とバルク upsert（単一トランザクション内で
    複数行 INSERT + 複数行 UPDATE、エラー時全件ロールバック）のインターフェースを定義
  - `mockItemRepo` にバルクメソッドの呼び出し回数カウンタを追加
  - _Requirements: 2.1, 2.2, 3.1, 3.3, NFR 3.1_

- [ ] 2. UpsertItems をバルク化する
  - サニタイズ・content_hash 計算を全件先行実行（現状アルゴリズム不変）
  - バッチ内 dedup（同一性キー後勝ち）
  - バッチ化 find → Go 側 3 段階優先順位判定 → 新規/更新の仕分け（判定結果不変）
  - バルク upsert を単一トランザクションで実行し、エラー時 (0,0,err) wrap + 構造化ログ
  - 0 件/nil は早期 return で (0,0,nil)
  - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, 2.1, 2.2, 3.1, 3.2, 3.3, 3.4, 4.1, 4.2, 4.3, 4.4, NFR 1.1, NFR 1.2, NFR 2.1, NFR 2.2, NFR 3.1_
  - _Boundary: ItemUpsertService_

- [ ] 3. Postgres 実装にバルク SQL を実装する
  - バッチ化 SELECT（`WHERE (feed_id, guid_or_id) IN (...)` 等、3 段階ぶん定数回）
  - バルク INSERT（複数行 VALUES）/ バルク UPDATE（`UPDATE ... FROM (VALUES ...)`）を単一トランザクション
  - エラー時はトランザクションロールバック
  - _Requirements: 2.1, 3.1, NFR 3.1_
  - _Boundary: PostgresItemRepo_

- [ ] 4. 全 AC を網羅する単体テストを追加する
  - 混在 50 件、id 保持、判定結果不変、サニタイズ&hash 再計算、往復定数オーダー、
    エラー時 (0,0,err)・ロールバック・wrap・ログ、0件/nil/1件、バッチ内重複後勝ち
  - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, 2.1, 2.2, 3.1, 3.2, 3.3, 3.4, 4.1, 4.2, 4.3, 4.4, NFR 2.1, NFR 2.2, NFR 3.1_
