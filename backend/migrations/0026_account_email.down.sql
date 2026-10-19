-- 0026_account_email 反向迁移：移除账号邮箱列及其唯一索引。
-- 反向迁移会丢弃已填写的邮箱数据，执行前请确认或先备份。

DROP INDEX IF EXISTS idx_accounts_email_unique;
ALTER TABLE accounts DROP COLUMN IF EXISTS email;
