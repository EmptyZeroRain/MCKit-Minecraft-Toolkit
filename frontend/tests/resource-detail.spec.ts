// 资源详情页测试（TODO §6 资源兼容性与来源信息）
// 覆盖：兼容性结论（未验证原则）、发布版本、依赖分组、来源/许可/作者、外链检查状态。
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory } from 'vue-router'
import type { ResourceDetail } from '@/types/resource'

vi.mock('@/api/public', () => ({ getResource: vi.fn() }))
vi.mock('@/api/versions', () => ({ listVersions: vi.fn() }))

import { getResource } from '@/api/public'
import { listVersions } from '@/api/versions'
import ResourceDetailView from '@/views/ResourceDetailView.vue'
import { resetVersionCache } from '@/utils/mc-versions'

const mockedGet = vi.mocked(getResource)
const mockedVersions = vi.mocked(listVersions)

function makeDetail(overrides: Partial<ResourceDetail> = {}): ResourceDetail {
  return {
    id: 'r1',
    slug: 'jei',
    name: 'JEI 物品管理器',
    summary: '查看物品与配方',
    description: '介绍内容',
    resource_type: 'mod',
    cover_image_url: null,
    author_name: 'mezz',
    source_platform: 'CurseForge',
    source_url: 'https://example.com/jei',
    license: 'MIT',
    game_edition: 'java',
    supported_versions: ['1.20.1'],
    loader_types: ['forge'],
    dependencies: [],
    install_guide: '',
    tags: [],
    compatibility_note: '',
    status: 'published',
    link_status: 'ok',
    last_checked_at: '2026-02-01T00:00:00Z',
    published_at: '2026-01-01T00:00:00Z',
    created_by: null,
    updated_by: null,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    releases: [],
    compatibility: {
      verified: true,
      game_versions: ['1.20.1'],
      loader_types: ['forge'],
      sources: ['resource'],
      note: '',
    },
    ...overrides,
  }
}

function makeRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: { template: '<div />' } },
      { path: '/resources', component: { template: '<div />' } },
      { path: '/resources/:slug', component: { template: '<div />' } },
      { path: '/search', component: { template: '<div />' } },
    ],
  })
}

async function mountDetail(detail: ResourceDetail) {
  setActivePinia(createPinia())
  const router = makeRouter()
  router.push('/resources/jei')
  await router.isReady()
  mockedGet.mockResolvedValue(detail)
  const wrapper = mount(ResourceDetailView, { global: { plugins: [router] } })
  await flushPromises()
  return wrapper
}

