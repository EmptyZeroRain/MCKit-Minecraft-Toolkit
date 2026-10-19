-- 0022_accounts 反向迁移（整理自正向迁移文件中的「回滚策略」注释）。
-- 反向迁移可能丢弃数据，执行前请确认或先备份。

CREATE TABLE admins (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username      VARCHAR(80) UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    role          VARCHAR(20) NOT NULL,
    is_active     BOOLEAN NOT NULL DEFAULT true,
    last_login_at TIMESTAMPTZ NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE admins ADD CONSTRAINT admins_role_fkey
    FOREIGN KEY (role) REFERENCES roles(name) ON UPDATE CASCADE ON DELETE RESTRICT;
CREATE INDEX idx_admins_username ON admins(username);
CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username      VARCHAR(40) NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    is_active     BOOLEAN NOT NULL DEFAULT true,
    last_login_at TIMESTAMPTZ NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO admins (id, username, password_hash, role, is_active, last_login_at, created_at, updated_at)
    SELECT id, username, password_hash, role, is_active, last_login_at, created_at, updated_at
    FROM accounts WHERE role <> 'user';
INSERT INTO users (id, username, password_hash, is_active, last_login_at, created_at, updated_at)
    SELECT id, username, password_hash, is_active, last_login_at, created_at, updated_at
    FROM accounts WHERE role = 'user';
ALTER TABLE audit_logs DROP CONSTRAINT audit_logs_actor_admin_id_fkey;
ALTER TABLE audit_logs ADD CONSTRAINT audit_logs_actor_admin_id_fkey
    FOREIGN KEY (actor_admin_id) REFERENCES admins(id) ON DELETE SET NULL;
ALTER TABLE resources DROP CONSTRAINT resources_created_by_fkey;
ALTER TABLE resources ADD CONSTRAINT resources_created_by_fkey
    FOREIGN KEY (created_by) REFERENCES admins(id) ON DELETE SET NULL;
ALTER TABLE resources DROP CONSTRAINT resources_updated_by_fkey;
ALTER TABLE resources ADD CONSTRAINT resources_updated_by_fkey
    FOREIGN KEY (updated_by) REFERENCES admins(id) ON DELETE SET NULL;
ALTER TABLE wiki_entries DROP CONSTRAINT wiki_entries_created_by_fkey;
ALTER TABLE wiki_entries ADD CONSTRAINT wiki_entries_created_by_fkey
    FOREIGN KEY (created_by) REFERENCES admins(id) ON DELETE SET NULL;
ALTER TABLE wiki_entries DROP CONSTRAINT wiki_entries_updated_by_fkey;
ALTER TABLE wiki_entries ADD CONSTRAINT wiki_entries_updated_by_fkey
    FOREIGN KEY (updated_by) REFERENCES admins(id) ON DELETE SET NULL;
ALTER TABLE user_favorites DROP CONSTRAINT user_favorites_user_id_fkey;
ALTER TABLE user_favorites ADD CONSTRAINT user_favorites_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE user_recents DROP CONSTRAINT user_recents_user_id_fkey;
ALTER TABLE user_recents ADD CONSTRAINT user_recents_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE user_seeds DROP CONSTRAINT user_seeds_user_id_fkey;
ALTER TABLE user_seeds ADD CONSTRAINT user_seeds_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
DROP TABLE accounts;
