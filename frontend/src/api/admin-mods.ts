// ===== Mod 与整合包内容审核 API（TODO §16，全 POST + 动作子路径，admin 专属）=====
// 对应后端 handler.ModAdminHandler：按审核状态 / 加载器筛选 + 审核留痕。
import http, { toPaged } from './http'
import type { ApiResponse } from '@/types/api'
import type { Mod, ModLoader, Modpack, ModReviewStatus } from '@/types/mods'

/** 管理端 Mod / 整合包列表查询参数（对齐后端 modListRequest） */
export interface ModAdminQuery {
  /** 审核状态；缺省表示不过滤 */
  review_status?: ModReviewStatus | ''
  /** 加载器；缺省表示不过滤 */
  loader?: ModLoader | ''
  page?: number
  page_size?: number
}

/** 分页查询 Mod（不过滤发布态，供审核使用）。 */
export async function listModsAdmin(params: ModAdminQuery = {}) {
  const { data } = await http.post<ApiResponse<Mod[]>>('/admin/mods/list', params)
  return toPaged({ data })
}

/** 审核 Mod：更新审核状态并留痕（审核人取自登录会话）。 */
export async function reviewMod(
  id: string,
  reviewStatus: ModReviewStatus,
  note = '',
): Promise<Mod> {
  const { data } = await http.post<ApiResponse<Mod>>('/admin/mods/review', {
    id,
    review_status: reviewStatus,
    note,
  })
  return data.data
}

/** 分页查询整合包（不过滤发布态，供审核使用）。 */
export async function listModpacksAdmin(params: ModAdminQuery = {}) {
  const { data } = await http.post<ApiResponse<Modpack[]>>('/admin/modpacks/list', params)
  return toPaged({ data })
}

/** 审核整合包：更新审核状态并留痕（审核人取自登录会话）。 */
export async function reviewModpack(
  id: string,
  reviewStatus: ModReviewStatus,
  note = '',
): Promise<Modpack> {
  const { data } = await http.post<ApiResponse<Modpack>>('/admin/modpacks/review', {
    id,
    review_status: reviewStatus,
    note,
  })
  return data.data
}
