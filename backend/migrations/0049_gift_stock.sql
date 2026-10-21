-- 0049 礼物库存。
--
-- stock = 0 表示不限库存；大于 0 时按累计兑换记录数控制剩余数量。
-- 使用累计兑换记录而不是单独的库存计数，避免历史数据与计数不一致。
ALTER TABLE gift_definitions
    ADD COLUMN stock BIGINT NOT NULL DEFAULT 0 CHECK (stock >= 0);
