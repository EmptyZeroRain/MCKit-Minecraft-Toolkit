// 资源链接健康与人工确认（TODO §6）类型与展示常量。
import type { PagedData } from '@/api/http'

/** 链接检查状态（与后端 domain 链接状态同口径）。 */
export type LinkCheckStatus = 'unchecked' | 'ok' | 'redirected' | 'broken' | 'blocked' | 'error'

/** 人工复核状态。 */
export type ReviewState = 'pending' | 'confirmed_broken' | 'confirmed_ok' | 'ignored'

/** 触发来源：auto = 后台批量复查，manual = 管理员手动触发。 */
export type LinkTriggerSource = 'auto' | 'manual'

export interface LinkHealthItem {
  id: string
  resource_id: string
  resource_name: string
  resource_slug: string
  url: string
  status: LinkCheckStatus
  status_code: number
  final_url: string
  trigger_source: LinkTriggerSource
  review_state: ReviewState
  reviewed_by: string
  reviewed_at: string | null
  review_note: string
  checked_at: string
  created_at: string
  updated_at: string
}

/** 批量复查汇总。 */
export interface LinkHealthRunSummary {
  checked: number
  ok: number
  redirected: number
  broken: number
  blocked: number
  error: number
  concurrency: number
  trigger_source: LinkTriggerSource
}

export interface LinkHealthQuery {
  review_state?: ReviewState | ''
  status?: LinkCheckStatus | ''
  failed_only?: boolean
  page?: number
  page_size?: number
}

export interface LinkHealthRunParams {
  resource_id?: string
  only_pending?: boolean
  limit?: number
}

export type LinkHealthList = PagedData<LinkHealthItem>

/** 复核动作。 */
export type ReviewAction = 'confirm_broken' | 'confirm_ok' | 'ignore'

export const LINK_CHECK_STATUS_LABELS: Record<LinkCheckStatus, string> = {
  unchecked: '未检查',
  ok: '正常',
  redirected: '已重定向',
  broken: '已失效',
  blocked: '被拦截',
  error: '请求错误',
}

export const REVIEW_STATE_LABELS: Record<ReviewState, string> = {
  pending: '待确认',
  confirmed_broken: '已确认失效',
  confirmed_ok: '已确认可用',
  ignored: '暂不处理',
}

/** 失败状态集合（默认复核列表只看这些）。 */
export const FAILED_STATUSES: LinkCheckStatus[] = ['broken', 'blocked', 'error']

export function isFailedStatus(status: LinkCheckStatus): boolean {
  return FAILED_STATUSES.includes(status)
}

/** 复核动作展示名。 */
export const REVIEW_ACTION_LABELS: Record<ReviewAction, string> = {
  confirm_broken: '确认失效',
  confirm_ok: '确认可用',
  ignore: '暂不处理',
}

/** 状态徽章配色（Tailwind 类）。 */
export function statusClass(status: LinkCheckStatus): string {
  switch (status) {
    case 'ok':
      return 'border-grass-500/40 bg-grass-500/10 text-grass-300'
    case 'redirected':
      return 'border-sky-500/40 bg-sky-500/10 text-sky-300'
    case 'broken':
      return 'border-red-500/40 bg-red-500/10 text-red-300'
    case 'blocked':
      return 'border-amber-500/40 bg-amber-500/10 text-amber-300'
    default:
      return 'border-border-light bg-ink-700 text-text-secondary'
  }
}
