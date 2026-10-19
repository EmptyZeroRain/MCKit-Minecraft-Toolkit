-- 0025 账号资料（昵称 / 头像）
--
-- 背景：后台「用户信息」需要允许账号自助维护展示资料（昵称、头像），
-- 原 accounts 表只有 username（登录标识，不宜直接当展示名）与 email。
-- 本迁移为统一账号表补充两个可自助编辑的展示字段：
--   * nickname：展示昵称，空串表示未设置（前端回退到 username）；
--   * avatar_url：头像图片地址，与资源封面一致采用外链 URL（空串表示未设置）。
-- 两列均有默认值，对既有数据（管理员 / 普通用户）零影响。
--
-- 事务：本文件由 internal/repository/db.go 在单个事务中执行，失败整体回滚。

ALTER TABLE accounts ADD COLUMN nickname VARCHAR(60) NOT NULL DEFAULT '';
ALTER TABLE accounts ADD COLUMN avatar_url TEXT NOT NULL DEFAULT '';
