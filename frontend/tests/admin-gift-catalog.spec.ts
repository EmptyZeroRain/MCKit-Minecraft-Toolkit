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
  allow_checkin: true,
  sort_order: 10,
  enabled: true,
  redeemed_count: 3,
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
    expect(wrapper.text()).toContain('dirt_block')
    expect(wrapper.text()).toContain('可兑换')
  })

  it('新建礼物时提交编码、价格与签到币开关', async () => {
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
    await form.find('input[placeholder="如 dirt_block"]').setValue('test_gift')
    await form.find('input[placeholder="如 泥土块"]').setValue('测试礼物')
    await form.find('input[type="number"]').setValue('8')
    await form.trigger('submit')
    await flushPromises()

    expect(createGiftDefinition).toHaveBeenCalledTimes(1)
    const payload = (createGiftDefinition as ReturnType<typeof vi.fn>).mock.calls[0][0]
    expect(payload.code).toBe('test_gift')
    expect(payload.name).toBe('测试礼物')
    expect(payload.price).toBe(8)
    expect(payload.allow_checkin).toBe(false)
    expect(wrapper.text()).toContain('礼物已创建')
  })
})

describe('后台签到设置', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    ;(fetchGiftSettings as ReturnType<typeof vi.fn>).mockResolvedValue({
      checkin_reward: 1,
      updated_at: '2026-10-20T00:00:00Z',
    })
  })

  it('展示并更新每日签到奖励', async () => {
    ;(updateGiftSettings as ReturnType<typeof vi.fn>).mockResolvedValue({
      checkin_reward: 5,
      updated_at: '2026-10-20T00:00:00Z',
    })

    const wrapper = mount(AdminGiftCheckinPanel)
    await flushPromises()

    const form = wrapper.find('form')
    await form.find('input[type="number"]').setValue('5')
    await form.trigger('submit')
    await flushPromises()

    expect(updateGiftSettings).toHaveBeenCalledWith(5)
    expect(wrapper.text()).toContain('签到奖励已更新为 5 签到礼物币/天')
  })
})
