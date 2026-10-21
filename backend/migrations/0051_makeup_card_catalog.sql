-- 0051 把补签卡纳入礼品目录。
--
-- 背景：补签卡原本是签到体系的内置道具，价格存在 gift_settings.makeup_card_price，
-- 后台只能在「签到设置」里改价，礼品目录里看不到，运营容易找不到。
--
-- 本迁移把补签卡建模为目录项的一种 kind：
--   * kind = 'gift'        普通礼物（可发兑换码 / 有库存）
--   * kind = 'makeup_card' 签到道具（只能签到币购买、购买后进个人库存、不发码）
--
-- 价格以目录行为唯一事实源，旧的 gift_settings.makeup_card_price 迁移后删除。
ALTER TABLE gift_definitions
    ADD COLUMN kind VARCHAR(16) NOT NULL DEFAULT 'gift'
        CHECK (kind IN ('gift', 'makeup_card'));

-- 补签卡是单例道具：允许存在多个普通礼物，但最多只能有一条补签卡目录项。
CREATE UNIQUE INDEX uq_gift_definitions_makeup_card
    ON gift_definitions (kind) WHERE kind = 'makeup_card';

-- 用旧配置的价格种子化补签卡目录项；配置行不存在时回退默认价 10。
INSERT INTO gift_definitions (code, name, description, icon, price, stock, allow_checkin, sort_order, enabled, kind)
SELECT 'makeup_card',
       '补签卡',
       '补签漏签日的经验；只能使用签到礼物币购买，补签不补签到天数、不恢复连续签到。',
       '🩹',
       COALESCE((SELECT makeup_card_price FROM gift_settings LIMIT 1), 10),
       0,
       true,
       0,
       true,
       'makeup_card'
WHERE NOT EXISTS (SELECT 1 FROM gift_definitions WHERE kind = 'makeup_card');

ALTER TABLE gift_settings DROP COLUMN makeup_card_price;
