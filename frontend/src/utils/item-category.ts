// ===== 物品资料展示辅助（统一物品资料库，TODO §1）=====
import type { Item, ItemCategory, ItemDetail } from '@/types/item'
import { commandSearchEntry, searchEntry } from './related-search'

/** 物品类别中文标签（与后端 domain.ItemCategories 一一对应） */
export const ITEM_CATEGORY_LABELS: Record<ItemCategory, string> = {
  block: '方块',
  item: '物品',
  tool: '工具',
  weapon: '武器',
  armor: '防具',
  food: '食物',
  material: '材料',
  other: '其他',
}

/** 类别标签；未知类别回退为「其他」，不伪造。 */
export function itemCategoryLabel(category: string | null | undefined): string {
  if (!category) return ITEM_CATEGORY_LABELS.other
  return ITEM_CATEGORY_LABELS[category as ItemCategory] ?? ITEM_CATEGORY_LABELS.other
}

/**
 * 该资料是否适用「耐久度」属性。
 * 仅工具 / 武器 / 防具具备耐久度；避免对不适用类别展示空值误导用户。
 */
export function supportsDurability(category: string | null | undefined): boolean {
  return category === 'tool' || category === 'weapon' || category === 'armor'
}

/**
 * 关联模块检索入口。
 * 说明：指令 / 百科 / 资源与物品之间没有可靠的元数据关联，
 * 因此这里提供「以物品名检索」的入口，而非伪造关联或直链未发布内容。
 */
export interface ItemSearchLinks {
  wiki: string
  resource: string
  command: string
}

export function buildItemSearchLinks(item: Pick<Item, 'name_zh' | 'item_id'>): ItemSearchLinks {
  const name = item.name_zh || item.item_id
  return {
    wiki: searchEntry(name, 'wiki'),
    resource: searchEntry(name, 'resource'),
    // 修正：指令不在全站搜索覆盖范围内，此前误指向工具搜索（type=tool）。
    command: commandSearchEntry(name),
  }
}

/** 配方关联总数（产出 + 作为材料），用于详情页概览与空态判定。 */
export function relatedRecipeCount(detail: ItemDetail | null): number {
  if (!detail?.related_recipes) return 0
  return detail.related_recipes.produced.length + detail.related_recipes.used_as.length
}
