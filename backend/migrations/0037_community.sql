-- 0037 社区模型（帖子 / 留言 / 点赞）
--
-- 背景：仓库此前只有成长系统的 post.* 事件类型，没有对应的社区业务表。
-- 本迁移补齐最小可用社区模型：
--   * community_posts：帖子（分类 + 标题 + 正文 + 审核状态）；
--   * community_comments：帖子下的留言；
--   * community_likes：账号对帖子的点赞（复合主键天然去重）。
--
-- 设计说明：
--   * 作者 / 点赞人外键指向 accounts(id)，账号删除时级联清除其社区内容；
--   * 帖子与留言均用 status 做审核（published / hidden），公开接口只返回 published；
--   * 计数（点赞数 / 留言数）读取时实时聚合，避免冗余计数漂移；
--   * 不引入匿名发帖：所有写操作都要求普通用户会话。
--
-- 事务：本文件由 internal/repository/db.go 在单个事务中执行，失败整体回滚。
-- 回滚：见 0037_community.down.sql。

CREATE TABLE community_posts (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    author_id  UUID        NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    category   VARCHAR(20) NOT NULL DEFAULT 'chat'
               CHECK (category IN ('chat', 'help', 'showcase', 'tutorial')),
    title      VARCHAR(120) NOT NULL,
    body       TEXT         NOT NULL,
    status     VARCHAR(20)  NOT NULL DEFAULT 'published'
               CHECK (status IN ('published', 'hidden')),
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- 公开列表：按状态 + 时间倒序；作者维度用于「我的帖子」与后台排查。
CREATE INDEX idx_community_posts_status_created ON community_posts(status, created_at DESC);
CREATE INDEX idx_community_posts_author ON community_posts(author_id, created_at DESC);

CREATE TABLE community_comments (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    post_id    UUID        NOT NULL REFERENCES community_posts(id) ON DELETE CASCADE,
    author_id  UUID        NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    body       TEXT         NOT NULL,
    status     VARCHAR(20)  NOT NULL DEFAULT 'published'
               CHECK (status IN ('published', 'hidden')),
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX idx_community_comments_post_created ON community_comments(post_id, created_at ASC);
CREATE INDEX idx_community_comments_author ON community_comments(author_id, created_at DESC);

CREATE TABLE community_likes (
    post_id    UUID        NOT NULL REFERENCES community_posts(id) ON DELETE CASCADE,
    account_id UUID        NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (post_id, account_id)
);

CREATE INDEX idx_community_likes_account ON community_likes(account_id, created_at DESC);

-- 后台新增权限点 content.community：内置 admin 保留全量权限，editor 同步获得社区审核能力。
UPDATE roles
SET permissions = permissions || '["content.community"]'::jsonb,
    updated_at  = now()
WHERE name IN ('admin', 'editor')
  AND NOT permissions ? 'content.community';
