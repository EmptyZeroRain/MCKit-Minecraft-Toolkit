-- 0022 统一账号体系（accounts）
--
-- 背景：0021 引入 roles 后 admins / users 仍是两套表，导致：
--   * 同一站点的账号被硬拆成两表，登录入口与会话互不相通，外键需重复选边；
--   * 「个人数据」类表必须固定指向 users，管理员无法复用同一套能力。
-- 本迁移把两者合并为 accounts：普通用户 = role 为内置角色 'user' 的账号，
-- 管理员 = role 非 'user' 的账号（含自定义角色）。角色决定身份，表不再分家。
--
-- 设计说明：
--   * username 全局唯一（取 admins 的 VARCHAR(80) 宽度），跨身份不可重名；
--   * role 外键引用 roles(name)，默认 'user'（注册普通用户无需显式指定角色）；
--   * 会话 Cookie 合并为单一 SESSION_COOKIE_NAME（原 USER_SESSION_COOKIE_NAME 废弃）；
--   * 审计列名保留 actor_admin_id：语义已变为「操作者账号」，改名会牵动审计查询与前端展示，
--     收益不足，故沿用（见 internal/domain/audit.go 注释）。
--
-- 数据量：开发库 admins 1 行、users 0 行且无重名冲突；测试库由迁移重建，均无风险。
-- 事务：本文件由 internal/repository/db.go 在单个事务中执行，失败整体回滚。
--
-- 回滚（反向 SQL，按需手工执行；执行前请先停止写入）：
--   CREATE TABLE admins (
--       id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
--       username      VARCHAR(80) UNIQUE NOT NULL,
--       password_hash TEXT NOT NULL,
--       role          VARCHAR(20) NOT NULL,
--       is_active     BOOLEAN NOT NULL DEFAULT true,
--       last_login_at TIMESTAMPTZ NULL,
--       created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
--       updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
--   );
--   ALTER TABLE admins ADD CONSTRAINT admins_role_fkey
--       FOREIGN KEY (role) REFERENCES roles(name) ON UPDATE CASCADE ON DELETE RESTRICT;
--   CREATE INDEX idx_admins_username ON admins(username);
--   CREATE TABLE users (
--       id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
--       username      VARCHAR(40) NOT NULL UNIQUE,
--       password_hash TEXT NOT NULL,
--       is_active     BOOLEAN NOT NULL DEFAULT true,
--       last_login_at TIMESTAMPTZ NULL,
--       created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
--       updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
--   );
--   INSERT INTO admins (id, username, password_hash, role, is_active, last_login_at, created_at, updated_at)
--       SELECT id, username, password_hash, role, is_active, last_login_at, created_at, updated_at
--       FROM accounts WHERE role <> 'user';
--   INSERT INTO users (id, username, password_hash, is_active, last_login_at, created_at, updated_at)
--       SELECT id, username, password_hash, is_active, last_login_at, created_at, updated_at
--       FROM accounts WHERE role = 'user';
--   ALTER TABLE audit_logs DROP CONSTRAINT audit_logs_actor_admin_id_fkey;
--   ALTER TABLE audit_logs ADD CONSTRAINT audit_logs_actor_admin_id_fkey
--       FOREIGN KEY (actor_admin_id) REFERENCES admins(id) ON DELETE SET NULL;
--   ALTER TABLE resources DROP CONSTRAINT resources_created_by_fkey;
--   ALTER TABLE resources ADD CONSTRAINT resources_created_by_fkey
--       FOREIGN KEY (created_by) REFERENCES admins(id) ON DELETE SET NULL;
--   ALTER TABLE resources DROP CONSTRAINT resources_updated_by_fkey;
--   ALTER TABLE resources ADD CONSTRAINT resources_updated_by_fkey
--       FOREIGN KEY (updated_by) REFERENCES admins(id) ON DELETE SET NULL;
--   ALTER TABLE wiki_entries DROP CONSTRAINT wiki_entries_created_by_fkey;
--   ALTER TABLE wiki_entries ADD CONSTRAINT wiki_entries_created_by_fkey
--       FOREIGN KEY (created_by) REFERENCES admins(id) ON DELETE SET NULL;
--   ALTER TABLE wiki_entries DROP CONSTRAINT wiki_entries_updated_by_fkey;
--   ALTER TABLE wiki_entries ADD CONSTRAINT wiki_entries_updated_by_fkey
--       FOREIGN KEY (updated_by) REFERENCES admins(id) ON DELETE SET NULL;
--   ALTER TABLE user_favorites DROP CONSTRAINT user_favorites_user_id_fkey;
--   ALTER TABLE user_favorites ADD CONSTRAINT user_favorites_user_id_fkey
--       FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
--   ALTER TABLE user_recents DROP CONSTRAINT user_recents_user_id_fkey;
--   ALTER TABLE user_recents ADD CONSTRAINT user_recents_user_id_fkey
--       FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
--   ALTER TABLE user_seeds DROP CONSTRAINT user_seeds_user_id_fkey;
--   ALTER TABLE user_seeds ADD CONSTRAINT user_seeds_user_id_fkey
--       FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
--   DROP TABLE accounts;

