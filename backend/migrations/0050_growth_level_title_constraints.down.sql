-- 回退 0050：移除本迁移新增的无限等级约束。
-- 9999 上限由后续执行的 0047 down 迁移统一恢复，并在此之前收敛超限数据。
ALTER TABLE growth_level_titles
    DROP CONSTRAINT IF EXISTS growth_level_titles_min_level_check;
ALTER TABLE growth_level_titles
    DROP CONSTRAINT IF EXISTS growth_level_titles_max_level_check;
ALTER TABLE growth_level_config
    DROP CONSTRAINT IF EXISTS growth_level_config_max_level_check;
