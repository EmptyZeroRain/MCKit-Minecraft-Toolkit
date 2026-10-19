// ===== 内容历史版本类型（gj.md §15 / §16.2）=====
// 与后端 /admin/revisions/{resources|tools|wiki}/* 的响应契约保持一致。

/** 可版本化的内容类型（对应后端 content_revisions.content_type） */
export type RevisionContentType = 'resource' | 'tool' | 'wiki'

/** 版本动作：新建 / 更新 / 回滚（回滚同样追加新版本，不覆盖历史） */
export type RevisionAction = 'create' | 'update' | 'rollback'

/** 历史版本完整记录（含整行快照） */
export interface ContentRevision {
  id: string
  content_type: RevisionContentType
  content_id: string
  revision_no: number
  action: RevisionAction
  snapshot: Record<string, unknown>
  change_summary: string[]
  actor_id: string | null
  actor_name: string
  note: string
  created_at: string
}

/** 历史版本列表项（不含快照，避免列表响应过大） */
export interface RevisionListItem {
  id: string
  content_type: RevisionContentType
  content_id: string
  revision_no: number
  action: RevisionAction
  change_summary: string[]
  actor_name: string
  note: string
  created_at: string
}

/** 单个字段的差异（before 为空表示新增字段，after 为 null 表示删除字段） */
export interface RevisionFieldChange {
  field: string
  before: unknown
  after: unknown
}

/** 某版本相对上一版的差异 */
export interface RevisionDiff {
  revision: ContentRevision
  previous?: ContentRevision
  changes: RevisionFieldChange[]
}
