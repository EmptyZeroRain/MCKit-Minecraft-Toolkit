-- 0047 取消等级上限。
--
-- max_level = 0 表示不设上限，等级仅受数据库 INT 字段范围限制。
-- 已有的 9999 上限配置与用户等级在这里一并放开，并重新按经验曲线计算等级。

-- ============ 1. 放开配置与称号的等级范围 ============

ALTER TABLE growth_level_config
    DROP CONSTRAINT IF EXISTS growth_level_config_max_level_check;
ALTER TABLE growth_level_config
    ADD CONSTRAINT growth_level_config_max_level_check CHECK (max_level >= 0);

ALTER TABLE growth_level_titles
    DROP CONSTRAINT IF EXISTS growth_level_titles_min_level_check;
ALTER TABLE growth_level_titles
    DROP CONSTRAINT IF EXISTS growth_level_titles_max_level_check;
ALTER TABLE growth_level_titles
    ADD CONSTRAINT growth_level_titles_min_level_check CHECK (min_level >= 0);
ALTER TABLE growth_level_titles
    ADD CONSTRAINT growth_level_titles_max_level_check CHECK (max_level >= min_level);

-- 默认改为不限等级；保留当前每级递增值。
INSERT INTO growth_level_config (id, max_level, step_xp)
VALUES (1, 0, 100)
ON CONFLICT (id) DO UPDATE
SET max_level = 0,
    updated_at = now();

-- ============ 2. 按无上限曲线重算用户等级 ============

UPDATE user_growth
SET level_override = LEAST(
        GREATEST(level_override, 0),
        2147483647
    )
WHERE level_override IS NOT NULL;

UPDATE user_growth
SET level = CASE
        WHEN active_days < 7 THEN 0
        WHEN level_override IS NOT NULL THEN level_override
        WHEN current_xp <= 0 THEN 1
        ELSE LEAST(
            2147483647::bigint,
            GREATEST(
                1::bigint,
                floor(
                    (1 + sqrt(
                        1 + 8.0 * current_xp
                            / COALESCE((SELECT step_xp FROM growth_level_config WHERE id = 1), 100)
                    )) / 2
                )::bigint
            )
        )::int
    END;

-- 等级变化后同步 level_reached 成就进度缓存；不主动解锁 / 发奖。
UPDATE achievement_progress ap
SET progress_count = ug.level,
    target_count = ad.target_value,
    updated_at = now()
FROM achievement_definitions ad, user_growth ug
WHERE ap.achievement_id = ad.id
  AND ap.user_id = ug.user_id
  AND ad.condition_type = 'level_reached';
