// ===== 反馈 / 审计日志管理 API（全 POST + 动作子路径）=====
import http, { toPaged } from './http'
import type { ApiResponse } from '@/types/api'
import type { Feedback, FeedbackListQuery, FeedbackReview, FeedbackUpdatePayload } from '@/types/feedback'
import type { AuditLog, AuditLogQuery } from '@/types/audit'

// ===== 反馈 =====
export async function listFeedback(params: FeedbackListQuery = {}) {
  const { data } = await http.post<ApiResponse<Feedback[]>>('/admin/feedback/list', params)
  return toPaged({ data })
}

export async function updateFeedbackStatus(payload: FeedbackUpdatePayload): Promise<Feedback> {
  const { data } = await http.post<ApiResponse<Feedback>>('/admin/feedback/update', payload)
  return data.data
}

/** 审核变更历史（TODO §17：记录审核人、审核结果和变更历史） */
export async function listFeedbackReviews(id: string): Promise<FeedbackReview[]> {
  const { data } = await http.post<ApiResponse<FeedbackReview[]>>('/admin/feedback/reviews', { id })
  return data.data
}

// ===== 审计日志 =====
export async function listAuditLogs(params: AuditLogQuery = {}) {
  const { data } = await http.post<ApiResponse<AuditLog[]>>('/admin/audit-logs/list', params)
  return toPaged({ data })
}

/** 导出审计日志 CSV；返回文件内容与是否被上限截断 */
export async function exportAuditLogs(
  params: AuditLogQuery = {},
): Promise<{ blob: Blob; truncated: boolean }> {
  const res = await http.post('/admin/audit-logs/export', params, { responseType: 'blob' })
  return {
    blob: res.data as Blob,
    truncated: String(res.headers['x-export-truncated'] ?? '') === 'true',
  }
}