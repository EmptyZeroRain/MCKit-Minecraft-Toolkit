// 后台新增控制项的前端测试：礼物钱包受控增减、单用户等级调整。
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
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

vi.mock('@/api/growth', () => ({
  fetchGrowthLevelConfig: vi.fn(),
  updateGrowthLevelConfig: vi.fn(),
  fetchGrowthLevelTitles: vi.fn(),
  createGrowthLevelTitle: vi.fn(),
  updateGrowthLevelTitle: vi.fn(),
  deleteGrowthLevelTitle: vi.fn(),
  recalculateGrowthLevels: vi.fn(),
  adjustUserLevel: vi.fn(),
  fetchAchievementDefs: vi.fn(),
  fetchAdminUserAchievements: vi.fn(),
  grantUserAchievement: vi.fn(),
  revokeUserAchievement: vi.fn(),
  createAchievementDef: vi.fn(),
  updateAchievementDef: vi.fn(),
  toggleAchievementDef: vi.fn(),
  recalculateAchievements: vi.fn(),
}))

import {
  adjustGiftWallet,
  fetchAdminGiftLedger,
  fetchAdminGiftWallet,
} from '@/api/gift'
import {
  adjustUserLevel,
  createGrowthLevelTitle,
  deleteGrowthLevelTitle,
  fetchAchievementDefs,
  fetchAdminUserAchievements,
  fetchGrowthLevelConfig,
  fetchGrowthLevelTitles,
  updateGrowthLevelConfig,
  updateGrowthLevelTitle,
  grantUserAchievement,
  revokeUserAchievement,
} from '@/api/growth'
import AdminGiftsView from '@/views/admin/AdminGiftsView.vue'
import AdminGrowthAchievementsPanel from '@/views/admin/AdminGrowthAchievementsPanel.vue'
import AdminGrowthLevelsPanel from '@/views/admin/AdminGrowthLevelsPanel.vue'
import type { GiftLedgerItem, GiftWallet } from '@/types/gift'
import type {
  AdminUserAchievement,
  GrowthLevelConfig,
  GrowthLevelTitle,
} from '@/types/growth'

const wallet: GiftWallet = {
  user_id: '00000000-0000-0000-0000-000000000001',
  user_no: 'U7K3M2QX9F',
  username: 'gift-user',
  general_balance: 100,
  checkin_balance: 0,
  total_balance: 100,
  updated_at: '2026-10-20T00:00:00Z',
}

const entry: GiftLedgerItem = {
  id: '00000000-0000-0000-0000-000000000099',
  user_id: wallet.user_id,
  amount: 10,
  coin_type: 'general',
  balance_after: 110,
  event_type: 'admin_adjustment',
  source_type: 'admin_action',
  source_id: 'req-1',
  idempotency_key: 'admin_adjust:req-1',
  reason: '测试发放',
  operator_admin_id: '00000000-0000-0000-0000-0000000000aa',
  request_id: 'request-1',
  created_at: '2026-10-20T00:00:00Z',
}

const levelConfig: GrowthLevelConfig = { max_level: 9999, step_xp: 100 }
const levelTitles: GrowthLevelTitle[] = [
  { id: 'title-0', min_level: 0, max_level: 0, title: '访客', enabled: true },
  { id: 'title-1', min_level: 1, max_level: 10, title: '初来乍到', enabled: true },
  { id: 'title-100', min_level: 100, max_level: 100, title: 'MCkit 传奇', enabled: true },
]

