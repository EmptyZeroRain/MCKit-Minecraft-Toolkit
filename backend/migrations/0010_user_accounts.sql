-- 0010 普通用户账号与个人数据（收藏 / 最近使用 / 常用种子）
--
-- 设计说明：
--   * 与管理员（admins）**完全分离**的表体系，普通用户不会获得任何后台权限；
--   * 密码沿用 Argon2id 哈希（与管理员一致），仅存哈希；
--   * 个人数据通过 user_id 外键 + ON DELETE CASCADE 级联，删除账号即清除全部个人数据；
--   * 唯一约束保证同一用户对同一对象不重复收藏 / 记录。
-- 回滚：
--   DROP TABLE IF EXISTS user_seeds, user_recents, user_favorites, users;

CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username      VARCHAR(40)  NOT NULL UNIQUE,
    password_hash TEXT         NOT NULL,
    is_active     BOOLEAN      NOT NULL DEFAULT true,
    last_login_at TIMESTAMPTZ  NULL,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- 收藏：kind 取值 tool / item / resource / wiki。
CREATE TABLE user_favorites (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind       VARCHAR(20)  NOT NULL,
    object_id  VARCHAR(200) NOT NULL,
    label      VARCHAR(200) NOT NULL DEFAULT '',
    url        TEXT         NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    UNIQUE (user_id, kind, object_id)
);
CREATE INDEX idx_user_favorites_user ON user_favorites(user_id, created_at DESC);

-- 最近使用：按 (user_id, kind, object_id) 唯一，重复访问仅刷新 visited_at。
CREATE TABLE user_recents (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind       VARCHAR(20)  NOT NULL,
    object_id  VARCHAR(200) NOT NULL,
    label      VARCHAR(200) NOT NULL DEFAULT '',
    url        TEXT         NOT NULL DEFAULT '',
    visited_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    UNIQUE (user_id, kind, object_id)
);
CREATE INDEX idx_user_recents_user ON user_recents(user_id, visited_at DESC);

-- 常用种子：用户保存自己的查询记录（种子为字符串，避免 int64 精度问题）。
CREATE TABLE user_seeds (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name       VARCHAR(120) NOT NULL,
    seed       VARCHAR(64)  NOT NULL,
    note       VARCHAR(500) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX idx_user_seeds_user ON user_seeds(user_id, created_at DESC);
