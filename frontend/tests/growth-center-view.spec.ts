// 成长中心页测试（jingyan.md §11.1 / §11.2）。
// 覆盖：等级 / 经验 / 进度、今日任务进度与状态、最近流水、最近成就，
// 以及「不提供领取按钮」的约束与服务端错误态。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'

vi.mock('@/api/growth', () => ({
  fetchGrowthOverview: vi.fn(),
  fetchGrowthDailyTasks: vi.fn(),
  fetchGrowthLedger: vi.fn(),
  fetchGrowthAchievements: vi.fn(),
}))

import {
  fetchGrowthAchievements,
  fetchGrowthDailyTasks,
  fetchGrowthLedger,
  fetchGrowthOverview,
} from '@/api/growth'
import GrowthCenterView from '@/views/GrowthCenterView.vue'
import { useUserAuthStore } from '@/stores/user-auth'
import { ApiError } from '@/types/api'
import type { GrowthAchievement, GrowthOverview } from '@/types/growth'

const overview: GrowthOverview = {
  current_xp: 450,
  lifetime_earned_xp: 470,
  lifetime_reversed_xp: 20,
  level: 3,
  level_title: '探索者',
  next_level_xp: 600,
  xp_to_next_level: 150,
  level_progress_percent: 50,
  is_max_level: false,
  is_level_overridden: false,
  active_days: 7,
  guest_active_days: 7,
  unlocked_achievements: 2,
  achievement_total: 4,
  today_tasks_completed: 1,
  today_tasks_total: 2,
  today_xp: 8,
}

const achievements: GrowthAchievement[] = [
  {
    code: 'account_registered',
    name: '初次见面',
    description: '完成注册',
    category: '探索',
    rarity: 'common',
    icon_url: '',
    is_hidden: false,
    unlocked: true,
    unlocked_at: '2026-10-18T02:00:00Z',
    progress: 1,
    target: 1,
    reward_xp: 0,
  },
  {
    code: 'level_3',
    name: '稳步成长',
    description: '达到 Lv.3',
    category: '成长',
    rarity: 'common',
    icon_url: '',
    is_hidden: false,
    unlocked: true,
    unlocked_at: '2026-10-19T02:00:00Z',
    progress: 3,
    target: 3,
    reward_xp: 0,
  },
]

async function mountView() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const userAuth = useUserAuthStore()
  userAuth.user = { id: 'u1', username: 'steve', nickname: '史蒂夫' } as never
  userAuth.initialized = true

  const wrapper = mount(GrowthCenterView, {
    global: {
      plugins: [pinia],
      stubs: { RouterLink: { template: '<a><slot /></a>' } },
    },
  })
  await flushPromises()
  return wrapper
}

function mockSuccess() {
  ;(fetchGrowthOverview as ReturnType<typeof vi.fn>).mockResolvedValue(overview)
  ;(fetchGrowthDailyTasks as ReturnType<typeof vi.fn>).mockResolvedValue({
    date: '2026-10-19',
    timezone: 'Asia/Shanghai',
    tasks: [
      { code: 'daily_login', name: '每日登录', description: '今天成功登录一次', progress: 1, target: 1, reward_xp: 5, status: 'claimed', completed_at: '2026-10-19T01:00:00Z' },
      { code: 'daily_tool_1', name: '工具试用', description: '有效使用 1 次工具', progress: 0, target: 1, reward_xp: 3, status: 'in_progress', completed_at: '' },
    ],
  })
  ;(fetchGrowthLedger as ReturnType<typeof vi.fn>).mockResolvedValue({
    items: [
      { id: 'l1', user_id: 'u1', delta_xp: 5, balance_after: 450, source_type: 'daily_task', source_id: 'daily_login', rule_code: 'daily_login', reason: '完成每日任务：每日登录', created_at: '2026-10-19T01:00:00Z' },
      { id: 'l2', user_id: 'u1', delta_xp: -2, balance_after: 445, source_type: 'reversal', source_id: 'l0', rule_code: '', reason: '冲正：误发', created_at: '2026-10-19T00:30:00Z' },
    ],
    total: 2,
    page: 1,
    pageSize: 10,
    totalPages: 1,
  })
  ;(fetchGrowthAchievements as ReturnType<typeof vi.fn>).mockResolvedValue(achievements)
}

