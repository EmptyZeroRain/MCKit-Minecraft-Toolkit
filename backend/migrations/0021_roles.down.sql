-- 0021_roles 反向迁移（整理自正向迁移文件中的「回滚策略」注释）。
-- 反向迁移可能丢弃数据，执行前请确认或先备份。

ALTER TABLE admins DROP CONSTRAINT IF EXISTS admins_role_fkey;
ALTER TABLE admins ADD CONSTRAINT admins_role_check CHECK (role IN ('admin', 'editor'));
DROP TABLE IF EXISTS roles;
