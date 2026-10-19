-- 标签关联表：把内容对象上的 JSONB `tags` 落到关系表，用于可靠筛选与统计。
--
-- 背景：resources / tools / wiki_entries 的 tags 一直是 JSONB 数组，
-- 按标签筛选只能退化为 `tags::text LIKE`（易误匹配、无法走索引），
-- 统计也只能全表扫描。此处新增三张关联表作为「标签 → 内容」的索引，
-- 由管理端保存时同步写入（JSONB 仍作为接口输出的数据来源，保持兼容）。
--
-- 反向 SQL（见 0023_tags_relations.down.sql）：
--   DROP TABLE IF EXISTS resource_tags / tool_tags / wiki_tags;

CREATE TABLE IF NOT EXISTS resource_tags (
    resource_id uuid NOT NULL REFERENCES resources(id) ON DELETE CASCADE,
    tag         text NOT NULL,
    PRIMARY KEY (resource_id, tag)
);

CREATE TABLE IF NOT EXISTS tool_tags (
    tool_id uuid NOT NULL REFERENCES tools(id) ON DELETE CASCADE,
    tag     text NOT NULL,
    PRIMARY KEY (tool_id, tag)
);

CREATE TABLE IF NOT EXISTS wiki_tags (
    wiki_entry_id uuid NOT NULL REFERENCES wiki_entries(id) ON DELETE CASCADE,
    tag           text NOT NULL,
    PRIMARY KEY (wiki_entry_id, tag)
);

-- 按标签反查内容（筛选的主要路径）。
CREATE INDEX IF NOT EXISTS idx_resource_tags_tag ON resource_tags (tag);
CREATE INDEX IF NOT EXISTS idx_tool_tags_tag ON tool_tags (tag);
CREATE INDEX IF NOT EXISTS idx_wiki_tags_tag ON wiki_tags (tag);

-- 回填既有数据：把 JSONB 数组展开为关联行（幂等，可重复执行）。
INSERT INTO resource_tags (resource_id, tag)
SELECT r.id, t.tag
FROM resources r
CROSS JOIN LATERAL jsonb_array_elements_text(r.tags) AS t(tag)
WHERE r.tags IS NOT NULL AND jsonb_typeof(r.tags) = 'array'
ON CONFLICT DO NOTHING;

INSERT INTO tool_tags (tool_id, tag)
SELECT x.id, t.tag
FROM tools x
CROSS JOIN LATERAL jsonb_array_elements_text(x.tags) AS t(tag)
WHERE x.tags IS NOT NULL AND jsonb_typeof(x.tags) = 'array'
ON CONFLICT DO NOTHING;

INSERT INTO wiki_tags (wiki_entry_id, tag)
SELECT w.id, t.tag
FROM wiki_entries w
CROSS JOIN LATERAL jsonb_array_elements_text(w.tags) AS t(tag)
WHERE w.tags IS NOT NULL AND jsonb_typeof(w.tags) = 'array'
ON CONFLICT DO NOTHING;
