-- 0044 回滚：恢复逐级等级表，移除曲线配置与称号规则。
--
-- 回滚为尽力恢复：新曲线下产生的 Lv.11+ 展示等级会按旧阈值表重新收敛，
-- 无法还原被删除的旧自定义等级号 / 阈值。需要保留旧配置请先导出。

-- ============ 1. 恢复旧等级表 ============

CREATE TABLE growth_levels (
    level        INT PRIMARY KEY CHECK (level >= 0),
    threshold_xp BIGINT      NOT NULL CHECK (threshold_xp >= 0),
    title        VARCHAR(100) NOT NULL,
    enabled      BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX ux_growth_levels_positive_threshold
    ON growth_levels(threshold_xp)
    WHERE threshold_xp > 0;

INSERT INTO growth_levels (level, threshold_xp, title, enabled) VALUES
    (0,  0,    '访客',       TRUE),
    (1,  0,    '初来乍到',   TRUE),
    (2,  100,  '方块学徒',   TRUE),
    (3,  300,  '探索者',     TRUE),
    (4,  600,  '熟练玩家',   TRUE),
    (5,  1000, '资深玩家',   TRUE),
    (6,  1600, '资源猎人',   TRUE),
    (7,  2400, '红石专家',   TRUE),
    (8,  3400, '资深作者',   TRUE),
    (9,  4600, '工匠大师',   TRUE),
    (10, 6000, 'MCkit 传奇', TRUE);

-- 按旧阈值表收敛展示等级；人工覆盖也限制在旧等级范围内。
UPDATE user_growth
SET level_override = LEAST(GREATEST(level_override, 0), 10)
WHERE level_override IS NOT NULL;

UPDATE user_growth g
SET level = CASE
        WHEN g.active_days < 7 THEN 0
        WHEN g.level_override IS NOT NULL THEN g.level_override
        ELSE COALESCE(
            (SELECT max(l.level)
             FROM growth_levels l
             WHERE l.enabled AND l.threshold_xp <= g.current_xp),
            0
        )
    END;

-- 同步 level_reached 成就进度缓存。
UPDATE achievement_progress ap
SET progress_count = ug.level,
    target_count = ad.target_value,
    updated_at = now()
FROM achievement_definitions ad, user_growth ug
WHERE ap.achievement_id = ad.id
  AND ap.user_id = ug.user_id
  AND ad.condition_type = 'level_reached';

-- ============ 2. 移除新配置 ============

DROP TRIGGER IF EXISTS trg_growth_level_titles_check_conflict ON growth_level_titles;
DROP FUNCTION IF EXISTS growth_level_titles_check_conflict();
DROP TABLE IF EXISTS growth_level_titles;
DROP TABLE IF EXISTS growth_level_config;
