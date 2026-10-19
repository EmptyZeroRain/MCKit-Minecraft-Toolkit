// ===== 合成配方实体类型 =====
import type { RecipeType } from './common'
import type { ItemRef } from './item'

export interface RecipeIngredient {
  item_id: string
  count: number
}

export interface Recipe {
  id: string
  recipe_key: string
  recipe_type: RecipeType
  game_version: string
  /** 适用版本声明（单版本或范围，TODO §2）；空数组表示未标注。 */
  supported_versions?: string[]
  /** 最后核验时间（TODO §2）。 */
  last_verified_at?: string | null
  result_item_id: string
  result_count: number
  pattern: string[]
  key_mapping: Record<string, string>
  ingredients: RecipeIngredient[]
  requires_crafting_table: boolean
  source_url: string | null
  data_version: string | null
  created_at: string
  updated_at: string
  /**
   * 本配方引用到的、**确实已收录**的物品摘要（键为 item_id，TODO §3）。
   * 未收录物品不会出现，页面需回退为纯文本 ID，避免死链。
   */
  resolved_items?: Record<string, ItemRef>
}

/** 配方查询参数 */
export interface RecipeQuery {
  item_id?: string
  game_version?: string
  recipe_type?: RecipeType | ''
  page?: number
  page_size?: number
}