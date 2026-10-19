-- 数据质量问题工单（gj.md 阶段三：分派 / 处理状态 / 直接跳转）
--
-- 背景：数据质量问题由 SQL 实时统计得出，本身不落库；但「谁在跟进、处理到哪一步」
-- 需要持久化，否则每次刷新概览都会丢失上下文。
--
-- 设计：
--   * 以 check_key 为业务主键（一个检查项一条工单），避免同一问题重复建单；
--   * count 是实时值，本表只存最后一次处理时的快照（snapshot_count）用于对比；
--   * 检查项归零后工单不会自动关闭（由运营确认后关闭），避免「抖动」导致误关。
CREATE TABLE IF NOT EXISTS data_quality_issues (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    check_key      varchar(80) NOT NULL,
    status         varchar(20) NOT NULL DEFAULT 'open',
    assignee_id    uuid REFERENCES accounts(id) ON DELETE SET NULL,
    assignee_name  varchar(80) NOT NULL DEFAULT '',
    note           text NOT NULL DEFAULT '',
    snapshot_count bigint NOT NULL DEFAULT 0,
    resolved_at    timestamptz,
    updated_by     varchar(80) NOT NULL DEFAULT '',
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT data_quality_issues_status_check
        CHECK (status IN ('open', 'assigned', 'resolved', 'ignored'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_data_quality_issues_key
    ON data_quality_issues (check_key);
CREATE INDEX IF NOT EXISTS idx_data_quality_issues_status
    ON data_quality_issues (status, updated_at DESC);
