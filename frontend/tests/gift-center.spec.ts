// 礼物中心：签到、双币余额展示、兑换与流水。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

vi.mock('@/api/gift', () => ({
  fetchGiftOverview: vi.fn(),
  fetchGiftCatalog: vi.fn(),
  fetchGiftLedger: vi.fn(),
  fetchGiftRedemptions: vi.fn(),
  checkinGift: vi.fn(),
  purchaseMakeupCard: vi.fn(),
  useMakeupCard: vi.fn(),
  redeemGift: vi.fn(),
}))

vi.mock('@/stores/user-auth', () => ({
  useUserAuthStore: () => ({
    initialized: true,
    restoreSession: vi.fn(),
  }),
}))

import {
  checkinGift,
  fetchGiftCatalog,
  fetchGiftLedger,
  fetchGiftOverview,
  fetchGiftRedemptions,
  redeemGift,
} from '@/api/gift'
import GiftCenterView from '@/views/GiftCenterView.vue'
import type {
  GiftCatalogItem,
  GiftCheckinResult,
  GiftOverview,
  GiftRedeemResult,
} from '@/types/gift'

const overview: GiftOverview = {
  user_id: '00000000-0000-0000-0000-000000000001',
  user_no: 'U7K3M2QX9F',
  username: 'gift-user',
  general_balance: 10,
  checkin_balance: 5,
  total_balance: 15,
  updated_at: '2026-10-20T00:00:00Z',
  recent: [],
  checkin_reward: 5,
  checked_in_today: false,
  checkin_date: '2026-10-20',
  checkin_streak: 2,
  checkin_longest_streak: 4,
  next_checkin_streak: 3,
  checkin_today_xp: 14,
  checkin_base_xp: 10,
  checkin_streak_xp: 4,
  checkin_milestone_xp: 0,
  checkin_penalty_xp: 0,
  makeup_cards: 2,
  makeup_card_price: 10,
  latest_missed_date: '2026-10-19',
}

const dirt: GiftCatalogItem = {
  id: '00000000-0000-0000-0000-000000000010',
  code: 'dirt_block',
  kind: 'gift',
  name: '泥土块',
  description: '最基础的礼物',
  icon: '🟫',
  price: 1,
  stock: 0,
  allow_checkin: true,
  sort_order: 10,
  enabled: true,
  redeemed_count: 0,
  remaining: null,
  created_at: '2026-10-20T00:00:00Z',
  updated_at: '2026-10-20T00:00:00Z',
  can_afford: true,
}

const diamond: GiftCatalogItem = {
  ...dirt,
  id: '00000000-0000-0000-0000-000000000011',
  code: 'diamond',
  name: '钻石',
  icon: '💎',
  price: 50,
  allow_checkin: false,
  can_afford: false,
}

const makeupCard: GiftCatalogItem = {
  ...dirt,
  id: '00000000-0000-0000-0000-000000000012',
  code: 'makeup_card',
  kind: 'makeup_card',
  name: '补签卡',
  icon: '🩹',
  price: 25,
  stock: 0,
  allow_checkin: true,
  redeemed_count: 0,
  remaining: null,
  can_afford: false,
}

const checkinResult: GiftCheckinResult = {
  checkin_date: '2026-10-20',
  reward: 5,
  checked_in: true,
  checkin_streak: 3,
  xp: 14,
  base_xp: 10,
  streak_xp: 4,
  milestone_xp: 0,
  penalty_xp: 0,
  wallet: {
    ...overview,
    checkin_balance: 10,
    total_balance: 20,
  },
}

const redeemResult: GiftRedeemResult = {
  redemption: {
    id: '00000000-0000-0000-0000-000000000020',
    gift_id: dirt.id,
    gift_code: dirt.code,
    gift_name: dirt.name,
    gift_icon: dirt.icon,
    price: 1,
    checkin_spent: 1,
    general_spent: 0,
    created_at: '2026-10-20T00:00:00Z',
  },
  wallet: {
    ...overview,
    checkin_balance: 4,
    total_balance: 14,
  },
}

