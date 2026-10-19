-- 0003: 管理端契约对齐（后端适配前端）
-- 1) 工具分类改为中文展示值（与前端 TOOL_CATEGORIES 一致）
-- 2) 反馈类型对齐前端 FEEDBACK_TYPES 枚举
-- 3) 工具 input_schema / output_schema 默认值改为数组（前端为 SchemaField[]）
-- 回滚策略：反向 UPDATE 存量数据并还原 CHECK 约束与默认值。
-- 注意：必须先删除旧 CHECK 约束，才能写入新的枚举值。

-- ---- 工具分类 ----
ALTER TABLE tools DROP CONSTRAINT IF EXISTS tools_category_check;
UPDATE tools SET category = CASE category
    WHEN 'query'       THEN '查询类'
    WHEN 'calculation' THEN '计算类'
    WHEN 'generation'  THEN '生成类'
    WHEN 'editing'     THEN '编辑类'
    WHEN 'auxiliary'   THEN '辅助类'
    ELSE category END
WHERE category IN ('query', 'calculation', 'generation', 'editing', 'auxiliary');
ALTER TABLE tools ADD CONSTRAINT tools_category_check
    CHECK (category IN ('查询类', '计算类', '生成类', '编辑类', '辅助类'));

-- ---- 工具 schema 字段默认值 ----
ALTER TABLE tools ALTER COLUMN input_schema SET DEFAULT '[]';
ALTER TABLE tools ALTER COLUMN output_schema SET DEFAULT '[]';
UPDATE tools SET input_schema = '[]' WHERE input_schema = '{}';
UPDATE tools SET output_schema = '[]' WHERE output_schema = '{}';

-- ---- 反馈类型 ----
ALTER TABLE feedback DROP CONSTRAINT IF EXISTS feedback_feedback_type_check;
UPDATE feedback SET feedback_type = CASE feedback_type
    WHEN 'broken_link' THEN 'resource_broken'
    WHEN 'copyright'   THEN 'infringement'
    ELSE feedback_type END
WHERE feedback_type IN ('broken_link', 'copyright');
ALTER TABLE feedback ADD CONSTRAINT feedback_feedback_type_check
    CHECK (feedback_type IN ('bug', 'resource_broken', 'infringement', 'other'));