describe('成长中心页', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('不展示「均由服务端判定」的说明文案', async () => {
    mockSuccess()
    const wrapper = await mountView()
    expect(wrapper.text()).not.toContain('经验、等级、任务与成就均由服务端判定')
    expect(wrapper.text()).toContain('成长中心')
  })

  it('访客阶段展示活跃天数，不展示经验升级目标', async () => {
    mockSuccess()
    ;(fetchGrowthOverview as ReturnType<typeof vi.fn>).mockResolvedValue({
      ...overview,
      level: 0,
      level_title: '访客',
      next_level_xp: null,
      xp_to_next_level: 0,
      level_progress_percent: 42,
      active_days: 3,
    })
    const wrapper = await mountView()
    const text = wrapper.text()

    expect(text).toContain('Lv.0 · 访客')
    expect(text).toContain('访客阶段：活跃 3/7 天')
    expect(text).not.toContain('距下一级还需')
    expect(text).toContain('42%')
  })

  it('人工覆盖等级时展示覆盖说明，不展示自动升级目标', async () => {
    mockSuccess()
    ;(fetchGrowthOverview as ReturnType<typeof vi.fn>).mockResolvedValue({
      ...overview,
      level: 2,
      level_title: '方块学徒',
      next_level_xp: null,
      xp_to_next_level: 0,
      level_progress_percent: 100,
      is_level_overridden: true,
    })
    const wrapper = await mountView()
    const text = wrapper.text()

    expect(text).toContain('人工覆盖')
    expect(text).toContain('当前等级由管理员调整，经验继续累积但不自动升级')
    expect(text).not.toContain('距下一级还需')
  })

  it('展示等级、经验、进度与今日数据', async () => {
    mockSuccess()
    const wrapper = await mountView()
    const text = wrapper.text()

    expect(text).toContain('史蒂夫')
    expect(text).toContain('Lv.3 · 探索者')
    expect(text).toContain('当前有效经验')
    expect(text).toContain('450')
    expect(text).toContain('距下一级还需 150 经验')
    expect(text).toContain('50%')
    expect(text).toContain('今日获得')
    expect(text).toContain('8')
    expect(fetchGrowthOverview).toHaveBeenCalledTimes(1)
  })

  it('展示今日任务进度与状态，且不提供领取按钮', async () => {
    mockSuccess()
    const wrapper = await mountView()
    const text = wrapper.text()

    expect(text).toContain('每日登录')
    expect(text).toContain('工具试用')
    expect(text).toContain('进度 1 / 1')
    expect(text).toContain('奖励 +5 经验')
    expect(text).toContain('已完成')
    expect(text).toContain('进行中')
    // 首期自动发奖，不应出现「领取」按钮
    expect(text).not.toContain('领取')
    expect(wrapper.findAll('button').some((b) => b.text().includes('领取'))).toBe(false)
  })

  it('展示最近经验流水（含冲正负值）与成就完成比例', async () => {
    mockSuccess()
    const wrapper = await mountView()
    const text = wrapper.text()

    expect(text).toContain('完成每日任务：每日登录')
    expect(text).toContain('+5')
    expect(text).toContain('-2')
    expect(text).toContain('最近解锁成就')
    expect(text).toContain('完成比例 50%（2/4）')
    expect(text).toContain('稳步成长')
  })

  it('接口失败时展示错误提示而非崩溃', async () => {
    ;(fetchGrowthOverview as ReturnType<typeof vi.fn>).mockRejectedValue(new ApiError('成长数据不可用', 'INTERNAL_ERROR'))
    ;(fetchGrowthDailyTasks as ReturnType<typeof vi.fn>).mockResolvedValue({ date: '2026-10-19', timezone: 'Asia/Shanghai', tasks: [] })
    ;(fetchGrowthLedger as ReturnType<typeof vi.fn>).mockResolvedValue({ items: [], total: 0, page: 1, pageSize: 10, totalPages: 0 })
    ;(fetchGrowthAchievements as ReturnType<typeof vi.fn>).mockResolvedValue([])

    const wrapper = await mountView()
    expect(wrapper.text()).toContain('成长数据不可用')
  })
})