describe('资源详情页', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    resetVersionCache()
    mockedVersions.mockResolvedValue({
      latest: { release: '1.21.4', snapshot: '' },
      versions: [
        { id: '1.21.4', type: 'release', url: '', time: '', releaseTime: '' },
        { id: '1.20.1', type: 'release', url: '', time: '', releaseTime: '' },
      ],
    } as never)
  })

  it('展示来源、许可与作者信息', async () => {
    const wrapper = await mountDetail(makeDetail())
    expect(wrapper.text()).toContain('来源与许可信息')
    expect(wrapper.text()).toContain('mezz')
    expect(wrapper.text()).toContain('CurseForge')
    expect(wrapper.text()).toContain('MIT')
  })

  it('展示外链检查状态与最后检查时间', async () => {
    const wrapper = await mountDetail(makeDetail())
    expect(wrapper.text()).toContain('外链检查')
    expect(wrapper.text()).toContain('正常')
  })

  it('有数据依据时展示已验证结论与依据来源', async () => {
    const wrapper = await mountDetail(makeDetail())
    expect(wrapper.text()).toContain('有数据依据')
    expect(wrapper.text()).toContain('结论依据：资源级字段')
    expect(wrapper.text()).not.toContain('未验证')
  })

  it('无数据依据时明确展示「未验证」，不描述为兼容', async () => {
    const wrapper = await mountDetail(
      makeDetail({
        supported_versions: [],
        loader_types: [],
        compatibility: {
          verified: false,
          game_versions: [],
          loader_types: [],
          sources: [],
          note: '',
          unverified_hint: '该资源未提供适用版本或加载器信息，兼容性尚未验证。',
        },
      }),
    )
    expect(wrapper.text()).toContain('未验证')
    expect(wrapper.text()).toContain('兼容性尚未验证')
    expect(wrapper.text()).not.toContain('有数据依据')
  })

  it('展示人工核验备注', async () => {
    const wrapper = await mountDetail(
      makeDetail({
        compatibility: {
          verified: true,
          game_versions: ['1.20.1'],
          loader_types: ['forge'],
          sources: ['resource'],
          note: '1.21 起以 NeoForge 为准',
        },
      }),
    )
    expect(wrapper.text()).toContain('核验备注')
    expect(wrapper.text()).toContain('1.21 起以 NeoForge 为准')
  })

  it('展示发布版本与「最新」标记', async () => {
    const wrapper = await mountDetail(
      makeDetail({
        releases: [
          {
            id: 'rel1',
            resource_id: 'r1',
            version_label: '19.5.0.38',
            game_versions: ['1.21.1'],
            loader_types: ['neoforge'],
            source_url: 'https://example.com/jei/19',
            released_at: '2024-09-12T00:00:00Z',
            notes: 'NeoForge 主线版本',
            is_latest: true,
            created_at: '2026-01-01T00:00:00Z',
            updated_at: '2026-01-01T00:00:00Z',
          },
        ],
      }),
    )
    expect(wrapper.text()).toContain('发布版本')
    expect(wrapper.text()).toContain('19.5.0.38')
    expect(wrapper.text()).toContain('最新')
    expect(wrapper.text()).toContain('NeoForge 主线版本')
    expect(wrapper.text()).toContain('查看该版本发布页')
  })

  it('无发布版本时不渲染发布版本区块', async () => {
    const wrapper = await mountDetail(makeDetail())
    expect(wrapper.text()).not.toContain('查看该版本发布页')
  })

  it('依赖按前置 / 依赖 / 冲突分组展示', async () => {
    const wrapper = await mountDetail(
      makeDetail({
        dependencies: [
          { name: 'Forge', type: 'prerequisite', required: true },
          { name: 'Cloth Config API', type: 'dependency', required: true },
          { name: 'OptiFine', type: 'conflict', note: '渲染冲突' },
        ],
      }),
    )
    const text = wrapper.text()
    expect(text).toContain('依赖与冲突')
    expect(text).toContain('前置 Mod')
    expect(text).toContain('已知冲突')
    expect(text).toContain('渲染冲突')
  })

  it('历史依赖缺少 type 时归入「依赖」分组', async () => {
    const wrapper = await mountDetail(makeDetail({ dependencies: [{ name: 'Legacy Dep' }] }))
    const text = wrapper.text()
    expect(text).toContain('依赖')
    expect(text).toContain('Legacy Dep')
    expect(text).not.toContain('已知冲突')
  })

  it('危险协议的外链不渲染为可点击链接', async () => {
    const wrapper = await mountDetail(
      makeDetail({ dependencies: [{ name: 'Bad', type: 'dependency', resource_url: 'javascript:alert(1)' }] }),
    )
    const hrefs = wrapper.findAll('a').map((a) => a.attributes('href') ?? '')
    expect(hrefs.some((h) => h.startsWith('javascript:'))).toBe(false)
  })

  it('声明「所有版本」时展示「全部 MC 版本」，可展开完整正式版清单', async () => {
    const wrapper = await mountDetail(
      makeDetail({
        compatibility: {
          verified: true,
          game_versions: ['all'],
          loader_types: ['forge'],
          sources: ['resource'],
          all_versions: true,
          note: '',
        },
      }),
    )
    expect(wrapper.text()).toContain('全部 MC 版本')
    // 通配声明不渲染为普通版本徽标
    expect(wrapper.text()).not.toContain('未标明')

    const btn = wrapper.findAll('button').find((b) => b.text().includes('查看完整正式版清单'))
    expect(btn).toBeTruthy()
    await btn!.trigger('click')
    await flushPromises()

    expect(mockedVersions).toHaveBeenCalledWith({ type: 'release' })
    expect(wrapper.text()).toContain('1.21.4')
    expect(wrapper.text()).toContain('收起完整版本清单')
  })

  it('未声明通配时不渲染完整清单入口', async () => {
    const wrapper = await mountDetail(makeDetail())
    expect(wrapper.text()).not.toContain('查看完整正式版清单')
  })
})
