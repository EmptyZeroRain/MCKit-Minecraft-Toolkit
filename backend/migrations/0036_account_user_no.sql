-- 0036 账号对外展示 ID（accounts.user_no）
--
-- 背景：accounts.id 是 UUID，适合做外键但不利于用户展示 / 口头传播。
-- 本迁移为每个账号新增一个随机生成的短 ID（形如 U7K3M2QX9F），用于前台
-- 个人资料、社区作者署名与后台用户列表展示；内部主键仍保持 UUID 不变。
--
-- 生成口径：
--   * 既有账号回填：'U' + md5(random() || id) 前 10 位大写十六进制（约 1.1e12 空间），
--     并把易混淆的 0 / 1 分别映射为 8 / 9，保持与服务层字符集一致；
--   * 新建账号：服务层用 crypto/rand 生成（字符集排除 I / L / O / U 等易混淆字符），
--     数据库默认值仅作为直接 INSERT（如测试夹具）的兜底。
--   * 唯一索引兜底；该列一旦写入不再更新。
--
-- 事务：本文件由 internal/repository/db.go 在单个事务中执行，失败整体回滚。
-- 回滚：见 0036_account_user_no.down.sql。

ALTER TABLE accounts ADD COLUMN user_no VARCHAR(12);

UPDATE accounts
SET user_no = 'U' || translate(upper(substr(md5(random()::text || id::text), 1, 10)), '01', '89')
WHERE user_no IS NULL;

ALTER TABLE accounts ALTER COLUMN user_no SET NOT NULL;
ALTER TABLE accounts ALTER COLUMN user_no
    SET DEFAULT ('U' || translate(upper(substr(md5(random()::text), 1, 10)), '01', '89'));

CREATE UNIQUE INDEX idx_accounts_user_no_unique ON accounts(user_no);
