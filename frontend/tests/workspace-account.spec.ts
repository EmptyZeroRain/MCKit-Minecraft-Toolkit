// 工作台顶部账号卡：集中展示身份、对外用户 ID 与常用入口。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'

vi.mock('@/api/gift', () => ({
  fetchGiftOverview: vi.fn().mockResolvedValue(null),
}))
vi.mock('@/api/growth', () => ({
  fetchGrowthOverview: vi.fn().mockResolvedValue(null),
}))

import WorkspaceView from '@/views/WorkspaceView.vue'
import { useUserAuthStore } from '@/stores/user-auth'
import { useUserWorkspaceStore } from '@/stores/user-workspace'
import { fetchGiftOverview } from '@/api/gift'
import { fetchGrowthOverview } from '@/api/growth'
import type { GiftOverview } from '@/types/gift'
import type { GrowthOverview } from '@/types/growth'

async function mountView(role: string) {
  const pinia = createPinia()
  setActivePinia(pinia)
  const userAuth = useUserAuthStore()
  userAuth.user = {
    id: 'u1',
    user_no: 'UE389F2938F',
    username: 'admin',
    nickname: '运营管理员',
    role,
  }
  userAuth.initialized = true

  const workspace = useUserWorkspaceStore()
  workspace.loadAll = vi.fn().mockResolvedValue(undefined)

  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/', component: { template: '<div />' } }],
  })

  const wrapper = mount(WorkspaceView, {
    global: {
      plugins: [pinia, router],
      stubs: { RouterLink: { template: '<a><slot /></a>' } },
    },
  })
  await flushPromises()
  return wrapper
}

describe('工作台顶部账号卡', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('管理员账号集中展示角色、用户 ID 与后台入口', async () => {
    const wrapper = await mountView('admin')
    const text = wrapper.text()

    expect(text).toContain('运营管理员')
    expect(text).toContain('管理员')
    expect(text).toContain('UE389F2938F')
    expect(text).toContain('管理后台')
  })

  it('普通用户不展示后台入口', async () => {
    const wrapper = await mountView('user')
    const text = wrapper.text()

    expect(text).toContain('普通用户')
    expect(text).not.toContain('管理后台')
  })

  it('展示礼物币余额、今日签到与等级摘要', async () => {
    vi.mocked(fetchGiftOverview).mockResolvedValueOnce({
      user_id: 'u1',
      user_no: 'UE389F2938F',
      username: 'alice',
      general_balance: 20,
      checkin_balance: 5,
      total_balance: 25,
      updated_at: null,
      recent: [],
      checkin_reward: 2,
      checked_in_today: true,
      checkin_date: '2026-10-20',
    } satisfies GiftOverview)
    vi.mocked(fetchGrowthOverview).mockResolvedValueOnce({
      current_xp: 320,
      lifetime_earned_xp: 360,
      lifetime_reversed_xp: 40,
      level: 3,
      level_title: '探索者',
      next_level_xp: 500,
      xp_to_next_level: 180,
      level_progress_percent: 64,
      is_max_level: false,
      is_level_overridden: false,
      active_days: 7,
      guest_active_days: 7,
      unlocked_achievements: 2,
      achievement_total: 8,
      today_tasks_completed: 1,
      today_tasks_total: 2,
      today_xp: 12,
    } satisfies GrowthOverview)

    const wrapper = await mountView('user')
    const text = wrapper.text()

    expect(text).toContain('礼物币与签到')
    expect(text).toContain('25')
    expect(text).toContain('已签到')
    expect(text).toContain('Lv.3')
  })

  it('工作台保留分区入口，账号表单已迁移到账户设置', async () => {
    const wrapper = await mountView('user')
    const text = wrapper.text()

    expect(text).toContain('常用功能')
    expect(text).toContain('我的收藏')
    expect(text).toContain('最近使用')
    expect(text).toContain('常用种子')
    expect(text).toContain('账户设置')
    expect(text).not.toContain('修改密码')
    expect(text).not.toContain('删除账号')
  })
})
