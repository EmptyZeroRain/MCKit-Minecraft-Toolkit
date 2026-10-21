-- 0046 修正历史迁移记录的元数据。
--
-- 0044 早期版本名为 growth_level_titles，后来文件改名为 growth_infinite_levels；
-- 迁移执行器只按 version 判断，已部署实例保留了旧名称。
-- 本迁移只更新 schema_migrations.name，不触碰业务表，避免审计时把版本与文件对不上。

UPDATE schema_migrations
SET name = 'growth_infinite_levels'
WHERE version = 44
  AND name <> 'growth_infinite_levels';
