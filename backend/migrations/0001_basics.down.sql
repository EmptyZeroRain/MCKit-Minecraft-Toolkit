-- 0001 反向迁移：删除基础表（按外键依赖顺序）。
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS tags;
DROP TABLE IF EXISTS categories;
DROP TABLE IF EXISTS admins;