function mountView() {
  return mount(GiftCenterView, {
    global: {
      stubs: {
        RouterLink: { template: '<a><slot /></a>' },
      },
    },
  })
}

describe('礼物中心', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    ;(fetchGiftOverview as ReturnType<typeof vi.fn>).mockResolvedValue(overview)
    ;(fetchGiftCatalog as ReturnType<typeof vi.fn>).mockResolvedValue([dirt, diamond])
    ;(fetchGiftLedger as ReturnType<typeof vi.fn>).mockResolvedValue({
      items: [],
      total: 0,
      page: 1,
      pageSize: 10,
      totalPages: 0,
    })
    ;(fetchGiftRedemptions as ReturnType<typeof vi.fn>).mockResolvedValue({
      items: [],
      total: 0,
      page: 1,
      pageSize: 10,
      totalPages: 0,
    })
  })

  it('展示双币余额、签到奖励与可兑换礼物', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('15')
    expect(wrapper.text()).toContain('签到礼物币')
    expect(wrapper.text()).toContain('通用礼物币')
    expect(wrapper.text()).toContain('泥土块')
    expect(wrapper.text()).toContain('钻石')
    expect(wrapper.text()).toContain('余额不足')
    expect(wrapper.findAll('button:disabled').some((b) => b.text() === '余额不足')).toBe(true)
    expect(wrapper.text()).toContain('补签卡')
    expect(wrapper.text()).toContain('最近漏签日：2026-10-19')
  })

  it('补签卡只在专用区块展示，价格取礼品目录', async () => {
    ;(fetchGiftCatalog as ReturnType<typeof vi.fn>).mockResolvedValue([dirt, makeupCard])

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('单价 25 签到币')
    // 礼物网格不重复渲染补签卡，只有普通礼物一个卡片。
    expect(wrapper.findAll('article')).toHaveLength(1)
  })

  it('签到成功后更新签到币余额并刷新目录', async () => {
    ;(checkinGift as ReturnType<typeof vi.fn>).mockResolvedValue(checkinResult)
    ;(fetchGiftOverview as ReturnType<typeof vi.fn>)
      .mockResolvedValueOnce(overview)
      .mockResolvedValueOnce({
        ...overview,
        checked_in_today: true,
        checkin_balance: 10,
        total_balance: 20,
        checkin_streak: 3,
        next_checkin_streak: 3,
        checkin_today_xp: 14,
        latest_missed_date: '2026-10-19',
      })

    const wrapper = mountView()
    await flushPromises()

    const button = wrapper.findAll('button').find((b) => b.text() === '立即签到')!
    await button.trigger('click')
    await flushPromises()

    expect(checkinGift).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('签到成功，获得 5 签到礼物币')
    expect(wrapper.text()).toContain('今日已签到')
    expect(wrapper.text()).toContain('10')
    expect(fetchGiftCatalog).toHaveBeenCalledTimes(2)
  })

  it('兑换礼物时提交幂等键并按签到币优先扣减', async () => {
    ;(redeemGift as ReturnType<typeof vi.fn>).mockResolvedValue(redeemResult)
    vi.spyOn(window, 'confirm').mockReturnValue(true)

    const wrapper = mountView()
    await flushPromises()

    const card = wrapper.findAll('article').find((c) => c.text().includes('泥土块'))!
    await card.findAll('button').find((b) => b.text() === '兑换')!.trigger('click')
    await flushPromises()

    expect(redeemGift).toHaveBeenCalledTimes(1)
    const [giftId, requestKey] = (redeemGift as ReturnType<typeof vi.fn>).mock.calls[0]
    expect(giftId).toBe(dirt.id)
    expect(requestKey).toBeTruthy()
    expect(wrapper.text()).toContain('成功兑换「泥土块」，消耗 1 签到币')
    expect(fetchGiftRedemptions).toHaveBeenCalled()
  })
})
