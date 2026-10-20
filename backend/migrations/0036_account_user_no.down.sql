-- 0036 回滚：移除账号对外展示 ID。
DROP INDEX IF EXISTS idx_accounts_user_no_unique;
ALTER TABLE accounts DROP COLUMN IF EXISTS user_no;
