-- 回滚 0031：删除强制改密标记。
ALTER TABLE accounts DROP COLUMN IF EXISTS must_change_password;
