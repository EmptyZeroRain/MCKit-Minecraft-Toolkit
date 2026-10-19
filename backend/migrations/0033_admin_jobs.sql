-- 通用后台运维任务（gj.md 阶段二：任务重试与失败详情）
--
-- 设计要点：
--   * 一个表覆盖所有「耗时 / 可能失败 / 需要重试」的后台运维动作，避免每种任务建一张表。
--   * 失败可自动重试（attempts < max_attempts 时回到 pending 并延迟 next_run_at），
--     也可由运营手动「重试」——手动重试会新建一行并记录 retry_of，保留原始失败现场。
--   * result / last_error 面向运营展示，写入前必须脱敏（不含 SQL、堆栈、密钥）。
CREATE TABLE IF NOT EXISTS admin_jobs (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    type          varchar(60) NOT NULL,
    status        varchar(20) NOT NULL DEFAULT 'pending',
    payload       jsonb NOT NULL DEFAULT '{}'::jsonb,
    result        jsonb NOT NULL DEFAULT '{}'::jsonb,
    attempts      integer NOT NULL DEFAULT 0,
    max_attempts  integer NOT NULL DEFAULT 3,
    last_error    text NOT NULL DEFAULT '',
    retry_of      uuid REFERENCES admin_jobs(id) ON DELETE SET NULL,
    created_by    varchar(80) NOT NULL DEFAULT '',
    next_run_at   timestamptz NOT NULL DEFAULT now(),
    started_at    timestamptz,
    finished_at   timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT admin_jobs_status_check
        CHECK (status IN ('pending', 'running', 'succeeded', 'failed', 'cancelled'))
);

-- 调度热路径：只扫 pending 且到点的任务。
CREATE INDEX IF NOT EXISTS idx_admin_jobs_dispatch
    ON admin_jobs (next_run_at) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_admin_jobs_created
    ON admin_jobs (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_admin_jobs_type_status
    ON admin_jobs (type, status);

-- 新增权限点 ops.jobs，并同步存量内置 admin 角色（幂等）。
UPDATE roles
SET permissions = permissions || '["ops.jobs"]'::jsonb,
    updated_at = now()
WHERE name = 'admin'
  AND NOT (permissions @> '["ops.jobs"]'::jsonb);
