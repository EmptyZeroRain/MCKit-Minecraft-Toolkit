// 内容建设任务类型（gj.md §8.3 搜索词闭环 / §15 内容任务）
//
// 任务把「无结果搜索词」沉淀为可指派、可跟踪的运营动作。任务只保存归一化
// 关键词与聚合计数快照，不含 IP / 会话 / 用户标识。

/** 任务处理状态 */
export type ContentTaskStatus = 'open' | 'in_progress' | 'done' | 'ignored'

/** 可关联的内容类型（空串表示尚未关联） */
export type ContentTaskContentType = '' | 'resource' | 'tool' | 'wiki'

/** 单个内容建设任务 */
export interface ContentTask {
  id: string
  /** 目标关键词（归一化搜索词，与搜索词统计同口径） */
  keyword: string
  /** 关联的搜索词统计行；统计行被清理时为 null */
  search_term_id: string | null
  /** 创建时的搜索总次数快照 */
  hits: number
  /** 创建时的无结果次数快照 */
  no_result_hits: number
  /** 关键词最近出现时间 */
  last_seen_at: string | null
  status: ContentTaskStatus
  /** 负责人账号 ID（后台账号） */
  assignee_id: string | null
  /** 负责人展示名（冗余保存，账号被删除后仍可读） */
  assignee_name: string
  related_content_type: ContentTaskContentType
  /** 为填补缺口而创建/更新的内容 ID */
  related_content_id: string | null
  note: string
  created_by: string | null
  created_at: string
  updated_at: string
}

/** 无结果词候选（达到阈值、可直接转为任务） */
export interface ContentTaskCandidate {
  term: string
  hits: number
  no_result_hits: number
  last_seen_at: string
  /** 该关键词已存在的未完结任务 ID；空串表示可直接创建 */
  task_id: string
}

/** 任务列表查询参数 */
export interface ContentTaskQuery {
  status?: ContentTaskStatus | ''
  /** 关键词模糊过滤 */
  q?: string
  page?: number
  page_size?: number
}

/** 从关键词创建任务 */
export interface CreateContentTaskPayload {
  keyword: string
  note?: string
  assignee_id?: string
}

/** 更新任务（仅传入的字段生效；assignee_id / related_content_id 传空串表示清空） */
export interface UpdateContentTaskPayload {
  id: string
  status?: ContentTaskStatus
  assignee_id?: string
  related_content_type?: ContentTaskContentType
  related_content_id?: string
  note?: string
}

/** 「重新检测」结果：当前关键词的搜索结果数（不自动改变任务状态） */
export interface ContentTaskRecheckResult {
  task_id: string
  keyword: string
  result_count: number
  /** 当前是否已有搜索结果（true 时可标记完成） */
  has_results: boolean
}

/** 状态展示名（顺序即流程顺序） */
export const CONTENT_TASK_STATUS_LABELS: Record<ContentTaskStatus, string> = {
  open: '待处理',
  in_progress: '处理中',
  done: '已完成',
  ignored: '已忽略',
}

/** 状态徽标配色类名 */
export const CONTENT_TASK_STATUS_CLASSES: Record<ContentTaskStatus, string> = {
  open: 'bg-amber-500/15 text-amber-300',
  in_progress: 'bg-sky-500/15 text-sky-300',
  done: 'bg-grass-500/15 text-grass-300',
  ignored: 'bg-ink-600 text-text-muted',
}

/** 关联内容类型展示名 */
export const CONTENT_TASK_CONTENT_LABELS: Record<Exclude<ContentTaskContentType, ''>, string> = {
  resource: '资源',
  tool: '工具',
  wiki: '百科',
}

/** 内容任务状态选项（用于筛选 / 下拉，按流程顺序） */
export const CONTENT_TASK_STATUS_OPTIONS: Array<{ value: ContentTaskStatus; label: string }> = [
  { value: 'open', label: CONTENT_TASK_STATUS_LABELS.open },
  { value: 'in_progress', label: CONTENT_TASK_STATUS_LABELS.in_progress },
  { value: 'done', label: CONTENT_TASK_STATUS_LABELS.done },
  { value: 'ignored', label: CONTENT_TASK_STATUS_LABELS.ignored },
]
