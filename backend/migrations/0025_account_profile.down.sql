-- 0025_account_profile 反向迁移：移除账号昵称与头像列。
-- 反向迁移会丢弃已设置的昵称 / 头像，执行前请确认或先备份。

ALTER TABLE accounts DROP COLUMN IF EXISTS nickname;
ALTER TABLE accounts DROP COLUMN IF EXISTS avatar_url;
