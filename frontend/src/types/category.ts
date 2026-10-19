// ===== 分类 / 标签实体类型 =====

export interface Category {
  id: string
  name: string
  slug: string
  content_type: 'tool' | 'resource' | 'wiki'
  sort_order: number
  created_at: string
  updated_at: string
}

export interface CategoryQuery {
  content_type?: 'tool' | 'resource' | 'wiki' | ''
}

export interface Tag {
  id: string
  name: string
  slug: string
  created_at: string
}

export interface TagQuery {
  q?: string
  page?: number
  page_size?: number
}

/** 标签使用统计行（来自 resource_tags / tool_tags / wiki_tags 关联表聚合）。 */
export interface TagUsageRow {
  tag: string
  total: number
  resource_count: number
  tool_count: number
  wiki_count: number
}