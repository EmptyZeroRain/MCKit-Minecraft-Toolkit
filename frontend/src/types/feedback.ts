// ===== 反馈 / 投稿实体类型（TODO §5.10、§17）=====
import type { FeedbackStatus, FeedbackTargetKind, FeedbackType } from './common'

export interface Feedback {
  id: string
  feedback_type: FeedbackType
  title: string
  content: string
  related_url: string | null
  contact: string | null
  status: FeedbackStatus
  /** 关联的资料条目类型与标识（slug / item_id） */
  target_kind: FeedbackTargetKind
  target_ref: string
  /** 最近一次审核的审核人、时间与结论 */
  reviewer: string
  reviewed_at: string | null
  review_note: string
  /** 投稿时接受的授权条款版本与时间 */
  license_version: string
  license_accepted_at: string | null
  created_at: string
  updated_at: string
}

/** 一次审核流转记录（TODO §17：审核人、审核结果与变更历史） */
export interface FeedbackReview {
  id: string
  feedback_id: string
  from_status: FeedbackStatus
  to_status: FeedbackStatus
  reviewer: string
  note: string
  created_at: string
}

/** 公开提交反馈 / 投稿参数 */
export interface FeedbackSubmitPayload {
  feedback_type: FeedbackType
  title: string
  content: string
  related_url?: string
  contact?: string
  target_kind?: FeedbackTargetKind
  target_ref?: string
  /** 投稿类型必须携带当前授权条款版本 */
  license_version?: string
}

/** 管理端列表查询 */
export interface FeedbackListQuery {
  status?: FeedbackStatus | ''
  feedback_type?: FeedbackType | ''
  page?: number
  page_size?: number
}

/** 管理端审核（更新状态 + 审核备注） */
export interface FeedbackUpdatePayload {
  id: string
  status: FeedbackStatus
  note?: string
}
