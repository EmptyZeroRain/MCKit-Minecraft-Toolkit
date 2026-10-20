-- 0043 回滚：移除 Lv.0 活跃天数与人工覆盖，恢复等级偏移列。
--
-- 回滚为尽力恢复：level_override 无法还原为“基础等级 + 偏移”的历史值，
-- 这里统一清零；访客 Lv.0 会回退为 Lv.1。需要保留的人工调整请在回滚前导出。

DROP INDEX IF EXISTS idx_growth_events_actor_status_occurred;
DROP INDEX IF EXISTS ux_growth_levels_positive_threshold;

-- 先把 Lv.0 用户抬回 Lv.1，再恢复等级号检查。
UPDATE user_growth SET level = 1 WHERE level < 1;

DELETE FROM growth_levels WHERE level = 0;

ALTER TABLE growth_levels
    DROP CONSTRAINT IF EXISTS growth_levels_level_check;
ALTER TABLE growth_levels
    ADD CONSTRAINT growth_levels_level_check CHECK (level >= 1);
ALTER TABLE growth_levels
    ADD CONSTRAINT growth_levels_threshold_xp_key UNIQUE (threshold_xp);

ALTER TABLE user_growth
    ADD COLUMN level_offset INT NOT NULL DEFAULT 0;

ALTER TABLE user_growth
    DROP COLUMN level_override,
    DROP COLUMN active_days;

ALTER TABLE user_growth
    ALTER COLUMN level SET DEFAULT 1;
ALTER TABLE user_growth
    DROP CONSTRAINT IF EXISTS user_growth_level_check;
ALTER TABLE user_growth
    ADD CONSTRAINT user_growth_level_check CHECK (level >= 1);
