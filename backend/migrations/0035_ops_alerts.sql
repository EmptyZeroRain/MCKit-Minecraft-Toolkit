-- 运营告警与告警历史（gj.md 阶段三：运营告警 / 缓存容量告警）
--
-- 设计：
--   * 规则名为 rule_key，同一规则在「未关闭」期间只允许一条告警（部分唯一索引），
--     避免指标持续异常时每分钟刷一条重复记录；
--   * 指标恢复后由评估器自动关闭（resolved_by = 'system'），运营也可手动确认 / 关闭；
--   * 关闭后的记录保留为历史，不删除，便于回溯「什么时候出过什么问题」。
CREATE TABLE IF NOT EXISTS ops_alerts (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    rule_key      varchar(60) NOT NULL,
    severity      varchar(10) NOT NULL DEFAULT 'warning',
    status        varchar(20) NOT NULL DEFAULT 'open',
    title         varchar(200) NOT NULL,
    detail        text NOT NULL DEFAULT '',
    metric        jsonb NOT NULL DEFAULT '{}'::jsonb,
    dedupe_key    varchar(80) NOT NULL,
    triggered_at  timestamptz NOT NULL DEFAULT now(),
    last_seen_at  timestamptz NOT NULL DEFAULT now(),
    acked_by      varchar(80) NOT NULL DEFAULT '',
    acked_at      timestamptz,
    resolved_by   varchar(80) NOT NULL DEFAULT '',
    resolved_at   timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT ops_alerts_status_check CHECK (status IN ('open', 'acked', 'resolved')),
    CONSTRAINT ops_alerts_severity_check CHECK (severity IN ('warning', 'error'))
);

-- 同一规则只保留一条未关闭告警。
CREATE UNIQUE INDEX IF NOT EXISTS idx_ops_alerts_active
    ON ops_alerts (dedupe_key) WHERE status IN ('open', 'acked');
CREATE INDEX IF NOT EXISTS idx_ops_alerts_history
    ON ops_alerts (triggered_at DESC);

-- 新增权限点 ops.alerts（告警中心），同步存量内置 admin 角色。
UPDATE roles
SET permissions = permissions || '["ops.alerts"]'::jsonb,
    updated_at = now()
WHERE name = 'admin'
  AND NOT (permissions @> '["ops.alerts"]'::jsonb);
