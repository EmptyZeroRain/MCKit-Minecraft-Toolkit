-- 0027 内容建设任务（gj.md §8.3 搜索词闭环、§15 内容任务）
--
-- 把「无结果搜索词」沉淀为可指派、可跟踪的运营任务：
--   * 任务记录目标关键词、搜索次数、最近出现时间、负责人、处理状态与关联内容；
--   * 关键词与 search_terms.term 同口径（归一化搜索词）；search_term_id 关联统计行，
--     并在创建时快照当时的计数，避免统计行后续变化导致任务上下文丢失；
--   * 同一关键词只允许存在一个「未完结」任务（open / in_progress），完成后可再次创建；
--   * 新增权限点 ops.contenttasks，需同步存量内置 admin 角色（见文件末尾 UPDATE）。
--
-- 隐私：本表只保存关键词与聚合计数，不含 IP / 会话 / 用户标识。
--
-- 反向 SQL（见 0027_content_tasks.down.sql）：
--   DROP TABLE IF EXISTS content_tasks;
--   UPDATE roles SET permissions = permissions - 'ops.contenttasks' WHERE name = 'admin';

CREATE TABLE IF NOT EXISTS content_tasks (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    keyword              varchar(64) NOT NULL,
    search_term_id       uuid NULL REFERENCES search_terms(id) ON DELETE SET NULL,
    hits                 bigint NOT NULL DEFAULT 0,
    no_result_hits       bigint NOT NULL DEFAULT 0,
    last_seen_at         timestamptz NULL,
    status               varchar(20) NOT NULL DEFAULT 'open',
    assignee_id          uuid NULL REFERENCES accounts(id) ON DELETE SET NULL,
    assignee_name        varchar(80) NOT NULL DEFAULT '',
    related_content_type varchar(20) NOT NULL DEFAULT '',
    related_content_id   uuid NULL,
    note                 text NOT NULL DEFAULT '',
    created_by           uuid NULL REFERENCES accounts(id) ON DELETE SET NULL,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT content_tasks_status_check
        CHECK (status IN ('open', 'in_progress', 'done', 'ignored')),
    CONSTRAINT content_tasks_content_type_check
        CHECK (related_content_type IN ('', 'resource', 'tool', 'wiki'))
);

-- 同一关键词只允许一个未完结任务；完成后可再次创建（部分唯一索引）。
CREATE UNIQUE INDEX IF NOT EXISTS ux_content_tasks_keyword_open
    ON content_tasks (keyword) WHERE status IN ('open', 'in_progress');

CREATE INDEX IF NOT EXISTS ix_content_tasks_status_updated
    ON content_tasks (status, updated_at DESC);

-- 新增权限点：同步存量内置 admin 角色（幂等；新库由 0021 种子 + 角色目录生成）。
UPDATE roles
SET permissions = permissions || '["ops.contenttasks"]'::jsonb,
    updated_at  = now()
WHERE name = 'admin'
  AND NOT (permissions @> '["ops.contenttasks"]'::jsonb);
