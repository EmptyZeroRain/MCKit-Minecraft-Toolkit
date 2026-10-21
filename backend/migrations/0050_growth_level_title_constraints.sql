-- 0050 修复称号等级范围约束残留。
--
-- 背景：部分实例的 growth_level_titles 由旧版迁移创建，范围校验是表级
-- CHECK（约束名可能是 growth_level_titles_check）。0047 只按列级约束名
-- 删除，遗漏了这类表级约束，导致后台创建 Lv.99999 称号时仍然触发
-- max_level <= 9999 的旧限制。
--
-- 本迁移扫描并重建所有涉及 min_level / max_level 的 CHECK 约束，
-- 确保无限等级配置下只保留基本范围与顺序校验。

DO $$
DECLARE
    con RECORD;
BEGIN
    FOR con IN
        SELECT c.conname
        FROM pg_constraint c
        WHERE c.conrelid = 'growth_level_titles'::regclass
          AND c.contype = 'c'
          AND (
              pg_get_constraintdef(c.oid) LIKE '%min_level%'
              OR pg_get_constraintdef(c.oid) LIKE '%max_level%'
          )
    LOOP
        EXECUTE format('ALTER TABLE growth_level_titles DROP CONSTRAINT %I', con.conname);
    END LOOP;
END $$;

ALTER TABLE growth_level_titles
    ADD CONSTRAINT growth_level_titles_min_level_check CHECK (min_level >= 0);
ALTER TABLE growth_level_titles
    ADD CONSTRAINT growth_level_titles_max_level_check CHECK (max_level >= min_level);

-- 配置表也可能残留表级 max_level <= 9999 约束，一并清理。
DO $$
DECLARE
    con RECORD;
BEGIN
    FOR con IN
        SELECT c.conname
        FROM pg_constraint c
        WHERE c.conrelid = 'growth_level_config'::regclass
          AND c.contype = 'c'
          AND pg_get_constraintdef(c.oid) LIKE '%max_level%'
    LOOP
        EXECUTE format('ALTER TABLE growth_level_config DROP CONSTRAINT %I', con.conname);
    END LOOP;
END $$;

ALTER TABLE growth_level_config
    ADD CONSTRAINT growth_level_config_max_level_check CHECK (max_level >= 0);
