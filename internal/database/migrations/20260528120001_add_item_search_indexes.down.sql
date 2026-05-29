-- 記事検索（Issue #120）向けに追加した GIN インデックスを削除する
-- pg_trgm 拡張は他用途で使用されうるため DROP EXTENSION は行わない
DROP INDEX IF EXISTS idx_items_content_trgm;
DROP INDEX IF EXISTS idx_items_title_trgm;
