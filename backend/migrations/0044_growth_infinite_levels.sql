-- 0044 无限等级曲线与称号规则
--
-- 设计说明：
--   * 等级不再逐条存储。Lv.1 - Lv.9999 由曲线自动生成：
--       达到 Lv.N 所需经验 = step_xp * N * (N-1) / 2
--     Lv.0 仍是访客，Lv.1 与 Lv.0 都是 0 XP。
--   * growth_level_config 是单行曲线配置（id 固定为 1），后台可调整
--     max_level / step_xp，调整后由服务层重算用户等级。
--   * growth_level_titles 保存称号规则：
--       - min_level = max_level：指定等级称号，优先级最高；
--       - min_level < max_level：区间称号，启用区间之间不允许重叠。
--   * 旧 growth_levels 中的称号在这里合并为连续区间 / 指定等级后删除。
--     若旧数据仍是 0028 的默认十级称号，则先统一为「初来乍到」，
--     保留此前 Lv.1-10 同称号的展示；若后台已改过称号则原样保留。

-- ============ 1. 等级曲线配置 ============

CREATE TABLE growth_level_config (
    id         INT PRIMARY KEY CHECK (id = 1),
    max_level  INT    NOT NULL DEFAULT 9999 CHECK (max_level >= 1 AND max_level <= 9999),
    step_xp    BIGINT NOT NULL DEFAULT 100  CHECK (step_xp > 0 AND step_xp <= 1000000000),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO growth_level_config (id, max_level, step_xp)
VALUES (1, 9999, 100)
ON CONFLICT (id) DO NOTHING;

-- ============ 2. 等级称号规则 ============

CREATE TABLE growth_level_titles (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    min_level  INT         NOT NULL CHECK (min_level >= 0 AND min_level <= 9999),
    max_level  INT         NOT NULL CHECK (max_level >= min_level AND max_level <= 9999),
    title      VARCHAR(100) NOT NULL CHECK (length(btrim(title)) > 0),
    enabled    BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 指定等级称号不能重复；区间称号由触发器检查重叠。
CREATE UNIQUE INDEX ux_growth_level_titles_exact
    ON growth_level_titles(min_level)
    WHERE min_level = max_level AND enabled;

CREATE OR REPLACE FUNCTION growth_level_titles_check_conflict()
RETURNS trigger AS $$
BEGIN
    IF NOT NEW.enabled THEN
        RETURN NEW;
    END IF;
    IF EXISTS (
        SELECT 1
        FROM growth_level_titles t
        WHERE t.enabled
          AND t.id <> NEW.id
          AND (
              (NEW.min_level = NEW.max_level
                  AND t.min_level = t.max_level
                  AND t.min_level = NEW.min_level)
              OR
              (NEW.min_level <> NEW.max_level
                  AND t.min_level <> t.max_level
                  AND NOT (t.max_level < NEW.min_level OR t.min_level > NEW.max_level))
          )
    ) THEN
        RAISE EXCEPTION 'growth level title conflict'
            USING ERRCODE = '23505';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_growth_level_titles_check_conflict
    BEFORE INSERT OR UPDATE ON growth_level_titles
    FOR EACH ROW EXECUTE FUNCTION growth_level_titles_check_conflict();

-- ============ 3. 从旧等级表回填称号规则 ============

-- 仅当旧 Lv.1-10 仍是 0028 的默认称号时，统一为「初来乍到」；
-- 只要有一级被后台改过，就完整保留旧称号，避免覆盖运营配置。
WITH expected(level, title) AS (
    VALUES
        (1, '初来乍到'), (2, '方块学徒'), (3, '探索者'), (4, '熟练玩家'),
        (5, '资深玩家'), (6, '资源猎人'), (7, '红石专家'), (8, '资深作者'),
        (9, '工匠大师'), (10, 'MCkit 传奇')
), matches AS (
    SELECT count(*) AS matched
    FROM growth_levels l
    JOIN expected e ON e.level = l.level AND e.title = l.title
    WHERE l.level BETWEEN 1 AND 10
      AND l.enabled
)
UPDATE growth_levels
SET title = '初来乍到', updated_at = now()
WHERE level BETWEEN 1 AND 10
  AND (SELECT matched FROM matches) = 10;

-- 连续同称号、同启用状态合并为一个区间；单级自动成为指定等级称号。
INSERT INTO growth_level_titles (min_level, max_level, title, enabled)
SELECT min(level), max(level), title, enabled
FROM (
    SELECT level,
           title,
           enabled,
           level - row_number() OVER (
               PARTITION BY title, enabled
               ORDER BY level
           ) AS grp
    FROM growth_levels
    WHERE level BETWEEN 0 AND 9999
) grouped
GROUP BY title, enabled, grp;

-- 兜底：确保 Lv.0 有「访客」称号。
INSERT INTO growth_level_titles (min_level, max_level, title, enabled)
SELECT 0, 0, '访客', TRUE
WHERE NOT EXISTS (
    SELECT 1 FROM growth_level_titles
    WHERE enabled AND min_level <= 0 AND max_level >= 0
);

-- 保留此前的 Lv.100 里程碑称号；若已有规则覆盖 Lv.100 则不重复插入。
INSERT INTO growth_level_titles (min_level, max_level, title, enabled)
SELECT 100, 100, 'MCkit 传奇', TRUE
WHERE NOT EXISTS (
    SELECT 1 FROM growth_level_titles
    WHERE enabled AND min_level <= 100 AND max_level >= 100
);

-- ============ 4. 按新曲线重算用户等级 ============

UPDATE user_growth
SET level_override = LEAST(GREATEST(level_override, 0), 9999)
WHERE level_override IS NOT NULL;

UPDATE user_growth
SET level = CASE
        WHEN active_days < 7 THEN 0
        WHEN level_override IS NOT NULL THEN level_override
        WHEN current_xp <= 0 THEN 1
        ELSE LEAST(
            9999,
            GREATEST(
                1,
                floor((1 + sqrt(1 + 8.0 * current_xp / 100)) / 2)::int
            )
        )
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

-- ============ 5. 删除旧的逐级等级表 ============

DROP TABLE growth_levels;
