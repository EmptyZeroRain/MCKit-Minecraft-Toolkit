// 用户成长系统 API 模块测试（jingyan.md §9）。
// 验证：用户端走 userHttp、管理端走 http，动作子路径与请求体字段与后端一致，
// 以及分页响应折叠，避免前后端接口漂移。
import { beforeEach, describe, expect, it, vi } from 'vitest'

const httpMock = vi.hoisted(() => ({ post: vi.fn() }))
const userHttpMock = vi.hoisted(() => ({ post: vi.fn() }))

vi.mock('@/api/http', () => ({ default: httpMock }))
vi.mock('@/api/user-http', () => ({ default: userHttpMock }))

import {
  adjustUserXP,
  createAchievementDef,
  createDailyTaskDef,
  createGrowthLevel,
  createGrowthRule,
  deleteDailyTaskDef,
  deleteGrowthLevel,
  deleteGrowthRule,
  fetchAchievementDefs,
  fetchAdminGrowthLedger,
  fetchAdminGrowthOverview,
  fetchDailyTaskDefs,
  fetchGrowthAchievements,
  fetchGrowthAudit,
  fetchGrowthDailyTaskHistory,
  fetchGrowthDailyTasks,
  fetchGrowthLedger,
  fetchGrowthLevels,
  fetchGrowthOverview,
  fetchGrowthRules,
  fetchPublicAchievements,
  fetchPublicGrowthProfile,
  recalculateAchievements,
  recalculateGrowthLevels,
  reverseGrowthLedger,
  toggleAchievementDef,
  toggleDailyTaskDef,
  toggleGrowthRule,
  updateAchievementDef,
  updateDailyTaskDef,
  updateGrowthLevel,
  updateGrowthRule,
} from '@/api/growth'

const meta = { total: 3, page: 2, page_size: 10, total_pages: 1 }

describe('growth API · 用户端（userHttp）', () => {
  beforeEach(() => {
    userHttpMock.post.mockReset()
    httpMock.post.mockReset()
  })

  it('概览 / 今日任务 / 成就目录走用户实例与正确路径', async () => {
    userHttpMock.post.mockResolvedValue({ data: { data: { current_xp: 120 } } })
    await fetchGrowthOverview()
    expect(userHttpMock.post).toHaveBeenCalledWith('/growth/overview', {})

    userHttpMock.post.mockResolvedValue({ data: { data: { date: '2026-10-19', timezone: 'Asia/Shanghai', tasks: [] } } })
    const daily = await fetchGrowthDailyTasks()
    expect(userHttpMock.post).toHaveBeenCalledWith('/growth/daily-tasks', {})
    expect(daily.date).toBe('2026-10-19')

    userHttpMock.post.mockResolvedValue({ data: { data: { items: [{ code: 'a' }], total: 1 } } })
    const ach = await fetchGrowthAchievements()
    expect(userHttpMock.post).toHaveBeenCalledWith('/growth/achievements', {})
    expect(ach).toHaveLength(1)

    expect(httpMock.post).not.toHaveBeenCalled()
  })

  it('流水 / 任务历史折叠分页元信息', async () => {
    userHttpMock.post.mockResolvedValue({ data: { data: [{ id: 'l1' }], meta } })
    const ledger = await fetchGrowthLedger({ page: 2, page_size: 10 })
    expect(userHttpMock.post).toHaveBeenCalledWith('/growth/ledger', { page: 2, page_size: 10 })
    expect(ledger).toMatchObject({ total: 3, page: 2, pageSize: 10, totalPages: 1 })

    userHttpMock.post.mockResolvedValue({ data: { data: [], meta: undefined } })
    const history = await fetchGrowthDailyTaskHistory()
    expect(userHttpMock.post).toHaveBeenCalledWith('/growth/daily-tasks/history', {})
    expect(history).toMatchObject({ total: 0, page: 1, totalPages: 0 })
  })

  it('公开主页走 /public/users/*', async () => {
    userHttpMock.post.mockResolvedValue({ data: { data: { user_id: 'u1' } } })
    await fetchPublicGrowthProfile('u1')
    expect(userHttpMock.post).toHaveBeenCalledWith('/public/users/detail', { id: 'u1' })

    userHttpMock.post.mockResolvedValue({ data: { data: { items: [] } } })
    await fetchPublicAchievements('u1')
    expect(userHttpMock.post).toHaveBeenCalledWith('/public/users/achievements', { id: 'u1' })
  })
})

