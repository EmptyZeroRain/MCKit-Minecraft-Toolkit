UPDATE roles
SET permissions = permissions - 'ops.alerts',
    updated_at = now()
WHERE name = 'admin'
  AND (permissions @> '["ops.alerts"]'::jsonb);

DROP TABLE IF EXISTS ops_alerts;
