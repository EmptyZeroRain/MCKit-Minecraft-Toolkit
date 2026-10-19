// ===== 运营告警 API（gj.md 阶段三：告警与告警历史）=====
// 契约来源：backend/internal/http/handler/alert.go
// 权限：ops.alerts。
import http, { toPaged } from './http'
import type { ApiResponse } from '@/types/api'
import type {
  AlertEvaluateSummary,
  AlertListQuery,
  AlertSummary,
  OpsAlert,
} from '@/types/alert'

/** 未关闭告警计数（按状态 / 级别） */
export async function fetchAlertSummary(): Promise<AlertSummary> {
  const { data } = await http.post<ApiResponse<AlertSummary>>('/admin/alerts/summary')
  return data.data
}

/** 分页查询告警（含已关闭历史） */
export async function listAlerts(params: AlertListQuery = {}) {
  const { data } = await http.post<ApiResponse<OpsAlert[]>>('/admin/alerts/list', params)
  return toPaged({ data })
}

/** 立即评估全部规则（定时评估默认 5 分钟一次） */
export async function evaluateAlerts(): Promise<AlertEvaluateSummary> {
  const { data } = await http.post<ApiResponse<AlertEvaluateSummary>>('/admin/alerts/evaluate')
  return data.data
}

/** 确认告警（表示已有人接手） */
export async function ackAlert(id: string): Promise<OpsAlert> {
  const { data } = await http.post<ApiResponse<OpsAlert>>('/admin/alerts/ack', { id })
  return data.data
}

/** 关闭告警并记录原因（历史保留） */
export async function resolveAlert(id: string, note = ''): Promise<OpsAlert> {
  const { data } = await http.post<ApiResponse<OpsAlert>>('/admin/alerts/resolve', { id, note })
  return data.data
}
