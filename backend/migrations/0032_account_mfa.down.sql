-- 回滚 0032 管理员 MFA。
--
-- 顺序与建表相反，先删依赖表再删主表；全部数据会丢失（MFA 绑定需重新配置）。

DROP TABLE IF EXISTS mfa_login_tickets;
DROP TABLE IF EXISTS account_mfa_recovery_codes;
DROP TABLE IF EXISTS account_mfa;
