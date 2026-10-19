-- 0017_feedback_submissions 反向迁移（整理自正向迁移文件中的「回滚策略」注释）。
-- 反向迁移可能丢弃数据，执行前请确认或先备份。

DROP TABLE IF EXISTS feedback_reviews;
ALTER TABLE feedback
    DROP CONSTRAINT IF EXISTS feedback_target_kind_check,
    DROP COLUMN IF EXISTS dedup_hash,
    DROP COLUMN IF EXISTS license_accepted_at,
    DROP COLUMN IF EXISTS license_version,
    DROP COLUMN IF EXISTS review_note,
    DROP COLUMN IF EXISTS reviewed_at,
    DROP COLUMN IF EXISTS reviewer,
    DROP COLUMN IF EXISTS target_ref,
    DROP COLUMN IF EXISTS target_kind;
ALTER TABLE feedback DROP CONSTRAINT IF EXISTS feedback_feedback_type_check;
ALTER TABLE feedback ADD CONSTRAINT feedback_feedback_type_check
    CHECK (feedback_type IN ('bug', 'resource_broken', 'infringement', 'other'));
