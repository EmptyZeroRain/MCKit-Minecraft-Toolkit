// ===== 后台运维任务 API（gj.md 阶段二：任务重试与失败详情）=====
// 契约来源：backend/internal/http/handler/jobs.go
// 权限：ops.jobs（入队 / 查询 / 重试 / 取消）。
import http, { toPaged } from './http'
import type { ApiResponse } from '@/types/api'
import type { AdminJob, JobListQuery, JobTypeInfo } from '@/types/jobs'

/** 已注册的任务类型（用于「新建任务」下拉） */
export async function listJobTypes(): Promise<JobTypeInfo[]> {
  const { data } = await http.post<ApiResponse<JobTypeInfo[]>>('/admin/jobs/types')
  return data.data
}

/** 分页查询任务（可按状态 / 类型筛选） */
export async function listJobs(params: JobListQuery = {}) {
  const { data } = await http.post<ApiResponse<AdminJob[]>>('/admin/jobs/list', params)
  return toPaged({ data })
}

/** 任务详情（含 payload / result / last_error） */
export async function getJob(id: string): Promise<AdminJob> {
  const { data } = await http.post<ApiResponse<AdminJob>>('/admin/jobs/detail', { id })
  return data.data
}

/** 入队一个新任务 */
export async function enqueueJob(type: string, payload: Record<string, unknown> = {}): Promise<AdminJob> {
  const { data } = await http.post<ApiResponse<AdminJob>>('/admin/jobs/enqueue', { type, payload })
  return data.data
}

/** 人工重试失败 / 已取消的任务（新建一条，保留原始失败现场） */
export async function retryJob(id: string): Promise<AdminJob> {
  const { data } = await http.post<ApiResponse<AdminJob>>('/admin/jobs/retry', { id })
  return data.data
}

/** 取消排队中的任务（执行中的任务不可取消） */
export async function cancelJob(id: string): Promise<AdminJob> {
  const { data } = await http.post<ApiResponse<AdminJob>>('/admin/jobs/cancel', { id })
  return data.data
}
