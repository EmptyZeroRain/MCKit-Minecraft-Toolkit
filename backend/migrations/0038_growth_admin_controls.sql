-- 0038 成长后台控制：单用户等级偏移 + 成就手动授予 / 撤销
--
-- 设计说明：
--   * level_offset 是人工等级偏移，有效等级 = 经验计算等级 + 偏移；
--     经验继续正常累积，后续升级仍按阈值自动推进，不会覆盖人工调整。
--   * 本迁移引入时等级定义仅允许编辑阈值 / 标题 / 启停；后续已支持追加新等级与删除最高级。
--   * user_achievements 增加来源与撤销字段；撤销为软删除，保留解锁快照与操作人。
--   * 撤销记录会阻止自动重算再次解锁，直到后台手动重新授予。

ALTER TABLE user_growth
    ADD COLUMN level_offset INT NOT NULL DEFAULT 0;

ALTER TABLE user_achievements
    ADD COLUMN grant_source VARCHAR(20) NOT NULL DEFAULT 'automatic',
    ADD COLUMN granted_by_admin_id UUID NULL REFERENCES accounts(id) ON DELETE SET NULL,
    ADD COLUMN revoked_at TIMESTAMPTZ NULL,
    ADD COLUMN revoked_by_admin_id UUID NULL REFERENCES accounts(id) ON DELETE SET NULL,
    ADD COLUMN revoke_reason VARCHAR(500) NOT NULL DEFAULT '';

CREATE INDEX idx_user_achievements_active
    ON user_achievements(user_id, revoked_at)
    WHERE revoked_at IS NULL;
