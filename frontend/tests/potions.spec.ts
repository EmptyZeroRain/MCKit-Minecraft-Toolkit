// 药水与酿造（TODO §9A）测试：
//   1) 类型层展示辅助（罗马数字 / 时长 / 效果摘要）；
//   2) 列表页按名称、类别、游戏版本检索，且状态与 URL 双向同步（§14）；
//   3) 通用酿造规则的原料链接「已收录才链接、未收录回退纯文本」（不前生死链）；
//   4) 详情页展示普通/延长变体、酿造步骤与后续变化，且不生成死链。
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory, type Router } from 'vue-router'

vi.mock('@/api/public', () => ({
  listPotions: vi.fn(),
  getBrewingRules: vi.fn(),
  getPotion: vi.fn(),
}))
vi.mock('@/api/versions', () => ({ listVersions: vi.fn() }))

import { getBrewingRules, getPotion, listPotions } from '@/api/public'
import { listVersions } from '@/api/versions'
import PotionsView from '@/views/PotionsView.vue'
import PotionDetailView from '@/views/PotionDetailView.vue'
import {
  effectSummary,
  formatDuration,
  potionEffectSummary,
  romanNumeral,
  type BrewingRules,
  type Potion,
  type PotionDetail,
} from '@/types/potion'

const PATHS: Array<{ path: string; name?: string }> = [
  { path: '/', name: 'home' },
  { path: '/potions', name: 'potions' },
  { path: '/potions/:slug', name: 'potion-detail' },
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

function makePotion(overrides: Partial<Potion> = {}): Potion {
  return {
    id: 'p1',
    slug: 'potion-of-swiftness',
    name_zh: '迅捷药水',
    name_en: 'Potion of Swiftness',
    aliases: [],
    category: 'positive',
    summary: '提高移动与疾跑速度。',
    effect_note: '',
    is_brewable: true,
    is_extendable: true,
    is_enhanceable: true,
    effects: [
      {
        variant: 'normal',
        effect_id: 'minecraft:speed',
        name_zh: '迅捷',
        name_en: 'Speed',
        amplifier: 1,
        duration_seconds: 180,
        description: '移动速度 +20%。',
      },
      {
        variant: 'extended',
        effect_id: 'minecraft:speed',
        name_zh: '迅捷',
        name_en: 'Speed',
        amplifier: 1,
        duration_seconds: 480,
        description: '移动速度 +20%。',
      },
      {
        variant: 'enhanced',
        effect_id: 'minecraft:speed',
        name_zh: '迅捷',
        name_en: 'Speed',
        amplifier: 2,
        duration_seconds: 90,
        description: '移动速度 +40%。',
      },
    ],
    game_edition: 'both',
    supported_versions: ['1.0+'],
    source_url: 'https://minecraft.wiki/w/Potion_of_Swiftness',
    data_version: 'wiki-2026-10-17',
    last_verified_at: '2026-10-17T00:00:00Z',
    created_at: '2026-10-17T00:00:00Z',
    updated_at: '2026-10-17T00:00:00Z',
    ...overrides,
  }
}

const RULES: BrewingRules = {
  rules: [
    {
      id: 'r1',
      recipe_key: 'modifier:extend/minecraft:redstone',
      step_kind: 'modifier',
      input_potion_slug: null,
      ingredient_item_id: 'minecraft:redstone',
      ingredient_name_zh: '红石粉',
      result_potion_slug: null,
      note: '延长持续时间。',
      game_edition: 'both',
      supported_versions: ['1.0+'],
      source_url: 'https://minecraft.wiki/w/Brewing',
      sort_order: 310,
      input_potion: null,
      result_potion: null,
    },
    {
      id: 'r2',
      recipe_key: 'form:splash/minecraft:gunpowder',
      step_kind: 'form',
      input_potion_slug: null,
      ingredient_item_id: 'minecraft:gunpowder',
      ingredient_name_zh: '火药',
      result_potion_slug: null,
      note: '变为喷溅药水。',
      game_edition: 'both',
      supported_versions: ['1.0+'],
      source_url: 'https://minecraft.wiki/w/Brewing',
      sort_order: 320,
      input_potion: null,
      result_potion: null,
    },
  ],
  // 仅 redstone 已收录；gunpowder 未收录 → 必须回退为纯文本
  ingredient_items: {
    'minecraft:redstone': {
      item_id: 'minecraft:redstone',
      name_zh: '红石粉',
      name_en: 'Redstone Dust',
      item_category: 'material',
      url: '/items/minecraft:redstone',
    },
  },
}

beforeEach(() => {
  vi.clearAllMocks()
  setActivePinia(createPinia())
  ;(listVersions as ReturnType<typeof vi.fn>).mockResolvedValue({ versions: [{ id: '1.21' }, { id: '1.20' }] })
})

async function push(router: Router, fullPath: string) {
  await router.push(fullPath)
  await router.isReady()
}

describe('§9A 药水展示辅助', () => {
  it('罗马数字在常规范围内转换，越界回退为「N 级」', () => {
    expect(romanNumeral(1)).toBe('I')
    expect(romanNumeral(2)).toBe('II')
    expect(romanNumeral(3)).toBe('III')
    expect(romanNumeral(4)).toBe('IV')
    expect(romanNumeral(6)).toBe('VI')
    expect(romanNumeral(0)).toBe('0 级')
    expect(romanNumeral(11)).toBe('11 级')
  })

  it('持续时间为 0 时显示「瞬间」，其余为 m:ss', () => {
    expect(formatDuration(0)).toBe('瞬间')
    expect(formatDuration(90)).toBe('1:30')
    expect(formatDuration(180)).toBe('3:00')
    expect(formatDuration(480)).toBe('8:00')
  })

  it('效果摘要与药水摘要取普通变体', () => {
    const potion = makePotion()
    expect(effectSummary(potion.effects[2])).toBe('迅捷 II · 1:30')
    expect(potionEffectSummary(potion)).toBe('迅捷 I · 3:00')
    expect(potionEffectSummary(makePotion({ effects: [] }))).toBe('无效果')
  })
})

describe('§9A 药水列表页', () => {
  it('从分享链接还原关键词 / 类别 / 版本 / 页码并据此查询', async () => {
    ;(listPotions as ReturnType<typeof vi.fn>).mockResolvedValue({ items: [makePotion()], total: 1, totalPages: 1 })
    ;(getBrewingRules as ReturnType<typeof vi.fn>).mockResolvedValue(RULES)
    const router = makeRouter()
    await push(router, '/potions?q=迅捷&category=positive&version=1.20&page=2')

    const wrapper = mount(PotionsView, { global: { plugins: [router] } })
    await flushPromises()

    expect(listPotions).toHaveBeenCalledWith({
      q: '迅捷',
      category: 'positive',
      version: '1.20',
      page: 2,
      page_size: 15,
    })
    expect((wrapper.find('input[type="search"]').element as HTMLInputElement).value).toBe('迅捷')
    expect(wrapper.text()).toContain('迅捷药水')
    expect(wrapper.text()).toContain('迅捷 I · 3:00')
  })

  it('修改筛选后 URL 更新且保留关键词，页码回到第 1 页', async () => {
    ;(listPotions as ReturnType<typeof vi.fn>).mockResolvedValue({ items: [], total: 0, totalPages: 1 })
    ;(getBrewingRules as ReturnType<typeof vi.fn>).mockResolvedValue(RULES)
    const router = makeRouter()
    await push(router, '/potions?q=迅捷&page=3')

    const wrapper = mount(PotionsView, { global: { plugins: [router] } })
    await flushPromises()

    const negative = wrapper.findAll('button').find((b) => b.text() === '负面药水')
    expect(negative).toBeTruthy()
    await negative!.trigger('click')
    await flushPromises()

    expect(router.currentRoute.value.query).toMatchObject({ q: '迅捷', category: 'negative' })
    expect(router.currentRoute.value.query.page).toBeUndefined()
  })

  it('清除全部筛选后链接保持干净（无冗余参数）', async () => {
    ;(listPotions as ReturnType<typeof vi.fn>).mockResolvedValue({ items: [], total: 0, totalPages: 1 })
    ;(getBrewingRules as ReturnType<typeof vi.fn>).mockResolvedValue(RULES)
    const router = makeRouter()
    await push(router, '/potions?q=迅捷&category=positive&version=1.20')

    const wrapper = mount(PotionsView, { global: { plugins: [router] } })
    await flushPromises()

    const clear = wrapper.findAll('button').find((b) => b.text().includes('清除全部筛选'))
    await clear!.trigger('click')
    await flushPromises()

    expect(router.currentRoute.value.query).toEqual({})
  })

  it('通用酿造规则：已收录原料给出物品链接，未收录原料回退为纯文本 ID', async () => {
    ;(listPotions as ReturnType<typeof vi.fn>).mockResolvedValue({ items: [], total: 0, totalPages: 1 })
    ;(getBrewingRules as ReturnType<typeof vi.fn>).mockResolvedValue(RULES)
    const router = makeRouter()
    await push(router, '/potions')

    const wrapper = mount(PotionsView, { global: { plugins: [router] } })
    await flushPromises()

    expect(wrapper.text()).toContain('通用酿造规则')
    expect(wrapper.text()).toContain('红石粉')
    expect(wrapper.text()).toContain('火药')

    const hrefs = wrapper.findAll('a').map((a) => a.attributes('href') ?? '')
    expect(hrefs).toContain('/items/minecraft:redstone')
    // 未收录的火药不得生成任何指向物品详情页的死链
    expect(hrefs.some((h) => h.includes('gunpowder'))).toBe(false)
    expect(wrapper.text()).toContain('minecraft:gunpowder')
  })
})

describe('§9A 药水详情页', () => {
  const detail: PotionDetail = {
    ...makePotion(),
    produced_by: [
      {
        id: 's1',
        recipe_key: 'effect:potion-of-swiftness/minecraft:sugar',
        step_kind: 'effect',
        input_potion_slug: 'awkward-potion',
        ingredient_item_id: 'minecraft:sugar',
        ingredient_name_zh: '糖',
        result_potion_slug: 'potion-of-swiftness',
        note: '',
        game_edition: 'both',
        supported_versions: ['1.0+'],
        source_url: 'https://minecraft.wiki/w/Brewing',
        sort_order: 114,
        input_potion: { slug: 'awkward-potion', name_zh: '粗制的药水', category: 'base', url: '/potions/awkward-potion' },
        result_potion: { slug: 'potion-of-swiftness', name_zh: '迅捷药水', category: 'positive', url: '/potions/potion-of-swiftness' },
      },
    ],
    follow_ups: [
      {
        id: 's2',
        recipe_key: 'corrupt:potion-of-slowness/minecraft:fermented_spider_eye',
        step_kind: 'corrupt',
        input_potion_slug: 'potion-of-swiftness',
        ingredient_item_id: 'minecraft:fermented_spider_eye',
        ingredient_name_zh: '发酵蛛眼',
        result_potion_slug: 'potion-of-slowness',
        note: '迅捷的反向效果。',
        game_edition: 'both',
        supported_versions: ['1.0+'],
        source_url: 'https://minecraft.wiki/w/Brewing',
        sort_order: 213,
        input_potion: { slug: 'potion-of-swiftness', name_zh: '迅捷药水', category: 'positive', url: '/potions/potion-of-swiftness' },
        result_potion: { slug: 'potion-of-slowness', name_zh: '迟缓药水', category: 'negative', url: '/potions/potion-of-slowness' },
      },
    ],
    rules: RULES.rules,
    ingredient_items: {
      'minecraft:sugar': {
        item_id: 'minecraft:sugar',
        name_zh: '糖',
        name_en: 'Sugar',
        item_category: 'material',
        url: '/items/minecraft:sugar',
      },
      'minecraft:redstone': {
        item_id: 'minecraft:redstone',
        name_zh: '红石粉',
        name_en: 'Redstone Dust',
        item_category: 'material',
        url: '/items/minecraft:redstone',
      },
    },
  }

  it('展示普通/延长/增强三种变体与酿造步骤、后续变化', async () => {
    ;(getPotion as ReturnType<typeof vi.fn>).mockResolvedValue(detail)
    const router = makeRouter()
    await push(router, '/potions/potion-of-swiftness')

    const wrapper = mount(PotionDetailView, { global: { plugins: [router] } })
    await flushPromises()

    expect(getPotion).toHaveBeenCalledWith('potion-of-swiftness')
    expect(wrapper.text()).toContain('迅捷药水')
    expect(wrapper.text()).toContain('效果与持续时间')
    expect(wrapper.text()).toContain('延长（红石）')
    expect(wrapper.text()).toContain('增强（荧石）')
    expect(wrapper.text()).toContain('3:00')
    expect(wrapper.text()).toContain('8:00')
    expect(wrapper.text()).toContain('1:30')
    // 酿造步骤：基底药水 → 原料 → 成品
    expect(wrapper.text()).toContain('粗制的药水')
    expect(wrapper.text()).toContain('糖')
    // 后续变化：腐化为迟缓药水
    expect(wrapper.text()).toContain('后续变化')
    expect(wrapper.text()).toContain('迟缓药水')
    // 来源与核验
    expect(wrapper.text()).toContain('来源与核验')
    expect(wrapper.text()).toContain('Minecraft Wiki')
  })

  it('未收录原料只展示纯文本 ID，不生成任何死链', async () => {
    ;(getPotion as ReturnType<typeof vi.fn>).mockResolvedValue(detail)
    const router = makeRouter()
    await push(router, '/potions/potion-of-swiftness')

    const wrapper = mount(PotionDetailView, { global: { plugins: [router] } })
    await flushPromises()

    const hrefs = wrapper.findAll('a').map((a) => a.attributes('href') ?? '')
    expect(hrefs).toContain('/items/minecraft:sugar')
    expect(hrefs).toContain('/potions/awkward-potion')
    // 火药未收录，不得出现指向物品详情页的链接
    expect(hrefs.some((h) => h.includes('gunpowder'))).toBe(false)
  })

  it('未收录药水时给出明确空态，而非空白页', async () => {
    ;(getPotion as ReturnType<typeof vi.fn>).mockRejectedValue(new Error('数据不存在'))
    const router = makeRouter()
    await push(router, '/potions/not-exist')

    const wrapper = mount(PotionDetailView, { global: { plugins: [router] } })
    await flushPromises()

    expect(wrapper.text()).toContain('加载失败')
  })
})
