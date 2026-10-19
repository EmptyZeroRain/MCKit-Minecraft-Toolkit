-- 0007: 搜索体验（TODO §5）
-- 记录匿名化的搜索词聚合统计：只保存归一化后的关键词与计数，
-- **不保存 IP / 会话 / 用户标识 / 原始时间序列**，避免记录不必要的个人信息。
--
-- 回滚策略（反向 SQL）：DROP TABLE IF EXISTS search_terms;

CREATE TABLE search_terms (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    term              VARCHAR(64) NOT NULL UNIQUE,
    hits              BIGINT NOT NULL DEFAULT 0,   -- 被搜索总次数
    no_result_hits    BIGINT NOT NULL DEFAULT 0,   -- 其中无结果的次数
    last_result_count INT NOT NULL DEFAULT 0,      -- 最近一次的结果数（0 表示无结果）
    first_seen_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_search_terms_hits ON search_terms(hits DESC);
CREATE INDEX idx_search_terms_no_result ON search_terms(no_result_hits DESC);
