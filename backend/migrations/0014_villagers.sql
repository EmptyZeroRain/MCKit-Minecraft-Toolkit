-- 0014 村民交易资料库（TODO §9，批次 B：村民职业 / 交易等级 / 交易表）
--
-- 设计说明：
--   * villager_professions 一行 = 一个职业（含流浪商人、傻子、失业者）；
--     kind 区分「可交易职业 normal / 流浪商人 wandering / 不参与交易 non_trading」，
--     工作站（job site block）以命名空间物品 ID + 中文名成对保存，便于关联物品详情页。
--   * villager_trade_levels 一行 = 一个职业等级（新手→大师），保存经验门槛与徽章说明；
--     等级定义属于**通用规则**，存库而非硬编码，前端不复制枚举。
--   * villager_trades 一行 = 一条交易：cost_items 为「村民想要」的清单（JSONB 数组，
--     每项 {item_id, name_zh, count_min, count_max}，用区间表达 19–33 这类浮动数量），
--     result_item_id / result_count 为「玩家获得」。方向不单列：哪一侧是绿宝石即可判断买卖，
--     与 Minecraft Wiki 交易表的「Villager wants / Player receives」两列一一对应，避免语义错位。
--   * 颜色 / 生物群系变体（羊毛、地毯、床、陶瓦、旗帜、船、迷之炖菜、药箭等）不逐一展开，
--     合并为单条交易并在 variant_note 说明「颜色随机 / 随生物群系变化」，避免上百条同构噪声。
--   * 版本口径：交易系统自 1.14（Village & Pillage）起为当前形态，故默认声明 1.14+；
--     个别交易（试炼密室地图 1.21+、蜡烛 1.17+、樱花/红树 1.19+/1.20+ 等）单独收紧版本。
--     查询侧沿用 versioning.Satisfies 的**精确版本语义**。
--
-- 回滚策略（反向 SQL，按需手工执行）：
--   DROP TABLE IF EXISTS villager_trades;
--   DROP TABLE IF EXISTS villager_trade_levels;
--   DROP TABLE IF EXISTS villager_professions;

CREATE TABLE villager_professions (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug                VARCHAR(120) UNIQUE NOT NULL,
    name_zh             VARCHAR(120) NOT NULL,
    name_en             VARCHAR(120) NOT NULL DEFAULT '',
    aliases             JSONB NOT NULL DEFAULT '[]',
    kind                VARCHAR(30)  NOT NULL DEFAULT 'normal',
    workstation_item_id VARCHAR(200) NULL,
    workstation_name_zh VARCHAR(160) NOT NULL DEFAULT '',
    summary             TEXT NOT NULL DEFAULT '',
    description         TEXT NOT NULL DEFAULT '',
    trade_note          TEXT NOT NULL DEFAULT '',
    game_edition        VARCHAR(20) NOT NULL DEFAULT 'java',
    supported_versions  JSONB NOT NULL DEFAULT '[]',
    source_url          TEXT NULL,
    data_version        VARCHAR(80) NULL,
    last_verified_at    TIMESTAMPTZ NULL,
    sort_order          INT NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT villager_professions_kind_check
        CHECK (kind IN ('normal', 'wandering', 'non_trading')),
    CONSTRAINT villager_professions_edition_check
        CHECK (game_edition IN ('java', 'bedrock', 'both'))
);

CREATE INDEX idx_villager_professions_kind ON villager_professions(kind);
CREATE INDEX idx_villager_professions_name_zh ON villager_professions(name_zh);

-- 职业等级（通用规则）：新手 → 学徒 → 老手 → 专家 → 大师。
CREATE TABLE villager_trade_levels (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    level               INT UNIQUE NOT NULL,
    slug                VARCHAR(40) UNIQUE NOT NULL,
    name_zh             VARCHAR(40) NOT NULL,
    name_en             VARCHAR(40) NOT NULL DEFAULT '',
    required_experience INT NOT NULL DEFAULT 0,
    badge_note          TEXT NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT villager_trade_levels_level_check CHECK (level BETWEEN 1 AND 5)
);

-- 交易表：cost_items（村民想要）+ result（玩家获得）。
CREATE TABLE villager_trades (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    trade_key            VARCHAR(240) UNIQUE NOT NULL,
    profession_slug      VARCHAR(120) NOT NULL,
    level                INT NOT NULL DEFAULT 0,
    trade_group          VARCHAR(30) NOT NULL DEFAULT 'level',
    cost_items           JSONB NOT NULL DEFAULT '[]',
    result_item_id       VARCHAR(200) NOT NULL,
    result_item_name_zh  VARCHAR(160) NOT NULL,
    result_count         INT NOT NULL DEFAULT 1,
    max_uses             INT NOT NULL DEFAULT 12,
    villager_experience  INT NOT NULL DEFAULT 0,
    price_multiplier     NUMERIC(4,2) NOT NULL DEFAULT 0.05,
    variant_note         TEXT NOT NULL DEFAULT '',
    note                 TEXT NOT NULL DEFAULT '',
    game_edition         VARCHAR(20) NOT NULL DEFAULT 'java',
    supported_versions   JSONB NOT NULL DEFAULT '[]',
    source_url           TEXT NULL,
    sort_order           INT NOT NULL DEFAULT 0,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT villager_trades_group_check
        CHECK (trade_group IN ('level', 'purchase', 'special', 'ordinary')),
    CONSTRAINT villager_trades_level_check CHECK (level BETWEEN 0 AND 5),
    CONSTRAINT villager_trades_edition_check
        CHECK (game_edition IN ('java', 'bedrock', 'both'))
);

CREATE INDEX idx_villager_trades_profession ON villager_trades(profession_slug);
CREATE INDEX idx_villager_trades_level ON villager_trades(level);
CREATE INDEX idx_villager_trades_group ON villager_trades(trade_group);
