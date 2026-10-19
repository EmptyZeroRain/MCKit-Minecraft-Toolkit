// ===== MC 版本信息 API（全 POST + body）=====
// 契约（后端由他人实现，移植自 CreeperCode 的版本信息站）：
//   POST /api/v1/versions        body: { type?, q? }  -> { latest, versions }
//   POST /api/v1/versions/detail body: { id }         -> VersionDetail
import http from './http'
import type { ApiResponse } from '@/types/api'
import type {
  VersionDetail,
  VersionDetailQuery,
  VersionListQuery,
  VersionListResponse,
} from '@/types/version'

/** 版本清单（支持类型过滤与关键词搜索） */
export async function listVersions(params: VersionListQuery = {}): Promise<VersionListResponse> {
  const { data } = await http.post<ApiResponse<VersionListResponse>>('/versions', params)
  return data.data
}

/** 版本详情（下载地址、启动命令、Java 要求、运行库） */
export async function getVersionDetail(params: VersionDetailQuery): Promise<VersionDetail> {
  const { data } = await http.post<ApiResponse<VersionDetail>>('/versions/detail', params)
  return data.data
}