CREATE TABLE accounts (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username      VARCHAR(80)  NOT NULL UNIQUE,
    password_hash TEXT         NOT NULL,
    role          VARCHAR(40)  NOT NULL DEFAULT 'user'
                  REFERENCES roles(name) ON UPDATE CASCADE ON DELETE RESTRICT,
    is_active     BOOLEAN      NOT NULL DEFAULT true,
    last_login_at TIMESTAMPTZ  NULL,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- 管理员侧与普通用户侧列表都按角色过滤，单独建索引避免全表扫描。
CREATE INDEX idx_accounts_role ON accounts(role);
CREATE INDEX idx_accounts_username ON accounts(username);

-- 数据搬迁：先管理员（保留原角色），再普通用户（固定 role='user'）。
INSERT INTO accounts (id, username, password_hash, role, is_active, last_login_at, created_at, updated_at)
SELECT id, username, password_hash, role, is_active, last_login_at, created_at, updated_at
FROM admins;

INSERT INTO accounts (id, username, password_hash, role, is_active, last_login_at, created_at, updated_at)
SELECT id, username, password_hash, 'user', is_active, last_login_at, created_at, updated_at
FROM users;

-- 8 个外键重指 accounts：审计 / 内容创建人可为空（SET NULL），个人数据级联删除（CASCADE）。
ALTER TABLE audit_logs DROP CONSTRAINT IF EXISTS audit_logs_actor_admin_id_fkey;
ALTER TABLE audit_logs ADD CONSTRAINT audit_logs_actor_admin_id_fkey
    FOREIGN KEY (actor_admin_id) REFERENCES accounts(id) ON DELETE SET NULL;

ALTER TABLE resources DROP CONSTRAINT IF EXISTS resources_created_by_fkey;
ALTER TABLE resources ADD CONSTRAINT resources_created_by_fkey
    FOREIGN KEY (created_by) REFERENCES accounts(id) ON DELETE SET NULL;
ALTER TABLE resources DROP CONSTRAINT IF EXISTS resources_updated_by_fkey;
ALTER TABLE resources ADD CONSTRAINT resources_updated_by_fkey
    FOREIGN KEY (updated_by) REFERENCES accounts(id) ON DELETE SET NULL;

ALTER TABLE wiki_entries DROP CONSTRAINT IF EXISTS wiki_entries_created_by_fkey;
ALTER TABLE wiki_entries ADD CONSTRAINT wiki_entries_created_by_fkey
    FOREIGN KEY (created_by) REFERENCES accounts(id) ON DELETE SET NULL;
ALTER TABLE wiki_entries DROP CONSTRAINT IF EXISTS wiki_entries_updated_by_fkey;
ALTER TABLE wiki_entries ADD CONSTRAINT wiki_entries_updated_by_fkey
    FOREIGN KEY (updated_by) REFERENCES accounts(id) ON DELETE SET NULL;

ALTER TABLE user_favorites DROP CONSTRAINT IF EXISTS user_favorites_user_id_fkey;
ALTER TABLE user_favorites ADD CONSTRAINT user_favorites_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES accounts(id) ON DELETE CASCADE;

ALTER TABLE user_recents DROP CONSTRAINT IF EXISTS user_recents_user_id_fkey;
ALTER TABLE user_recents ADD CONSTRAINT user_recents_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES accounts(id) ON DELETE CASCADE;

ALTER TABLE user_seeds DROP CONSTRAINT IF EXISTS user_seeds_user_id_fkey;
ALTER TABLE user_seeds ADD CONSTRAINT user_seeds_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES accounts(id) ON DELETE CASCADE;

-- 旧表下线（数据已完整复制，外键已重指）。
DROP TABLE admins;
DROP TABLE users;
