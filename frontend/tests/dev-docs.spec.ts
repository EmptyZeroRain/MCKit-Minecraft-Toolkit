// 我的世界开发栏目（TODO §21）测试：
//   1) 类型层展示辅助（平台 / 主题 / 接口类型标签、版本声明展示、映射名字回退）；
//   2) 开发文档列表页按名称 / 平台 / 主题 / 版本检索，状态与 URL 双向同步（§14）；
//   3) 开发文档详情页展示接口速查、正文、来源与校验信息、相关条目，失败给出错误态；
//   4) 映射查询页在「未导入数据集」时给出可执行的导入指引（不伪造示例数据），
//      已导入时按类聚合并可展开查看成员明细。
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory, type Router } from 'vue-router'

vi.mock('@/api/dev', () => ({
  listDevDocs: vi.fn(),
  getDevDocFacets: vi.fn(),
  getDevDoc: vi.fn(),
  listMappingSets: vi.fn(),
  searchMappings: vi.fn(),
  getMappingClass: vi.fn(),
}))
vi.mock('@/api/versions', () => ({ listVersions: vi.fn() }))

import {
  getDevDoc,
  getDevDocFacets,
  getMappingClass,
  listDevDocs,
  listMappingSets,
  searchMappings,
} from '@/api/dev'
import { listVersions } from '@/api/versions'
import DevDocsView from '@/views/DevDocsView.vue'
import DevDocDetailView from '@/views/DevDocDetailView.vue'
import MappingsView from '@/views/MappingsView.vue'
import {
  apiEntryText,
  devApiKindLabel,
  devCategoryLabel,
  devLoaderLabel,
  type DevDocDetail,
  type DevDocSummary,
} from '@/types/devdoc'
import {
  displayName,
  mappingMemberKindLabel,
  mappingTypeLabel,
  shortClassName,
  type MappingClassHit,
  type MappingSet,
} from '@/types/mapping'
import { formatVersionLabel, formatVersionList } from '@/utils/format'

