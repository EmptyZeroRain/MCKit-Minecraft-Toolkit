-- 回退 0047：恢复 Lv.9999 上限。
--
-- 超过 9999 的等级 / 称号会被收敛到 9999，回退是有损的。

UPDATE growth_level_config
SET max_level = 9999,
    updated_at = now()
WHERE id = 1;

UPDATE growth_level_titles
SET min_level = LEAST(min_level, 9999),
    max_level = LEAST(max_level, 9999),
    updated_at = now()
WHERE min_level > 9999 OR max_level > 9999;

UPDATE user_growth
SET level = LEAST(level, 9999)
WHERE level > 9999;

UPDATE user_growth
SET level_override = LEAST(level_override, 9999)
WHERE level_override > 9999;

ALTER TABLE growth_level_config
    DROP CONSTRAINT IF EXISTS growth_level_config_max_level_check;
ALTER TABLE growth_level_config
    ADD CONSTRAINT growth_level_config_max_level_check CHECK (max_level >= 1 AND max_level <= 9999);

ALTER TABLE growth_level_titles
    DROP CONSTRAINT IF EXISTS growth_level_titles_min_level_check;
ALTER TABLE growth_level_titles
    DROP CONSTRAINT IF EXISTS growth_level_titles_max_level_check;
ALTER TABLE growth_level_titles
    ADD CONSTRAINT growth_level_titles_min_level_check CHECK (min_level >= 0 AND min_level <= 9999);
ALTER TABLE growth_level_titles
    ADD CONSTRAINT growth_level_titles_max_level_check CHECK (max_level >= min_level AND max_level <= 9999);
