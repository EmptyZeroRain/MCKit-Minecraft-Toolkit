// ===== 礼物币钱包 / 礼物中心类型 =====
// 契约来源：backend/internal/http/handler/gift.go 与 internal/service/gift。
// 余额只能通过流水变更；签到币与通用币分账管理。

/** 礼物币币种 */
export type GiftCoinType = 'general' | 'checkin'

/** 钱包展示信息 */
export interface GiftWallet {
  user_id: string
  user_no: string
  username: string
  /** 通用礼物币（非签到来源） */
  general_balance: number
  /** 签到礼物币 */
  checkin_balance: number
  /** 两类余额之和 */
  total_balance: number
  updated_at: string | null
}

/** 本人可见的脱敏流水（不含操作人、幂等键等内部字段） */
export interface GiftPublicLedgerItem {
  id: string
  amount: number
  coin_type: GiftCoinType
  /** 该币种在本次变动后的余额 */
  balance_after: number
  event_type: string
  reason: string
  created_at: string
}

/** 本人钱包概览（含最近 5 条流水与今日签到状态） */
export interface GiftOverview extends GiftWallet {
  recent: GiftPublicLedgerItem[]
  checkin_reward: number
  checked_in_today: boolean
  checkin_date: string
}

/** 礼物币流水（正数为增加，负数为扣减） */
export interface GiftLedgerItem {
  id: string
  user_id: string
  amount: number
  coin_type: GiftCoinType
  balance_after: number
  event_type: string
  source_type: string
  source_id: string
  idempotency_key: string
  reason: string
  operator_admin_id: string | null
  request_id: string
  created_at: string
}

/** 后台礼物币增减入参 */
export interface GiftAdjustInput {
  target_user_id: string
  coin_type: GiftCoinType
  amount: number
  reason: string
  request_key?: string
}

/** 礼物币流水筛选 */
export interface GiftLedgerFilter {
  user_id?: string
  coin_type?: GiftCoinType
  event_type?: string
  page?: number
  page_size?: number
}

/** 礼物目录条目 */
export interface GiftDefinition {
  id: string
  code: string
  name: string
  description: string
  icon: string
  price: number
  allow_checkin: boolean
  sort_order: number
  enabled: boolean
  /** 后台列表返回：累计兑换次数 */
  redeemed_count: number
  created_at: string
  updated_at: string
}

/** 用户侧礼物目录条目 */
export interface GiftCatalogItem extends GiftDefinition {
  can_afford: boolean
}

/** 兑换记录 */
export interface GiftRedemption {
  id: string
  gift_id: string
  gift_code: string
  gift_name: string
  gift_icon: string
  price: number
  checkin_spent: number
  general_spent: number
  created_at: string
}

/** 签到结果 */
export interface GiftCheckinResult {
  checkin_date: string
  reward: number
  checked_in: boolean
  wallet: GiftWallet
}

/** 兑换结果 */
export interface GiftRedeemResult {
  redemption: GiftRedemption
  wallet: GiftWallet
}

/** 礼物中心配置 */
export interface GiftSettings {
  checkin_reward: number
  updated_at: string
}

/** 新建 / 编辑礼物入参 */
export interface GiftDefinitionInput {
  code?: string
  name: string
  description: string
  icon: string
  price: number
  allow_checkin: boolean
  sort_order: number
  enabled: boolean
}

/** 排行榜查询参数；days=0 表示全部时间 */
export interface GiftRankingFilter {
  days?: number
  coin_type?: GiftCoinType
  page?: number
  page_size?: number
}

/** 签到排行榜条目 */
export interface GiftCheckinRankingItem {
  rank: number
  user_id: string
  user_no: string
  username: string
  nickname: string
  avatar_url: string
  checkin_days: number
  checkin_reward: number
  last_checkin_date: string
}

/** 礼物排行榜条目 */
export interface GiftRankingItem {
  rank: number
  gift_id: string
  code: string
  name: string
  icon: string
  enabled: boolean
  redemption_count: number
  total_coins: number
  checkin_coins: number
  general_coins: number
}

/** 礼物币排行榜条目 */
export interface GiftCoinRankingItem {
  rank: number
  user_id: string
  user_no: string
  username: string
  nickname: string
  avatar_url: string
  general_balance: number
  checkin_balance: number
  total_balance: number
}
