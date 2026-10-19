// 数据质量与导入任务类型（TODO §2 版本化资料规则 / §4 数据质量监控）

/** 检查项严重级别：error 表示数据不一致，warning 表示字段缺失 */
export type CheckSeverity = 'error' | 'warning' | 'info'

/** 检查项类别：对应资料库的不同数据域 */
export type CheckCategory = 'items' | 'recipes' | 'commands' | 'import' | string

/** 单项数据质量检查结果 */
export interface DataQualityCheck {
  key: string
  label: string
  category: CheckCategory
  severity: CheckSeverity
  /** 命中问题的条目数 */
  count: number
  /** 该数据域总条目数（info 类等于 count） */
  total: number
  hint: string
}

/** 数据质量概览 */
export interface DataQualityOverview {
  generated_at: string
  checks: DataQualityCheck[]
  /** 存在严重问题的检查项数 */
  error_checks: number
  /** 存在提醒的检查项数 */
  warning_checks: number
}

/** 一次数据导入任务的执行记录 */
export interface DataImportJob {
  id: string
  data_type: string
  source_file: string
  source_version: string
  dry_run: boolean
  total_count: number
  created_count: number
  updated_count: number
  skipped_count: number
  failed_count: number
  error_summary: string[] | null
  operator: string
  started_at: string
  finished_at: string | null
  created_at: string
}

/** 导入任务列表查询参数 */
export interface DataImportJobQuery {
  data_type?: string
  page?: number
  page_size?: number
}
