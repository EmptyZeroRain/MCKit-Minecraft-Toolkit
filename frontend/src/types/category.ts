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