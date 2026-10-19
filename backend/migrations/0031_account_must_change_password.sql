-- 0031 强制改密标记（账号恢复流程）
--
-- 背景（gj.md §10.1）：管理员重置密码后不得向操作者展示新密码以外的秘密，
-- 且应优先采用一次性重置流程或要求首次登录改密。本迁移增加 must_change_password，
-- 管理员重置密码 / 账号恢复时置为 true；账号完成自助改密后清空。
--
-- 事务：本文件由 internal/repository/db.go 在单个事务中执行，失败整体回滚。
-- 回滚：ALTER TABLE accounts DROP COLUMN IF EXISTS must_change_password;

ALTER TABLE accounts ADD COLUMN must_change_password BOOLEAN NOT NULL DEFAULT false;
