-- 0001: 基础表：admins、categories、tags、audit_logs
-- 所有表使用 UUID 主键，时间字段使用 timestamptz。

-- 管理员
CREATE TABLE admins (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username      VARCHAR(80) UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    role          VARCHAR(20) NOT NULL CHECK (role IN ('admin', 'editor')),
    is_active     BOOLEAN NOT NULL DEFAULT true,
    last_login_at TIMESTAMPTZ NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_admins_username ON admins(username);

-- 分类（按内容类型区分：tool/resource/wiki）
CREATE TABLE categories (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name         VARCHAR(120) NOT NULL,
    slug         VARCHAR(180) NOT NULL,
    content_type VARCHAR(40) NOT NULL CHECK (content_type IN ('tool', 'resource', 'wiki')),
    sort_order   INT NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (content_type, slug)
);

-- 标签（全站通用）
CREATE TABLE tags (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       VARCHAR(120) NOT NULL,
    slug       VARCHAR(180) UNIQUE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 审计日志
CREATE TABLE audit_logs (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_admin_id UUID NULL REFERENCES admins(id) ON DELETE SET NULL,
    action         VARCHAR(80) NOT NULL,
    object_type    VARCHAR(80) NOT NULL,
    object_id      UUID NULL,
    before_summary JSONB NULL,
    after_summary  JSONB NULL,
    request_id     VARCHAR(100) NULL,
    source_ip      INET NULL,
    result         VARCHAR(20) NOT NULL CHECK (result IN ('success', 'failure')),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_audit_logs_actor ON audit_logs(actor_admin_id);
CREATE INDEX idx_audit_logs_object ON audit_logs(object_type, object_id);
CREATE INDEX idx_audit_logs_created ON audit_logs(created_at DESC);