// ===== 礼物币 / 礼物中心 API =====
// 用户端使用 userHttp；后台管理使用 http（管理员会话 + CSRF）。
import http from './http'
import userHttp from './user-http'
import type { ApiResponse, ListResult, PageQuery } from '@/types/api'
import type {
  GiftAdjustInput,
  GiftCatalogItem,
  GiftCheckinRankingItem,
  GiftCheckinResult,
  GiftCoinRankingItem,
  GiftDefinition,
  GiftDefinitionInput,
  GiftRankingItem,
  GiftLedgerFilter,
  GiftLedgerItem,
  GiftOverview,
  GiftPublicLedgerItem,
  GiftRankingFilter,
  GiftRedeemResult,
  GiftRedemption,
  GiftSettings,
  GiftWallet,
} from '@/types/gift'

function toListResult<T>(data: ApiResponse<T[]>): ListResult<T> {
  const meta = data.meta
  return {
    items: data.data ?? [],
    total: meta?.total ?? 0,
    page: meta?.page ?? 1,
    pageSize: meta?.page_size ?? 0,
    totalPages: meta?.total_pages ?? 0,
  }
}

// ---- 用户端 ----

/** 本人余额 + 最近流水 + 今日签到状态 */
export async function fetchGiftOverview(): Promise<GiftOverview> {
  const { data } = await userHttp.post<ApiResponse<GiftOverview>>('/gifts/overview', {})
  return data.data
}

/** 本人礼物币流水（分页） */
export async function fetchGiftLedger(
  params: Omit<GiftLedgerFilter, 'user_id'> = {},
): Promise<ListResult<GiftPublicLedgerItem>> {
  const { data } = await userHttp.post<ApiResponse<GiftPublicLedgerItem[]>>(
    '/gifts/ledger/list',
    params,
  )
  return toListResult(data)
}

/** 可兑换礼物目录（仅启用项） */
export async function fetchGiftCatalog(): Promise<GiftCatalogItem[]> {
  const { data } = await userHttp.post<ApiResponse<GiftCatalogItem[]>>('/gifts/catalog', {})
  return data.data ?? []
}

/** 每日签到，获得签到礼物币 */
export async function checkinGift(): Promise<GiftCheckinResult> {
  const { data } = await userHttp.post<ApiResponse<GiftCheckinResult>>('/gifts/checkin', {})
  return data.data
}

/** 兑换礼物；requestKey 用于防重复扣款 */
export async function redeemGift(giftId: string, requestKey: string): Promise<GiftRedeemResult> {
  const { data } = await userHttp.post<ApiResponse<GiftRedeemResult>>('/gifts/redeem', {
    gift_id: giftId,
    request_key: requestKey,
  })
  return data.data
}

/** 本人礼物兑换记录（分页） */
export async function fetchGiftRedemptions(
  params: PageQuery = {},
): Promise<ListResult<GiftRedemption>> {
  const { data } = await userHttp.post<ApiResponse<GiftRedemption[]>>(
    '/gifts/redemptions/list',
    params,
  )
  return toListResult(data)
}

// ---- 后台 ----

/** 查询指定用户钱包（不存在时返回 0 余额） */
export async function fetchAdminGiftWallet(targetUserId: string): Promise<GiftWallet> {
  const { data } = await http.post<ApiResponse<GiftWallet>>('/admin/gifts/wallet/get', {
    target_user_id: targetUserId,
  })
  return data.data
}

/** 后台受控增减礼物币（正数增加，负数扣减，余额不得为负） */
export async function adjustGiftWallet(payload: GiftAdjustInput): Promise<GiftLedgerItem> {
  const { data } = await http.post<ApiResponse<GiftLedgerItem>>(
    '/admin/gifts/wallet/adjust',
    payload,
  )
  return data.data
}

/** 后台礼物币流水（分页，可按用户 / 币种 / 事件类型过滤） */
export async function fetchAdminGiftLedger(
  filter: GiftLedgerFilter = {},
): Promise<ListResult<GiftLedgerItem>> {
  const { data } = await http.post<ApiResponse<GiftLedgerItem[]>>(
    '/admin/gifts/ledger/list',
    filter,
  )
  return toListResult(data)
}

/** 后台礼物目录（含停用项） */
export async function fetchGiftDefinitions(): Promise<GiftDefinition[]> {
  const { data } = await http.post<ApiResponse<GiftDefinition[]>>(
    '/admin/gifts/definitions/list',
    {},
  )
  return data.data ?? []
}

/** 新建礼物 */
export async function createGiftDefinition(
  payload: GiftDefinitionInput,
): Promise<GiftDefinition> {
  const { data } = await http.post<ApiResponse<GiftDefinition>>(
    '/admin/gifts/definitions/create',
    payload,
  )
  return data.data
}

/** 编辑礼物（编码不可变） */
export async function updateGiftDefinition(
  id: string,
  payload: GiftDefinitionInput,
): Promise<GiftDefinition> {
  const { data } = await http.post<ApiResponse<GiftDefinition>>(
    '/admin/gifts/definitions/update',
    { ...payload, id },
  )
  return data.data
}

/** 删除礼物（已有兑换记录时后端会拒绝，只能停用） */
export async function deleteGiftDefinition(id: string): Promise<void> {
  await http.post('/admin/gifts/definitions/delete', { id })
}

/** 后台读取礼物中心配置 */
export async function fetchGiftSettings(): Promise<GiftSettings> {
  const { data } = await http.post<ApiResponse<GiftSettings>>('/admin/gifts/settings/get', {})
  return data.data
}

/** 后台更新签到奖励 */
export async function updateGiftSettings(checkinReward: number): Promise<GiftSettings> {
  const { data } = await http.post<ApiResponse<GiftSettings>>('/admin/gifts/settings/update', {
    checkin_reward: checkinReward,
  })
  return data.data
}

/** 后台签到排行榜 */
export async function fetchAdminCheckinRanking(
  filter: GiftRankingFilter = {},
): Promise<ListResult<GiftCheckinRankingItem>> {
  const { data } = await http.post<ApiResponse<GiftCheckinRankingItem[]>>(
    '/admin/gifts/rankings/checkin',
    filter,
  )
  return toListResult(data)
}

/** 后台礼物排行榜 */
export async function fetchAdminGiftRanking(
  filter: GiftRankingFilter = {},
): Promise<ListResult<GiftRankingItem>> {
  const { data } = await http.post<ApiResponse<GiftRankingItem[]>>(
    '/admin/gifts/rankings/gifts',
    filter,
  )
  return toListResult(data)
}

/** 后台礼物币排行榜 */
export async function fetchAdminCoinRanking(
  filter: GiftRankingFilter = {},
): Promise<ListResult<GiftCoinRankingItem>> {
  const { data } = await http.post<ApiResponse<GiftCoinRankingItem[]>>(
    '/admin/gifts/rankings/coins',
    filter,
  )
  return toListResult(data)
}
