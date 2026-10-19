// 内置工具使用统计上报测试（TODO §13）
// 覆盖：路径 -> slug 反查、测试环境静默、visit/use/fail 三类事件、空 slug 保护、
// 失败静默，以及 CopyButton 带 tool 属性时把「复制产出」记为一次使用。
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { BUILTIN_TOOL_ROUTES } from '@/utils/builtin-tools'

const { reportToolEventMock } = vi.hoisted(() => ({
  reportToolEventMock: vi.fn().mockResolvedValue({}),
}))
vi.mock('@/api/stats', () => ({ reportToolEvent: reportToolEventMock }))

import CopyButton from '@/components/common/CopyButton.vue'
import {
  builtinToolSlugByPath,
  reportToolFailure,
  reportToolUse,
  reportToolVisit,
} from '@/utils/tool-stats'

describe('内置工具统计上报', () => {
  beforeEach(() => {
    reportToolEventMock.mockClear()
    reportToolEventMock.mockResolvedValue({})
  })

  afterEach(() => {
    vi.unstubAllEnvs()
  })

  it('路径可反查内置工具 slug，且只认内置工具落地页', () => {
    for (const [slug, route] of Object.entries(BUILTIN_TOOL_ROUTES)) {
      expect(builtinToolSlugByPath(route), route).toBe(slug)
    }
    // 详情页 / 后台页 / 首页都不是内置工具页面
    expect(builtinToolSlugByPath('/items/minecraft:stone')).toBeNull()
    expect(builtinToolSlugByPath('/admin/analytics')).toBeNull()
    expect(builtinToolSlugByPath('/')).toBeNull()
  })

  it('测试环境不发送任何上报（避免单测产生网络噪音）', () => {
    reportToolVisit('/recipes')
    reportToolUse('recipes')
    reportToolFailure('recipes')
    expect(reportToolEventMock).not.toHaveBeenCalled()
  })

  it('非测试环境按 visit / use / fail 上报，非内置工具路径自动忽略', () => {
    vi.stubEnv('MODE', 'production')
    reportToolVisit('/recipes')
    reportToolUse('recipes')
    reportToolFailure('recipes')
    reportToolVisit('/admin/analytics')

    expect(reportToolEventMock).toHaveBeenCalledTimes(3)
    expect(reportToolEventMock).toHaveBeenNthCalledWith(1, 'recipes', 'visit')
    expect(reportToolEventMock).toHaveBeenNthCalledWith(2, 'recipes', 'use')
    expect(reportToolEventMock).toHaveBeenNthCalledWith(3, 'recipes', 'fail')
  })

  it('空 slug / undefined 不发送', () => {
    vi.stubEnv('MODE', 'production')
    reportToolUse(undefined)
    reportToolUse(null)
    reportToolUse('')
    reportToolFailure(undefined)
    expect(reportToolEventMock).not.toHaveBeenCalled()
  })

  it('上报失败被静默吞掉，不抛出也不产生未处理拒绝', async () => {
    vi.stubEnv('MODE', 'production')
    reportToolEventMock.mockRejectedValueOnce(new Error('network down'))
    expect(() => reportToolUse('recipes')).not.toThrow()
    await new Promise((r) => setTimeout(r, 0))
  })
})

describe('CopyButton 的 tool 属性（复制产出 = 一次使用）', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    reportToolEventMock.mockClear()
    reportToolEventMock.mockResolvedValue({})
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText: vi.fn().mockResolvedValue(undefined) },
    })
    Object.defineProperty(window, 'isSecureContext', { configurable: true, value: true })
  })

  afterEach(() => {
    vi.unstubAllEnvs()
  })

  it('复制成功且声明 tool 时上报 use', async () => {
    vi.stubEnv('MODE', 'production')
    const wrapper = mount(CopyButton, { props: { text: '/give @s stone', tool: 'command-generator' } })
    await wrapper.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(reportToolEventMock).toHaveBeenCalledWith('command-generator', 'use')
  })

  it('未声明 tool 时不上报', async () => {
    vi.stubEnv('MODE', 'production')
    const wrapper = mount(CopyButton, { props: { text: 'plain' } })
    await wrapper.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(reportToolEventMock).not.toHaveBeenCalled()
  })
})
