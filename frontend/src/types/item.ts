// ===== 物品实体类型（统一物品资料库，TODO §1）=====
import type { Recipe } from './recipe'

/** 物品资料类别（与后端 domain.ItemCategories 契约一致） */
export type ItemCategory =
  | 'block'
  | 'item'
  | 'tool'
  | 'weapon'
  | 'armor'
  | 'food'
  | 'material'
  | 'other'

export interface Item {
  id: string
  item_id: string // 例如 minecraft:stone
  name_zh: string
  name_en: string | null
  aliases: string[]
  description: string
  item_category: ItemCategory
  stack_size: number | null
  durability: number | null
  obtain_method: string
  usage: string
  image_url: string | null
  game_edition: 'java' | 'bedrock'
  supported_versions: string[]
  source_url: string | null
  data_version: string | null
  last_verified_at: string | null
  created_at: string
  updated_at: string
}

/** 关联展示用的物品摘要（由后端在确认物品已收录后返回，TODO §3） */
export interface ItemRef {
  item_id: string
  name_zh: string
  name_en: string | null
  item_category: ItemCategory
  url: string
}

/** 关联配方：产出该物品 + 以该物品为材料 */
export interface ItemRecipeLinks {
  produced: Recipe[]
  used_as: Recipe[]
  /**
   * 关联配方引用到的、**确实已收录**的物品摘要（键为 item_id，TODO §3）。
   * 未收录物品不会出现在此映射中，页面必须回退为纯文本 ID，避免死链。
   */
  resolved_items?: Record<string, ItemRef>
}

/** 物品详情（物品本体 + 关联配方 + 生成式获取指令） */
export interface ItemDetail extends Item {
  related_recipes: ItemRecipeLinks
  obtain_command: string
}

/** 物品查询参数 */
export interface ItemQuery {
  q?: string
  page?: number
  page_size?: number
}
