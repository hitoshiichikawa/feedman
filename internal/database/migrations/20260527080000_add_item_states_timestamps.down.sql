-- item_states テーブルから追加したタイムスタンプカラム（read_at, starred_at, created_at）を削除する
ALTER TABLE item_states DROP COLUMN IF EXISTS read_at;
ALTER TABLE item_states DROP COLUMN IF EXISTS starred_at;
ALTER TABLE item_states DROP COLUMN IF EXISTS created_at;
