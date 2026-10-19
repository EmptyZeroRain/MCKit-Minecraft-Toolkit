-- 0020: 开发文档全文检索（TODO §21 补齐「开发文档接入全站搜索」）
--
-- 背景：dev_docs 在 0013 中未建立 search_vector，导致全站搜索（internal/search）
-- 无法命中开发文档内容。本迁移按 0004 的同一范式，为其增加 STORED 生成列 + GIN 索引。
--
-- 权重：A=标题（中文 / 英文）/ slug，B=摘要，C=正文（body_md）。
--
-- 说明：PostgreSQL 默认分词器按标点/空格切分，对连续中文按整段作为单个词元，
-- 因此中文子串检索仍依赖查询层的 ILIKE 兜底（见 internal/search.searchDevDoc）。
-- 本迁移的 tsvector 主要服务英文标识/空格分词关键词的加权匹配。
--
-- 回滚策略（反向 SQL，按需手工执行）：
--   DROP INDEX IF EXISTS idx_dev_docs_search_vector;
--   ALTER TABLE dev_docs DROP COLUMN IF EXISTS search_vector;

ALTER TABLE dev_docs ADD COLUMN search_vector tsvector
    GENERATED ALWAYS AS (
        setweight(to_tsvector('simple', coalesce(title_zh, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(title_en, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(slug, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(summary, '')), 'B') ||
        setweight(to_tsvector('simple', coalesce(body_md, '')), 'C')
    ) STORED;
CREATE INDEX idx_dev_docs_search_vector ON dev_docs USING GIN (search_vector);
