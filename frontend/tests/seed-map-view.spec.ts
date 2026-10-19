// 种子地图页交互测试（需求 13.3：表单校验、API 错误状态）
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory } from 'vue-router'

vi.mock('@/api/seed', () => ({ analyzeSeed: vi.fn() }))
vi.mock('@/api/mc', () => ({ generateSeeds: vi.fn() }))

import { analyzeSeed } from '@/api/seed'
import SeedMapView from '@/views/SeedMapView.vue'

/** jsdom 无 canvas 实现，注入一个吞掉所有调用的 2D 上下文 */
function stubCanvas() {
  const ctx = new Proxy({}, { get: () => () => {} })
  HTMLCanvasElement.prototype.getContext = vi.fn(
    () => ctx,
  ) as unknown as typeof HTMLCanvasElement.prototype.getContext
}

async function mountView() {
  setActivePinia(createPinia())
  stubCanvas()
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: { template: '<div />' } },
      { path: '/tools', component: { template: '<div />' } },
    ],
  })
  const wrapper = mount(SeedMapView, { global: { plugins: [router] } })
  await flushPromises()
  return wrapper
}

describe('种子地图页', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('空种子提交时给出字段错误，不调用接口', async () => {
    const wrapper = await mountView()
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('请输入世界种子')
    expect(analyzeSeed).not.toHaveBeenCalled()
  })

  it('分析成功后渲染世界种子、结构列表与图例', async () => {
    ;(analyzeSeed as ReturnType<typeof vi.fn>).mockResolvedValue({
      world_seed: '-1234567890',
      radius: 8,
      structures: [
        { type: 'slime_chunk', x: 8, z: 8 },
        { type: 'village', x: 100, z: -200, label: '理论候选位置' },
      ],
    })

    const wrapper = await mountView()
    await wrapper.find('input#seed').setValue('-1234567890')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(analyzeSeed).toHaveBeenCalledTimes(1)
    const text = wrapper.text()
    expect(text).toContain('-1234567890')
    expect(text).toContain('村庄')
    expect(text).toContain('史莱姆区块')
    expect(text).toContain('理论候选位置')
    expect(wrapper.findAll('tbody tr').length).toBe(2)
  })

  it('后端失败时降级为本地史莱姆区块并提示', async () => {
    ;(analyzeSeed as ReturnType<typeof vi.fn>).mockRejectedValue(new Error('boom'))

    const wrapper = await mountView()
    await wrapper.find('input#seed').setValue('12345')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('结构数据获取失败')
    // 本地引擎仍会给出史莱姆区块（可能为 0 个，但不应崩溃）
    expect(wrapper.text()).toContain('区块地图')
  })
})
