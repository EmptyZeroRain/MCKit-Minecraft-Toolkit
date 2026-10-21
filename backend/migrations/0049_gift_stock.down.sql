-- 回退 0049：移除礼物库存字段。
ALTER TABLE gift_definitions DROP COLUMN IF EXISTS stock;
