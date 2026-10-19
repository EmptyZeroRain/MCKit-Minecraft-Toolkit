-- 0005_item_catalog 反向迁移（整理自正向迁移文件中的「回滚策略」注释）。
-- 反向迁移可能丢弃数据，执行前请确认或先备份。

DROP INDEX IF EXISTS idx_items_category;
ALTER TABLE items DROP COLUMN IF EXISTS last_verified_at;
ALTER TABLE items DROP COLUMN IF EXISTS image_url;
ALTER TABLE items DROP COLUMN IF EXISTS usage_notes;
ALTER TABLE items DROP COLUMN IF EXISTS obtain_method;
ALTER TABLE items DROP CONSTRAINT IF EXISTS items_durability_check;
ALTER TABLE items DROP COLUMN IF EXISTS durability;
ALTER TABLE items DROP CONSTRAINT IF EXISTS items_category_check;
ALTER TABLE items DROP COLUMN IF EXISTS item_category;
