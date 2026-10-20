-- 0041 礼物商店：礼物目录、兑换、签到币与通用币
--
-- 背景：0039 只有单一礼物币余额。本次把礼物币拆成两类：
--   * checkin_balance：每日签到获得的礼物币；
--   * general_balance：其他来源（活动 / 后台补偿等）的礼物币。
--
-- 礼物可配置「是否允许签到币兑换」：
--   * 允许：优先扣签到币，不足部分再扣通用币；
--   * 不允许：只能扣通用币。
--
-- 兼容性：gift_wallets.balance 重命名为 general_balance，历史流水默认
-- coin_type='general'；所有余额变更仍必须写 gift_ledger。

-- 1) 钱包拆分为两类余额
ALTER TABLE gift_wallets RENAME COLUMN balance TO general_balance;
ALTER TABLE gift_wallets
    ADD COLUMN checkin_balance BIGINT NOT NULL DEFAULT 0
        CHECK (checkin_balance >= 0);

-- 2) 账本记录币种；余额快照按币种解释
ALTER TABLE gift_ledger RENAME COLUMN balance_after TO coin_balance_after;
ALTER TABLE gift_ledger
    ADD COLUMN coin_type VARCHAR(16) NOT NULL DEFAULT 'general'
        CHECK (coin_type IN ('checkin', 'general'));

CREATE INDEX idx_gift_ledger_coin_type ON gift_ledger(user_id, coin_type, created_at DESC);

-- 3) 礼物目录（后台可维护）
CREATE TABLE gift_definitions (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code          VARCHAR(64)  NOT NULL UNIQUE
        CHECK (code ~ '^[a-z][a-z0-9_]{2,63}$'),
    name          VARCHAR(80)  NOT NULL CHECK (length(name) > 0),
    description   VARCHAR(500) NOT NULL DEFAULT '',
    icon          VARCHAR(32)  NOT NULL DEFAULT '🎁',
    price         BIGINT       NOT NULL CHECK (price > 0 AND price <= 1000000),
    allow_checkin BOOLEAN      NOT NULL DEFAULT false,
    sort_order    INTEGER      NOT NULL DEFAULT 0,
    enabled       BOOLEAN      NOT NULL DEFAULT true,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX idx_gift_definitions_enabled_sort ON gift_definitions(enabled, sort_order, created_at);

-- 4) 兑换记录（价格与名称快照，避免目录变更改写历史）
CREATE TABLE gift_redemptions (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id              UUID         NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    gift_id              UUID         NOT NULL REFERENCES gift_definitions(id) ON DELETE RESTRICT,
    gift_code_snapshot   VARCHAR(64)  NOT NULL,
    gift_name_snapshot   VARCHAR(80)  NOT NULL,
    gift_icon_snapshot   VARCHAR(32)  NOT NULL DEFAULT '🎁',
    price                BIGINT       NOT NULL CHECK (price > 0),
    checkin_spent        BIGINT       NOT NULL DEFAULT 0 CHECK (checkin_spent >= 0),
    general_spent        BIGINT       NOT NULL DEFAULT 0 CHECK (general_spent >= 0),
    idempotency_key      VARCHAR(128) NOT NULL,
    created_at           TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CHECK (checkin_spent + general_spent = price),
    UNIQUE (user_id, idempotency_key)
);

CREATE INDEX idx_gift_redemptions_user_created ON gift_redemptions(user_id, created_at DESC);
CREATE INDEX idx_gift_redemptions_gift ON gift_redemptions(gift_id);

-- 5) 每日签到（每人每天一条，日期按站点业务时区计算）
CREATE TABLE gift_checkins (
    user_id      UUID        NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    checkin_date DATE        NOT NULL,
    reward       BIGINT      NOT NULL DEFAULT 0 CHECK (reward >= 0),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, checkin_date)
);

-- 6) 签到奖励配置（单行表）
CREATE TABLE gift_settings (
    id             BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (id),
    checkin_reward BIGINT      NOT NULL DEFAULT 1 CHECK (checkin_reward >= 0 AND checkin_reward <= 1000000),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO gift_settings (id, checkin_reward) VALUES (TRUE, 1)
ON CONFLICT (id) DO NOTHING;

-- 7) 初始礼物目录：签到币可兑换入门三件套（泥土 / 圆石 / 煤炭）。
INSERT INTO gift_definitions (code, name, description, icon, price, allow_checkin, sort_order) VALUES
    ('dirt_block',       '泥土块',       '最基础、低成本的支持，随手可得。',       '🟫', 1,   true,  10),
    ('cobblestone',      '圆石',         '木镐即可获取的基础建材。',               '🪨', 2,   true,  20),
    ('coal',             '煤炭',         '浅层常见的照明与燃料。',                 '⚫', 3,   true,  30),
    ('iron_ingot',       '铁锭',         '石镐起步的装备基础。',                   '⚙️', 5,   false, 40),
    ('copper_ingot',     '铜锭',         '常见但偏装饰，可以氧化。',               '🟠', 8,   false, 50),
    ('redstone',         '红石',         '较深层，技术流与动力。',                 '🔴', 10,  false, 60),
    ('lapis_lazuli',     '青金石',       '较深层，附魔消耗品。',                   '🔵', 15,  false, 70),
    ('gold_ingot',       '金锭',         '主世界稀有、下界常见，交易与贵气。',     '🟡', 20,  false, 80),
    ('emerald',          '绿宝石',       '高山零星生成，村民交易硬通货。',         '💚', 30,  false, 90),
    ('diamond',          '钻石',         '深层稀有，稀有礼物锚点。',               '💎', 50,  false, 100),
    ('amethyst_shard',   '紫水晶碎片',   '紫水晶洞获取，量不稳定，回声主题。',     '🟣', 60,  false, 110),
    ('netherite_ingot',  '下界合金锭',   '远古残骸 + 冶炼 + 锻造，高阶稀有。',     '⬛', 100, false, 120),
    ('nether_star',      '下界之星',     '击败凋灵获得，顶级礼物。',               '⭐', 200, false, 130)
ON CONFLICT (code) DO NOTHING;
