// 运营告警类型（后端 domain.OpsAlert + service/alert）。

export type AlertStatus = 'open' | 'acked' | 'resolved'
export type AlertSeverity = 'warning' | 'error'

export interface OpsAlert {
  id: string
  rule_key: string
  severity: AlertSeverity
  status: AlertStatus
  title: string
  detail: string
  /** 触发时的指标快照 */
  metric: Record<string, unknown>
  dedupe_key: string
  triggered_at: string
  /** 最近一次仍然命中的时间 */
  last_seen_at: string
  acked_by: string
  acked_at?: string
  resolved_by: string
  resolved_at?: string
  created_at: string
  updated_at: string
}

export interface AlertSummary {
  open: number
  acked: number
  error: number
  warning: number
}

export interface AlertListQuery {
  status?: AlertStatus | ''
  severity?: AlertSeverity | ''
  rule_key?: string
  page?: number
  page_size?: number
}

export interface AlertEvaluateSummary {
  evaluated: number
  triggered: number
  refreshed: number
  auto_closed: number
  errors?: string[]
}

export const ALERT_STATUS_LABELS: Record<AlertStatus, string> = {
  open: '待处理',
  acked: '已确认',
  resolved: '已关闭',
}

export const ALERT_SEVERITY_LABELS: Record<AlertSeverity, string> = {
  warning: '提醒',
  error: '严重',
}

/** 复用 StatusBadge 调色板（仅借用颜色，文字由 label 覆盖） */
export const ALERT_BADGE_STATUS: Record<AlertStatus, string> = {
  open: 'failure',
  acked: 'processing',
  resolved: 'resolved',
}
