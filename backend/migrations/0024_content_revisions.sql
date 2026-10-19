-- 内容历史版本：记录 resources / tools / wiki_entries 的每次变更快照，
-- 支持差异查看与回滚（gj.md §15 推荐数据模型、§16.2 内容与审核测试）。
--
-- 设计要点：
--   1. 采用「通用表 + 内容类型」而非每种内容各建一张历史表，便于统一查看与审计；
--   2. snapshot 保存整行 JSON 快照，回滚时按快照还原，保证历史数据不受当前校验规则变化影响；
--   3. revision_no 在 (content_type, content_id) 内自增，写入时用会话级咨询锁串行化，
--      避免并发更新产生重复版本号；
--   4. 回滚也会**追加**一条新版本（action=rollback），不覆盖或删除历史（§16.2）。
--
-- 反向 SQL（见 0024_content_revisions.down.sql）：
--   DROP TABLE IF EXISTS content_revisions;

CREATE TABLE IF NOT EXISTS content_revisions (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    content_type   text NOT NULL,
    content_id     uuid NOT NULL,
    revision_no    integer NOT NULL,
    action         text NOT NULL,
    snapshot       jsonb NOT NULL,
    change_summary jsonb NOT NULL DEFAULT '[]',
    actor_id       uuid,
    actor_name     text NOT NULL DEFAULT '',
    note           text NOT NULL DEFAULT '',
    created_at     timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT content_revisions_type_check CHECK (content_type IN ('resource', 'tool', 'wiki')),
    CONSTRAINT content_revisions_action_check CHECK (action IN ('create', 'update', 'rollback')),
    CONSTRAINT content_revisions_no_positive CHECK (revision_no > 0)
);

-- 同一内容对象的版本号唯一（也是并发分配版本号的最终防线）。
CREATE UNIQUE INDEX IF NOT EXISTS ux_content_revisions_target_no
    ON content_revisions (content_type, content_id, revision_no);

-- 按内容对象倒序查看历史（列表的主要路径）。
CREATE INDEX IF NOT EXISTS ix_content_revisions_target_created
    ON content_revisions (content_type, content_id, created_at DESC);
