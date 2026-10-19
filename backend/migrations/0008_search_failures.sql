-- 0008: 搜索失败统计（TODO §5「记录……查询失败情况」）
-- 在 search_terms 上增加失败计数列：搜索查询本身失败（如超时/数据库错误）时累加。
-- 仍只保存聚合计数，不含任何用户标识。
--
-- 回滚策略（反向 SQL）：
--   ALTER TABLE search_terms DROP COLUMN IF EXISTS failed_hits;

ALTER TABLE search_terms ADD COLUMN failed_hits BIGINT NOT NULL DEFAULT 0;

CREATE INDEX idx_search_terms_failed ON search_terms(failed_hits DESC);
