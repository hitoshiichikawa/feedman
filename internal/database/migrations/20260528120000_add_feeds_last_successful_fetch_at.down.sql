-- feeds テーブルから last_successful_fetch_at カラムを削除する
ALTER TABLE feeds DROP COLUMN IF EXISTS last_successful_fetch_at;
