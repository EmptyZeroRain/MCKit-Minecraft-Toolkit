-- 0006_versioning_quality 反向迁移（整理自正向迁移文件中的「回滚策略」注释）。
-- 反向迁移可能丢弃数据，执行前请确认或先备份。

DROP TABLE IF EXISTS data_import_jobs;
ALTER TABLE commands DROP COLUMN IF EXISTS last_verified_at;
ALTER TABLE commands DROP COLUMN IF EXISTS source_url;
ALTER TABLE recipes DROP COLUMN IF EXISTS last_verified_at;
ALTER TABLE recipes DROP COLUMN IF EXISTS supported_versions;
