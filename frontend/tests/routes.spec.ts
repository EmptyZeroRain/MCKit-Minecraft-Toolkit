// 路由接线测试：确认内置工具清单里的每个页面都真的被注册、可解析，
// 而不是「组件写了但路由没接」的死代码（这是本次移植修复的核心缺陷）。
// 采用遍历清单的方式，今后新增内置工具若漏注册路由会立即失败。
import { describe, expect, it, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import router from '@/router'
import { BUILTIN_TOOLS, builtinToolRoute } from '@/utils/builtin-tools'

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
