// 后台礼物页：六个 Tab 的接线与按需加载。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

vi.mock('@/api/gift', () => ({
  fetchAdminGiftLedger: vi.fn(),
  fetchAdminGiftWallet: vi.fn(),
  adjustGiftWallet: vi.fn(),
  fetchGiftDefinitions: vi.fn(),
  fetchGiftSettings: vi.fn(),
  createGiftDefinition: vi.fn(),
  updateGiftDefinition: vi.fn(),
  deleteGiftDefinition: vi.fn(),
  updateGiftSettings: vi.fn(),
  fetchAdminCheckinRanking: vi.fn(),
  fetchAdminGiftRanking: vi.fn(),
  fetchAdminCoinRanking: vi.fn(),
}))

import {
  fetchAdminCheckinRanking,
  fetchAdminCoinRanking,
  fetchAdminGiftLedger,
  fetchAdminGiftRanking,
  fetchGiftDefinitions,
  fetchGiftSettings,
} from '@/api/gift'
import AdminGiftsView from '@/views/admin/AdminGiftsView.vue'
import type { GiftDefinition } from '@/types/gift'

const emptyPage = { items: [], total: 0, page: 1, pageSize: 20, totalPages: 0 }

const dirt: GiftDefinition = {
  id: '00000000-0000-0000-0000-000000000010',
  code: 'dirt_block',
  kind: 'gift',
  name: '泥土块',
  description: '最基础的礼物',
  icon: '🟫',
  price: 1,
  stock: 10,
  allow_checkin: true,
  sort_order: 10,
  enabled: true,
  redeemed_count: 3,
  remaining: 7,
  created_at: '2026-10-20T00:00:00Z',
  updated_at: '2026-10-20T00:00:00Z',
}

describe('后台礼物页 Tab', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    window.history.replaceState({}, '', '/')
    ;(fetchAdminGiftLedger as ReturnType<typeof vi.fn>).mockResolvedValue(emptyPage)
    ;(fetchGiftDefinitions as ReturnType<typeof vi.fn>).mockResolvedValue([dirt])
    ;(fetchGiftSettings as ReturnType<typeof vi.fn>).mockResolvedValue({
      checkin_reward: 1,
      updated_at: '2026-10-20T00:00:00Z',
    })
    ;(fetchAdminCheckinRanking as ReturnType<typeof vi.fn>).mockResolvedValue({
      ...emptyPage,
      items: [
        {
          rank: 1,
          user_id: '00000000-0000-0000-0000-000000000001',
          user_no: 'U7K3M2QX9F',
          username: 'alice',
          nickname: '爱丽丝',
          avatar_url: '',
          checkin_days: 9,
          checkin_reward: 9,
          last_checkin_date: '2026-10-20',
        },
      ],
      total: 1,
      totalPages: 1,
    })
    ;(fetchAdminGiftRanking as ReturnType<typeof vi.fn>).mockResolvedValue({
      ...emptyPage,
      items: [
        {
          rank: 1,
          gift_id: dirt.id,
          code: dirt.code,
          name: dirt.name,
          icon: dirt.icon,
          enabled: true,
          redemption_count: 3,
          total_coins: 3,
          checkin_coins: 3,
          general_coins: 0,
        },
      ],
      total: 1,
      totalPages: 1,
    })
    ;(fetchAdminCoinRanking as ReturnType<typeof vi.fn>).mockResolvedValue({
      ...emptyPage,
      items: [
        {
          rank: 1,
          user_id: '00000000-0000-0000-0000-000000000001',
          user_no: 'U7K3M2QX9F',
          username: 'alice',
          nickname: '爱丽丝',
          avatar_url: '',
          general_balance: 10,
          checkin_balance: 5,
          total_balance: 15,
        },
      ],
      total: 1,
      totalPages: 1,
    })
  })

  it('默认展示钱包，切换后按需加载目录、设置与排行榜', async () => {
    const wrapper = mount(AdminGiftsView)
    await flushPromises()

    expect(wrapper.text()).toContain('用户钱包')
    expect(fetchGiftDefinitions).not.toHaveBeenCalled()

    await wrapper.findAll('button').find((b) => b.text() === '礼品目录')!.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('泥土块')
    expect(fetchGiftDefinitions).toHaveBeenCalledTimes(1)

    await wrapper.findAll('button').find((b) => b.text() === '签到设置')!.trigger('click')
    await flushPromises()
    expect(fetchGiftSettings).toHaveBeenCalledTimes(1)

    await wrapper.findAll('button').find((b) => b.text() === '签到排行榜')!.trigger('click')
    await flushPromises()
    expect(fetchAdminCheckinRanking).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('爱丽丝')
    expect(wrapper.text()).toContain('9 天')

    await wrapper.findAll('button').find((b) => b.text() === '礼物排行榜')!.trigger('click')
    await flushPromises()
    expect(fetchAdminGiftRanking).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('兑换次数')

    await wrapper.findAll('button').find((b) => b.text() === '礼物币排行榜')!.trigger('click')
    await flushPromises()
    expect(fetchAdminCoinRanking).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('总余额')
  })

  it('支持通过 ?tab=catalog 直达礼品目录', async () => {
    window.history.replaceState({}, '', '/admin/gifts?tab=catalog')

    const wrapper = mount(AdminGiftsView)
    await flushPromises()

    expect(fetchGiftDefinitions).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('泥土块')
  })
})
