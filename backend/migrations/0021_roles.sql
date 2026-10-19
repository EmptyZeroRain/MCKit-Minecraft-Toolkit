-- 0021 角色与权限点（后台「角色管理」底座）
--
-- 设计说明：
--   * roles 以 name 为主键（稳定标识，供外键引用），label 为展示名，permissions 为权限点数组（JSONB）；
--   * is_system = true 的内置角色不可删除、不可改名；权限可调整，但 admin 必须始终保留全量权限
--     （由服务层校验，避免把系统改到无人可管理）；
--   * admins.role 由「CHECK (role IN ('admin','editor'))」改为外键引用 roles(name)，
--     使自定义角色可立即分配给管理员（同时为后续统一账号 accounts 铺路）；
--   * 内置角色：admin（全量权限）、editor（内容管理 + 反馈）、user（前台基础，无后台权限点）。
-- 回滚：
--   ALTER TABLE admins DROP CONSTRAINT IF EXISTS admins_role_fkey;
--   ALTER TABLE admins ADD CONSTRAINT admins_role_check CHECK (role IN ('admin', 'editor'));
--   DROP TABLE IF EXISTS roles;

CREATE TABLE roles (
    name        VARCHAR(40)  PRIMARY KEY,
    label       VARCHAR(80)  NOT NULL,
    description TEXT         NOT NULL DEFAULT '',
    permissions JSONB        NOT NULL DEFAULT '[]'::jsonb,
    is_system   BOOLEAN      NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);

INSERT INTO roles (name, label, description, permissions, is_system) VALUES
    (
        'admin',
        '管理员',
        '内置角色，拥有全部权限；不可删除、不可改名，且必须保留全量权限。',
        '["content.resources","content.tools","content.wiki","content.categories","content.tags",
          "content.feedback","review.redstone","review.mods","accounts.admins","accounts.users",
          "roles.manage","ops.audit","ops.dataquality","ops.searchstats","ops.analytics",
          "ops.linkhealth","ops.mcsrc"]'::jsonb,
        true
    ),
    (
        'editor',
        '编辑',
        '内置角色，负责内容管理与反馈处理，不含账号 / 权限 / 系统运维能力。',
        '["content.resources","content.tools","content.wiki","content.categories","content.tags",
          "content.feedback"]'::jsonb,
        true
    ),
    (
        'user',
        '普通用户',
        '内置角色，前台账号基础身份，不含任何后台权限点。',
        '[]'::jsonb,
        true
    );

-- admins.role：CHECK 约束 -> 外键引用 roles(name)
ALTER TABLE admins DROP CONSTRAINT IF EXISTS admins_role_check;
ALTER TABLE admins
    ADD CONSTRAINT admins_role_fkey FOREIGN KEY (role) REFERENCES roles(name)
    ON UPDATE CASCADE ON DELETE RESTRICT;
