-- 0013_dev_docs 反向迁移（整理自正向迁移文件中的「回滚策略」注释）。
-- 反向迁移可能丢弃数据，执行前请确认或先备份。

DROP TABLE IF EXISTS mapping_members;
DROP TABLE IF EXISTS mapping_classes;
DROP TABLE IF EXISTS mapping_sets;
DROP TABLE IF EXISTS dev_docs;
