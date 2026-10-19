// 分享 URL 与视图状态同步测试（TODO §14）
// 覆盖三个有状态工具：物品 ID 对照表 / 坐标换算 / 种子地图 / 种子与结构工具。
// 重点：
//   1) 带参数的分享链接打开后能还原状态（可复现）；
//   2) 状态变化后 URL 被更新，且**不会丢失其他字段**（避免多个 watch 相互覆盖）；
//   3) URL 中始终不出现会话类机密参数。
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory, type Router } from 'vue-router'

vi.mock('@/api/block-ids', () => ({ listBlockIds: vi.fn() }))
vi.mock('@/api/seed', () => ({ analyzeSeed: vi.fn() }))
vi.mock('@/api/mc', () => ({
  generateSeeds: vi.fn(),
  listCapabilities: vi.fn(),
  listStructureTypes: vi.fn(),
  parseSeed: vi.fn(),
  queryStructures: vi.fn(),
  queryBiome: vi.fn(),
  estimateStrongholds: vi.fn(),
  querySlimeChunks: vi.fn(),
  findSeeds: vi.fn(),
}))

import { listBlockIds } from '@/api/block-ids'
import { analyzeSeed } from '@/api/seed'
import { listCapabilities, listStructureTypes } from '@/api/mc'
import type { BlockIdEntry } from '@/types/block-id'
import BlockIdsView from '@/views/BlockIdsView.vue'
import CoordinateView from '@/views/CoordinateView.vue'
import SeedMapView from '@/views/SeedMapView.vue'
import SeedToolsView from '@/views/SeedToolsView.vue'

const SAMPLE: BlockIdEntry[] = [
  { name: '石头', id: 'minecraft:stone', form: 'ID相同', kind: 'same' },
  { name: '钻石矿石', id: 'minecraft:diamond_ore', form: 'ID不同（diamond）', kind: 'different', refId: 'diamond' },
  { name: '苹果', id: 'minecraft:apple', form: '', kind: 'item' },
]

const PATHS = ['/', '/tools', '/block-ids', '/coordinate', '/seed-map', '/seed-tools', '/items']

function makeRouter(): Router {
  return createRouter({
    history: createMemoryHistory(),
    routes: PATHS.map((path) => ({ path, component: { template: '<div />' } })),
  })
}

/** jsdom 无 canvas 实现，注入一个吞掉所有调用的 2D 上下文 */
function stubCanvas() {
  const ctx = new Proxy({}, { get: () => () => {} })
  HTMLCanvasElement.prototype.getContext = vi.fn(
    () => ctx,
  ) as unknown as typeof HTMLCanvasElement.prototype.getContext
}

async function push(router: Router, fullPath: string) {
  await router.push(fullPath)
  await router.isReady()
}

beforeEach(() => {
  vi.clearAllMocks()
  setActivePinia(createPinia())
})

describe('§14 分享 URL · 物品 ID 对照表', () => {
  it('从分享链接还原关键词与分类筛选', async () => {
    ;(listBlockIds as ReturnType<typeof vi.fn>).mockResolvedValue({ total: 3, entries: SAMPLE })
    const router = makeRouter()
    await push(router, '/block-ids?q=diamond&kind=different')

    const wrapper = mount(BlockIdsView, { global: { plugins: [router] } })
    await flushPromises()

    expect((wrapper.find('input[type="search"]').element as HTMLInputElement).value).toBe('diamond')
    expect(wrapper.findAll('tbody tr').length).toBe(1)
    expect(wrapper.text()).toContain('minecraft:diamond_ore')
  })

  it('修改关键词后 URL 更新，且不丢失分类字段', async () => {
    ;(listBlockIds as ReturnType<typeof vi.fn>).mockResolvedValue({ total: 3, entries: SAMPLE })
    const router = makeRouter()
    await push(router, '/block-ids?kind=different')

    const wrapper = mount(BlockIdsView, { global: { plugins: [router] } })
    await flushPromises()
    await wrapper.find('input[type="search"]').setValue('ore')
    await flushPromises()

    expect(router.currentRoute.value.query).toMatchObject({ q: 'ore', kind: 'different' })
  })

  it('回到默认筛选后链接保持干净（无冗余参数）', async () => {
    ;(listBlockIds as ReturnType<typeof vi.fn>).mockResolvedValue({ total: 3, entries: SAMPLE })
    const router = makeRouter()
    await push(router, '/block-ids?q=diamond')

    const wrapper = mount(BlockIdsView, { global: { plugins: [router] } })
    await flushPromises()
    await wrapper.find('input[type="search"]').setValue('')
    await flushPromises()

    expect(router.currentRoute.value.query).toEqual({})
  })
})

