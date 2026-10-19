-- 反向迁移：删除内容建设任务表并回收权限点（0027_content_tasks.sql）。
-- 注意：回退会丢失全部任务数据；权限点回收只影响内置 admin 角色。
DROP TABLE IF EXISTS content_tasks;

UPDATE roles
SET permissions = permissions - 'ops.contenttasks',
    updated_at  = now()
WHERE name = 'admin'
  AND (permissions @> '["ops.contenttasks"]'::jsonb);
