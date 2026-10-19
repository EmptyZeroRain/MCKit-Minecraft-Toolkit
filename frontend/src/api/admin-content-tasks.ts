// ===== 内容建设任务 API（全 POST + 动作子路径，需 ops.contenttasks 权限）=====
import http, { toPaged } from './http'
import type { ApiResponse } from '@/types/api'
import type {
  ContentTask,
  ContentTaskCandidate,
  ContentTaskQuery,
  ContentTaskRecheckResult,
  CreateContentTaskPayload,
  UpdateContentTaskPayload,
} from '@/types/content-task'

/** 分页查询内容建设任务 */
export async function listContentTasks(params: ContentTaskQuery = {}) {
  const { data } = await http.post<ApiResponse<ContentTask[]>>('/admin/content-tasks/list', params)
  return toPaged({ data })
}

/** 无结果词候选（达到阈值、尚无未完结任务） */
export async function listContentTaskCandidates(limit = 20): Promise<ContentTaskCandidate[]> {
  const { data } = await http.post<ApiResponse<ContentTaskCandidate[]>>(
    '/admin/content-tasks/candidates',
    { limit },
  )
  return data.data ?? []
}

/** 从关键词创建任务（快照当前搜索统计） */
export async function createContentTask(payload: CreateContentTaskPayload): Promise<ContentTask> {
  const { data } = await http.post<ApiResponse<ContentTask>>(
    '/admin/content-tasks/create',
    payload,
  )
  return data.data
}

/** 更新任务（状态 / 负责人 / 关联内容 / 备注） */
export async function updateContentTask(payload: UpdateContentTaskPayload): Promise<ContentTask> {
  const { data } = await http.post<ApiResponse<ContentTask>>(
    '/admin/content-tasks/update',
    payload,
  )
  return data.data
}

/** 重新检测关键词的搜索结果（不自动改变任务状态） */
export async function recheckContentTask(id: string): Promise<ContentTaskRecheckResult> {
  const { data } = await http.post<ApiResponse<ContentTaskRecheckResult>>(
    '/admin/content-tasks/recheck',
    { id },
  )
  return data.data
}

/** 删除任务（用于纠错，不影响搜索统计） */
export async function deleteContentTask(id: string): Promise<void> {
  await http.post('/admin/content-tasks/delete', { id })
}
