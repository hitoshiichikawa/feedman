-- item_states テーブルにタイムスタンプカラム（read_at, starred_at, created_at）を追加する
ALTER TABLE item_states ADD COLUMN read_at TIMESTAMPTZ;
ALTER TABLE item_states ADD COLUMN starred_at TIMESTAMPTZ;
ALTER TABLE item_states ADD COLUMN created_at TIMESTAMPTZ NOT NULL DEFAULT now();
