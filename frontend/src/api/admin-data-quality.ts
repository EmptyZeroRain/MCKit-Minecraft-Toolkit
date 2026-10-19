// ===== 数据质量 / 导入任务 API（全 POST + 动作子路径，admin 专属）=====
import http, { toPaged } from './http'
import type { ApiResponse } from '@/types/api'
import type {
  DataImportJob,
  DataImportJobQuery,
  DataQualityOverview,
  QualityAssignee,
  QualityIssueState,
  QualityIssueStatus,
} from '@/types/data-quality'

/** 获取数据质量概览（检查项 + 汇总） */
export async function fetchDataQualityOverview(): Promise<DataQualityOverview> {
  const { data } = await http.post<ApiResponse<DataQualityOverview>>('/admin/data-quality/overview')
  return data.data
}

/** 分页查询数据导入任务，可按 data_type 过滤 */
export async function listDataImportJobs(params: DataImportJobQuery = {}) {
  const { data } = await http.post<ApiResponse<DataImportJob[]>>('/admin/data-import-jobs/list', params)
  return toPaged({ data })
}

/** 可指派的数据质量负责人（启用中的管理员，昵称优先） */
export async function listQualityAssignees(): Promise<QualityAssignee[]> {
  const { data } = await http.post<ApiResponse<QualityAssignee[]>>('/admin/data-quality/assignees')
  return data.data
}

/** 新建 / 更新数据质量问题工单（分派、解决、忽略、重新打开） */
export async function updateQualityIssue(params: {
  check_key: string
  status: QualityIssueStatus
  assignee_id?: string
  note?: string
  snapshot_count?: number
}): Promise<QualityIssueState> {
  const { data } = await http.post<ApiResponse<QualityIssueState>>(
    '/admin/data-quality/issues/update',
    params,
  )
  return data.data
}
