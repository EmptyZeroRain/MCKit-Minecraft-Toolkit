-- 0005: 统一物品资料库（TODO §1）
-- 目标：在既有 items 表基础上补全「资料类别 + 可选属性 + 来源/校验时间」，
--       支撑物品独立详情页、导入校验与跨模块关联。
--
-- 回滚策略（反向 SQL，按需手工执行）：
--   DROP INDEX IF EXISTS idx_items_category;
--   ALTER TABLE items DROP COLUMN IF EXISTS last_verified_at;
--   ALTER TABLE items DROP COLUMN IF EXISTS image_url;
--   ALTER TABLE items DROP COLUMN IF EXISTS usage_notes;
--   ALTER TABLE items DROP COLUMN IF EXISTS obtain_method;
--   ALTER TABLE items DROP CONSTRAINT IF EXISTS items_durability_check;
--   ALTER TABLE items DROP COLUMN IF EXISTS durability;
--   ALTER TABLE items DROP CONSTRAINT IF EXISTS items_category_check;
--   ALTER TABLE items DROP COLUMN IF EXISTS item_category;

-- ---- 资料类别：决定可选属性的适用范围（方块/物品/工具/武器/防具/食物/材料/其他）----
ALTER TABLE items ADD COLUMN item_category VARCHAR(40) NOT NULL DEFAULT 'other';
ALTER TABLE items ADD CONSTRAINT items_category_check
    CHECK (item_category IN ('block', 'item', 'tool', 'weapon', 'armor', 'food', 'material', 'other'));

-- ---- 可选属性 ----
-- durability 仅对有耐久度的资料有意义，允许为空（不适用）。
ALTER TABLE items ADD COLUMN durability INT NULL;
ALTER TABLE items ADD CONSTRAINT items_durability_check
    CHECK (durability IS NULL OR durability >= 0);

ALTER TABLE items ADD COLUMN obtain_method TEXT NOT NULL DEFAULT '';
ALTER TABLE items ADD COLUMN usage_notes TEXT NOT NULL DEFAULT '';
ALTER TABLE items ADD COLUMN image_url TEXT NULL;

-- ---- 数据可信度：最后校验时间（来源 URL / 来源版本沿用已有列）----
ALTER TABLE items ADD COLUMN last_verified_at TIMESTAMPTZ NULL;

CREATE INDEX idx_items_category ON items(item_category);
