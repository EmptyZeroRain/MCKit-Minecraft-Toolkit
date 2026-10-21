-- 0048 签到经验算法、连续签到与补签卡
--
-- 设计约束：
--   * 签到经验只影响成长模块，礼物币仍按 gift_settings.checkin_reward 发放；
--   * 一次签到 / 一次补签的经验入账与礼物币变更必须在同一事务；
--   * 补签卡只补经验，不写 gift_checkins、不增加签到天数、不改变连续签到；
--   * 连续签到以 gift_checkin_streaks 投影，gift_checkins 仍是唯一签到事实源；
--   * 里程碑只在真实签到达到对应连续天数时发放，补签不触发里程碑。

-- ============ 1. 签到经验配置 ============

ALTER TABLE gift_settings
    ADD COLUMN base_xp BIGINT NOT NULL DEFAULT 10
        CHECK (base_xp >= 0 AND base_xp <= 1000000),
    ADD COLUMN streak_step_xp BIGINT NOT NULL DEFAULT 2
        CHECK (streak_step_xp >= 0 AND streak_step_xp <= 1000000),
    ADD COLUMN streak_bonus_cap BIGINT NOT NULL DEFAULT 100
        CHECK (streak_bonus_cap >= 0 AND streak_bonus_cap <= 1000000),
    ADD COLUMN milestone_bonus_cap BIGINT NOT NULL DEFAULT 500
        CHECK (milestone_bonus_cap >= 0 AND milestone_bonus_cap <= 1000000),
    ADD COLUMN break_penalty_xp BIGINT NOT NULL DEFAULT 5
        CHECK (break_penalty_xp >= 0 AND break_penalty_xp <= 1000000),
    ADD COLUMN makeup_card_price BIGINT NOT NULL DEFAULT 10
        CHECK (makeup_card_price >= 1 AND makeup_card_price <= 1000000);

-- ============ 2. 连续签到里程碑 ============

CREATE TABLE gift_checkin_milestones (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    streak_days INT    NOT NULL UNIQUE CHECK (streak_days >= 1 AND streak_days <= 1000000),
    bonus_xp    BIGINT NOT NULL CHECK (bonus_xp >= 0 AND bonus_xp <= 1000000),
    enabled     BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_gift_checkin_milestones_enabled
    ON gift_checkin_milestones(enabled, streak_days);

INSERT INTO gift_checkin_milestones (streak_days, bonus_xp) VALUES
    (3, 10),
    (7, 30),
    (15, 80),
    (30, 200)
ON CONFLICT (streak_days) DO NOTHING;

-- ============ 3. 连续签到投影（可由 gift_checkins 重建） ============

CREATE TABLE gift_checkin_streaks (
    user_id           UUID PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    current_streak    INT  NOT NULL DEFAULT 0 CHECK (current_streak >= 0),
    longest_streak    INT  NOT NULL DEFAULT 0 CHECK (longest_streak >= 0),
    last_checkin_date DATE NULL,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (longest_streak >= current_streak)
);

-- 从历史签到回填连续天数；最后一段连续日期即当前连续天数。
WITH ranked AS (
    SELECT user_id,
           checkin_date,
           checkin_date - (row_number() OVER (
               PARTITION BY user_id ORDER BY checkin_date
           ))::int AS grp
    FROM gift_checkins
), runs AS (
    SELECT user_id, grp, MIN(checkin_date) AS start_date,
           MAX(checkin_date) AS end_date, COUNT(*)::int AS run_len
    FROM ranked
    GROUP BY user_id, grp
), latest AS (
    SELECT DISTINCT ON (user_id) user_id, end_date, run_len
    FROM runs
    ORDER BY user_id, end_date DESC
), longest AS (
    SELECT user_id, MAX(run_len) AS longest_streak
    FROM runs
    GROUP BY user_id
)
INSERT INTO gift_checkin_streaks
    (user_id, current_streak, longest_streak, last_checkin_date)
SELECT l.user_id, l.run_len, g.longest_streak, l.end_date
FROM latest l
JOIN longest g ON g.user_id = l.user_id
ON CONFLICT (user_id) DO NOTHING;

-- ============ 4. 补签卡库存 ============

CREATE TABLE gift_makeup_cards (
    user_id    UUID PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    quantity   BIGINT NOT NULL DEFAULT 0 CHECK (quantity >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============ 5. 签到经验明细（可审计，不作为签到天数来源） ============

CREATE TABLE gift_checkin_xp (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    xp_date       DATE NOT NULL,
    kind          VARCHAR(16) NOT NULL CHECK (kind IN ('actual', 'makeup', 'break')),
    request_key   VARCHAR(128) NOT NULL DEFAULT '',
    streak_day    INT NOT NULL DEFAULT 0 CHECK (streak_day >= 0),
    base_xp       BIGINT NOT NULL DEFAULT 0 CHECK (base_xp >= 0),
    streak_xp     BIGINT NOT NULL DEFAULT 0 CHECK (streak_xp >= 0),
    milestone_xp  BIGINT NOT NULL DEFAULT 0 CHECK (milestone_xp >= 0),
    penalty_xp    BIGINT NOT NULL DEFAULT 0 CHECK (penalty_xp >= 0),
    total_xp      BIGINT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, xp_date, kind),
    CHECK (
        (kind = 'break' AND total_xp <= 0)
        OR (kind IN ('actual', 'makeup') AND total_xp >= 0)
    )
);

CREATE INDEX idx_gift_checkin_xp_user_date
    ON gift_checkin_xp(user_id, xp_date DESC);

-- 补签使用请求幂等：同一用户同一 request_key 只补一次。
CREATE UNIQUE INDEX idx_gift_checkin_xp_request_key
    ON gift_checkin_xp(user_id, request_key)
    WHERE request_key <> '';

-- ============ 6. 默认配置（已有行不覆盖） ============

INSERT INTO gift_settings (id, checkin_reward) VALUES (TRUE, 1)
ON CONFLICT (id) DO NOTHING;
