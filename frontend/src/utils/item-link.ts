// 关联物品展示工具（TODO §3 跨模块资料关联）
//
// 关联严格基于后端返回的 resolved_items：只有后端确认「物品已收录」时才给出链接，
// 否则回退为纯文本 ID（url 为 null），避免出现指向不存在详情页的死链。
import type { ItemRef } from '@/types/item'
import type { Recipe } from '@/types/recipe'

/** 解析后的关联物品：ref/url 为 null 表示未收录，只能作为纯文本展示。 */
export interface RelatedItem {
  itemId: string
  ref: ItemRef | null
  /** 展示文案：优先中文名，其次命名空间 ID。 */
  label: string
  /** 详情页路径；未收录时为 null，调用方不得生成链接。 */
  url: string | null
}

/** 解析单个物品 ID 的关联信息。 */
export function resolveRelatedItem(
  refs: Record<string, ItemRef> | undefined,
  itemId: string,
): RelatedItem {
  const ref = itemId ? (refs?.[itemId] ?? null) : null
  return {
    itemId,
    ref,
    label: ref?.name_zh || itemId,
    url: ref?.url ? ref.url : null,
  }
}

/** 配方产出物品的关联信息。 */
export function resolveResultItem(recipe: Recipe): RelatedItem {
  return resolveRelatedItem(recipe.resolved_items, recipe.result_item_id)
}

/** 配方材料的关联信息（保留数量与顺序）。 */
export function resolveIngredients(recipe: Recipe): Array<RelatedItem & { count: number }> {
  return (recipe.ingredients ?? []).map((ing) => ({
    ...resolveRelatedItem(recipe.resolved_items, ing.item_id),
    count: ing.count,
  }))
}
