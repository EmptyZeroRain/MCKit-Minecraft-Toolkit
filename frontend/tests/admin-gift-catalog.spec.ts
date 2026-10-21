// 后台礼物：礼品目录与签到设置面板。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

vi.mock('@/api/gift', () => ({
  fetchGiftDefinitions: vi.fn(),
  fetchGiftSettings: vi.fn(),
  createGiftDefinition: vi.fn(),
  updateGiftDefinition: vi.fn(),
  deleteGiftDefinition: vi.fn(),
  updateGiftSettings: vi.fn(),
}))

import {
  createGiftDefinition,
  fetchGiftDefinitions,
  fetchGiftSettings,
  updateGiftSettings,
} from '@/api/gift'
import AdminGiftCatalogPanel from '@/views/admin/gift/AdminGiftCatalogPanel.vue'
import AdminGiftCheckinPanel from '@/views/admin/gift/AdminGiftCheckinPanel.vue'
import type { GiftDefinition } from '@/types/gift'

const dirt: GiftDefinition = {
  id: '00000000-0000-0000-0000-000000000010',
  code: 'dirt_block',
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

describe('后台礼品目录', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    ;(fetchGiftDefinitions as ReturnType<typeof vi.fn>).mockResolvedValue([dirt])
  })

  it('展示礼品目录与签到币可兑状态', async () => {
    const wrapper = mount(AdminGiftCatalogPanel)
    await flushPromises()

    expect(wrapper.text()).toContain('泥土块')
    expect(wrapper.text()).not.toContain('dirt_block')
    expect(wrapper.text()).not.toContain('编码')
    expect(wrapper.text()).toContain('可兑换')
    expect(wrapper.text()).toContain('库存')
    expect(wrapper.text()).toContain('已兑换')
    expect(wrapper.text()).toContain('剩余')
    expect(wrapper.text()).toContain('7')
  })

  it('新建礼物时提交价格、库存与签到币开关', async () => {
    ;(createGiftDefinition as ReturnType<typeof vi.fn>).mockResolvedValue({
      ...dirt,
      id: '00000000-0000-0000-0000-000000000011',
      code: 'test_gift',
      name: '测试礼物',
      price: 8,
    })

    const wrapper = mount(AdminGiftCatalogPanel)
    await flushPromises()

    await wrapper.findAll('button').find((b) => b.text() === '新建礼物')!.trigger('click')
    const form = wrapper.find('form')
    await form.find('input[placeholder="如 泥土块"]').setValue('测试礼物')
    const numberInputs = form.findAll('input[type="number"]')
    await numberInputs[0].setValue('8')
    await numberInputs[1].setValue('20')
    await form.trigger('submit')
    await flushPromises()

    expect(createGiftDefinition).toHaveBeenCalledTimes(1)
    const payload = (createGiftDefinition as ReturnType<typeof vi.fn>).mock.calls[0][0]
    expect(payload.code).toBeUndefined()
    expect(payload.name).toBe('测试礼物')
    expect(payload.price).toBe(8)
    expect(payload.stock).toBe(20)
    expect(payload.allow_checkin).toBe(false)
    expect(wrapper.text()).toContain('礼物已创建')
  })
})

describe('后台签到设置', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    ;(fetchGiftSettings as ReturnType<typeof vi.fn>).mockResolvedValue({
      checkin_reward: 1,
      base_xp: 10,
      streak_step_xp: 2,
      streak_bonus_cap: 100,
      milestone_bonus_cap: 500,
      break_penalty_xp: 5,
      makeup_card_price: 10,
      milestones: [{ streak_days: 3, bonus_xp: 10, enabled: true }],
      updated_at: '2026-10-20T00:00:00Z',
    })
  })

  it('展示并保存签到奖励、经验算法与里程碑', async () => {
    ;(updateGiftSettings as ReturnType<typeof vi.fn>).mockResolvedValue({
      checkin_reward: 5,
      base_xp: 10,
      streak_step_xp: 2,
      streak_bonus_cap: 100,
      milestone_bonus_cap: 500,
      break_penalty_xp: 5,
      makeup_card_price: 10,
      milestones: [{ streak_days: 3, bonus_xp: 10, enabled: true }],
      updated_at: '2026-10-20T00:00:00Z',
    })

    const wrapper = mount(AdminGiftCheckinPanel)
    await flushPromises()

    const form = wrapper.find('form')
    await form.find('input[type="number"]').setValue('5')
    await form.trigger('submit')
    await flushPromises()

    expect(updateGiftSettings).toHaveBeenCalledWith({
      checkin_reward: 5,
      base_xp: 10,
      streak_step_xp: 2,
      streak_bonus_cap: 100,
      milestone_bonus_cap: 500,
      break_penalty_xp: 5,
      makeup_card_price: 10,
      milestones: [{ streak_days: 3, bonus_xp: 10, enabled: true }],
    })
    expect(wrapper.text()).toContain('签到配置已保存')
  })
})
