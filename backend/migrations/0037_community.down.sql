-- 0037 回滚：移除社区模型与 content.community 权限点。
DROP TABLE IF EXISTS community_likes;
DROP TABLE IF EXISTS community_comments;
DROP TABLE IF EXISTS community_posts;

UPDATE roles
SET permissions = permissions - 'content.community',
    updated_at  = now()
WHERE permissions ? 'content.community';
