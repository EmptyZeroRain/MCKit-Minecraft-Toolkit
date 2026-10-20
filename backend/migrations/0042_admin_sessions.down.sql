-- 0042 回滚：移除全站会话管理权限点。
UPDATE roles
SET permissions = permissions - 'ops.sessions',
    updated_at = now()
WHERE permissions ? 'ops.sessions';
