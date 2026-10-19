-- 回退角色权限点（先做，再删表；均幂等）。
UPDATE roles
SET permissions = permissions - 'ops.jobs',
    updated_at = now()
WHERE name = 'admin'
  AND (permissions @> '["ops.jobs"]'::jsonb);

DROP TABLE IF EXISTS admin_jobs;
