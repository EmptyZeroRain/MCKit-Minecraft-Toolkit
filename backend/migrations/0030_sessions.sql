-- 0030 共享会话存储（多实例部署）
--
-- 背景（gj.md §3.4）：原会话仅存进程内存，进程重启即失效，且多实例之间不一致。
-- 本迁移提供共享会话表，使多实例部署下会话创建 / 续期 / 撤销即时一致。
--
-- 设计说明：
--   * account_id 外键指向 accounts(id) 并 ON DELETE CASCADE：删除账号自动清理其会话，
--     无需应用层额外兜底（gj.md §10.1）。
--   * expires_at 为绝对过期时间；last_seen_at 记录最近活动时间，用于空闲超时。
--   * 单实例部署仍可使用内存存储，本表可为空。
-- 回滚：DROP TABLE IF EXISTS sessions;

CREATE TABLE sessions (
    session_id           VARCHAR(128) PRIMARY KEY,
    account_id           UUID         NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    username             VARCHAR(80)  NOT NULL,
    role                 VARCHAR(40)  NOT NULL,
    csrf_token           VARCHAR(128) NOT NULL,
    must_change_password BOOLEAN      NOT NULL DEFAULT false,
    created_at           TIMESTAMPTZ  NOT NULL DEFAULT now(),
    last_seen_at         TIMESTAMPTZ  NOT NULL DEFAULT now(),
    expires_at           TIMESTAMPTZ  NOT NULL
);

CREATE INDEX idx_sessions_account_id ON sessions (account_id);
CREATE INDEX idx_sessions_expires_at ON sessions (expires_at);
CREATE INDEX idx_sessions_last_seen_at ON sessions (last_seen_at);
