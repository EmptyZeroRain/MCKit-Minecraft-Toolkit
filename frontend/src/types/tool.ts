// ===== 工具实体类型 =====
import type { ToolCategory } from './common'

/** 输入/输出字段描述（input_schema / output_schema 的 JSON 结构约定） */
export interface SchemaField {
  key: string
  label: string
  type: 'number' | 'text' | 'select'
  placeholder?: string
  hint?: string
  options?: Array<{ label: string; value: string }>
  required?: boolean
}

/** 工具 */
export interface Tool {
  id: string
  slug: string
  name: string
  summary: string
  description: string
  category: ToolCategory
  icon: string | null
  tags: string[]
  supported_versions: string[]
  input_schema: SchemaField[]
  output_schema: SchemaField[]
  is_featured: boolean
  sort_order: number
  status: 'published' | 'draft'
  created_at: string
  updated_at: string
}

/** 工具列表查询参数 */
export interface ToolListQuery {
  category?: ToolCategory | ''
  q?: string
  game_version?: string
  page?: number
  page_size?: number
}

/** 工具创建/更新负载 */
export type ToolPayload = Omit<
  Tool,
  'id' | 'created_at' | 'updated_at' | 'status'
> & {
  status?: 'published' | 'draft'
}