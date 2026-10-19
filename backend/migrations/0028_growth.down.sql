-- 0028 用户成长系统（反向迁移）
-- 逆序删除全部成长相关表；默认数据随表一并清除。
-- 注意：accounts.role 权限点中的 ops.growth 不在此回退（角色权限由 roles 表管理，
-- 保留该权限点对系统无害；如需彻底回退可手工移除）。

DROP TABLE IF EXISTS growth_audit_logs;
DROP TABLE IF EXISTS achievement_progress;
DROP TABLE IF EXISTS user_achievements;
DROP TABLE IF EXISTS achievement_definitions;
DROP TABLE IF EXISTS user_daily_tasks;
DROP TABLE IF EXISTS daily_task_definitions;
DROP TABLE IF EXISTS growth_levels;
DROP TABLE IF EXISTS growth_rules;
DROP TABLE IF EXISTS growth_events;
DROP TABLE IF EXISTS experience_ledger;
DROP TABLE IF EXISTS user_growth;
