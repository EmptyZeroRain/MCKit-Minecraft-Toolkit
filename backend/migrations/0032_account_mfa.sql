-- 0032 管理员 MFA（TOTP + 一次性恢复码）
--
-- 背景（gj.md §10.1 / §17 阶段一）：后台账号权限高，仅靠密码不足以抵御
-- 暴力破解与密码泄露，因此为后台账号提供 TOTP 双因子认证。
--
-- 设计说明：
--   * secret_enc 存 AES-256-GCM 密文（base64(nonce||ciphertext)），密钥来自
--     MFA_ENCRYPTION_KEY；数据库泄露不等于 MFA 密钥泄露；
--   * 恢复码只存 SHA-256 哈希，一次性使用（used_at 标记），明文仅在生成时返回一次；
--   * mfa_login_tickets 用于「密码已通过、等待第二因子」的短期票据：
--     服务端只存 SHA-256 哈希，单次有效（used_at），默认 5 分钟过期；
--   * 普通用户不强制 MFA，但表结构同样适用（account_id 指向统一账号）。
--
-- 回滚：DROP TABLE mfa_login_tickets; DROP TABLE account_mfa_recovery_codes; DROP TABLE account_mfa;

CREATE TABLE account_mfa (
    account_id   UUID PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    secret_enc     TEXT        NOT NULL,
    -- enabled=false 表示「已生成 secret、尚未完成首次验证」的待启用状态；
    -- 此时登录不要求第二因子，避免用户扫码到一半就被锁在门外。
    enabled        BOOLEAN     NOT NULL DEFAULT false,
    enabled_at     TIMESTAMPTZ,
    last_used_at   TIMESTAMPTZ,
    -- last_used_step 记录最近一次成功校验命中的 TOTP 时间步，
    -- 同一时间步只接受一次，防止验证码在 30 秒窗口内被重放。
    last_used_step BIGINT      NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE account_mfa_recovery_codes (
    id         BIGSERIAL PRIMARY KEY,
    account_id UUID        NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    code_hash  CHAR(64)    NOT NULL,
    used_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 同一账号的恢复码哈希唯一，避免重复写入同一码。
CREATE UNIQUE INDEX ux_mfa_recovery_code ON account_mfa_recovery_codes (account_id, code_hash);
CREATE INDEX idx_mfa_recovery_unused ON account_mfa_recovery_codes (account_id) WHERE used_at IS NULL;

CREATE TABLE mfa_login_tickets (
    token_hash CHAR(64)    PRIMARY KEY,
    account_id UUID        NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    -- attempts 记录该票据已消耗的错误验证码次数；达到上限后票据作废，
    -- 使「一次密码认证只能换取有限次 TOTP 猜测」。
    attempts   INT         NOT NULL DEFAULT 0,
    used_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_mfa_tickets_expires ON mfa_login_tickets (expires_at);
