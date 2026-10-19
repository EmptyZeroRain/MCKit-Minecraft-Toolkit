// 生物资料（TODO §9C）测试：
//   1) 类型层展示辅助（生命值 / 数量区间 / 概率格式化）；
//   2) 列表页按名称、生物类别、游戏版本检索，且状态与 URL 双向同步（§14）；
//   3) 掉落物链接「已收录才链接、未收录回退纯文本 ID」（不生成死链）；
//   4) 详情页展示掉落数量区间、概率、生成条件与驯服繁殖说明。
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory, type Router } from 'vue-router'

vi.mock('@/api/public', () => ({
  listMobs: vi.fn(),
  getMob: vi.fn(),
}))
vi.mock('@/api/versions', () => ({ listVersions: vi.fn() }))

import { getMob, listMobs } from '@/api/public'
import { listVersions } from '@/api/versions'
import MobsView from '@/views/MobsView.vue'
import MobDetailView from '@/views/MobDetailView.vue'
import {
  formatChance,
  formatCount,
  formatHealth,
  type Mob,
  type MobDetail,
} from '@/types/mob'
import { resetVersionCache } from '@/utils/mc-versions'

const PATHS: Array<{ path: string; name?: string }> = [
  { path: '/', name: 'home' },
  { path: '/mobs', name: 'mobs' },
  { path: '/mobs/:slug', name: 'mob-detail' },
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

function makeMob(overrides: Partial<Mob> = {}): Mob {
  return {
    id: 'm1',
    slug: 'zombie',
    name_zh: '僵尸',
    name_en: 'Zombie',
    aliases: [],
    category: 'hostile',
    health: 20,
    attack_note: '近战攻击；困难难度下会破门。',
    behavior_note: '敌对型，追踪玩家与村民。',
    spawn_note: '生成于亮度等级 ≤ 7 的区域。',
    taming_note: '',
    breeding_note: '',
    dimensions: ['overworld'],
    biomes_note: '任意低亮度区域。',
    summary: '最常见的敌对生物，掉落腐肉。',
    game_edition: 'java',
    supported_versions: ['1.0+'],
    source_url: 'https://minecraft.wiki/w/Zombie',
    data_version: 'wiki-2026-10',
    last_verified_at: '2026-10-18T00:00:00Z',
    sort_order: 30,
    created_at: '2026-10-18T00:00:00Z',
    updated_at: '2026-10-18T00:00:00Z',
    ...overrides,
  }
}

beforeEach(() => {
  vi.clearAllMocks()
  setActivePinia(createPinia())
  resetVersionCache()
  ;(listVersions as ReturnType<typeof vi.fn>).mockResolvedValue({
    versions: [{ id: '1.21' }, { id: '1.20' }],
  })
})

async function push(router: Router, fullPath: string) {
  await router.push(fullPath)
  await router.isReady()
}

describe('§9C 生物资料展示辅助', () => {
  it('生命值换算为心数，未知显示占位符', () => {
    expect(formatHealth(20)).toBe('20（10 颗心）')
    expect(formatHealth(30)).toBe('30（15 颗心）')
    expect(formatHealth(null)).toBe('—')
  })

  it('数量区间：固定数量显示单值，区间显示 min–max', () => {
    expect(formatCount(1, 1)).toBe('1')
    expect(formatCount(1, 3)).toBe('1–3')
  })

  it('概率：1 显示「必定」，其余显示百分比', () => {
    expect(formatChance(1)).toBe('必定')
    expect(formatChance(0.5)).toBe('50%')
    expect(formatChance(0.025)).toBe('2.5%')
  })
})

describe('§9C 生物列表页', () => {
  it('从分享链接还原关键词 / 类别 / 版本 / 页码并据此查询', async () => {
    ;(listMobs as ReturnType<typeof vi.fn>).mockResolvedValue({
      items: [makeMob()],
      total: 1,
      totalPages: 1,
    })
    const router = makeRouter()
    await push(router, '/mobs?q=僵尸&category=hostile&version=1.20&page=2')

    const wrapper = mount(MobsView, { global: { plugins: [router] } })
    await flushPromises()

    expect(listMobs).toHaveBeenCalledWith({
      q: '僵尸',
      category: 'hostile',
      version: '1.20',
      page: 2,
      page_size: 12,
    })
    expect((wrapper.find('input[type="search"]').element as HTMLInputElement).value).toBe('僵尸')
    expect(wrapper.text()).toContain('僵尸')
    // 类别与维度徽章来自后端契约枚举
    expect(wrapper.text()).toContain('敌对型')
    expect(wrapper.text()).toContain('主世界')
    expect(wrapper.text()).toContain('20（10 颗心）')
  })

  it('修改类别筛选后 URL 更新且保留关键词，页码回到第 1 页', async () => {
    ;(listMobs as ReturnType<typeof vi.fn>).mockResolvedValue({ items: [], total: 0, totalPages: 1 })
    const router = makeRouter()
    await push(router, '/mobs?q=僵尸&page=3')

    const wrapper = mount(MobsView, { global: { plugins: [router] } })
    await flushPromises()

    const boss = wrapper.findAll('button').find((b) => b.text() === 'Boss')
    expect(boss).toBeTruthy()
    await boss!.trigger('click')
    await flushPromises()

    expect(router.currentRoute.value.query).toMatchObject({ q: '僵尸', category: 'boss' })
    expect(router.currentRoute.value.query.page).toBeUndefined()
  })

  it('清除全部筛选后链接保持干净（无冗余参数）', async () => {
    ;(listMobs as ReturnType<typeof vi.fn>).mockResolvedValue({ items: [], total: 0, totalPages: 1 })
    const router = makeRouter()
    await push(router, '/mobs?q=僵尸&category=hostile&version=1.20')

    const wrapper = mount(MobsView, { global: { plugins: [router] } })
    await flushPromises()

    const clear = wrapper.findAll('button').find((b) => b.text().includes('清除全部筛选'))
    await clear!.trigger('click')
    await flushPromises()

    expect(router.currentRoute.value.query).toEqual({})
  })
})

describe('§9C 生物详情页', () => {
  const detail: MobDetail = {
    ...makeMob({ taming_note: '无法驯服。', breeding_note: '' }),
    drops: [
      {
        id: 'd1',
        drop_key: 'zombie:rotten_flesh',
        mob_slug: 'zombie',
        item_id: 'minecraft:rotten_flesh',
        item_name_zh: '腐肉',
        count_min: 0,
        count_max: 2,
        chance: 1,
        conditions: '',
        note: '',
        sort_order: 1,
        item_ref: {
          item_id: 'minecraft:rotten_flesh',
          name_zh: '腐肉',
          name_en: 'Rotten Flesh',
          item_category: 'food',
          url: '/items/minecraft:rotten_flesh',
        },
      },
      {
        id: 'd2',
        drop_key: 'zombie:iron_ingot',
        mob_slug: 'zombie',
        item_id: 'minecraft:iron_ingot',
        item_name_zh: '铁锭',
        count_min: 0,
        count_max: 1,
        chance: 0.025,
        conditions: '',
        note: '小概率掉落。',
        sort_order: 2,
        item_ref: null,
      },
    ],
    resolved_items: {
      'minecraft:rotten_flesh': {
        item_id: 'minecraft:rotten_flesh',
        name_zh: '腐肉',
        name_en: 'Rotten Flesh',
        item_category: 'food',
        url: '/items/minecraft:rotten_flesh',
      },
    },
  }

  it('展示基础属性、生成条件与掉落（数量区间 / 概率 / 说明）', async () => {
    ;(getMob as ReturnType<typeof vi.fn>).mockResolvedValue(detail)
    const router = makeRouter()
    await push(router, '/mobs/zombie')

    const wrapper = mount(MobDetailView, { global: { plugins: [router] } })
    await flushPromises()

    expect(getMob).toHaveBeenCalledWith('zombie')
    expect(wrapper.text()).toContain('基础属性')
    expect(wrapper.text()).toContain('20（10 颗心）')
    expect(wrapper.text()).toContain('生成条件')
    expect(wrapper.text()).toContain('生成于亮度等级 ≤ 7 的区域。')
    // 掉落：数量区间、必定 / 百分比、备注
    expect(wrapper.text()).toContain('腐肉')
    expect(wrapper.text()).toContain('0–2')
    expect(wrapper.text()).toContain('必定')
    expect(wrapper.text()).toContain('2.5%')
    expect(wrapper.text()).toContain('小概率掉落。')
    expect(wrapper.text()).toContain('驯服与繁殖')
  })

  it('未收录的掉落物回退为纯文本 ID，不生成死链', async () => {
    ;(getMob as ReturnType<typeof vi.fn>).mockResolvedValue(detail)
    const router = makeRouter()
    await push(router, '/mobs/zombie')

    const wrapper = mount(MobDetailView, { global: { plugins: [router] } })
    await flushPromises()

    const hrefs = wrapper.findAll('a').map((a) => a.attributes('href') ?? '')
    // 已收录的腐肉给出物品链接
    expect(hrefs).toContain('/items/minecraft:rotten_flesh')
    // 铁锭未收录，不得生成物品死链，但需展示纯文本 ID
    expect(hrefs.some((h) => h.includes('iron_ingot'))).toBe(false)
    expect(wrapper.text()).toContain('minecraft:iron_ingot')
  })
})
