-- 回退 0048：移除签到经验、里程碑、连续签到、补签卡与经验明细。
--
-- 已写入 experience_ledger 的签到经验不会被删除；本迁移只移除签到侧的
-- 配置与明细表，避免回滚后签到服务继续引用不存在的表。

DROP TABLE IF EXISTS gift_checkin_xp;
DROP TABLE IF EXISTS gift_makeup_cards;
DROP TABLE IF EXISTS gift_checkin_streaks;
DROP TABLE IF EXISTS gift_checkin_milestones;

ALTER TABLE gift_settings
    DROP COLUMN IF EXISTS makeup_card_price,
    DROP COLUMN IF EXISTS break_penalty_xp,
    DROP COLUMN IF EXISTS milestone_bonus_cap,
    DROP COLUMN IF EXISTS streak_bonus_cap,
    DROP COLUMN IF EXISTS streak_step_xp,
    DROP COLUMN IF EXISTS base_xp;
