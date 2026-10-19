-- 0003 反向迁移：还原工具分类 / 反馈类型枚举与 schema 字段默认值。
-- 注意：必须先删除新 CHECK 约束，才能写回旧枚举值。

-- ---- 工具分类 ----
ALTER TABLE tools DROP CONSTRAINT IF EXISTS tools_category_check;
UPDATE tools SET category = CASE category
    WHEN '查询类' THEN 'query'
    WHEN '计算类' THEN 'calculation'
    WHEN '生成类' THEN 'generation'
    WHEN '编辑类' THEN 'editing'
    WHEN '辅助类' THEN 'auxiliary'
    ELSE category END
WHERE category IN ('查询类', '计算类', '生成类', '编辑类', '辅助类');
ALTER TABLE tools ADD CONSTRAINT tools_category_check
    CHECK (category IN ('query', 'calculation', 'generation', 'editing', 'auxiliary'));

-- ---- 工具 schema 字段默认值 ----
ALTER TABLE tools ALTER COLUMN input_schema SET DEFAULT '{}';
ALTER TABLE tools ALTER COLUMN output_schema SET DEFAULT '{}';
UPDATE tools SET input_schema = '{}' WHERE input_schema = '[]';
UPDATE tools SET output_schema = '{}' WHERE output_schema = '[]';

-- ---- 反馈类型 ----
ALTER TABLE feedback DROP CONSTRAINT IF EXISTS feedback_feedback_type_check;
UPDATE feedback SET feedback_type = CASE feedback_type
    WHEN 'resource_broken' THEN 'broken_link'
    WHEN 'infringement'    THEN 'copyright'
    ELSE feedback_type END
WHERE feedback_type IN ('resource_broken', 'infringement');
ALTER TABLE feedback ADD CONSTRAINT feedback_feedback_type_check
    CHECK (feedback_type IN ('bug', 'broken_link', 'copyright', 'other'));
