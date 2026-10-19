-- 0029_resource_submissions 反向迁移：删除资源投稿表。
-- 反向迁移会丢弃全部投稿与审核留痕；已生成的 resources 记录不受影响（外键为 SET NULL）。

DROP TABLE IF EXISTS resource_submissions;
