// ===== 资源投稿审核 API（后台，全 POST + 动作子路径）=====
// 契约来源：backend/internal/http/handler/submission.go
// 权限：content.resources（审核通过会创建 resources 记录，与资源管理同权能）。
import http, { toPaged } from './http'
import type { ApiResponse } from '@/types/api'
import type { BatchReviewResult } from '@/types/batch'
import type { Resource } from '@/types/resource'
import type {
  ResourceSubmission,
  ReviewSubmissionPayload,
  SubmissionStatus,
} from '@/types/submission'

/** 审核列表查询条件 */
export interface SubmissionListQuery {
  status?: SubmissionStatus | ''
  resource_type?: string
  q?: string
  page?: number
  page_size?: number
}

/** 投稿审核列表（分页 + 筛选，默认由调用方传 pending） */
export async function listSubmissions(params: SubmissionListQuery = {}) {
  const { data } = await http.post<ApiResponse<ResourceSubmission[]>>(
    '/admin/submissions/list',
    params,
  )
  return toPaged({ data })
}

/** 投稿详情 */
export async function getSubmission(id: string): Promise<ResourceSubmission | null> {
  const { data } = await http.post<ApiResponse<ResourceSubmission>>('/admin/submissions/detail', {
    id,
  })
  return data.data
}

/** 审核结果：通过时额外返回生成的资源（默认草稿） */
export interface ReviewSubmissionResult {
  submission: ResourceSubmission
  resource?: Resource
}

/** 审核投稿（approved = 通过并生成资源；rejected = 驳回） */
export async function reviewSubmission(
  payload: ReviewSubmissionPayload,
): Promise<ReviewSubmissionResult> {
  const { data } = await http.post<ApiResponse<ReviewSubmissionResult>>(
    '/admin/submissions/review',
    payload,
  )
  return data.data
}

/**
 * 批量审核投稿（单次最多 50 条）。
 *
 * 后端逐条独立执行，返回逐条结果：部分失败（如「已被他人审核」）是正常情况，
 * 调用方必须展示失败明细而不是当作整体失败。
 */
export async function batchReviewSubmissions(params: {
  ids: string[]
  status: 'approved' | 'rejected'
  note?: string
  publish?: boolean
}): Promise<BatchReviewResult> {
  const { data } = await http.post<ApiResponse<BatchReviewResult>>(
    '/admin/submissions/batch-review',
    params,
  )
  return data.data
}