describe('后台礼物钱包', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    ;(fetchAdminGiftLedger as ReturnType<typeof vi.fn>).mockResolvedValue({
      items: [],
      total: 0,
      page: 1,
      pageSize: 20,
      totalPages: 0,
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('查询钱包后可受控增加礼物币，并提交幂等键', async () => {
    ;(fetchAdminGiftWallet as ReturnType<typeof vi.fn>).mockResolvedValue(wallet)
    ;(adjustGiftWallet as ReturnType<typeof vi.fn>).mockResolvedValue(entry)
    vi.spyOn(window, 'confirm').mockReturnValue(true)

    const wrapper = mount(AdminGiftsView, {
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } },
    })
    await flushPromises()

    const target = wrapper.find('input[placeholder="UUID / U7K3M2QX9F"]')
    await target.setValue('U7K3M2QX9F')
    await wrapper.findAll('button').find((b) => b.text() === '查询钱包')!.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('gift-user')
    expect(wrapper.text()).toContain('100')

    await wrapper.find('input[type="number"]').setValue('10')
    await wrapper
      .find('input[placeholder="如 活动奖励 / 误发扣回"]')
      .setValue('测试发放')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(fetchAdminGiftWallet).toHaveBeenCalledWith('U7K3M2QX9F')
    expect(adjustGiftWallet).toHaveBeenCalledTimes(1)
    const payload = (adjustGiftWallet as ReturnType<typeof vi.fn>).mock.calls[0][0]
    expect(payload.target_user_id).toBe('U7K3M2QX9F')
    expect(payload.coin_type).toBe('general')
    expect(payload.amount).toBe(10)
    expect(payload.reason).toBe('测试发放')
    expect(payload.request_key).toBeTruthy()
    expect(wrapper.text()).toContain('调整完成')
  })

  it('未先查询钱包时拒绝提交，避免误调整其他账号', async () => {
    const wrapper = mount(AdminGiftsView, {
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } },
    })
    await flushPromises()

    await wrapper.find('input[placeholder="UUID / U7K3M2QX9F"]').setValue('U7K3M2QX9F')
    await wrapper.find('input[type="number"]').setValue('10')
    await wrapper
      .find('input[placeholder="如 活动奖励 / 误发扣回"]')
      .setValue('测试发放')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(adjustGiftWallet).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('请先查询目标用户钱包')
  })
})

describe('后台单用户等级调整', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    ;(fetchGrowthLevelConfig as ReturnType<typeof vi.fn>).mockResolvedValue(levelConfig)
    ;(fetchGrowthLevelTitles as ReturnType<typeof vi.fn>).mockResolvedValue(levelTitles)
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('提交等级调整并展示人工覆盖结果', async () => {
    ;(adjustUserLevel as ReturnType<typeof vi.fn>).mockResolvedValue({
      user_id: '00000000-0000-0000-0000-000000000001',
      base_level: 0,
      level: 2,
      level_title: '方块学徒',
      is_level_overridden: true,
    })

    const wrapper = mount(AdminGrowthLevelsPanel)
    await flushPromises()
    expect(wrapper.text()).toContain('Lv.0 为访客')

    await wrapper.find('input[placeholder="U7K3M2QX9F"]').setValue('u7k3m2qx9f')
    await wrapper.find('input[placeholder="如 3"]').setValue('2')
    await wrapper.find('input[placeholder="如 活动补偿"]').setValue('测试升级')
    await wrapper.find('form[data-test="level-adjust-form"]').trigger('submit')
    await flushPromises()

    expect(adjustUserLevel).toHaveBeenCalledWith({
      target_user_id: 'U7K3M2QX9F',
      target_level: 2,
      reason: '测试升级',
    })
    expect(wrapper.text()).toContain('调整完成：Lv.2 方块学徒')
    expect(wrapper.text()).toContain('已人工覆盖')
  })

  it('拒绝内部 UUID，只允许对外用户 ID', async () => {
    const wrapper = mount(AdminGrowthLevelsPanel)
    await flushPromises()

    await wrapper
      .find('input[placeholder="U7K3M2QX9F"]')
      .setValue('00000000-0000-0000-0000-000000000001')
    await wrapper.find('input[placeholder="如 3"]').setValue('1')
    await wrapper.find('input[placeholder="如 活动补偿"]').setValue('测试升级')
    await wrapper.find('form[data-test="level-adjust-form"]').trigger('submit')
    await flushPromises()

    expect(adjustUserLevel).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('不支持内部 UUID')
  })

  it('保存等级曲线并重算用户等级', async () => {
    ;(updateGrowthLevelConfig as ReturnType<typeof vi.fn>).mockResolvedValue(4)

    const wrapper = mount(AdminGrowthLevelsPanel)
    await flushPromises()

    const form = wrapper.find('form[data-test="level-config-form"]')
    const numbers = form.findAll('input[type="number"]')
    await numbers[0].setValue('5000')
    await numbers[1].setValue('200')
    await form.trigger('submit')
    await flushPromises()

    expect(updateGrowthLevelConfig).toHaveBeenCalledWith({ max_level: 5000, step_xp: 200 })
    expect(wrapper.text()).toContain('重算修正 4 个用户等级')
  })

  it('新增区间称号、指定等级称号并保存 / 删除规则', async () => {
    ;(createGrowthLevelTitle as ReturnType<typeof vi.fn>).mockResolvedValue({
      id: 'title-20',
      min_level: 20,
      max_level: 30,
      title: '探索者',
      enabled: true,
    })
    ;(updateGrowthLevelTitle as ReturnType<typeof vi.fn>).mockResolvedValue({
      id: 'title-1',
      min_level: 1,
      max_level: 10,
      title: '初来乍到·改',
      enabled: true,
    })
    ;(deleteGrowthLevelTitle as ReturnType<typeof vi.fn>).mockResolvedValue(undefined)
    vi.spyOn(window, 'confirm').mockReturnValue(true)

    const wrapper = mount(AdminGrowthLevelsPanel)
    await flushPromises()

    const rangeForm = wrapper.find('form[data-test="range-title-form"]')
    const rangeNumbers = rangeForm.findAll('input[type="number"]')
    await rangeNumbers[0].setValue('20')
    await rangeNumbers[1].setValue('30')
    await rangeForm.find('input[type="text"]').setValue('探索者')
    await rangeForm.trigger('submit')
    await flushPromises()

    expect(createGrowthLevelTitle).toHaveBeenCalledWith({
      min_level: 20,
      max_level: 30,
      title: '探索者',
      enabled: true,
    })

    const exactForm = wrapper.find('form[data-test="exact-title-form"]')
    await exactForm.find('input[type="number"]').setValue('100')
    await exactForm.find('input[type="text"]').setValue('百级传奇')
    await exactForm.trigger('submit')
    await flushPromises()

    expect(createGrowthLevelTitle).toHaveBeenCalledWith({
      min_level: 100,
      max_level: 100,
      title: '百级传奇',
      enabled: true,
    })

    // 第二行是 Lv.1-10 区间规则，修改称号后保存。
    const rows = wrapper.findAll('tbody tr')
    await rows[1].find('input[type="text"]').setValue('初来乍到·改')
    await rows[1].findAll('button').find((b) => b.text() === '保存')!.trigger('click')
    await flushPromises()
    expect(updateGrowthLevelTitle).toHaveBeenCalledWith('title-1', {
      min_level: 1,
      max_level: 10,
      title: '初来乍到·改',
      enabled: true,
    })

    await rows[1].findAll('button').find((b) => b.text() === '删除')!.trigger('click')
    await flushPromises()
    expect(window.confirm).toHaveBeenCalled()
    expect(deleteGrowthLevelTitle).toHaveBeenCalledWith('title-1')
  })
})

