// 物品 ID 对照表测试（需求 13.3：搜索/筛选状态、空结果反馈）
// 覆盖两层：纯筛选函数（表驱动）+ 视图交互（关键词 / 分类 / 空态）。
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import type { VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory } from 'vue-router'
import { filterBlockIds, countBlockIds, matchBlockId } from '@/utils/block-ids'
import type { BlockIdEntry } from '@/types/block-id'

const SAMPLE: BlockIdEntry[] = [
  { name: '石头', id: 'minecraft:stone', form: 'ID相同', kind: 'same' },
  { name: '钻石矿石', id: 'minecraft:diamond_ore', form: 'ID不同（diamond）', kind: 'different', refId: 'diamond' },
  { name: '墙上的火把', id: 'minecraft:wall_torch', form: '变种（torch）', kind: 'variant', refId: 'torch' },
  { name: '空气', id: 'minecraft:air', form: '等同于空物品槽', kind: 'special' },
  { name: '苹果', id: 'minecraft:apple', form: '', kind: 'item' },
]

describe('物品 ID 对照表 · 筛选函数', () => {
  it('关键词匹配 ID 或中文名，且大小写不敏感', () => {
    expect(matchBlockId(SAMPLE[0], 'STONE')).toBe(true)
    expect(matchBlockId(SAMPLE[0], '石头')).toBe(true)
    expect(matchBlockId(SAMPLE[1], 'diamond')).toBe(true)
    expect(matchBlockId(SAMPLE[1], '石头')).toBe(false)
  })

  it('空关键词命中全部', () => {
    expect(matchBlockId(SAMPLE[0], '')).toBe(true)
    expect(matchBlockId(SAMPLE[0], '   ')).toBe(true)
    expect(filterBlockIds(SAMPLE, '', '').length).toBe(SAMPLE.length)
  })

  it("按分类过滤：'' 与 all 等价于不限", () => {
    expect(filterBlockIds(SAMPLE, '', 'all').length).toBe(SAMPLE.length)
    expect(filterBlockIds(SAMPLE, '', 'variant').map((e) => e.id)).toEqual(['minecraft:wall_torch'])
  })

  it('关键词与分类叠加生效', () => {
    expect(filterBlockIds(SAMPLE, 'ore', 'different').length).toBe(1)
    expect(filterBlockIds(SAMPLE, 'ore', 'variant').length).toBe(0)
  })

  it('无匹配时返回空数组，不伪造结果', () => {
    expect(filterBlockIds(SAMPLE, 'zzz-not-exist', '')).toEqual([])
  })

  it('分类计数：all 返回总数，具体分类返回子集', () => {
    expect(countBlockIds(SAMPLE, 'all')).toBe(5)
    expect(countBlockIds(SAMPLE, '')).toBe(5)
    expect(countBlockIds(SAMPLE, 'item')).toBe(1)
    expect(countBlockIds(SAMPLE, 'overlay')).toBe(0)
  })
})

// ===== 视图交互 =====
vi.mock('@/api/block-ids', () => ({
  listBlockIds: vi.fn(),
}))

import { listBlockIds } from '@/api/block-ids'
import BlockIdsView from '@/views/BlockIdsView.vue'

function makeRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/', component: { template: '<div />' } }],
  })
}

async function mountView() {
  setActivePinia(createPinia())
  ;(listBlockIds as ReturnType<typeof vi.fn>).mockResolvedValue({
    total: SAMPLE.length,
    entries: SAMPLE,
  })
  const wrapper = mount(BlockIdsView, {
    global: { plugins: [makeRouter()] },
  })
  await flushPromises()
  return wrapper
}

function buttonByText(wrapper: VueWrapper, text: string) {
  return wrapper.findAll('button').find((b) => b.text().includes(text))
}

describe('物品 ID 对照表 · 视图', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('加载后渲染全部条目', async () => {
    const wrapper = await mountView()
    expect(listBlockIds).toHaveBeenCalledWith({})
    expect(wrapper.findAll('tbody tr').length).toBe(SAMPLE.length)
    expect(wrapper.text()).toContain('共 5 条')
  })

  it('关键词过滤后行数变化，并展示筛选后条数', async () => {
    const wrapper = await mountView()
    await wrapper.find('input[type="search"]').setValue('diamond')
    await flushPromises()
    expect(wrapper.findAll('tbody tr').length).toBe(1)
    expect(wrapper.text()).toContain('minecraft:diamond_ore')
    expect(wrapper.text()).toContain('全部 5 条')
  })

  it('分类筛选生效', async () => {
    const wrapper = await mountView()
    const btn = buttonByText(wrapper, '变种')
    expect(btn).toBeTruthy()
    await btn!.trigger('click')
    await flushPromises()
    expect(wrapper.findAll('tbody tr').length).toBe(1)
    expect(wrapper.text()).toContain('minecraft:wall_torch')
  })

  it('无匹配时展示空状态而非空表', async () => {
    const wrapper = await mountView()
    await wrapper.find('input[type="search"]').setValue('zzz-not-exist')
    await flushPromises()
    expect(wrapper.findAll('tbody tr').length).toBe(0)
    expect(wrapper.text()).toContain('没有匹配的条目')
  })
})
