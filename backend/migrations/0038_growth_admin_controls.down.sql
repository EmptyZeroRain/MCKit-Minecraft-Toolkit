-- 0038 回滚：移除等级偏移与成就手动控制字段。
DROP INDEX IF EXISTS idx_user_achievements_active;

ALTER TABLE user_achievements
    DROP COLUMN IF EXISTS revoke_reason,
    DROP COLUMN IF EXISTS revoked_by_admin_id,
    DROP COLUMN IF EXISTS revoked_at,
    DROP COLUMN IF EXISTS granted_by_admin_id,
    DROP COLUMN IF EXISTS grant_source;

ALTER TABLE user_growth
    DROP COLUMN IF EXISTS level_offset;
