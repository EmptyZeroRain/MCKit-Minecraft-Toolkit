// 红石与机器设计资料库（TODO §15）测试：
//   1) 类型层展示辅助（材料数量区间 / 材料文案 / 外链判定）；
//   2) 列表页双 tab：元件按类别检索、设计按用途与难度检索，状态与 URL 双向同步（§14）；
//   3) 详情页元件 / 设计共用组件按 kind 分支渲染；
//   4) 材料「已收录才链接、未收录回退纯文本」（不生成死链）；
//   5) 版权红线：无授权存档不给下载链接。
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory, type Router } from 'vue-router'

vi.mock('@/api/public', () => ({
  listRedstoneComponents: vi.fn(),
  listRedstoneDesigns: vi.fn(),
  getRedstoneComponent: vi.fn(),
  getRedstoneDesign: vi.fn(),
}))
vi.mock('@/api/versions', () => ({ listVersions: vi.fn() }))

import {
  getRedstoneComponent,
  getRedstoneDesign,
  listRedstoneComponents,
  listRedstoneDesigns,
} from '@/api/public'
import { listVersions } from '@/api/versions'
import RedstoneView from '@/views/RedstoneView.vue'
import RedstoneDetailView from '@/views/RedstoneDetailView.vue'
import {
  formatMaterialCount,
  formatMaterialLabel,
  isExternalUrl,
  type RedstoneComponent,
  type RedstoneDesign,
  type RedstoneDesignDetail,
} from '@/types/redstone'
import { resetVersionCache } from '@/utils/mc-versions'

