// ===== 资源链接健康与人工确认 API（TODO §6，全 POST + 动作子路径，admin 专属）=====
import http, { toPaged } from './http'
import type { ApiResponse } from '@/types/api'
import type {
  LinkHealthItem,
  LinkHealthQuery,
  LinkHealthRunParams,
  LinkHealthRunSummary,
  ReviewAction,
} from '@/types/link-health'

/** 后台批量复查资源来源链接（受限并发，服务端限流）。 */
export async function runLinkHealthCheck(params: LinkHealthRunParams = {}): Promise<LinkHealthRunSummary> {
  const { data } = await http.post<ApiResponse<LinkHealthRunSummary>>('/admin/link-health/run', params)
  return data.data
}

/** 分页查询复核列表（默认只看失败项）。 */
export async function listLinkHealth(params: LinkHealthQuery = {}) {
  const { data } = await http.post<ApiResponse<LinkHealthItem[]>>('/admin/link-health/list', params)
  return toPaged({ data })
}

/** 人工确认链接结论。 */
export async function confirmLinkHealth(
  id: string,
  action: ReviewAction,
  note = '',
): Promise<LinkHealthItem> {
  const { data } = await http.post<ApiResponse<LinkHealthItem>>('/admin/link-health/confirm', {
    id,
    action,
    note,
  })
  return data.data
}
