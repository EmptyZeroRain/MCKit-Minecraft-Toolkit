-- 0011 使用统计与运营看板（TODO §13）
--
-- 设计说明（隐私优先）：
--   * 全部为**按天聚合**计数，不保存逐次请求明细、IP、会话、用户标识、User-Agent、
--     query、请求体，也不保存可还原「谁在何时做了什么」的时间序列；
--   * 工具 / 资源维度只保存**公开 slug**，不关联访问者身份；
--   * API 指标只保存 chi **路由模板**（如 /api/v1/tools/{slug}），不含具体参数值；
--   * 延迟以固定分桶记录，用桶上界近似分位数，不保存原始逐次耗时；
--   * 日聚合口径：按**服务器本地日期**归组，落库为当日 UTC 零点（DATE 列稳定）。
--   * 保留期限由 STATS_RETENTION_DAYS（默认 180 天）控制，超期由采集器定期清理。
--   * 全表无外键、无用户维度，删号不涉及本表。
-- 回滚：
--   DROP TABLE IF EXISTS search_stats_daily, api_stats_daily,
--     resource_stats_daily, tool_stats_daily;

-- 工具访问 / 使用 / 失败（日聚合，按工具 slug）。
CREATE TABLE tool_stats_daily (
    stat_date DATE         NOT NULL,
    tool_slug VARCHAR(180) NOT NULL,
    visits    BIGINT       NOT NULL DEFAULT 0,
    uses      BIGINT       NOT NULL DEFAULT 0,
    failures  BIGINT       NOT NULL DEFAULT 0,
    PRIMARY KEY (stat_date, tool_slug)
);
CREATE INDEX idx_tool_stats_date ON tool_stats_daily(stat_date);

-- 资源详情访问量（日聚合，按资源 slug）。
CREATE TABLE resource_stats_daily (
    stat_date     DATE         NOT NULL,
    resource_slug VARCHAR(180) NOT NULL,
    views         BIGINT       NOT NULL DEFAULT 0,
    PRIMARY KEY (stat_date, resource_slug)
);
CREATE INDEX idx_resource_stats_date ON resource_stats_daily(stat_date);

-- 关键 API 指标（日聚合，按路由模板 + 方法）。
-- errors：5xx；client_errors：4xx（不含 429）；rate_limited：429（不重复计入 client_errors）。
-- 延迟分桶上界（毫秒）：50 / 200 / 500 / 1000 / 3000，超出计入 latency_gt_3000。
CREATE TABLE api_stats_daily (
    stat_date         DATE         NOT NULL,
    route             VARCHAR(200) NOT NULL,
    method            VARCHAR(10)  NOT NULL,
    total             BIGINT       NOT NULL DEFAULT 0,
    errors            BIGINT       NOT NULL DEFAULT 0,
    client_errors     BIGINT       NOT NULL DEFAULT 0,
    rate_limited      BIGINT       NOT NULL DEFAULT 0,
    latency_ms_sum    BIGINT       NOT NULL DEFAULT 0,
    latency_ms_max    BIGINT       NOT NULL DEFAULT 0,
    latency_le_50     BIGINT       NOT NULL DEFAULT 0,
    latency_le_200    BIGINT       NOT NULL DEFAULT 0,
    latency_le_500    BIGINT       NOT NULL DEFAULT 0,
    latency_le_1000   BIGINT       NOT NULL DEFAULT 0,
    latency_le_3000   BIGINT       NOT NULL DEFAULT 0,
    latency_gt_3000   BIGINT       NOT NULL DEFAULT 0,
    PRIMARY KEY (stat_date, route, method)
);
CREATE INDEX idx_api_stats_date ON api_stats_daily(stat_date);

-- 搜索总量（日聚合）；词级明细仍保存在 search_terms（TODO §5）。
CREATE TABLE search_stats_daily (
    stat_date   DATE   NOT NULL PRIMARY KEY,
    queries     BIGINT NOT NULL DEFAULT 0,
    zero_result BIGINT NOT NULL DEFAULT 0,
    failed      BIGINT NOT NULL DEFAULT 0
);
