// ===== 物品 ID 对照表 API（全 POST + body）=====
// 契约：POST /api/v1/block-ids  body: { q?, kind? } -> { total, entries }
import http from './http'
import type { ApiResponse } from '@/types/api'
import type { BlockIdListResponse, BlockIdQuery } from '@/types/block-id'

/** 物品对照表查询（数据量约千余条，通常一次全量加载后本地过滤） */
export async function listBlockIds(params: BlockIdQuery = {}): Promise<BlockIdListResponse> {
  const { data } = await http.post<ApiResponse<BlockIdListResponse>>('/block-ids', params)
  return data.data
}