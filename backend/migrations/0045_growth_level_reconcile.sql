-- 0045 修复「0044 已被记录为已应用、但新表未真正建立」的实例。
--
-- 背景：
--   0044 曾在已部署数据库中应用过一次，随后迁移文件内容被改写为「无限等级」版本，
--   但迁移执行器只按版本号判断是否已应用，导致：
--     * schema_migrations 已有 version = 44；
--     * growth_level_config / growth_level_titles 实际不存在；
--     * 旧 growth_levels 仍然存在；
--     * 成长后台列表等接口查询 growth_level_config 时直接 500。
--
-- 本迁移做幂等修复：
--   1. 缺少配置表 / 称号表时补建；
--   2. 若旧 growth_levels 仍在且称号表为空，按 0044 的规则回填称号；
--   3. 按当前曲线重算用户等级与 level_reached 成就进度；
--   4. 删除旧 growth_levels。
-- 对已经完整执行新 0044 的实例，本迁移只做存在性检查，不会重复建表或覆盖配置。

-- ============ 1. 等级曲线配置 ============

CREATE TABLE IF NOT EXISTS growth_level_config (
    id         INT PRIMARY KEY CHECK (id = 1),
    max_level  INT    NOT NULL DEFAULT 9999 CHECK (max_level >= 1 AND max_level <= 9999),
    step_xp    BIGINT NOT NULL DEFAULT 100  CHECK (step_xp > 0 AND step_xp <= 1000000000),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO growth_level_config (id, max_level, step_xp)
VALUES (1, 9999, 100)
ON CONFLICT (id) DO NOTHING;

-- ============ 2. 等级称号规则 ============

CREATE TABLE IF NOT EXISTS growth_level_titles (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    min_level  INT         NOT NULL CHECK (min_level >= 0 AND min_level <= 9999),
    max_level  INT         NOT NULL CHECK (max_level >= min_level AND max_level <= 9999),
    title      VARCHAR(100) NOT NULL CHECK (length(btrim(title)) > 0),
    enabled    BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_growth_level_titles_exact
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

DROP TRIGGER IF EXISTS trg_growth_level_titles_check_conflict ON growth_level_titles;
CREATE TRIGGER trg_growth_level_titles_check_conflict
    BEFORE INSERT OR UPDATE ON growth_level_titles
    FOR EACH ROW EXECUTE FUNCTION growth_level_titles_check_conflict();

-- ============ 3. 从旧等级表回填称号规则 ============

-- 仅在旧表仍存在且称号表为空时执行，避免覆盖已存在的运营配置。
DO $$
BEGIN
    IF to_regclass('growth_levels') IS NOT NULL
       AND NOT EXISTS (SELECT 1 FROM growth_level_titles) THEN
        -- 若旧 Lv.1-10 仍是默认十级称号，统一为「初来乍到」。
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
    END IF;
END $$;

-- 兜底：确保 Lv.0 有「访客」称号。
INSERT INTO growth_level_titles (min_level, max_level, title, enabled)
SELECT 0, 0, '访客', TRUE
WHERE NOT EXISTS (
    SELECT 1 FROM growth_level_titles
    WHERE enabled AND min_level <= 0 AND max_level >= 0
);

-- 保留 Lv.100 里程碑称号；若已有规则覆盖 Lv.100 则不重复插入。
INSERT INTO growth_level_titles (min_level, max_level, title, enabled)
SELECT 100, 100, 'MCkit 传奇', TRUE
WHERE NOT EXISTS (
    SELECT 1 FROM growth_level_titles
    WHERE enabled AND min_level <= 100 AND max_level >= 100
);

-- ============ 4. 按当前曲线重算用户等级 ============

UPDATE user_growth
SET level_override = LEAST(
        GREATEST(level_override, 0),
        COALESCE((SELECT max_level FROM growth_level_config WHERE id = 1), 9999)
    )
WHERE level_override IS NOT NULL;

UPDATE user_growth
SET level = CASE
        WHEN active_days < 7 THEN 0
        WHEN level_override IS NOT NULL THEN level_override
        WHEN current_xp <= 0 THEN 1
        ELSE LEAST(
            COALESCE((SELECT max_level FROM growth_level_config WHERE id = 1), 9999),
            GREATEST(
                1,
                floor(
                    (1 + sqrt(
                        1 + 8.0 * current_xp
                            / COALESCE((SELECT step_xp FROM growth_level_config WHERE id = 1), 100)
                    )) / 2
                )::int
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

DROP TABLE IF EXISTS growth_levels;
