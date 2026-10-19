// ===== 指令实体类型 =====

export interface CommandParameter {
  name: string
  type: string
  description: string
  required?: boolean
}

export interface CommandExample {
  command: string
  description?: string
}

export interface Command {
  id: string
  slug: string
  name: string
  syntax: string
  description: string
  parameters: CommandParameter[]
  examples: CommandExample[]
  permission_notes: string
  game_versions: string[]
  source_references: Array<{ title: string; url: string }>
  status: 'published' | 'draft'
  created_at: string
  updated_at: string
}

/** 指令列表查询参数 */
export interface CommandQuery {
  q?: string
  page?: number
  page_size?: number
}