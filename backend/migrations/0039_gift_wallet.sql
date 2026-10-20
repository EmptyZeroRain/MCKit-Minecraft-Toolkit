-- 0039 礼物币钱包与账本
--
-- 背景：GiftCoins.md 定义了礼物币体系。本次先补齐后台可运营的最小闭环：
--   * gift_wallets：余额投影；
--   * gift_ledger：不可物理删除的收支流水；
--   * ops.gifts：独立的礼物钱包管理权限，默认只授予内置 admin。
--
-- 安全约束：
--   * 余额不得为负；金额与余额均使用 BIGINT；
--   * 同一用户 + 幂等键只允许一笔流水；
--   * 不提供直接改余额的接口，所有变更必须写账本。

CREATE TABLE gift_wallets (
    user_id    UUID PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    balance    BIGINT      NOT NULL DEFAULT 0 CHECK (balance >= 0),
    version    BIGINT      NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE gift_ledger (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id           UUID         NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    amount            BIGINT       NOT NULL CHECK (amount <> 0),
    balance_after     BIGINT       NOT NULL CHECK (balance_after >= 0),
    event_type        VARCHAR(32)  NOT NULL,
    source_type       VARCHAR(32)  NOT NULL DEFAULT 'admin_action',
    source_id         VARCHAR(128) NOT NULL DEFAULT '',
    idempotency_key   VARCHAR(128) NOT NULL,
    reason            VARCHAR(500) NOT NULL,
    operator_admin_id UUID         NULL REFERENCES accounts(id) ON DELETE SET NULL,
    request_id        VARCHAR(128) NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT now(),
    UNIQUE (user_id, idempotency_key)
);

CREATE INDEX idx_gift_ledger_user_created ON gift_ledger(user_id, created_at DESC);
CREATE INDEX idx_gift_ledger_source ON gift_ledger(source_type, source_id);

-- 内置 admin 角色补充 ops.gifts 权限点（幂等）。
UPDATE roles
SET permissions = permissions || '["ops.gifts"]'::jsonb,
    updated_at  = now()
WHERE name = 'admin'
  AND NOT (permissions ? 'ops.gifts');