describe('后台单用户成就授予 / 撤销', () => {
  const locked: AdminUserAchievement = {
    achievement_id: '00000000-0000-0000-0000-000000000010',
    code: 'level_3',
    name: '探索者',
    category: '成长',
    rarity: 'common',
    enabled: true,
    reward_xp: 20,
    unlocked: false,
    revoked: false,
    grant_source: '',
    unlocked_at: null,
    revoked_at: null,
    revoke_reason: '',
  }

  beforeEach(() => {
    vi.clearAllMocks()
    ;(fetchAchievementDefs as ReturnType<typeof vi.fn>).mockResolvedValue([])
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('查询用户成就后可授予并撤销，操作必须填写原因', async () => {
    ;(fetchAdminUserAchievements as ReturnType<typeof vi.fn>)
      .mockResolvedValueOnce([locked])
      .mockResolvedValue([{ ...locked, unlocked: true, unlocked_at: '2026-10-20T00:00:00Z' }])
    ;(grantUserAchievement as ReturnType<typeof vi.fn>).mockResolvedValue(undefined)
    ;(revokeUserAchievement as ReturnType<typeof vi.fn>).mockResolvedValue(undefined)
    vi.spyOn(window, 'confirm').mockReturnValue(true)

    const wrapper = mount(AdminGrowthAchievementsPanel)
    await flushPromises()

    await wrapper.find('input[placeholder="UUID / U7K3M2QX9F"]').setValue('U7K3M2QX9F')
    await wrapper.findAll('button').find((b) => b.text() === '查询用户成就')!.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('探索者')
    expect(fetchAdminUserAchievements).toHaveBeenCalledWith('U7K3M2QX9F')

    // 未填原因时拒绝授予。
    await wrapper.findAll('button').find((b) => b.text() === '授予')!.trigger('click')
    await flushPromises()
    expect(grantUserAchievement).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('请填写授予原因')

    await wrapper
      .find('input[placeholder="如 活动补发 / 违规撤销"]')
      .setValue('测试补发')
    await wrapper.findAll('button').find((b) => b.text() === '授予')!.trigger('click')
    await flushPromises()
    expect(grantUserAchievement).toHaveBeenCalledWith(
      'U7K3M2QX9F',
      locked.achievement_id,
      '测试补发',
    )

    await wrapper.findAll('button').find((b) => b.text() === '撤销')!.trigger('click')
    await flushPromises()
    expect(revokeUserAchievement).toHaveBeenCalledWith(
      'U7K3M2QX9F',
      locked.achievement_id,
      '测试补发',
    )
  })
})
