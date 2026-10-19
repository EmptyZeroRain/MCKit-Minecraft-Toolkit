// ===== 红石机器设计内容审核 API（TODO §15，全 POST + 动作子路径，admin 专属）=====
// 对应后端 handler.RedstoneAdminHandler：按审核状态 / 用途筛选 + 审核留痕。
import http, { toPaged } from './http'
import type { ApiResponse } from '@/types/api'
import type { BatchReviewResult } from '@/types/batch'
import type { RedstoneDesign, RedstoneDesignPurpose, RedstoneReviewStatus } from '@/types/redstone'

/** 管理端机器设计列表查询参数（对齐后端 redstoneDesignListRequest） */
export interface RedstoneDesignAdminQuery {
  /** 审核状态；缺省表示不过滤 */
  review_status?: RedstoneReviewStatus | ''
  /** 设计用途；缺省表示不过滤 */
  purpose?: RedstoneDesignPurpose | ''
  page?: number
  page_size?: number
}

/** 分页查询机器设计（不过滤发布态，供审核使用）。 */
export async function listRedstoneDesignsAdmin(params: RedstoneDesignAdminQuery = {}) {
  const { data } = await http.post<ApiResponse<RedstoneDesign[]>>('/admin/redstone/designs/list', params)
  return toPaged({ data })
}

/** 审核机器设计：更新审核状态并留痕（审核人取自登录会话）。 */
export async function reviewRedstoneDesign(
  id: string,
  reviewStatus: RedstoneReviewStatus,
  note = '',
): Promise<RedstoneDesign> {
  const { data } = await http.post<ApiResponse<RedstoneDesign>>('/admin/redstone/designs/review', {
    id,
    review_status: reviewStatus,
    note,
  })
  return data.data
}

/** 批量审核红石设计（单次最多 50 条；逐条返回结果）。 */
export async function batchReviewRedstoneDesigns(
  ids: string[],
  reviewStatus: RedstoneReviewStatus,
  note = '',
): Promise<BatchReviewResult> {
  const { data } = await http.post<ApiResponse<BatchReviewResult>>(
    '/admin/redstone/designs/batch-review',
    { ids, review_status: reviewStatus, note },
  )
  return data.data
}
