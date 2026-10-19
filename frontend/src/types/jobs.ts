// 后台运维任务（后端 domain.AdminJob + service/jobs）。
//
// 任务与 HTTP 请求解耦：接口只负责入队，执行由进程内调度协程完成，
// 因此前端需要轮询列表获取状态，并展示失败详情 / 重试入口。

export type JobStatus = 'pending' | 'running' | 'succeeded' | 'failed' | 'cancelled'

/** 任务类型（由后端注册表下发的描述） */
export interface JobTypeInfo {
  type: string
  desc: string
}

/** 一条后台任务 */
export interface AdminJob {
  id: string
  type: string
  status: JobStatus
  /** 任务入参（结构由任务类型约定） */
  payload: Record<string, unknown>
  /** 任务产出摘要 */
  result: Record<string, unknown>
  /** 已执行次数（含自动重试） */
  attempts: number
  /** 自动重试上限 */
  max_attempts: number
  /** 最近一次失败原因（已脱敏） */
  last_error: string
  /** 人工重试来源任务 ID */
  retry_of?: string
  created_by: string
  next_run_at: string
  started_at?: string
  finished_at?: string
  created_at: string
  updated_at: string
}

export interface JobListQuery {
  status?: JobStatus | ''
  type?: string
  page?: number
  page_size?: number
}

export const JOB_STATUS_LABELS: Record<JobStatus, string> = {
  pending: '排队中',
  running: '执行中',
  succeeded: '已成功',
  failed: '已失败',
  cancelled: '已取消',
}

/** 复用 StatusBadge 调色板（仅借用颜色，文字由 label 覆盖） */
export const JOB_BADGE_STATUS: Record<JobStatus, string> = {
  pending: 'processing',
  running: 'processing',
  succeeded: 'published',
  failed: 'removed',
  cancelled: 'draft',
}

/** 是否处于「运行中」状态（前端据此决定是否继续轮询） */
export function isJobActive(job: AdminJob): boolean {
  return job.status === 'pending' || job.status === 'running'
}
