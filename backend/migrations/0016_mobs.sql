-- 0016 生物资料库（TODO §9，批次 C：生物掉落 / 生成条件 / 驯服与繁殖）
--
-- 设计说明：
--   * mobs 一行 = 一种生物（含被动 / 中立 / 敌对 / Boss / 功能型）；
--     血量 health 用 Java 版生命值（1 点 = 半颗心）表达，无法确定时为 NULL；
--     攻击伤害因难度与版本差异较大，改用 attack_note 文字说明，不写死单一数字。
--   * mob_drops 一行 = 一条掉落：item_id + 中文名成对保存，数量用区间
--     （count_min / count_max）表达，概率用 chance（0–1，1 表示必定掉落）。
--     掉落物与物品资料库关联：已收录的掉落物在详情页给出物品链接，未收录回退纯文本 ID。
--   * 生成条件、驯服、繁殖、行为差异均以文字保存（spawn_note / taming_note /
--     breeding_note / behavior_note），不虚构数值；维度用 dimensions 数组表达。
--   * 版本口径：沿用 versioning 的「单版本 + 范围」声明；查询侧用精确版本语义。
--
-- 回滚策略（反向 SQL，按需手工执行）：
--   DROP TABLE IF EXISTS mob_drops;
--   DROP TABLE IF EXISTS mobs;

CREATE TABLE IF NOT EXISTS mobs (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug               VARCHAR(120) NOT NULL UNIQUE,
    name_zh            VARCHAR(120) NOT NULL,
    name_en            VARCHAR(120) NOT NULL DEFAULT '',
    aliases            JSONB NOT NULL DEFAULT '[]',
    category           VARCHAR(30) NOT NULL DEFAULT 'other',
    health             NUMERIC(6,1),
    attack_note        TEXT NOT NULL DEFAULT '',
    behavior_note      TEXT NOT NULL DEFAULT '',
    spawn_note         TEXT NOT NULL DEFAULT '',
    taming_note        TEXT NOT NULL DEFAULT '',
    breeding_note      TEXT NOT NULL DEFAULT '',
    dimensions         JSONB NOT NULL DEFAULT '[]',
    biomes_note        TEXT NOT NULL DEFAULT '',
    summary            TEXT NOT NULL DEFAULT '',
    game_edition       VARCHAR(20) NOT NULL DEFAULT 'java',
    supported_versions JSONB NOT NULL DEFAULT '[]',
    source_url         TEXT NOT NULL DEFAULT '',
    data_version       VARCHAR(80) NOT NULL DEFAULT '',
    last_verified_at   TIMESTAMPTZ,
    sort_order         INTEGER NOT NULL DEFAULT 0,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT mobs_category_check
        CHECK (category IN ('passive', 'neutral', 'hostile', 'boss', 'utility', 'other'))
);

CREATE INDEX IF NOT EXISTS idx_mobs_category ON mobs (category, sort_order, name_zh);

CREATE TABLE IF NOT EXISTS mob_drops (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    drop_key           VARCHAR(240) NOT NULL UNIQUE,
    mob_slug           VARCHAR(120) NOT NULL REFERENCES mobs (slug) ON DELETE CASCADE,
    item_id            VARCHAR(200) NOT NULL,
    item_name_zh       VARCHAR(160) NOT NULL,
    count_min          INTEGER NOT NULL DEFAULT 1,
    count_max          INTEGER NOT NULL DEFAULT 1,
    chance             NUMERIC(5,4) NOT NULL DEFAULT 1,
    conditions         TEXT NOT NULL DEFAULT '',
    note               TEXT NOT NULL DEFAULT '',
    sort_order         INTEGER NOT NULL DEFAULT 0,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT mob_drops_count_check CHECK (count_min >= 0 AND count_max >= count_min),
    CONSTRAINT mob_drops_chance_check CHECK (chance >= 0 AND chance <= 1)
);

CREATE INDEX IF NOT EXISTS idx_mob_drops_mob ON mob_drops (mob_slug, sort_order);
