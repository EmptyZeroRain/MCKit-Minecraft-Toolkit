-- 0026 账号邮箱（accounts.email）
--
-- 背景：注册仅收用户名 + 密码，既不能作为登录标识，也无法承载后续的账号找回。
-- 本迁移为统一账号表补充 email：
--   * 普通用户注册必填；后台账号可留空，不影响既有数据；
--   * 全局唯一且大小写不敏感（服务端统一按小写存储，故索引直接建在 email 列）；
--   * 空串（未填写）不参与唯一性判定，避免历史账号与后台账号互相冲突。
--
-- 事务：本文件由 internal/repository/db.go 在单个事务中执行，失败整体回滚。

ALTER TABLE accounts ADD COLUMN email VARCHAR(254) NOT NULL DEFAULT '';

-- 部分唯一索引：只约束「已填写」的邮箱。
CREATE UNIQUE INDEX idx_accounts_email_unique ON accounts (email) WHERE email <> '';
