-- 0017 用户投稿与纠错（TODO §17）
--
-- 在既有 feedback 基础上扩展为「投稿 / 纠错」通道：
--   * feedback_type 扩展 data_correction（资料纠错）/ resource_supplement（资源补充信息）；
--   * target_kind / target_ref 关联到具体资料条目（资源 / 物品 / 开发文档 / 生物 / 村民 / 药水 / 工具），
--     使投稿能定位到「哪一条资料」，而不是只留一个自由文本链接；
--   * 审核留痕：reviewer / reviewed_at / review_note 记录最近一次审核，
--     feedback_reviews 表保存**完整审核历史**（状态流转 + 审核人 + 结论），不覆盖历史；
--   * 授权条款：license_version / license_accepted_at —— 投稿必须明确接受授权条款后才受理；
--   * 反垃圾：dedup_hash 为「类型 + 目标 + 标题 + 正文」的稳定摘要，同一来源在时间窗内重复提交直接拒绝。
--
-- 回滚策略（反向 SQL，按需手工执行）：
--   DROP TABLE IF EXISTS feedback_reviews;
--   ALTER TABLE feedback
--       DROP CONSTRAINT IF EXISTS feedback_target_kind_check,
--       DROP COLUMN IF EXISTS dedup_hash,
--       DROP COLUMN IF EXISTS license_accepted_at,
--       DROP COLUMN IF EXISTS license_version,
--       DROP COLUMN IF EXISTS review_note,
--       DROP COLUMN IF EXISTS reviewed_at,
--       DROP COLUMN IF EXISTS reviewer,
--       DROP COLUMN IF EXISTS target_ref,
--       DROP COLUMN IF EXISTS target_kind;
--   ALTER TABLE feedback DROP CONSTRAINT IF EXISTS feedback_feedback_type_check;
--   ALTER TABLE feedback ADD CONSTRAINT feedback_feedback_type_check
--       CHECK (feedback_type IN ('bug', 'resource_broken', 'infringement', 'other'));

ALTER TABLE feedback DROP CONSTRAINT IF EXISTS feedback_feedback_type_check;
ALTER TABLE feedback ADD CONSTRAINT feedback_feedback_type_check
    CHECK (feedback_type IN ('bug', 'resource_broken', 'infringement', 'data_correction', 'resource_supplement', 'other'));

ALTER TABLE feedback
    ADD COLUMN IF NOT EXISTS target_kind VARCHAR(40) NOT NULL DEFAULT 'other',
    ADD COLUMN IF NOT EXISTS target_ref VARCHAR(200) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS reviewer VARCHAR(120) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS reviewed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS review_note TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS license_version VARCHAR(40) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS license_accepted_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS dedup_hash VARCHAR(64) NOT NULL DEFAULT '';

ALTER TABLE feedback DROP CONSTRAINT IF EXISTS feedback_target_kind_check;
ALTER TABLE feedback ADD CONSTRAINT feedback_target_kind_check
    CHECK (target_kind IN ('resource', 'item', 'dev_doc', 'mob', 'villager', 'potion', 'tool', 'other'));

-- 反垃圾：按 dedup_hash 在时间窗内查重（见 internal/service/content）。
CREATE INDEX IF NOT EXISTS idx_feedback_dedup ON feedback (dedup_hash, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_feedback_type_status ON feedback (feedback_type, status, created_at DESC);

-- 审核历史：每次状态流转追加一行，保留审核人、结论与备注。
CREATE TABLE IF NOT EXISTS feedback_reviews (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    feedback_id UUID NOT NULL REFERENCES feedback (id) ON DELETE CASCADE,
    from_status VARCHAR(20) NOT NULL,
    to_status   VARCHAR(20) NOT NULL,
    reviewer    VARCHAR(120) NOT NULL DEFAULT '',
    note        TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_feedback_reviews_feedback ON feedback_reviews (feedback_id, created_at DESC);
