// ===== 搜索词统计 API（全 POST + 动作子路径，admin 专属）=====
import http, { toPaged } from './http'
import type { ApiResponse } from '@/types/api'
import type { SearchTermQuery, SearchTermStat } from '@/types/search-term'

/** 分页查询搜索词统计；kind=hot（热门）/ no_result（高频无结果） */
export async function listSearchTerms(params: SearchTermQuery = {}) {
  const { data } = await http.post<ApiResponse<SearchTermStat[]>>('/admin/search-terms/list', params)
  return toPaged({ data })
}
