// ===== 百科条目类型 =====
import type { WikiContentType } from './common'

/** 来源参考 */
export interface SourceReference {
  title: string
  url: string
}

export interface WikiEntry {
  id: string
  slug: string
  title: string
  summary: string
  content: string
  content_format: 'markdown' | 'html'
  content_type: WikiContentType
  game_edition: 'java' | 'bedrock'
  game_versions: string[]
  tags: string[]
  source_references: SourceReference[]
  status: 'draft' | 'published'
  published_at: string | null
  created_by: string | null
  updated_by: string | null
  created_at: string
  updated_at: string
}

/** 百科列表查询参数 */
export interface WikiListQuery {
  q?: string
  content_type?: WikiContentType | ''
  status?: 'draft' | 'published' | ''
  page?: number
  page_size?: number
}

/** 创建/更新百科负载 */
export type WikiPayload = Omit<
  WikiEntry,
  'id' | 'published_at' | 'created_by' | 'updated_by' | 'created_at' | 'updated_at'
> & {
  status?: 'draft' | 'published'
}