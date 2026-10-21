-- 回退 0046：恢复旧版 0044 的迁移记录名称。
-- 仅用于回滚演练；业务表结构不受影响。

UPDATE schema_migrations
SET name = 'growth_level_titles'
WHERE version = 44
  AND name = 'growth_infinite_levels';
