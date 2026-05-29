-- feeds テーブルに last_successful_fetch_at カラムを追加する
-- 用途: 手動フェッチ機能 (Issue #115) のクールダウン判定 (10 分) の起点となる
-- 自動ワーカー / 手動フェッチの両経路から成功時に更新される
-- 既存行はバックフィルしない (NULL = 過去成功実績なし = クールダウン非適用の safe default)
ALTER TABLE feeds ADD COLUMN last_successful_fetch_at TIMESTAMPTZ NULL;
