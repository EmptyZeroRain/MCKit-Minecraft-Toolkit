-- 0004: 全文检索（需求 §5.1 / §7.11）
--
-- 为 tools / resources / wiki_entries / items 增加由正文字段派生的 tsvector 生成列，
-- 并建立 GIN 索引。生成列为 STORED，插入/更新时自动同步维护，无需触发器。
--
-- 权重：A=标题/标识（name/title/slug/item_id），B=摘要，C=正文。
--
-- 说明：PostgreSQL 默认分词器按标点/空格切分，对连续中文按整段作为单个词元，
-- 因此中文子串检索仍依赖查询层的 ILIKE 兜底（见 internal/search）。本迁移的
-- tsvector 主要服务英文标识/空格分词关键词的加权匹配。
--
-- 回滚策略（反向 SQL，按需手工执行）：
--   DROP INDEX IF EXISTS idx_tools_search_vector;
--   ALTER TABLE tools DROP COLUMN IF EXISTS search_vector;
--   （resources / wiki_entries / items 同理）

-- ---- tools ----
ALTER TABLE tools ADD COLUMN search_vector tsvector
    GENERATED ALWAYS AS (
        setweight(to_tsvector('simple', coalesce(name, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(slug, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(summary, '')), 'B') ||
        setweight(to_tsvector('simple', coalesce(description, '')), 'C')
    ) STORED;
CREATE INDEX idx_tools_search_vector ON tools USING GIN (search_vector);

-- ---- resources ----
ALTER TABLE resources ADD COLUMN search_vector tsvector
    GENERATED ALWAYS AS (
        setweight(to_tsvector('simple', coalesce(name, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(slug, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(summary, '')), 'B') ||
        setweight(to_tsvector('simple', coalesce(description, '')), 'C')
    ) STORED;
CREATE INDEX idx_resources_search_vector ON resources USING GIN (search_vector);

-- ---- wiki_entries ----
ALTER TABLE wiki_entries ADD COLUMN search_vector tsvector
    GENERATED ALWAYS AS (
        setweight(to_tsvector('simple', coalesce(title, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(slug, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(summary, '')), 'B') ||
        setweight(to_tsvector('simple', coalesce(content, '')), 'C')
    ) STORED;
CREATE INDEX idx_wiki_search_vector ON wiki_entries USING GIN (search_vector);

-- ---- items ----
ALTER TABLE items ADD COLUMN search_vector tsvector
    GENERATED ALWAYS AS (
        setweight(to_tsvector('simple', coalesce(item_id, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(name_zh, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(name_en, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(description, '')), 'C')
    ) STORED;
CREATE INDEX idx_items_search_vector ON items USING GIN (search_vector);
