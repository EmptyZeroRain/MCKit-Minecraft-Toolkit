// 用户成长后台页测试（jingyan.md §10）。
// 覆盖：Tab 渲染、概览指标、切 Tab 渲染规则表、无权限空态。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory } from 'vue-router'

vi.mock('@/api/growth', () => ({
  fetchAdminGrowthOverview: vi.fn(),
  fetchGrowthRules: vi.fn(),
  fetchGrowthLevelConfig: vi.fn(),
  fetchGrowthLevelTitles: vi.fn(),
  updateGrowthLevelConfig: vi.fn(),
  createGrowthLevelTitle: vi.fn(),
  updateGrowthLevelTitle: vi.fn(),
  deleteGrowthLevelTitle: vi.fn(),
  fetchDailyTaskDefs: vi.fn(),
  fetchAchievementDefs: vi.fn(),
  fetchAdminGrowthLedger: vi.fn(),
  fetchGrowthAudit: vi.fn(),
}))

import {
  fetchAdminGrowthOverview,
  fetchGrowthRules,
} from '@/api/growth'
import AdminGrowthView from '@/views/admin/AdminGrowthView.vue'
import { useAuthStore } from '@/stores/auth'
import type { AdminGrowthOverview, GrowthRule } from '@/types/growth'

const overview: AdminGrowthOverview = {
  timezone: 'Asia/Shanghai',
  date: '2026-10-19',
  active_users: 12,
  xp_issued_today: 340,
  xp_issued_7days: 2000,
  ledger_today: 40,
  reversed_total: 15,
  manual_adjust_count: 3,
  unlocked_total: 25,
  achievement_count: 20,
  tasks_completed_today: 18,
  task_participants_today: 9,
  level_distribution: [
    { level: 1, count: 5 },
    { level: 3, count: 4 },
  ],
  total_rules: 11,
  total_tasks: 5,
}

const rules: GrowthRule[] = [
  {
    id: 'r1',
    code: 'resource_approved',
    version: 2,
    name: '投稿审核通过',
    event_type: 'resource.approved',
    xp_amount: 50,
    daily_limit: null,
    per_subject_limit: 1,
    enabled: true,
    effective_from: null,
    effective_until: null,
    created_by: null,
    updated_by: null,
    created_at: '2026-10-01T00:00:00Z',
    updated_at: '2026-10-10T00:00:00Z',
  },
]

async function mountView(permissions: string[]) {
  const pinia = createPinia()
  setActivePinia(pinia)
  const auth = useAuthStore()
  auth.user = {
    id: 'a1',
    username: 'admin',
    role: 'admin',
    permissions,
  } as never
  auth.initialized = true

  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/admin/growth', component: { template: '<div />' } }],
  })
  await router.push('/admin/growth')
  await router.isReady()

  const wrapper = mount(AdminGrowthView, { global: { plugins: [pinia, router] } })
  await flushPromises()
  return wrapper
}

describe('用户成长后台页', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    ;(fetchAdminGrowthOverview as ReturnType<typeof vi.fn>).mockResolvedValue(overview)
    ;(fetchGrowthRules as ReturnType<typeof vi.fn>).mockResolvedValue(rules)
  })

  it('具备 ops.growth 时渲染全部 Tab 与概览指标', async () => {
    const wrapper = await mountView(['ops.growth'])
    const text = wrapper.text()

    expect(text).toContain('用户成长')
    expect(text).toContain('成长概览')
    expect(text).toContain('经验规则')
    expect(text).toContain('每日任务')
    expect(text).toContain('审计日志')
    expect(fetchAdminGrowthOverview).toHaveBeenCalledTimes(1)

    // 概览指标与口径
    expect(text).toContain('活跃成长用户数')
    expect(text).toContain('340')
    expect(text).toContain('近 7 日发放经验')
    expect(text).toContain('等级分布')
  })

  it('切换「经验规则」Tab 渲染规则表', async () => {
    const wrapper = await mountView(['ops.growth'])
    const tab = wrapper.findAll('button[role="tab"]').find((b) => b.text() === '经验规则')
    expect(tab).toBeTruthy()
    await tab!.trigger('click')
    await flushPromises()

    expect(fetchGrowthRules).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('resource_approved')
    expect(wrapper.text()).toContain('投稿审核通过')
    expect(wrapper.text()).toContain('v2')
  })

  it('缺少 ops.growth 时展示无权限空态且不请求数据', async () => {
    const wrapper = await mountView([])
    expect(wrapper.text()).toContain('无权限访问')
    expect(fetchAdminGrowthOverview).not.toHaveBeenCalled()
  })
})
