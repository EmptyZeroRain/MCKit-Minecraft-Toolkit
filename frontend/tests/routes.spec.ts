// 路由接线测试：确认内置工具清单里的每个页面都真的被注册、可解析，
// 而不是「组件写了但路由没接」的死代码（这是本次移植修复的核心缺陷）。
// 采用遍历清单的方式，今后新增内置工具若漏注册路由会立即失败。
import { describe, expect, it, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import router from '@/router'
import { BUILTIN_TOOLS, builtinToolRoute } from '@/utils/builtin-tools'
import { useAuthStore } from '@/stores/auth'
import { useUserAuthStore } from '@/stores/user-auth'

describe('内置工具路由接线', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    // router 的 scrollBehavior 会调用 window.scrollTo，jsdom 未实现，静音
    vi.stubGlobal('scrollTo', vi.fn())
  })

  it('清单自身无重复 slug / route', () => {
    const slugs = BUILTIN_TOOLS.map((t) => t.slug)
    const routes = BUILTIN_TOOLS.map((t) => t.route)
    expect(new Set(slugs).size).toBe(slugs.length)
    expect(new Set(routes).size).toBe(routes.length)
  })

  it('slug -> route 映射与清单一致', () => {
    for (const tool of BUILTIN_TOOLS) {
      expect(builtinToolRoute(tool.slug), tool.slug).toBe(tool.route)
    }
    expect(builtinToolRoute('not-a-tool')).toBeNull()
  })

  it.each(BUILTIN_TOOLS.map((t) => [t.route, t.name] as const))(
    '%s 可解析到已注册路由（%s）',
    async (route, _name) => {
      await router.push(route)
      await router.isReady()
      expect(router.currentRoute.value.name).not.toBe('not-found')
      expect(router.currentRoute.value.matched.length).toBeGreaterThan(0)
    },
  )

  it('CreeperCode 移植页已接线：/versions 与 /block-ids', async () => {
    await router.push('/versions')
    expect(router.currentRoute.value.name).toBe('versions')
    await router.push('/block-ids')
    expect(router.currentRoute.value.name).toBe('block-ids')
  })

  it('路由 name 全局唯一（防并发编辑撞车导致重复注册）', () => {
    const names = router
      .getRoutes()
      .map((r) => r.name)
      .filter((n): n is string => typeof n === 'string')
    expect(new Set(names).size, `重复的路由 name: ${names.join(', ')}`).toBe(names.length)
  })

  it('未知路径仍落到 404 兜底（未被新路由吞掉）', async () => {
    await router.push('/definitely-not-a-real-page')
    await router.isReady()
    expect(router.currentRoute.value.name).toBe('not-found')
  })
})

// 后台信息架构合并（gj.md §2.2）：多个功能页合并为带 Tab 的单一页面，
// 旧入口保留为重定向，避免既有书签 / 外链失效。
describe('后台信息架构合并', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.stubGlobal('scrollTo', vi.fn())
    // 管理页受路由守卫保护，这里直接置为已登录（前端守卫仅体验优化）。
    const auth = useAuthStore()
    auth.user = {
      id: '00000000-0000-0000-0000-000000000000',
      username: 'it-admin',
      role: 'admin',
      permissions: [
        'content.categories',
        'content.tags',
        'review.redstone',
        'review.mods',
        'ops.analytics',
        'ops.searchstats',
        'accounts.admins',
        'accounts.users',
      ],
    } as never
    auth.initialized = true
  })

  const merged = [
    { path: '/admin/taxonomy', name: 'admin-taxonomy' },
    { path: '/admin/review', name: 'admin-review' },
    { path: '/admin/insights', name: 'admin-insights' },
    { path: '/admin/accounts', name: 'admin-accounts' },
  ] as const

  it.each(merged)('合并页 $path 已接线', async ({ path, name }) => {
    await router.push(path)
    await router.isReady()
    expect(router.currentRoute.value.name).toBe(name)
  })

  const legacy = [
    { path: '/admin/categories', name: 'admin-taxonomy', tab: 'categories' },
    { path: '/admin/tags', name: 'admin-taxonomy', tab: 'tags' },
    { path: '/admin/redstone', name: 'admin-review', tab: 'redstone' },
    { path: '/admin/mods', name: 'admin-review', tab: 'mods' },
    { path: '/admin/analytics', name: 'admin-insights', tab: 'analytics' },
    { path: '/admin/search-terms', name: 'admin-insights', tab: 'search-terms' },
    { path: '/admin/admins', name: 'admin-accounts', tab: 'admins' },
    { path: '/admin/users', name: 'admin-accounts', tab: 'users' },
  ] as const

  it.each(legacy)('旧入口 $path 重定向到 $name?tab=$tab', async ({ path, name, tab }) => {
    await router.push(path)
    await router.isReady()
    expect(router.currentRoute.value.name).toBe(name)
    expect(router.currentRoute.value.query.tab).toBe(tab)
  })

  it('新增「内容建设任务」页已接线（gj.md §8.3）', async () => {
    await router.push('/admin/content-tasks')
    await router.isReady()
    expect(router.currentRoute.value.name).toBe('admin-content-tasks')
  })
})

// 成长系统路由（jingyan.md §10 / §11）：用户成长中心 + 后台用户成长。
describe('成长系统路由接线', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.stubGlobal('scrollTo', vi.fn())

    const auth = useAuthStore()
    auth.user = {
      id: '00000000-0000-0000-0000-000000000000',
      username: 'it-admin',
      role: 'admin',
      permissions: ['ops.growth'],
    } as never
    auth.initialized = true

    const userAuth = useUserAuthStore()
    userAuth.user = {
      id: '00000000-0000-0000-0000-000000000001',
      username: 'it-user',
    } as never
    userAuth.initialized = true
  })

  it('用户成长中心 /growth 已接线且要求登录', async () => {
    await router.push('/growth')
    await router.isReady()
    expect(router.currentRoute.value.name).toBe('growth')
    expect(router.currentRoute.value.meta.requiresUser).toBe(true)
  })

  it('后台用户成长 /admin/growth 已接线', async () => {
    await router.push('/admin/growth')
    await router.isReady()
    expect(router.currentRoute.value.name).toBe('admin-growth')
  })
})