const PATHS: Array<{ path: string; name?: string }> = [
  { path: '/', name: 'home' },
  { path: '/redstone', name: 'redstone' },
  { path: '/redstone/components/:slug', name: 'redstone-component-detail' },
  { path: '/redstone/designs/:slug', name: 'redstone-design-detail' },
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

function makeComponent(overrides: Partial<RedstoneComponent> = {}): RedstoneComponent {
  return {
    id: 'c1',
    slug: 'redstone_dust',
    name_zh: '红石粉',
    name_en: 'Redstone Dust',
    aliases: ['红石线'],
    category: 'transmission',
    mechanism_note: '放置后可承载并传导红石信号，信号强度随距离逐格衰减。',
    signal_note: '承载 0–15 的信号强度。',
    summary: '最基础的红石传输元件。',
    game_edition: 'java',
    supported_versions: ['1.0+'],
    source_url: 'https://minecraft.wiki/w/Redstone_Dust',
    data_version: 'wiki-2026-10',
    last_verified_at: '2026-10-18T00:00:00Z',
    sort_order: 1,
    created_at: '2026-10-18T00:00:00Z',
    updated_at: '2026-10-18T00:00:00Z',
    ...overrides,
  }
}

function makeDesign(overrides: Partial<RedstoneDesign> = {}): RedstoneDesign {
  return {
    id: 'd1',
    slug: 'sugar_cane_farm',
    name_zh: '甘蔗农场',
    name_en: 'Sugar Cane Farm',
    aliases: [],
    purpose: 'farm',
    difficulty: 'easy',
    summary: '利用侦测器驱动活塞收割的自动甘蔗农场。',
    steps_md: '1. 沿水渠种满甘蔗。\n2. 放置侦测器与活塞。',
    materials: [
      { item_id: 'minecraft:cobblestone', name_zh: '圆石', count_min: 16, count_max: 64 },
      { item_id: '', name_zh: '活塞', count_min: 2, count_max: 8 },
    ],
    author: '本站整理',
    author_url: null,
    screenshot_url: null,
    video_url: null,
    world_download_url: null,
    world_download_note: '未提供存档下载，请按步骤自行搭建。',
    license: 'CC BY-NC-SA 3.0',
    license_note: '整理自 Minecraft Wiki，按其许可署名。',
    supported_versions: ['1.11+'],
    game_edition: 'java',
    source_url: 'https://minecraft.wiki/w/Tutorials/Sugar_cane_farming',
    data_version: 'wiki-2026-10',
    last_verified_at: '2026-10-18T00:00:00Z',
    review_status: 'published',
    reviewer: 'seed',
    reviewed_at: '2026-10-18T00:00:00Z',
    review_note: '种子数据。',
    sort_order: 1,
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

describe('§15 红石资料展示辅助', () => {
  it('材料数量区间：固定数量显示单值，区间显示 min–max', () => {
    expect(formatMaterialCount(4, 4)).toBe('4')
    expect(formatMaterialCount(16, 64)).toBe('16–64')
  })

  it('材料文案优先中文名，回退 item_id', () => {
    expect(formatMaterialLabel({ item_id: '', name_zh: '活塞', count_min: 2, count_max: 8 })).toBe('活塞 ×2–8')
    expect(
      formatMaterialLabel({ item_id: 'minecraft:stone', name_zh: '', count_min: 1, count_max: 1 }),
    ).toBe('minecraft:stone ×1')
  })

  it('外链判定只接受 http(s)，避免生成无效链接', () => {
    expect(isExternalUrl('https://minecraft.wiki/w/Lever')).toBe(true)
    expect(isExternalUrl('')).toBe(false)
    expect(isExternalUrl(null)).toBe(false)
    expect(isExternalUrl('javascript:alert(1)')).toBe(false)
  })
})

describe('§15 红石资料列表页', () => {
  it('元件 tab：从分享链接还原关键词 / 类别 / 版本 / 页码并据此查询', async () => {
    ;(listRedstoneComponents as ReturnType<typeof vi.fn>).mockResolvedValue({
      items: [makeComponent()],
      total: 1,
      totalPages: 1,
    })
    const router = makeRouter()
    await push(router, '/redstone?q=红石&category=transmission&version=1.20&page=2')

    const wrapper = mount(RedstoneView, { global: { plugins: [router] } })
    await flushPromises()

    expect(listRedstoneComponents).toHaveBeenCalledWith({
      q: '红石',
      category: 'transmission',
      version: '1.20',
      page: 2,
      page_size: 12,
    })
    expect((wrapper.find('input[type="search"]').element as HTMLInputElement).value).toBe('红石')
    expect(wrapper.text()).toContain('红石粉')
    expect(wrapper.text()).toContain('传输')
  })

  it('设计 tab：从 URL 的 tab 参数进入，按用途与难度查询', async () => {
    ;(listRedstoneDesigns as ReturnType<typeof vi.fn>).mockResolvedValue({
      items: [makeDesign()],
      total: 1,
      totalPages: 1,
    })
    const router = makeRouter()
    await push(router, '/redstone?tab=designs&q=甘蔗&purpose=farm&difficulty=easy&version=1.20')

    const wrapper = mount(RedstoneView, { global: { plugins: [router] } })
    await flushPromises()

    expect(listRedstoneDesigns).toHaveBeenCalledWith({
      q: '甘蔗',
      purpose: 'farm',
      difficulty: 'easy',
      version: '1.20',
      page: 1,
      page_size: 12,
    })
    expect(wrapper.text()).toContain('甘蔗农场')
    expect(wrapper.text()).toContain('农场')
    expect(wrapper.text()).toContain('难度：简单')
  })

  it('切换 tab 后 URL 更新、页码回到第 1 页且不残留另一 tab 的筛选', async () => {
    ;(listRedstoneComponents as ReturnType<typeof vi.fn>).mockResolvedValue({
      items: [],
      total: 0,
      totalPages: 1,
    })
    ;(listRedstoneDesigns as ReturnType<typeof vi.fn>).mockResolvedValue({
      items: [],
      total: 0,
      totalPages: 1,
    })
    const router = makeRouter()
    await push(router, '/redstone?q=红石&page=3')

    const wrapper = mount(RedstoneView, { global: { plugins: [router] } })
    await flushPromises()

    const designsTab = wrapper.findAll('button').find((b) => b.text() === '机器设计')
    expect(designsTab).toBeTruthy()
    await designsTab!.trigger('click')
    await flushPromises()

    expect(router.currentRoute.value.query).toMatchObject({ tab: 'designs', q: '红石' })
    expect(router.currentRoute.value.query.page).toBeUndefined()
  })

  it('清除全部筛选后链接保持干净（无冗余参数）', async () => {
    ;(listRedstoneComponents as ReturnType<typeof vi.fn>).mockResolvedValue({
      items: [],
      total: 0,
      totalPages: 1,
    })
    const router = makeRouter()
    await push(router, '/redstone?q=红石&category=logic&version=1.20')

    const wrapper = mount(RedstoneView, { global: { plugins: [router] } })
    await flushPromises()

    const clear = wrapper.findAll('button').find((b) => b.text().includes('清除全部筛选'))
    await clear!.trigger('click')
    await flushPromises()

    expect(router.currentRoute.value.query).toEqual({})
  })
})

describe('§15 红石元件详情页', () => {
  it('展示核心工作机制、信号说明与来源核验', async () => {
    ;(getRedstoneComponent as ReturnType<typeof vi.fn>).mockResolvedValue(makeComponent())
    const router = makeRouter()
    await push(router, '/redstone/components/redstone_dust')

    const wrapper = mount(RedstoneDetailView, {
      global: { plugins: [router] },
      props: { kind: 'component' },
    })
    await flushPromises()

    expect(getRedstoneComponent).toHaveBeenCalledWith('redstone_dust')
    expect(wrapper.text()).toContain('核心工作机制')
    expect(wrapper.text()).toContain('信号说明')
    expect(wrapper.text()).toContain('来源与核验')
    expect(wrapper.text()).toContain('红石线')
    expect(wrapper.text()).toContain('1.0+')
  })
})

describe('§15 机器设计详情页', () => {
  const detail: RedstoneDesignDetail = {
    ...makeDesign(),
    materials: [
      {
        item_id: 'minecraft:cobblestone',
        name_zh: '圆石',
        count_min: 16,
        count_max: 64,
        item_ref: {
          item_id: 'minecraft:cobblestone',
          name_zh: '圆石',
          name_en: 'Cobblestone',
          item_category: 'block',
          url: '/items/minecraft:cobblestone',
        },
      },
      {
        item_id: 'minecraft:piston',
        name_zh: '活塞',
        count_min: 2,
        count_max: 8,
        item_ref: null,
      },
    ],
    resolved_items: {
      'minecraft:cobblestone': {
        item_id: 'minecraft:cobblestone',
        name_zh: '圆石',
        name_en: 'Cobblestone',
        item_category: 'block',
        url: '/items/minecraft:cobblestone',
      },
    },
  }

  it('展示材料数量区间、搭建步骤与授权信息', async () => {
    ;(getRedstoneDesign as ReturnType<typeof vi.fn>).mockResolvedValue(detail)
    const router = makeRouter()
    await push(router, '/redstone/designs/sugar_cane_farm')

    const wrapper = mount(RedstoneDetailView, {
      global: { plugins: [router] },
      props: { kind: 'design' },
    })
    await flushPromises()

    expect(getRedstoneDesign).toHaveBeenCalledWith('sugar_cane_farm')
    expect(wrapper.text()).toContain('材料清单')
    expect(wrapper.text()).toContain('×16–64')
    expect(wrapper.text()).toContain('搭建步骤')
    expect(wrapper.text()).toContain('沿水渠种满甘蔗')
    expect(wrapper.text()).toContain('CC BY-NC-SA 3.0')
    expect(wrapper.text()).toContain('已发布')
  })

  it('未收录材料回退为纯文本，不生成死链', async () => {
    ;(getRedstoneDesign as ReturnType<typeof vi.fn>).mockResolvedValue(detail)
    const router = makeRouter()
    await push(router, '/redstone/designs/sugar_cane_farm')

    const wrapper = mount(RedstoneDetailView, {
      global: { plugins: [router] },
      props: { kind: 'design' },
    })
    await flushPromises()

    const hrefs = wrapper.findAll('a').map((a) => a.attributes('href') ?? '')
    // 已收录的圆石给出物品链接
    expect(hrefs).toContain('/items/minecraft:cobblestone')
    // 活塞未收录，不得生成物品死链，但需展示纯文本 ID
    expect(hrefs.some((h) => h.includes('piston'))).toBe(false)
    expect(wrapper.text()).toContain('minecraft:piston')
  })

  it('版权红线：未授权存档不展示下载链接', async () => {
    ;(getRedstoneDesign as ReturnType<typeof vi.fn>).mockResolvedValue(detail)
    const router = makeRouter()
    await push(router, '/redstone/designs/sugar_cane_farm')

    const wrapper = mount(RedstoneDetailView, {
      global: { plugins: [router] },
      props: { kind: 'design' },
    })
    await flushPromises()

    expect(wrapper.text()).not.toContain('前往下载')
    expect(wrapper.text()).toContain('未提供存档下载，请按步骤自行搭建。')
  })
})
