-- 反向迁移：删除内容历史版本表（0024_content_revisions.sql）。
-- 注意：回退会丢失全部历史版本，但不会影响业务内容本身。
DROP TABLE IF EXISTS content_revisions;
