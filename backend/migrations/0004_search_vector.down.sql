-- 0004 反向迁移：删除全文检索生成列与其 GIN 索引。
DROP INDEX IF EXISTS idx_tools_search_vector;
ALTER TABLE tools DROP COLUMN IF EXISTS search_vector;

DROP INDEX IF EXISTS idx_resources_search_vector;
ALTER TABLE resources DROP COLUMN IF EXISTS search_vector;

DROP INDEX IF EXISTS idx_wiki_search_vector;
ALTER TABLE wiki_entries DROP COLUMN IF EXISTS search_vector;

DROP INDEX IF EXISTS idx_items_search_vector;
ALTER TABLE items DROP COLUMN IF EXISTS search_vector;