describe('growth API · 管理端（http）', () => {
  beforeEach(() => {
    userHttpMock.post.mockReset()
    httpMock.post.mockReset()
  })

  it('概览 / 流水 / 审计 / 规则 / 等级 / 任务 / 成就列表路径正确', async () => {
    httpMock.post.mockResolvedValue({ data: { data: { active_users: 1 } } })
    await fetchAdminGrowthOverview()
    expect(httpMock.post).toHaveBeenCalledWith('/admin/growth/overview', {})

    httpMock.post.mockResolvedValue({ data: { data: [], meta } })
    await fetchAdminGrowthLedger({ user_id: 'u1', source_type: 'manual' })
    expect(httpMock.post).toHaveBeenCalledWith('/admin/growth/ledger/list', {
      user_id: 'u1',
      source_type: 'manual',
    })

    httpMock.post.mockResolvedValue({ data: { data: [], meta: undefined } })
    await fetchGrowthAudit({ action: 'xp.adjust' })
    expect(httpMock.post).toHaveBeenCalledWith('/admin/growth/audit/list', { action: 'xp.adjust' })

    httpMock.post.mockResolvedValue({ data: { data: { items: [] } } })
    await fetchGrowthRules()
    expect(httpMock.post).toHaveBeenCalledWith('/admin/growth/rules/list', {})

    await fetchGrowthLevels()
    expect(httpMock.post).toHaveBeenCalledWith('/admin/growth/levels/list', {})

    await fetchDailyTaskDefs()
    expect(httpMock.post).toHaveBeenCalledWith('/admin/daily-tasks/list', {})

    await fetchAchievementDefs()
    expect(httpMock.post).toHaveBeenCalledWith('/admin/achievements/list', {})

    expect(userHttpMock.post).not.toHaveBeenCalled()
  })

  it('人工调整 / 冲正请求体正确', async () => {
    httpMock.post.mockResolvedValue({ data: { data: { id: 'l1' } } })
    await adjustUserXP({ target_user_id: 'u1', delta_xp: 50, reason: '活动补发' })
    expect(httpMock.post).toHaveBeenCalledWith('/admin/growth/adjust', {
      target_user_id: 'u1',
      delta_xp: 50,
      reason: '活动补发',
    })

    await reverseGrowthLedger('l1', '误发')
    expect(httpMock.post).toHaveBeenCalledWith('/admin/growth/reverse', { id: 'l1', reason: '误发' })
  })

  it('规则创建 / 更新 / 启停 / 删除路径与请求体正确', async () => {
    httpMock.post.mockResolvedValue({ data: { data: { id: 'r1' } } })
    await createGrowthRule({ name: '测试', event_type: 'post.liked', xp_amount: 1, daily_limit: 5, per_subject_limit: 1, enabled: true })
    expect(httpMock.post).toHaveBeenCalledWith('/admin/growth/rules/create', {
      name: '测试',
      event_type: 'post.liked',
      xp_amount: 1,
      daily_limit: 5,
      per_subject_limit: 1,
      enabled: true,
    })

    await updateGrowthRule('r1', { name: '改', event_type: 'post.liked', xp_amount: 2, daily_limit: null, per_subject_limit: null, enabled: true })
    expect(httpMock.post).toHaveBeenCalledWith('/admin/growth/rules/update', {
      id: 'r1',
      name: '改',
      event_type: 'post.liked',
      xp_amount: 2,
      daily_limit: null,
      per_subject_limit: null,
      enabled: true,
    })

    await toggleGrowthRule('r1', false)
    expect(httpMock.post).toHaveBeenCalledWith('/admin/growth/rules/toggle', { id: 'r1', enabled: false })

    await deleteGrowthRule('r1')
    expect(httpMock.post).toHaveBeenCalledWith('/admin/growth/rules/delete', { id: 'r1' })
  })

  it('等级更新 / 重算、任务与成就定义管理路径正确', async () => {
    httpMock.post.mockResolvedValue({ data: { data: { fixed: 2 } } })
    const fixed = await recalculateGrowthLevels()
    expect(httpMock.post).toHaveBeenCalledWith('/admin/growth/levels/recalculate', {})
    expect(fixed).toBe(2)

    httpMock.post.mockResolvedValue({ data: { data: null } })
    await updateGrowthLevel({ level: 3, threshold_xp: 300, title: '探索者', enabled: true })
    expect(httpMock.post).toHaveBeenCalledWith('/admin/growth/levels/update', {
      level: 3,
      threshold_xp: 300,
      title: '探索者',
      enabled: true,
    })

    httpMock.post.mockResolvedValue({
      data: { data: { level: 11, threshold_xp: 8000, title: '测试等级', enabled: true } },
    })
    const created = await createGrowthLevel({
      threshold_xp: 8000,
      title: '测试等级',
      enabled: true,
    })
    expect(httpMock.post).toHaveBeenCalledWith('/admin/growth/levels/create', {
      threshold_xp: 8000,
      title: '测试等级',
      enabled: true,
    })
    expect(created.level).toBe(11)

    httpMock.post.mockResolvedValue({ data: { data: null } })
    await deleteGrowthLevel(11)
    expect(httpMock.post).toHaveBeenCalledWith('/admin/growth/levels/delete', { level: 11 })

    await createDailyTaskDef({ name: 't', description: '', event_type: 'post.liked', target_count: 3, reward_xp: 3, enabled: true, sort_order: 1 })
    expect(httpMock.post).toHaveBeenCalledWith('/admin/daily-tasks/create', {
      name: 't',
      description: '',
      event_type: 'post.liked',
      target_count: 3,
      reward_xp: 3,
      enabled: true,
      sort_order: 1,
    })

    await updateDailyTaskDef('t1', { name: 't', description: '', event_type: 'post.liked', target_count: 3, reward_xp: 3, enabled: false, sort_order: 1 })
    expect(httpMock.post).toHaveBeenCalledWith('/admin/daily-tasks/update', {
      id: 't1',
      name: 't',
      description: '',
      event_type: 'post.liked',
      target_count: 3,
      reward_xp: 3,
      enabled: false,
      sort_order: 1,
    })

    await toggleDailyTaskDef('t1', false)
    expect(httpMock.post).toHaveBeenCalledWith('/admin/daily-tasks/toggle', { id: 't1', enabled: false })

    await deleteDailyTaskDef('t1')
    expect(httpMock.post).toHaveBeenCalledWith('/admin/daily-tasks/delete', { id: 't1' })
  })

  it('成就定义管理与历史补算路径正确', async () => {
    httpMock.post.mockResolvedValue({ data: { data: { id: 'a1' } } })
    await createAchievementDef({
      name: 'x',
      description: '',
      category: '探索',
      rarity: 'common',
      icon_url: '',
      condition_type: 'event_count',
      condition_event_type: 'tool.used_validly',
      condition_subject_type: '',
      target_value: 1,
      reward_xp: 0,
      is_hidden: false,
      enabled: true,
      sort_order: 1,
    })
    expect(httpMock.post).toHaveBeenCalledWith('/admin/achievements/create', expect.objectContaining({ name: 'x' }))

    await updateAchievementDef('a1', {
      name: 'x',
      description: '',
      category: '探索',
      rarity: 'common',
      icon_url: '',
      condition_type: 'level_reached',
      condition_event_type: '',
      condition_subject_type: '',
      target_value: 3,
      reward_xp: 0,
      is_hidden: false,
      enabled: true,
      sort_order: 1,
    })
    expect(httpMock.post).toHaveBeenCalledWith('/admin/achievements/update', expect.objectContaining({ id: 'a1' }))

    await toggleAchievementDef('a1', false)
    expect(httpMock.post).toHaveBeenCalledWith('/admin/achievements/toggle', { id: 'a1', enabled: false })

    httpMock.post.mockResolvedValue({ data: { data: { unlocked: 7 } } })
    const unlocked = await recalculateAchievements()
    expect(httpMock.post).toHaveBeenCalledWith('/admin/achievements/recalculate', { user_id: '' })
    expect(unlocked).toBe(7)
  })
})
