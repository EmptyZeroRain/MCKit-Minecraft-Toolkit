// 村民交易（TODO §9B）测试：
//   1) 类型层展示辅助（数量区间格式化）；
//   2) 列表页按名称、职业类别、游戏版本检索，且状态与 URL 双向同步（§14）；
//   3) 工作站 / 成本 / 结果物品链接「已收录才链接、未收录回退纯文本」（不生成死链）；
//   4) 详情页按等级/分组展示交易表，并展示等级规则。
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory, type Router } from 'vue-router'

vi.mock('@/api/public', () => ({
  listVillagers: vi.fn(),
  getVillagerTradeLevels: vi.fn(),
  getVillager: vi.fn(),
}))
vi.mock('@/api/versions', () => ({ listVersions: vi.fn() }))

import { getVillager, getVillagerTradeLevels, listVillagers } from '@/api/public'
import { listVersions } from '@/api/versions'
import VillagersView from '@/views/VillagersView.vue'
import VillagerDetailView from '@/views/VillagerDetailView.vue'
import {
  formatCostCount,
  type VillagerProfession,
  type VillagerProfessionDetail,
  type VillagerTradeLevel,
} from '@/types/villager'

const PATHS: Array<{ path: string; name?: string }> = [
  { path: '/', name: 'home' },
  { path: '/villagers', name: 'villagers' },
  { path: '/villagers/:slug', name: 'villager-detail' },
  { path: '/items/:itemId', name: 'item-detail' },
]

function makeRouter(): Router {
  return createRouter({
    history: createMemoryHistory(),
    routes: PATHS.map((r) => ({
      path: r.path,
      name: r.name,
      component: { template: '<div />' },
    })),
  })
}

function makeProfession(overrides: Partial<VillagerProfession> = {}): VillagerProfession {
  return {
    id: 'v1',
    slug: 'armorer',
    name_zh: '盔甲匠',
    name_en: 'Armorer',
    aliases: ['护甲匠'],
    kind: 'normal',
    workstation_item_id: 'minecraft:blast_furnace',
    workstation_name_zh: '高炉',
    summary: '收购煤炭与矿物，出售盔甲。',
    description: '盔甲匠的工作站是高炉。',
    trade_note: '价格会随交易次数上涨。',
    game_edition: 'java',
    supported_versions: ['1.14+'],
    source_url: 'https://minecraft.wiki/w/Trading',
    data_version: 'wiki-2026-10',
    last_verified_at: '2026-10-18T00:00:00Z',
    sort_order: 1,
    created_at: '2026-10-18T00:00:00Z',
    updated_at: '2026-10-18T00:00:00Z',
    ...overrides,
  }
}

const LEVELS: VillagerTradeLevel[] = [
  { id: 'l1', level: 1, slug: 'novice', name_zh: '新手', name_en: 'Novice', required_experience: 0, badge_note: '初始等级。' },
  { id: 'l2', level: 2, slug: 'apprentice', name_zh: '学徒', name_en: 'Apprentice', required_experience: 10, badge_note: '累计 10 点经验。' },
]

beforeEach(() => {
  vi.clearAllMocks()
  setActivePinia(createPinia())
  ;(listVersions as ReturnType<typeof vi.fn>).mockResolvedValue({ versions: [{ id: '1.21' }, { id: '1.20' }] })
})

async function push(router: Router, fullPath: string) {
  await router.push(fullPath)
  await router.isReady()
}

describe('§9B 村民交易展示辅助', () => {
  it('数量区间：固定数量显示单值，区间显示 min–max', () => {
    expect(formatCostCount({ item_id: 'minecraft:coal', name_zh: '煤炭', count_min: 15, count_max: 15 })).toBe('15')
    expect(formatCostCount({ item_id: 'minecraft:emerald', name_zh: '绿宝石', count_min: 19, count_max: 33 })).toBe('19–33')
  })
})