const PATHS: Array<{ path: string; name?: string }> = [
  { path: '/', name: 'home' },
  { path: '/dev', name: 'dev-docs' },
  { path: '/dev/:slug', name: 'dev-doc-detail' },
  { path: '/mappings', name: 'mappings' },
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

async function push(router: Router, fullPath: string) {
  await router.push(fullPath)
  await router.isReady()
}

function makeSummary(overrides: Partial<DevDocSummary> = {}): DevDocSummary {
  return {
    slug: 'neoforge-events',
    title_zh: 'NeoForge 事件系统与生命周期',
    title_en: 'Events and Lifecycle in NeoForge',
    loader: 'neoforge',
    category: 'events',
    summary: '讲清 NeoForge 双事件总线、监听方式与生命周期阶段。',
    mc_versions: ['1.20.1+'],
    loaders: ['neoforge'],
    tags: ['事件', '总线'],
    api_entry_count: 6,
    official_url: 'https://docs.neoforged.net/docs/concepts/events',
    sort_order: 20,
    last_verified_at: '2026-10-18T00:00:00Z',
    ...overrides,
  }
}

function makeDetail(): DevDocDetail {
  return {
    doc: {
      ...makeSummary(),
      id: 'd1',
      body_md: '## 两条总线\n\n模组总线与游戏总线。',
      api_entries: [
        {
          name: 'net.neoforged.neoforge.common.NeoForge.EVENT_BUS',
          kind: 'field',
          signature: 'NeoForge.EVENT_BUS',
          note: '游戏总线单例。',
        },
        {
          name: 'net.neoforged.bus.api.SubscribeEvent',
          kind: 'class',
          signature: '@SubscribeEvent',
          note: '标注事件监听方法。',
        },
      ],
      source_url: 'https://github.com/neoforged/Documentation/blob/main/docs/concepts/events.md',
      status: 'published',
      data_version: 'docs-main-2026-10-18',
      created_at: '2026-10-18T00:00:00Z',
      updated_at: '2026-10-18T00:00:00Z',
    },
    related: [makeSummary({ slug: 'neoforge-registries', title_zh: 'NeoForge 注册表与 DeferredRegister', category: 'registry' })],
  }
}

const SET: MappingSet = {
  id: 'set-1',
  set_key: 'yarn-1.21.1-1.21.1.build.3',
  mc_version: '1.21.1',
  mapping_type: 'yarn',
  channel: 'fabric',
  build_label: '1.21.1+build.3',
  class_count: 1200,
  member_count: 30000,
  source_url: 'https://meta.fabricmc.net/v2/versions/yarn',
  license: 'CC0',
  note: '各名字列对应的命名空间：obf 列 = official，mid 列 = intermediary，named 列 = named（未提供对应命名空间的列为空）。',
  imported_at: '2026-10-18T00:00:00Z',
  created_at: '2026-10-18T00:00:00Z',
  updated_at: '2026-10-18T00:00:00Z',
}

const HIT: MappingClassHit = {
  id: 'class-1',
  set_id: 'set-1',
  set_key: 'yarn-1.21.1-1.21.1.build.3',
  mc_version: '1.21.1',
  mapping_type: 'yarn',
  obf_name: 'eza',
  mid_name: 'net/minecraft/class_1297',
  named_name: 'net/minecraft/entity/Entity',
  package: 'net/minecraft/entity',
  member_count: 120,
  matched_members: [
    {
      kind: 'method',
      obf_name: 'a',
      mid_name: 'method_5773',
      named_name: 'tick',
      obf_desc: '()V',
      named_desc: '()V',
      side: '',
    },
  ],
  matched_total: 1,
}

beforeEach(() => {
  vi.clearAllMocks()
  setActivePinia(createPinia())
  ;(listVersions as ReturnType<typeof vi.fn>).mockResolvedValue({
    versions: [{ id: '1.21' }, { id: '1.20' }],
  })
  ;(getDevDocFacets as ReturnType<typeof vi.fn>).mockResolvedValue({
    loaders: [
      { value: 'neoforge', count: 11 },
      { value: 'fabric', count: 9 },
    ],
    categories: [{ value: 'events', count: 4 }],
    total: 40,
  })
})

describe('§21 开发栏目展示辅助', () => {
  it('平台 / 主题 / 接口类型标签与后端枚举一致，未知取值回退原样', () => {
    expect(devLoaderLabel('neoforge')).toBe('NeoForge')
    expect(devLoaderLabel('vanilla')).toBe('原版数据')
    expect(devLoaderLabel('unknown')).toBe('unknown')
    expect(devCategoryLabel('getting_started')).toBe('入门与结构')
    expect(devCategoryLabel('mapping')).toBe('映射与混淆')
    expect(devApiKindLabel('event')).toBe('事件')
    expect(devApiKindLabel('command')).toBe('命令')
  })

  it('接口速查文本包含名称、签名与说明', () => {
    expect(
      apiEntryText({ name: 'X#y', kind: 'method', signature: 'y()', note: '示例' }),
    ).toBe('X#y — y()（示例）')
    expect(apiEntryText({ name: 'X', kind: 'class', signature: '', note: '' })).toBe('X')
  })

  it('版本声明展示把 all 转为「全版本」，列表用 / 连接', () => {
    expect(formatVersionLabel('all')).toBe('全版本')
    expect(formatVersionLabel('1.20.1+')).toBe('1.20.1+')
    expect(formatVersionList(['all'])).toBe('全版本')
    expect(formatVersionList(['1.20.1+', '1.14+', '1.13+'], 2)).toBe('1.20.1+ / 1.14+')
    expect(formatVersionList([])).toBe('—')
  })

  it('映射名字回退顺序为 named > mid > obf，短名去掉包路径', () => {
    expect(displayName({ named_name: 'a', mid_name: 'b', obf_name: 'c' })).toBe('a')
    expect(displayName({ named_name: '', mid_name: 'b', obf_name: 'c' })).toBe('b')
    expect(displayName({ named_name: '', mid_name: '', obf_name: 'c' })).toBe('c')
    expect(displayName({})).toBe('—')
    expect(shortClassName('net/minecraft/entity/Entity')).toBe('Entity')
    expect(shortClassName('net.minecraft.entity.Entity')).toBe('Entity')
    expect(mappingTypeLabel('yarn')).toBe('Fabric Yarn')
    expect(mappingMemberKindLabel('field')).toBe('字段')
  })
})

describe('§21 开发文档列表页', () => {
  it('从分享链接还原关键词 / 平台 / 主题 / 版本 / 页码并据此查询', async () => {
    ;(listDevDocs as ReturnType<typeof vi.fn>).mockResolvedValue({
      items: [makeSummary()],
      total: 1,
      totalPages: 1,
    })
    const router = makeRouter()
    await push(router, '/dev?q=事件&loader=neoforge&category=events&version=1.21&page=2')

    const wrapper = mount(DevDocsView, { global: { plugins: [router] } })
    await flushPromises()

    expect(listDevDocs).toHaveBeenCalledWith({
      q: '事件',
      loader: 'neoforge',
      category: 'events',
      version: '1.21',
      page: 2,
      page_size: 12,
    })
    expect((wrapper.find('input[type="search"]').element as HTMLInputElement).value).toBe('事件')
    expect(wrapper.text()).toContain('NeoForge 事件系统与生命周期')
    expect(wrapper.text()).toContain('6 个关键接口')
    // 平台 / 主题筛选项来自后端统计，带计数
    expect(wrapper.text()).toContain('NeoForge（11）')
    expect(wrapper.text()).toContain('事件与生命周期（4）')
  })

  it('切换平台筛选后 URL 更新并回到第 1 页', async () => {
    ;(listDevDocs as ReturnType<typeof vi.fn>).mockResolvedValue({ items: [], total: 0, totalPages: 1 })
    const router = makeRouter()
    await push(router, '/dev?q=事件&page=3')

    const wrapper = mount(DevDocsView, { global: { plugins: [router] } })
    await flushPromises()

    const fabric = wrapper.findAll('button').find((b) => b.text().includes('Fabric'))
    expect(fabric).toBeTruthy()
    await fabric!.trigger('click')
    await flushPromises()

    expect(router.currentRoute.value.query).toMatchObject({ q: '事件', loader: 'fabric' })
    expect(router.currentRoute.value.query.page).toBeUndefined()
    expect(listDevDocs).toHaveBeenLastCalledWith(
      expect.objectContaining({ loader: 'fabric', page: 1 }),
    )
  })

  it('清除全部筛选后链接保持干净', async () => {
    ;(listDevDocs as ReturnType<typeof vi.fn>).mockResolvedValue({ items: [], total: 0, totalPages: 1 })
    const router = makeRouter()
    await push(router, '/dev?q=事件&loader=neoforge&category=events&version=1.21')

    const wrapper = mount(DevDocsView, { global: { plugins: [router] } })
    await flushPromises()

    const clear = wrapper.findAll('button').find((b) => b.text().includes('清除全部筛选'))
    await clear!.trigger('click')
    await flushPromises()

    expect(router.currentRoute.value.query).toEqual({})
  })
})

describe('§21 开发文档详情页', () => {
  it('展示接口速查、正文、来源与校验信息以及同平台相关条目', async () => {
    ;(getDevDoc as ReturnType<typeof vi.fn>).mockResolvedValue(makeDetail())
    const router = makeRouter()
    await push(router, '/dev/neoforge-events')

    const wrapper = mount(DevDocDetailView, { global: { plugins: [router] } })
    await flushPromises()

    expect(getDevDoc).toHaveBeenCalledWith('neoforge-events')
    expect(wrapper.text()).toContain('NeoForge 事件系统与生命周期')
    // 接口速查
    expect(wrapper.text()).toContain('关键接口速查')
    expect(wrapper.text()).toContain('NeoForge.EVENT_BUS')
    expect(wrapper.text()).toContain('@SubscribeEvent')
    // 正文（Markdown 渲染）
    expect(wrapper.text()).toContain('两条总线')
    // 来源与校验
    expect(wrapper.text()).toContain('资料来源与校验')
    expect(wrapper.text()).toContain('docs-main-2026-10-18')
    const hrefs = wrapper.findAll('a').map((a) => a.attributes('href') ?? '')
    expect(hrefs).toContain('https://github.com/neoforged/Documentation/blob/main/docs/concepts/events.md')
    // 相关条目
    expect(wrapper.text()).toContain('同平台其它条目')
    expect(wrapper.text()).toContain('NeoForge 注册表与 DeferredRegister')
  })

  it('未收录条目时给出错误态而非空白页', async () => {
    ;(getDevDoc as ReturnType<typeof vi.fn>).mockRejectedValue(new Error('数据不存在'))
    const router = makeRouter()
    await push(router, '/dev/not-exist')

    const wrapper = mount(DevDocDetailView, { global: { plugins: [router] } })
    await flushPromises()

    expect(wrapper.text()).toContain('加载失败')
  })
})

describe('§21 映射查询页', () => {
  it('未导入任何数据集时展示导入指引（含可复制命令），不伪造示例数据', async () => {
    ;(listMappingSets as ReturnType<typeof vi.fn>).mockResolvedValue([])
    const router = makeRouter()
    await push(router, '/mappings')

    const wrapper = mount(MappingsView, { global: { plugins: [router] } })
    await flushPromises()

    expect(wrapper.text()).toContain('尚未导入映射数据')
    expect(wrapper.text()).toContain('cmd/import-mappings')
    // 未导入数据时不应发起查询
    expect(searchMappings).not.toHaveBeenCalled()
  })

  it('已导入数据集时按类聚合查询，展开可查看成员明细', async () => {
    ;(listMappingSets as ReturnType<typeof vi.fn>).mockResolvedValue([SET])
    ;(searchMappings as ReturnType<typeof vi.fn>).mockResolvedValue({
      hits: [HIT],
      total: 1,
      truncated: false,
      page: 1,
      page_size: 10,
      total_pages: 1,
    })
    ;(getMappingClass as ReturnType<typeof vi.fn>).mockResolvedValue({
      class: HIT,
      members: [
        {
          kind: 'method',
          obf_name: 'a',
          mid_name: 'method_5773',
          named_name: 'tick',
          obf_desc: '()V',
          named_desc: '()V',
          side: '',
        },
      ],
      member_total: 1,
      set: SET,
    })
    const router = makeRouter()
    await push(router, '/mappings')

    const wrapper = mount(MappingsView, { global: { plugins: [router] } })
    await flushPromises()

    // 默认选中第一个数据集，三列名字全部展示
    expect(searchMappings).toHaveBeenCalledWith(
      expect.objectContaining({ set_id: 'set-1', kind: 'all', page: 1, page_size: 10 }),
    )
    expect(wrapper.text()).toContain('net/minecraft/entity/Entity')
    expect(wrapper.text()).toContain('net/minecraft/class_1297')
    expect(wrapper.text()).toContain('各名字列对应的命名空间')
    expect(wrapper.text()).toContain('CC0')

    const expand = wrapper.findAll('button').find((b) => b.text().includes('查看成员'))
    expect(expand).toBeTruthy()
    await expand!.trigger('click')
    await flushPromises()

    expect(getMappingClass).toHaveBeenCalledWith('class-1', expect.objectContaining({ page: 1 }))
    expect(wrapper.text()).toContain('method_5773')
    expect(wrapper.text()).toContain('成员列表')
  })

  it('命中结果被截断时给出明确提示', async () => {
    ;(listMappingSets as ReturnType<typeof vi.fn>).mockResolvedValue([SET])
    ;(searchMappings as ReturnType<typeof vi.fn>).mockResolvedValue({
      hits: [HIT],
      total: 5000,
      truncated: true,
      page: 1,
      page_size: 10,
      total_pages: 500,
    })
    const router = makeRouter()
    await push(router, '/mappings')

    const wrapper = mount(MappingsView, { global: { plugins: [router] } })
    await flushPromises()

    expect(wrapper.text()).toContain('已按扫描上限截断')
  })
})
