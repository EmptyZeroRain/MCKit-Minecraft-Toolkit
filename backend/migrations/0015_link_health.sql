-- 0015 资源链接健康与人工确认（TODO §6：失效链接后台复查与人工确认流程）
--
-- 设计说明：
--   * resource_link_checks 一行 = 一个资源的**最新一次**链接检查结果与复核状态，
--     以 resource_id 唯一键 upsert，避免堆积历史噪声；变更历史由审计日志（audit_logs）承担。
--   * status 复用 domain 的链接状态枚举（ok / redirected / broken / blocked / error），
--     与资源表的 link_status 同口径，前端无需二次映射。
--   * review_state 表达「人工确认」流程：
--       pending          —— 自动复查发现异常，等待人工确认（默认）
--       confirmed_broken —— 人工确认链接已失效（应下架或替换来源）
--       confirmed_ok     —— 人工确认链接实际可用（自动检查误报，例如需要 UA / 反爬）
--       ignored          —— 人工确认后决定暂不处理（保留记录，不反复告警）
--     确认动作记录审核人（reviewed_by）、确认时间（reviewed_at）与备注（review_note）。
--   * trigger_source 区分来源：auto（后台批量复查）/ manual（管理员单条触发）。
--
-- 回滚策略（反向 SQL，按需手工执行）：
--   DROP TABLE IF EXISTS resource_link_checks;

CREATE TABLE IF NOT EXISTS resource_link_checks (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    resource_id   UUID NOT NULL UNIQUE REFERENCES resources (id) ON DELETE CASCADE,
    url           TEXT NOT NULL DEFAULT '',
    status        VARCHAR(20) NOT NULL DEFAULT 'unchecked',
    status_code   INTEGER NOT NULL DEFAULT 0,
    final_url     TEXT NOT NULL DEFAULT '',
    trigger_source VARCHAR(10) NOT NULL DEFAULT 'auto',
    review_state  VARCHAR(20) NOT NULL DEFAULT 'pending',
    reviewed_by   VARCHAR(120) NOT NULL DEFAULT '',
    reviewed_at   TIMESTAMPTZ,
    review_note   TEXT NOT NULL DEFAULT '',
    checked_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT resource_link_checks_status_check
        CHECK (status IN ('unchecked', 'ok', 'redirected', 'broken', 'blocked', 'error')),
    CONSTRAINT resource_link_checks_review_check
        CHECK (review_state IN ('pending', 'confirmed_broken', 'confirmed_ok', 'ignored')),
    CONSTRAINT resource_link_checks_source_check
        CHECK (trigger_source IN ('auto', 'manual'))
);

-- 复核列表：按复核状态 + 链接状态筛选，最近检查在前。
CREATE INDEX IF NOT EXISTS idx_resource_link_checks_review
    ON resource_link_checks (review_state, status, checked_at DESC);

-- 资源反向关联（详情页展示其最新检查）。
CREATE INDEX IF NOT EXISTS idx_resource_link_checks_checked_at
    ON resource_link_checks (checked_at DESC);
