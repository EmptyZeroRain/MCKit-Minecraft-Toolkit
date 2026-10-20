-- 0043 成长等级 Lv.0「访客」与活跃天数
--
-- 设计说明：
--   * Lv.0「访客」是默认展示等级；新用户累计满 7 个活跃日后才进入 Lv.1 经验等级。
--   * active_days 是活跃天数缓存：从 growth_events 按业务时区去重统计，
--     达到 7 后不再增长；只读页面刷新不会产生成长事件，因此不会增加活跃天数。
--   * 人工等级调整由“经验等级 + 偏移”改为“直接覆盖”：level_override 非空时
--     固定展示该等级，经验继续累积但不自动改级；存量非零偏移用户迁移为覆盖值。
--   * Lv.0 与 Lv.1 都是 0 XP，因此删除 threshold_xp 的全局唯一约束，
--     改为只对正阈值建唯一索引；等级号检查放宽到 >= 0。
--
-- 注意：活跃日回填使用站点默认业务时区 Asia/Shanghai。若部署使用其他时区，
-- 迁移完成后可调用后台“重算等级”或等待用户产生新事件，由服务按实际时区刷新。

-- ============ 1. user_growth：活跃天数 + 人工覆盖 ============

ALTER TABLE user_growth
    ADD COLUMN active_days INT NOT NULL DEFAULT 0 CHECK (active_days >= 0),
    ADD COLUMN level_override INT NULL CHECK (level_override >= 0);

-- 回填活跃天数：同一业务日期至少一个已处理事件计 1 天，封顶 7 天。
WITH active AS (
    SELECT actor_user_id,
           LEAST(COUNT(DISTINCT (occurred_at AT TIME ZONE 'Asia/Shanghai')::date), 7) AS days
    FROM growth_events
    WHERE actor_user_id IS NOT NULL
      AND status = 'processed'
    GROUP BY actor_user_id
)
UPDATE user_growth g
SET active_days = COALESCE(a.days, 0)
FROM active a
WHERE a.actor_user_id = g.user_id;

-- 存量非零偏移且已满 7 天的用户：把当前展示等级固化为人工覆盖值。
-- 未满 7 天的用户按访客规则处理，人工偏移不生效。
UPDATE user_growth
SET level_override = level
WHERE level_offset <> 0
  AND active_days >= 7;

-- 旧约束要求 level >= 1，必须先移除，否则下面的访客回填会失败。
ALTER TABLE user_growth
    DROP CONSTRAINT IF EXISTS user_growth_level_check;

-- 访客阶段统一展示 Lv.0；已满 7 天用户保留当前经验等级。
UPDATE user_growth
SET level = 0
WHERE active_days < 7;

ALTER TABLE user_growth
    DROP COLUMN level_offset,
    ALTER COLUMN level SET DEFAULT 0;

ALTER TABLE user_growth
    ADD CONSTRAINT user_growth_level_check CHECK (level >= 0);

-- ============ 2. growth_levels：允许 Lv.0 与双 0 XP ============

ALTER TABLE growth_levels
    DROP CONSTRAINT IF EXISTS growth_levels_threshold_xp_key;
ALTER TABLE growth_levels
    DROP CONSTRAINT IF EXISTS growth_levels_level_check;
ALTER TABLE growth_levels
    ADD CONSTRAINT growth_levels_level_check CHECK (level >= 0);

INSERT INTO growth_levels (level, threshold_xp, title, enabled)
VALUES (0, 0, '访客', TRUE)
ON CONFLICT (level) DO NOTHING;

-- 正阈值仍然全局唯一；0 XP 允许 Lv.0 / Lv.1 同时存在。
CREATE UNIQUE INDEX IF NOT EXISTS ux_growth_levels_positive_threshold
    ON growth_levels(threshold_xp)
    WHERE threshold_xp > 0;

-- ============ 3. 活跃日统计索引 ============

CREATE INDEX IF NOT EXISTS idx_growth_events_actor_status_occurred
    ON growth_events(actor_user_id, status, occurred_at);
