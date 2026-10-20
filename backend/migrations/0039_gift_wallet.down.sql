-- 0039 回滚：移除礼物钱包、账本与 ops.gifts 权限点。
DROP TABLE IF EXISTS gift_ledger;
DROP TABLE IF EXISTS gift_wallets;

UPDATE roles
SET permissions = permissions - 'ops.gifts',
    updated_at  = now()
WHERE permissions ? 'ops.gifts';