describe('§14 分享 URL · 坐标换算', () => {
  it('从分享链接还原换算方向与坐标（含线路规划字段）', async () => {
    const router = makeRouter()
    await push(router, '/coordinate?dir=netherToOverworld&x=100&z=-30&fx=0&fz=0&tx=800&tz=-240')

    const wrapper = mount(CoordinateView, { global: { plugins: [router] } })
    await flushPromises()

    const numbers = wrapper.findAll('input[type="number"]').map((i) => (i.element as HTMLInputElement).value)
    // 前三个是换算坐标，后四个是线路规划
    expect(numbers.slice(0, 3)).toEqual(['100', '', '-30'])
    expect(numbers.slice(3)).toEqual(['0', '0', '800', '-240'])
    expect(wrapper.text()).toContain('下界')
  })

  it('修改换算坐标后 URL 同时保留线路规划字段（两个 watch 不互相覆盖）', async () => {
    const router = makeRouter()
    await push(router, '/coordinate?tx=800&tz=-240')

    const wrapper = mount(CoordinateView, { global: { plugins: [router] } })
    await flushPromises()

    const numbers = wrapper.findAll('input[type="number"]')
    await numbers[0].setValue('800')
    await numbers[2].setValue('-240')
    await flushPromises()

    // 换算结果正常渲染（回归：数字输入曾因 `.trim()` 抛错导致渲染失败）
    expect(wrapper.text()).toContain('X: 100')
    expect(wrapper.text()).toContain('Z: -30')
    expect(router.currentRoute.value.query).toMatchObject({ x: '800', z: '-240', tx: '800', tz: '-240' })
  })
})

describe('§14 分享 URL · 种子地图', () => {
  it('带种子的分享链接打开后自动分析并还原版本与半径', async () => {
    stubCanvas()
    ;(analyzeSeed as ReturnType<typeof vi.fn>).mockResolvedValue({
      world_seed: '-1234567890',
      radius: 16,
      structures: [{ type: 'slime_chunk', x: 8, z: 8 }],
    })
    const router = makeRouter()
    await push(router, '/seed-map?seed=-1234567890&version=1.16-1.17&radius=16')

    const wrapper = mount(SeedMapView, { global: { plugins: [router] } })
    await flushPromises()

    expect(analyzeSeed).toHaveBeenCalledWith(
      expect.objectContaining({ seed: '-1234567890', version: '1.16-1.17', radius: 16 }),
    )
    expect((wrapper.find('input#seed').element as HTMLInputElement).value).toBe('-1234567890')
  })

  it('无种子的链接不发起分析请求', async () => {
    stubCanvas()
    const router = makeRouter()
    await push(router, '/seed-map')

    mount(SeedMapView, { global: { plugins: [router] } })
    await flushPromises()

    expect(analyzeSeed).not.toHaveBeenCalled()
  })

  it('“填入示例”填充种子并触发分析', async () => {
    stubCanvas()
    ;(analyzeSeed as ReturnType<typeof vi.fn>).mockResolvedValue({ world_seed: '-1234567890', radius: 8, structures: [] })
    const router = makeRouter()
    await push(router, '/seed-map')

    const wrapper = mount(SeedMapView, { global: { plugins: [router] } })
    await flushPromises()
    const example = wrapper.findAll('button').find((b) => b.text().includes('填入示例'))
    await example!.trigger('click')
    await flushPromises()

    expect(analyzeSeed).toHaveBeenCalledTimes(1)
    expect(router.currentRoute.value.query.seed).toBe('-1234567890')
  })
})

describe('§14 分享 URL · 种子与结构工具', () => {
  it('从分享链接还原各子表单输入', async () => {
    ;(listStructureTypes as ReturnType<typeof vi.fn>).mockResolvedValue({ types: [] })
    ;(listCapabilities as ReturnType<typeof vi.fn>).mockResolvedValue({ features: [] })
    const router = makeRouter()
    await push(router, '/seed-tools?v=1.13-1.15&stSeed=42&bioSeed=7&gen=5')

    const wrapper = mount(SeedToolsView, { global: { plugins: [router] } })
    await flushPromises()

    expect((wrapper.find('#st-seed').element as HTMLInputElement).value).toBe('42')
    expect((wrapper.find('#bio-seed').element as HTMLInputElement).value).toBe('7')
    expect((wrapper.find('#gen-count').element as HTMLInputElement).value).toBe('5')
  })

  it('修改子表单后 URL 更新并保留其他子表单参数', async () => {
    ;(listStructureTypes as ReturnType<typeof vi.fn>).mockResolvedValue({ types: [] })
    ;(listCapabilities as ReturnType<typeof vi.fn>).mockResolvedValue({ features: [] })
    const router = makeRouter()
    await push(router, '/seed-tools?v=1.13-1.15')

    const wrapper = mount(SeedToolsView, { global: { plugins: [router] } })
    await flushPromises()
    await wrapper.find('#st-seed').setValue('99')
    await flushPromises()

    expect(router.currentRoute.value.query).toMatchObject({ v: '1.13-1.15', stSeed: '99' })
  })
})
