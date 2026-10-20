// 用户成长 / 用户详情：搜索用户、聚合等级 / 成就 / 经验 / 礼物 / 审计。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import type { AdminGrowthUserDetail, AdminGrowthUserSummary } from '@/types/growth'

const api = vi.hoisted(() => ({
  fetchAdminGrowthUsers: vi.fn(),
  fetchAdminGrowthUserDetail: vi.fn(),
}))
vi.mock('@/api/growth', () => api)

const AdminGrowthUserDetailPanel = (
  await import('@/views/admin/AdminGrowthUserDetailPanel.vue')
).default

const USER: AdminGrowthUserSummary = {
  user_id: 'u1',
  user_no: 'U23456789AB',
  username: 'alice',
  nickname: '爱丽丝',
  role: 'user',
  is_active: true,
  level: 5,
  level_title: '红石学徒',
  is_overridden: false,
  active_days: 7,
  guest_active_days: 7,
  current_xp: 320,
  lifetime_earned_xp: 360,
  lifetime_reversed_xp: 40,
  achievement_count: 2,
  gift_balance: 18,
  gift_general_balance: 18,
  gift_checkin_balance: 0,
  created_at: '2026-09-01T00:00:00Z',
  updated_at: '2026-10-01T00:00:00Z',
}

const DETAIL: AdminGrowthUserDetail = {
  user: USER,
  base_level: 5,
  level_override: null,
  next_level_xp: 500,
  xp_to_next_level: 180,
  level_progress_percent: 64,
  is_max_level: false,
  achievement_total: 8,
  experience_total: 1,
  gift_total: 1,
  audit_total: 1,
  experience: [
    {
      id: 'e1',
      user_id: 'u1',
      delta_xp: 20,
      balance_after: 320,
      source_type: 'rule',
      source_id: 'post-1',
      rule_code: 'post.comment_created',
      reason: '发表留言',
      created_at: '2026-10-01T01:00:00Z',
    },
  ],
  achievements: [
    {
      id: 'ua1',
      code: 'first_post',
      name: '初次发帖',
      description: '发布第一个帖子',
      category: '社区',
      rarity: 'common',
      reward_xp: 10,
      unlocked_at: '2026-10-01T02:00:00Z',
      grant_source: 'automatic',
      is_public: true,
      definition_id: 'a1',
    },
  ],
  gifts: [
    {
      id: 'g1',
      amount: 18,
      coin_type: 'general',
      balance_after: 18,
      event_type: 'admin_adjustment',
      source_type: 'admin_action',
      source_id: 'req-1',
      reason: '活动奖励',
      created_at: '2026-10-01T03:00:00Z',
    },
  ],
  audits: [
    {
      id: 'log1',
      action: 'xp.adjust',
      entity_type: 'user_growth',
      entity_id: 'u1',
      reason: '活动补发',
      operator_user_id: 'admin-1',
      created_at: '2026-10-01T04:00:00Z',
    },
  ],
}

function paged<T>(items: T[]) {
  return { items, total: items.length, page: 1, pageSize: 15, totalPages: 1 }
}

function mountView() {
  const pinia = createPinia()
  setActivePinia(pinia)
  return mount(AdminGrowthUserDetailPanel, { global: { plugins: [pinia] } })
}

beforeEach(() => {
  vi.clearAllMocks()
  api.fetchAdminGrowthUsers.mockResolvedValue(paged([USER]))
  api.fetchAdminGrowthUserDetail.mockResolvedValue(DETAIL)
})

describe('后台用户成长详情', () => {
  it('搜索用户并展示摘要', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(api.fetchAdminGrowthUsers).toHaveBeenCalledWith(
      expect.objectContaining({ page: 1, page_size: 15 }),
    )
    expect(wrapper.text()).toContain('爱丽丝')
    expect(wrapper.text()).toContain('Lv.5')
    expect(wrapper.text()).toContain('18')
  })

  it('打开用户详情后树状展示等级 / 成就 / 经验 / 礼物 / 日志', async () => {
    const wrapper = mountView()
    await flushPromises()

    const open = wrapper.findAll('button').find((b) => b.text() === '查看详情')
    await open!.trigger('click')
    await flushPromises()

    expect(api.fetchAdminGrowthUserDetail).toHaveBeenCalledWith('u1')
    const text = wrapper.text()
    expect(text).toContain('等级与经验进度')
    expect(text).toContain('初次发帖')
    expect(text).toContain('发表留言')
    expect(text).toContain('活动奖励')
    expect(text).toContain('xp.adjust')
    expect(text).toContain('礼物币')
    expect(text).toContain('2 / 8')

    // 每个条目使用 details，可继续展开查看获取时间与详细日志。
    expect(wrapper.findAll('details').length).toBeGreaterThanOrEqual(6)
  })
})
