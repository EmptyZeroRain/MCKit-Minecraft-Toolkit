-- 0029 资源投稿（用户提交资源 → 人工审核 → 一键转为 resources 记录）
--
-- 背景：资源此前只能由后台管理员在「资源管理」中创建，普通用户没有结构化提交入口，
-- 只能在 /feedback 里提交纯文本「资源补充信息」，无法携带资源类型 / 版本 / 下载链接等字段。
-- 本迁移为普通用户提供**结构化资源投稿**：提交后进入待审核队列，管理员审核通过时
-- 由服务端调用既有 resource.Service.Create 生成正式 resources 记录（默认 draft），
-- 保证资源的校验、标签关联与历史版本写入完全复用既有链路。
--
-- 设计要点：
--   * 与 feedback 的取舍：feedback 是「自由文本 + 关联已有资料」，用于纠错与举报；
--     资源投稿是「新资源的结构化字段」，字段与 resources 表对齐，避免二次录入；
--   * account_id 外键级联删除（删号即清除投稿），并冗余 submitter_name 快照，
--     使账号被删除后审核记录仍可追溯来源；
--   * resource_id 关联审核通过后生成的资源（ON DELETE SET NULL：资源被删不影响投稿留痕）；
--   * 部分唯一索引 (account_id, source_url) WHERE status = 'pending'：同一用户对同一
--     来源链接同时只能有一个待审核投稿，防止重复刷单（已完结后允许再次提交）。
--
-- 事务：本文件由 internal/repository/db.go 在单个事务中执行，失败整体回滚。

CREATE TABLE resource_submissions (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id         UUID NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    -- submitter_name 提交时的用户名快照（账号删除后仍可追溯）。
    submitter_name     VARCHAR(40) NOT NULL DEFAULT '',
    name               VARCHAR(200) NOT NULL,
    summary            VARCHAR(500) NOT NULL,
    description        TEXT NOT NULL DEFAULT '',
    resource_type      VARCHAR(40) NOT NULL
        CHECK (resource_type IN ('mod', 'modpack', 'resource_pack', 'shader', 'world', 'datapack', 'plugin')),
    author_name        VARCHAR(160) NOT NULL DEFAULT '',
    source_platform    VARCHAR(100) NOT NULL DEFAULT '',
    source_url         TEXT NOT NULL,
    license            VARCHAR(160) NOT NULL DEFAULT '',
    game_edition       VARCHAR(20) NOT NULL DEFAULT 'java' CHECK (game_edition IN ('java', 'bedrock')),
    supported_versions JSONB NOT NULL DEFAULT '[]',
    loader_types       JSONB NOT NULL DEFAULT '[]',
    install_guide      TEXT NOT NULL DEFAULT '',
    -- note 提交者给审核者的说明（不是资源正文，不写入 resources）。
    note               TEXT NOT NULL DEFAULT '',
    -- license_version / license_accepted_at 投稿时接受的授权条款版本与时间（TODO §17）。
    -- 与 feedback 的授权留痕一致：结构化投稿同样属于「向本站贡献内容」。
    license_version    VARCHAR(40) NOT NULL DEFAULT '',
    license_accepted_at TIMESTAMPTZ,
    status             VARCHAR(20) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'approved', 'rejected')),
    reviewer           VARCHAR(40) NOT NULL DEFAULT '',
    reviewed_at        TIMESTAMPTZ,
    review_note        TEXT NOT NULL DEFAULT '',
    -- resource_id 审核通过后生成的资源；未通过为空。
    resource_id        UUID REFERENCES resources (id) ON DELETE SET NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 审核队列：按状态 + 时间排序取待审核项。
CREATE INDEX idx_resource_submissions_status ON resource_submissions (status, created_at DESC);
-- 用户「我的投稿」列表。
CREATE INDEX idx_resource_submissions_account ON resource_submissions (account_id, created_at DESC);
-- 反重复：同一用户对同一来源链接同时只能有一个待审核投稿。
CREATE UNIQUE INDEX idx_resource_submissions_pending_unique
    ON resource_submissions (account_id, source_url) WHERE status = 'pending';
