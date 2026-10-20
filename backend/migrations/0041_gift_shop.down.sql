-- 0041 回滚：恢复单一礼物币余额
--
-- 先把签到币余额并入通用币，避免回滚丢失用户资产；再移除目录 / 兑换 / 签到表。

DROP TABLE IF EXISTS gift_settings;
DROP TABLE IF EXISTS gift_checkins;
DROP TABLE IF EXISTS gift_redemptions;
DROP TABLE IF EXISTS gift_definitions;

DROP INDEX IF EXISTS idx_gift_ledger_coin_type;
ALTER TABLE gift_ledger DROP COLUMN IF EXISTS coin_type;
ALTER TABLE gift_ledger RENAME COLUMN coin_balance_after TO balance_after;

UPDATE gift_wallets SET general_balance = general_balance + checkin_balance;
ALTER TABLE gift_wallets DROP COLUMN IF EXISTS checkin_balance;
ALTER TABLE gift_wallets RENAME COLUMN general_balance TO balance;