describe('§9B 村民交易列表页', () => {
  it('从分享链接还原关键词 / 类别 / 版本 / 页码并据此查询', async () => {
    ;(listVillagers as ReturnType<typeof vi.fn>).mockResolvedValue({
      items: [makeProfession()],
      total: 1,
      totalPages: 1,
    })
    ;(getVillagerTradeLevels as ReturnType<typeof vi.fn>).mockResolvedValue(LEVELS)
    const router = makeRouter()
    await push(router, '/villagers?q=盔甲&kind=normal&version=1.20&page=2')

    const wrapper = mount(VillagersView, { global: { plugins: [router] } })
    await flushPromises()

    expect(listVillagers).toHaveBeenCalledWith({
      q: '盔甲',
      kind: 'normal',
      version: '1.20',
      page: 2,
      page_size: 15,
    })
    expect((wrapper.find('input[type="search"]').element as HTMLInputElement).value).toBe('盔甲')
    expect(wrapper.text()).toContain('盔甲匠')
    expect(wrapper.text()).toContain('高炉')
    // 等级规则来自后端，不硬编码
    expect(wrapper.text()).toContain('新手')
    expect(wrapper.text()).toContain('学徒')
  })

  it('修改筛选后 URL 更新且保留关键词，页码回到第 1 页', async () => {
    ;(listVillagers as ReturnType<typeof vi.fn>).mockResolvedValue({ items: [], total: 0, totalPages: 1 })
    ;(getVillagerTradeLevels as ReturnType<typeof vi.fn>).mockResolvedValue(LEVELS)
    const router = makeRouter()
    await push(router, '/villagers?q=盔甲&page=3')

    const wrapper = mount(VillagersView, { global: { plugins: [router] } })
    await flushPromises()

    const wandering = wrapper.findAll('button').find((b) => b.text() === '流浪商人')
    expect(wandering).toBeTruthy()
    await wandering!.trigger('click')
    await flushPromises()

    expect(router.currentRoute.value.query).toMatchObject({ q: '盔甲', kind: 'wandering' })
    expect(router.currentRoute.value.query.page).toBeUndefined()
  })

  it('清除全部筛选后链接保持干净（无冗余参数）', async () => {
    ;(listVillagers as ReturnType<typeof vi.fn>).mockResolvedValue({ items: [], total: 0, totalPages: 1 })
    ;(getVillagerTradeLevels as ReturnType<typeof vi.fn>).mockResolvedValue(LEVELS)
    const router = makeRouter()
    await push(router, '/villagers?q=盔甲&kind=normal&version=1.20')

    const wrapper = mount(VillagersView, { global: { plugins: [router] } })
    await flushPromises()

    const clear = wrapper.findAll('button').find((b) => b.text().includes('清除全部筛选'))
    await clear!.trigger('click')
    await flushPromises()

    expect(router.currentRoute.value.query).toEqual({})
  })
})

