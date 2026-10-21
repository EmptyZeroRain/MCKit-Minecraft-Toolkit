-- 回退 0051：补签卡退出礼品目录，价格迁回 gift_settings。
ALTER TABLE gift_settings
    ADD COLUMN makeup_card_price BIGINT NOT NULL DEFAULT 10
        CHECK (makeup_card_price >= 1 AND makeup_card_price <= 1000000);

UPDATE gift_settings
SET makeup_card_price = COALESCE(
    (SELECT price FROM gift_definitions WHERE kind = 'makeup_card' LIMIT 1),
    makeup_card_price
);

DELETE FROM gift_definitions WHERE kind = 'makeup_card';

DROP INDEX IF EXISTS uq_gift_definitions_makeup_card;

ALTER TABLE gift_definitions DROP COLUMN IF EXISTS kind;
