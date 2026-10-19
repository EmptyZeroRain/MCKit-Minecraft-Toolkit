// ===== 全站搜索类型 =====

export interface SearchResultItem {
  type: 'tool' | 'resource' | 'wiki' | 'item' | 'devdoc'
  id: string
  slug?: string
  item_id?: string
  title: string
  summary: string
  url: string
}

/** 搜索排序方式：相关度优先（默认）或最近更新优先（TODO §5）。 */
export type SearchSort = 'relevance' | 'updated_desc'

export interface SearchQuery {
  q: string
  type?: 'tool' | 'resource' | 'wiki' | 'item' | 'devdoc' | ''
  game_version?: string
  sort?: SearchSort
  page?: number
  page_size?: number
}