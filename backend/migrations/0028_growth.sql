-- 0028 用户成长系统（经验 / 等级 / 成就 / 每日任务 / 成长审计）
--
-- 依据 jingyan.md《MCkit 用户成长系统开发规格说明》实现。设计原则：
--   * 服务端判定：经验、等级、任务完成与成就解锁全部由服务端计算，前端不可提交；
--   * 业务事件驱动：奖励只由内部已确认的业务事件推进（growth_events 幂等去重）；
--   * 经验账本可审计：experience_ledger 只增不删，错误奖励用负向冲正流水处理；
--   * 重复事件不重复领奖：账本 idempotency_key、任务 (user,task,date)、成就 (user,achievement)
--     均有唯一约束作为最后一道防线。
--
-- 说明（仓库差异，见 PROJECT.md §差异记录）：
--   现有仓库没有帖子 / 评论 / 点赞等社区模型，故 post.liked / post.comment_created 等
--   事件类型仅预置规则与任务定义，等待社区功能落地后再接入，不虚构事件来源。
--
-- 事务：本文件由 internal/repository/db.go 在单个事务中执行，失败整体回滚。
-- 初始化数据使用 ON CONFLICT DO NOTHING，可重复执行且不影响既有数据。

-- ============ 1. 用户成长投影 ============
CREATE TABLE user_growth (
    user_id              UUID PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    current_xp           BIGINT      NOT NULL DEFAULT 0 CHECK (current_xp >= 0),
    lifetime_earned_xp   BIGINT      NOT NULL DEFAULT 0 CHECK (lifetime_earned_xp >= 0),
    lifetime_reversed_xp BIGINT      NOT NULL DEFAULT 0 CHECK (lifetime_reversed_xp >= 0),
    level                INT         NOT NULL DEFAULT 1 CHECK (level >= 1),
    version              BIGINT      NOT NULL DEFAULT 0,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============ 2. 经验账本（不可物理删除） ============
CREATE TABLE experience_ledger (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id           UUID         NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    delta_xp          BIGINT       NOT NULL,
    balance_after     BIGINT       NOT NULL,
    source_type       VARCHAR(64)  NOT NULL,
    source_id         VARCHAR(128) NOT NULL DEFAULT '',
    event_id          UUID         NULL,
    idempotency_key   VARCHAR(255) NOT NULL UNIQUE,
    rule_code         VARCHAR(100) NOT NULL DEFAULT '',
    rule_version      INT          NOT NULL DEFAULT 0,
    reason            TEXT         NOT NULL DEFAULT '',
    operator_user_id  UUID         NULL REFERENCES accounts(id) ON DELETE SET NULL,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX idx_experience_ledger_user_created ON experience_ledger(user_id, created_at DESC);
CREATE INDEX idx_experience_ledger_source ON experience_ledger(source_type, source_id);
CREATE INDEX idx_experience_ledger_event ON experience_ledger(event_id);

-- ============ 3. 成长事件（接收与幂等处理） ============
CREATE TABLE growth_events (
    event_id      UUID PRIMARY KEY,
    event_type    VARCHAR(100) NOT NULL,
    actor_user_id UUID         NULL REFERENCES accounts(id) ON DELETE SET NULL,
    subject_type  VARCHAR(64)  NOT NULL DEFAULT '',
    subject_id    VARCHAR(128) NOT NULL DEFAULT '',
    source_module VARCHAR(100) NOT NULL,
    occurred_at   TIMESTAMPTZ  NOT NULL,
    processed_at  TIMESTAMPTZ  NULL,
    status        VARCHAR(20)  NOT NULL DEFAULT 'pending',
    payload       JSONB        NOT NULL DEFAULT '{}'::jsonb,
    error_message TEXT         NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX idx_growth_events_actor ON growth_events(actor_user_id, created_at DESC);
CREATE INDEX idx_growth_events_status ON growth_events(status);
CREATE INDEX idx_growth_events_type ON growth_events(event_type);

-- ============ 4. 经验规则（版本化） ============
CREATE TABLE growth_rules (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code            VARCHAR(100) NOT NULL,
    version         INT          NOT NULL DEFAULT 1,
    name            VARCHAR(150) NOT NULL,
    event_type      VARCHAR(100) NOT NULL,
    xp_amount       INT          NOT NULL DEFAULT 0,
    daily_limit     INT          NULL,
    per_subject_limit INT        NULL,
    enabled         BOOLEAN      NOT NULL DEFAULT TRUE,
    effective_from  TIMESTAMPTZ  NULL,
    effective_until TIMESTAMPTZ  NULL,
    created_by      UUID         NULL,
    updated_by      UUID         NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    UNIQUE (code, version)
);

CREATE INDEX idx_growth_rules_event ON growth_rules(event_type);

-- ============ 5. 等级阈值 ============
CREATE TABLE growth_levels (
    level        INT PRIMARY KEY CHECK (level >= 1),
    threshold_xp BIGINT      NOT NULL UNIQUE CHECK (threshold_xp >= 0),
    title        VARCHAR(100) NOT NULL,
    enabled      BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============ 6. 每日任务定义 ============
CREATE TABLE daily_task_definitions (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code          VARCHAR(100) NOT NULL UNIQUE,
    name          VARCHAR(150) NOT NULL,
    description   TEXT         NOT NULL DEFAULT '',
    event_type    VARCHAR(100) NOT NULL,
    target_count  INT          NOT NULL CHECK (target_count > 0),
    reward_xp     INT          NOT NULL DEFAULT 0,
    enabled       BOOLEAN      NOT NULL DEFAULT TRUE,
    sort_order    INT          NOT NULL DEFAULT 0,
    version       INT          NOT NULL DEFAULT 1,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- ============ 7. 用户每日任务进度 ============
CREATE TABLE user_daily_tasks (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id               UUID         NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    task_code             VARCHAR(100) NOT NULL,
    task_date             DATE         NOT NULL,
    progress_count        INT          NOT NULL DEFAULT 0,
    target_count_snapshot INT          NOT NULL,
    reward_xp_snapshot    INT          NOT NULL DEFAULT 0,
    status                VARCHAR(20)  NOT NULL DEFAULT 'in_progress',
    completed_at          TIMESTAMPTZ  NULL,
    claimed_at            TIMESTAMPTZ  NULL,
    definition_version    INT          NOT NULL DEFAULT 1,
    created_at            TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ  NOT NULL DEFAULT now(),
    UNIQUE (user_id, task_code, task_date)
);

CREATE INDEX idx_user_daily_tasks_user_date ON user_daily_tasks(user_id, task_date DESC);

-- ============ 8. 成就定义 ============
CREATE TABLE achievement_definitions (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code           VARCHAR(100) NOT NULL UNIQUE,
    name           VARCHAR(150) NOT NULL,
    description    TEXT         NOT NULL DEFAULT '',
    category       VARCHAR(50)  NOT NULL,
    rarity         VARCHAR(30)  NOT NULL DEFAULT 'common',
    icon_url       TEXT         NOT NULL DEFAULT '',
    condition_type VARCHAR(100) NOT NULL,
    condition_event_type VARCHAR(100) NOT NULL DEFAULT '',
    condition_subject_type VARCHAR(64) NOT NULL DEFAULT '',
    target_value   BIGINT       NOT NULL DEFAULT 1 CHECK (target_value > 0),
    reward_xp      INT          NOT NULL DEFAULT 0,
    is_hidden      BOOLEAN      NOT NULL DEFAULT FALSE,
    enabled        BOOLEAN      NOT NULL DEFAULT TRUE,
    version        INT          NOT NULL DEFAULT 1,
    sort_order     INT          NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- ============ 9. 用户已解锁成就（永久保留） ============
CREATE TABLE user_achievements (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id                  UUID         NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    achievement_id           UUID         NOT NULL REFERENCES achievement_definitions(id) ON DELETE CASCADE,
    achievement_code_snapshot VARCHAR(100) NOT NULL,
    name_snapshot            VARCHAR(150) NOT NULL,
    description_snapshot     TEXT         NOT NULL DEFAULT '',
    definition_version       INT          NOT NULL DEFAULT 1,
    unlocked_at              TIMESTAMPTZ  NOT NULL DEFAULT now(),
    source_event_id          UUID         NULL,
    is_public                BOOLEAN      NOT NULL DEFAULT TRUE,
    UNIQUE (user_id, achievement_id)
);

CREATE INDEX idx_user_achievements_user ON user_achievements(user_id, unlocked_at DESC);

-- ============ 10. 成就进度缓存（可重建） ============
CREATE TABLE achievement_progress (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    achievement_id UUID NOT NULL REFERENCES achievement_definitions(id) ON DELETE CASCADE,
    progress_count BIGINT NOT NULL DEFAULT 0,
    target_count  BIGINT NOT NULL,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, achievement_id)
);

-- ============ 11. 成长审计日志 ============
CREATE TABLE growth_audit_logs (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    operator_user_id UUID         NULL REFERENCES accounts(id) ON DELETE SET NULL,
    action           VARCHAR(100) NOT NULL,
    target_user_id   UUID         NULL,
    entity_type      VARCHAR(100) NOT NULL,
    entity_id        VARCHAR(128) NOT NULL DEFAULT '',
    reason           TEXT         NOT NULL,
    before_data      JSONB        NULL,
    after_data       JSONB        NULL,
    request_id       VARCHAR(128) NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX idx_growth_audit_created ON growth_audit_logs(created_at DESC);
CREATE INDEX idx_growth_audit_target ON growth_audit_logs(target_user_id, created_at DESC);

-- ============ 默认数据（可重复执行） ============

-- 等级阈值（jingyan.md §5，10 级）
INSERT INTO growth_levels (level, threshold_xp, title) VALUES
    (1,  0,    '初来乍到'),
    (2,  100,  '方块学徒'),
    (3,  300,  '探索者'),
    (4,  600,  '熟练玩家'),
    (5,  1000, '资深玩家'),
    (6,  1600, '资源猎人'),
    (7,  2400, '红石专家'),
    (8,  3400, '资深作者'),
    (9,  4600, '工匠大师'),
    (10, 6000, 'MCkit 传奇')
ON CONFLICT (level) DO NOTHING;

-- 经验规则（jingyan.md §3；任务奖金由 daily_task_definitions 承载，不重复建规则）
INSERT INTO growth_rules (code, version, name, event_type, xp_amount, daily_limit, per_subject_limit) VALUES
    ('profile_completed',         1, '完善个人资料',   'user.profile_completed',        20, NULL, 1),
    ('email_verified',            1, '验证邮箱',       'user.email_verified',           10, NULL, 1),
    ('resource_favorited',        1, '收藏资源',       'resource.favorited',             2, NULL, 1),
    ('tool_daily_valid_use',      1, '有效使用工具',   'tool.used_validly',              3, 1,    NULL),
    ('resource_submission_valid', 1, '首次有效投稿',   'resource.submission_created',   10, NULL, 1),
    ('resource_approved',         1, '投稿审核通过',   'resource.approved',             50, NULL, 1),
    ('broken_resource_confirmed', 1, '确认失效资源',   'resource.broken_report_confirmed', 10, NULL, 1),
    ('correction_accepted',       1, '有效纠错',       'feedback.correction_accepted',   5, NULL, 1),
    ('post_like_daily',           1, '有效点赞',       'post.liked',                     1, 5,    1),
    ('post_comment_daily',        1, '有效留言',       'post.comment_created',           2, 3,    1),
    ('daily_login',               1, '每日登录任务奖励', 'user.login_succeeded',          5, 1,    NULL)
ON CONFLICT (code, version) DO NOTHING;

-- 每日任务定义（jingyan.md §4.1）
INSERT INTO daily_task_definitions (code, name, description, event_type, target_count, reward_xp, sort_order) VALUES
    ('daily_login',     '每日登录', '今天成功登录一次',                 'user.login_succeeded',   1, 5, 1),
    ('daily_like_3',    '点赞 3 次', '给 3 个不同帖子有效点赞',          'post.liked',             3, 3, 2),
    ('daily_comment_1', '留下足迹', '发表 1 条有效留言',                'post.comment_created',   1, 5, 3),
    ('daily_tool_1',    '工具试用', '有效使用 1 次工具',                'tool.used_validly',      1, 3, 4),
    ('daily_favorite_1','收藏资源', '当天首次收藏 1 个资源',            'resource.favorited',     1, 2, 5)
ON CONFLICT (code) DO NOTHING;

-- 成就定义（jingyan.md §6.1；condition_type 必须在后端允许列表内）
INSERT INTO achievement_definitions
    (code, name, description, category, rarity, condition_type, condition_event_type, condition_subject_type, target_value, sort_order) VALUES
    ('account_registered',       '初次见面',   '完成注册',                     '探索', 'common', 'event_count',          'user.registered',            '',         1,  1),
    ('profile_complete',         '新手上路',   '完善个人资料',                 '探索', 'common', 'event_count',          'user.profile_completed',     '',         1,  2),
    ('first_tool_use',           '初次探索',   '首次有效使用工具',             '探索', 'common', 'event_count',          'tool.used_validly',          '',         1,  3),
    ('tool_explorer_10',         '工具达人',   '使用 10 种不同工具',           '探索', 'rare',   'distinct_subject_count','tool.used_validly',         'tool',     10, 4),
    ('favorite_first',           '收藏新手',   '收藏 1 个不同资源',            '收藏', 'common', 'event_count',          'resource.favorited',         '',         1,  5),
    ('favorite_20',              '收藏家',     '收藏 20 个不同资源',           '收藏', 'rare',   'distinct_subject_count','resource.favorited',        'resource', 20, 6),
    ('favorite_50',              '资源考古学家','收藏 50 个不同资源',          '收藏', 'epic',   'distinct_subject_count','resource.favorited',        'resource', 50, 7),
    ('first_resource_submission','初次投稿',   '首次有效提交资源',             '贡献', 'common', 'event_count',          'resource.submission_created','',         1,  8),
    ('resource_approved_1',      '资源贡献者', '1 份资源审核通过',             '贡献', 'common', 'distinct_subject_count','resource.approved',         'resource', 1,  9),
    ('resource_approved_5',      '资源收藏家', '5 份资源审核通过',             '贡献', 'rare',   'distinct_subject_count','resource.approved',         'resource', 5,  10),
    ('resource_approved_10',     '优质贡献者', '10 份资源审核通过',            '贡献', 'epic',   'distinct_subject_count','resource.approved',         'resource', 10, 11),
    ('first_post_like',          '点赞同行',   '首次有效点赞他人帖子',         '社区', 'common', 'event_count',          'post.liked',                 '',         1,  12),
    ('comment_10',               '交流新手',   '累计 10 条有效留言',           '社区', 'common', 'event_count',          'post.comment_created',       '',         10, 13),
    ('comment_100',              '社区交流者', '累计 100 条有效留言',          '社区', 'epic',   'event_count',          'post.comment_created',       '',         100,14),
    ('daily_tasks_7',            '坚持探索',   '累计 7 个不同日期完成每日任务', '成长', 'rare',   'daily_task_days',      '',                           '',         7,  15),
    ('level_3',                  '稳步成长',   '达到 Lv.3',                    '成长', 'common', 'level_reached',        '',                           '',         3,  16),
    ('level_5',                  '经验丰富',   '达到 Lv.5',                    '成长', 'rare',   'level_reached',        '',                           '',         5,  17),
    ('level_10',                 '社区达人',   '达到 Lv.10',                   '成长', 'legendary','level_reached',      '',                           '',         10, 18),
    ('correction_1',             '纠错先锋',   '1 次纠错审核通过',             '特殊', 'rare',   'event_count',          'feedback.correction_accepted','',         1,  19),
    ('wiki_contribution_1',      '百科贡献者', '1 次百科贡献审核通过',         '特殊', 'rare',   'event_count',          'wiki.contribution_approved', '',         1,  20)
ON CONFLICT (code) DO NOTHING;

-- 内置 admin 角色补充 ops.growth 权限点（幂等）。
UPDATE roles
SET permissions = permissions || '["ops.growth"]'::jsonb,
    updated_at  = now()
WHERE name = 'admin'
  AND NOT (permissions @> '["ops.growth"]'::jsonb);
