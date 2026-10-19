-- 0023 反向迁移：删除标签关联表（数据可从 JSONB 重新回填）。
DROP TABLE IF EXISTS wiki_tags;
DROP TABLE IF EXISTS tool_tags;
DROP TABLE IF EXISTS resource_tags;
