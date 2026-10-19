-- 0012 酿造与药水资料库（TODO §9，批次 A：药水与酿造）
--
-- 设计说明：
--   * potions 一行 = 一种药水；效果按时长变体（普通/延长/增强）逐条存于 effects JSONB，
--     与本站既有 JSONB 用法（aliases / supported_versions）保持一致，避免为"变体"建第三张表；
--   * brewing_recipes 一行 = 一条酿造步骤（基底→效果→腐化→通用修饰/形态规则）；
--     通用规则（红石延长、萤石增强、火药喷溅、龙息滞留）用 input_potion_slug/result_potion_slug
--     为 NULL 表示"作用于任意适用药水"，避免为每种药水复制 4 条相同规则；
--   * 版本适用范围使用"自某版本起"（如 1.8+）的语义化声明，由 versioning 校验合法性；
--     查询侧用 versioning.Satisfies 做精确判断（1.20 可命中 1.8+，但不命中 1.21+），
--     因此药水页的版本筛选比工具/资源的"声明文本匹配"更精确。
--
-- 回滚策略（反向 SQL，按需手工执行）：
--   DROP TABLE IF EXISTS brewing_recipes;
--   DROP TABLE IF EXISTS potions;

CREATE TABLE potions (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug               VARCHAR(160) UNIQUE NOT NULL,
    name_zh            VARCHAR(160) NOT NULL,
    name_en            VARCHAR(160) NULL,
    aliases            JSONB NOT NULL DEFAULT '[]',
    category           VARCHAR(40) NOT NULL DEFAULT 'effect',
    summary            TEXT NOT NULL DEFAULT '',
    effect_note        TEXT NOT NULL DEFAULT '',
    is_brewable        BOOLEAN NOT NULL DEFAULT true,
    is_extendable      BOOLEAN NOT NULL DEFAULT false,
    is_enhanceable     BOOLEAN NOT NULL DEFAULT false,
    effects            JSONB NOT NULL DEFAULT '[]',
    game_edition       VARCHAR(20) NOT NULL DEFAULT 'java',
    supported_versions JSONB NOT NULL DEFAULT '[]',
    source_url         TEXT NULL,
    data_version       VARCHAR(80) NULL,
    last_verified_at   TIMESTAMPTZ NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT potions_category_check
        CHECK (category IN ('water', 'base', 'positive', 'negative', 'mixed', 'other')),
    CONSTRAINT potions_edition_check
        CHECK (game_edition IN ('java', 'bedrock', 'both'))
);

CREATE INDEX idx_potions_name_zh ON potions(name_zh);
CREATE INDEX idx_potions_category ON potions(category);

-- 酿造步骤：原料物品 → 输入药水 → 结果药水。
-- input_potion_slug 为 NULL 表示"从水瓶起步"；result_potion_slug 为 NULL 表示通用修饰/形态规则。
CREATE TABLE brewing_recipes (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    recipe_key         VARCHAR(240) UNIQUE NOT NULL,
    step_kind          VARCHAR(40) NOT NULL,
    input_potion_slug  VARCHAR(160) NULL,
    ingredient_item_id VARCHAR(200) NOT NULL,
    ingredient_name_zh VARCHAR(160) NOT NULL,
    result_potion_slug VARCHAR(160) NULL,
    note               TEXT NOT NULL DEFAULT '',
    game_edition       VARCHAR(20) NOT NULL DEFAULT 'java',
    supported_versions JSONB NOT NULL DEFAULT '[]',
    source_url         TEXT NULL,
    sort_order         INT NOT NULL DEFAULT 0,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT brewing_step_kind_check
        CHECK (step_kind IN ('base', 'effect', 'corrupt', 'modifier', 'form')),
    CONSTRAINT brewing_edition_check
        CHECK (game_edition IN ('java', 'bedrock', 'both'))
);

CREATE INDEX idx_brewing_result ON brewing_recipes(result_potion_slug);
CREATE INDEX idx_brewing_input ON brewing_recipes(input_potion_slug);
CREATE INDEX idx_brewing_kind ON brewing_recipes(step_kind);
