-- 記事検索（Issue #120）向けに pg_trgm 拡張と GIN インデックスを追加する
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE INDEX idx_items_title_trgm ON items USING GIN (title gin_trgm_ops);
CREATE INDEX idx_items_content_trgm ON items USING GIN (content gin_trgm_ops) WHERE content IS NOT NULL;
