-- 0042 全站会话管理权限
--
-- 新增 ops.sessions：允许后台查看普通用户与管理员的有效会话，并按展示指纹
-- 踢下线单个会话。该权限可影响任意账号登录态，默认只授予内置 admin 角色，
-- 不授予 editor，避免内容编辑角色获得账号安全能力。
--
-- 说明：会话数据沿用 sessions 表，不新增表结构；后台列表不返回 SessionID 原文。

UPDATE roles
SET permissions = permissions || '["ops.sessions"]'::jsonb,
    updated_at = now()
WHERE name = 'admin'
  AND NOT (permissions @> '["ops.sessions"]'::jsonb);