describe('§9B 村民职业详情页', () => {
  const detail: VillagerProfessionDetail = {
    ...makeProfession(),
    levels: LEVELS,
    workstation_item: {
      item_id: 'minecraft:blast_furnace',
      name_zh: '高炉',
      name_en: 'Blast Furnace',
      item_category: 'block',
      url: '/items/minecraft:blast_furnace',
    },
    groups: [
      {
        level: 1,
        level_slug: 'novice',
        level_name_zh: '新手',
        required_experience: 0,
        trade_group: 'level',
        trades: [
          {
            id: 't1',
            trade_key: 'armorer_l1_coal',
            profession_slug: 'armorer',
            level: 1,
            trade_group: 'level',
            cost_items: [{ item_id: 'minecraft:coal', name_zh: '煤炭', count_min: 15, count_max: 15 }],
            result_item_id: 'minecraft:emerald',
            result_item_name_zh: '绿宝石',
            result_count: 1,
            max_uses: 16,
            villager_experience: 2,
            price_multiplier: 0.05,
            variant_note: '',
            note: '',
            game_edition: 'java',
            supported_versions: ['1.14+'],
            source_url: 'https://minecraft.wiki/w/Trading',
            sort_order: 1,
            // 煤炭已收录，绿宝石未收录
            cost_item_refs: {
              'minecraft:coal': {
                item_id: 'minecraft:coal',
                name_zh: '煤炭',
                name_en: 'Coal',
                item_category: 'material',
                url: '/items/minecraft:coal',
              },
            },
          },
          {
            id: 't2',
            trade_key: 'armorer_l5_diamond_chestplate',
            profession_slug: 'armorer',
            level: 5,
            trade_group: 'level',
            cost_items: [{ item_id: 'minecraft:emerald', name_zh: '绿宝石', count_min: 21, count_max: 35 }],
            result_item_id: 'minecraft:diamond_chestplate',
            result_item_name_zh: '钻石胸甲',
            result_count: 1,
            max_uses: 3,
            villager_experience: 30,
            price_multiplier: 0.2,
            variant_note: '',
            note: '附魔随机',
            game_edition: 'java',
            supported_versions: ['1.14+'],
            source_url: 'https://minecraft.wiki/w/Trading',
            sort_order: 18,
            cost_item_refs: {},
          },
        ],
      },
    ],
    trades: [],
    resolved_items: {
      'minecraft:coal': {
        item_id: 'minecraft:coal',
        name_zh: '煤炭',
        name_en: 'Coal',
        item_category: 'material',
        url: '/items/minecraft:coal',
      },
      'minecraft:blast_furnace': {
        item_id: 'minecraft:blast_furnace',
        name_zh: '高炉',
        name_en: 'Blast Furnace',
        item_category: 'block',
        url: '/items/minecraft:blast_furnace',
      },
    },
  }

  it('展示工作站链接、等级规则与按等级分组的交易表', async () => {
    ;(getVillager as ReturnType<typeof vi.fn>).mockResolvedValue(detail)
    const router = makeRouter()
    await push(router, '/villagers/armorer')

    const wrapper = mount(VillagerDetailView, { global: { plugins: [router] } })
    await flushPromises()

    expect(getVillager).toHaveBeenCalledWith('armorer')
    expect(wrapper.text()).toContain('工作站')
    expect(wrapper.text()).toContain('高炉')
    // 等级分组标题带经验门槛
    expect(wrapper.text()).toContain('新手（0 经验）')
    // 交易：村民想要 → 玩家获得
    expect(wrapper.text()).toContain('煤炭')
    expect(wrapper.text()).toContain('×15')
    expect(wrapper.text()).toContain('绿宝石')
    expect(wrapper.text()).toContain('钻石胸甲')
    expect(wrapper.text()).toContain('21–35')
    expect(wrapper.text()).toContain('附魔随机')

    const hrefs = wrapper.findAll('a').map((a) => a.attributes('href') ?? '')
    // 已收录的煤炭 / 高炉给出物品链接
    expect(hrefs).toContain('/items/minecraft:coal')
    expect(hrefs).toContain('/items/minecraft:blast_furnace')
  })

  it('未收录的成本 / 结果物品回退为纯文本 ID，不生成死链', async () => {
    ;(getVillager as ReturnType<typeof vi.fn>).mockResolvedValue(detail)
    const router = makeRouter()
    await push(router, '/villagers/armorer')

    const wrapper = mount(VillagerDetailView, { global: { plugins: [router] } })
    await flushPromises()

    const hrefs = wrapper.findAll('a').map((a) => a.attributes('href') ?? '')
    // 绿宝石与钻石胸甲未收录于 resolved_items，不得生成物品死链
    expect(hrefs.some((h) => h.includes('emerald'))).toBe(false)
    expect(hrefs.some((h) => h.includes('diamond_chestplate'))).toBe(false)
    // 但纯文本 ID 仍需展示
    expect(wrapper.text()).toContain('minecraft:emerald')
    expect(wrapper.text()).toContain('minecraft:diamond_chestplate')
  })
})
