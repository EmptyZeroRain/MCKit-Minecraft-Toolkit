// ===== 数据质量 / 导入任务 API（全 POST + 动作子路径，admin 专属）=====
import http, { toPaged } from './http'
import type { ApiResponse } from '@/types/api'
import type { DataQualityOverview, DataImportJob, DataImportJobQuery } from '@/types/data-quality'

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
