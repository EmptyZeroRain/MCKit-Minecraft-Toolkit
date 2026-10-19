// Mod 与整合包兼容性查询（TODO §16）测试：
//   1) 类型层展示辅助（授权下载入口判定 / 整合包清单展示与链接 / 依赖目标展示）；
//   2) 列表页双 tab：Mod 按加载器、类别与版本检索，整合包按加载器与目标版本检索，状态与 URL 双向同步（§14）；
//   3) 详情页：发布版本表、依赖与冲突分组 + 验证徽章、反向依赖；
//   4) 版权红线：无授权说明不给下载链接，回退 download_note 文字说明；
//   5) 整合包清单「已收录才链接、未收录回退纯文本」（不生成死链）。
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory, type Router } from 'vue-router'

vi.mock('@/api/public', () => ({
  listMods: vi.fn(),
  getMod: vi.fn(),
  listModpacks: vi.fn(),
  getModpack: vi.fn(),
}))
vi.mock('@/api/versions', () => ({ listVersions: vi.fn() }))

import { getMod, getModpack, listMods, listModpacks } from '@/api/public'
import { listVersions } from '@/api/versions'
import ModsView from '@/views/ModsView.vue'
import ModDetailView from '@/views/ModDetailView.vue'
import ModpackDetailView from '@/views/ModpackDetailView.vue'
import {
  MOD_DEPENDENCY_KIND_LABELS,
  MOD_LOADER_LABELS,
  authorizedDownloadUrl,
  dependencyTargetLabel,
  isExternalUrl,
  modpackModLabel,
  modpackModUrl,
  type Mod,
  type ModDependency,
  type ModDetail,
  type ModVersion,
  type Modpack,
  type ModpackDetail,
  type ModpackMod,
} from '@/types/mods'
import { resetVersionCache } from '@/utils/mc-versions'

const PATHS: Array<{ path: string; name?: string }> = [
  { path: '/', name: 'home' },
  { path: '/mods', name: 'mods' },
  { path: '/mods/:slug', name: 'mod-detail' },
  { path: '/modpacks/:slug', name: 'modpack-detail' },
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

function makeMod(overrides: Partial<Mod> = {}): Mod {
  return {
    id: 'm1',
    slug: 'fabric-api',
    name_zh: 'Fabric API',
    name_en: 'Fabric API',
    aliases: ['FAPI'],
    summary: 'Fabric 生态的通用前置库。',
    description: '提供 Fabric Mod 通用的钩子与接口。',
    categories: ['library'],
    loaders: ['fabric'],
    mc_versions: ['1.14+'],
    homepage_url: 'https://fabricmc.net/',
    source_url: 'https://github.com/FabricMC/fabric',
    license: 'Apache-2.0',
    license_note: '按官方仓库 LICENSE 署名。',
    game_edition: 'java',
    data_version: 'curated-2026-10',
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

function makeVersion(overrides: Partial<ModVersion> = {}): ModVersion {
  return {
    id: 'v1',
    mod_id: 'm1',
    version: '0.92.2+1.20.1',
    release_type: 'release',
    mc_versions: ['1.20.1'],
    loaders: ['fabric'],
    download_url: 'https://modrinth.com/mod/fabric-api',
    download_note: '',
    file_name: 'fabric-api-0.92.2.jar',
    published_at: '2026-01-01T00:00:00Z',
    source_url: 'https://modrinth.com/mod/fabric-api/version/0.92.2',
    verified: true,
    verify_note: '来源与入口均已核验。',
    sort_order: 1,
    created_at: '2026-10-18T00:00:00Z',
    updated_at: '2026-10-18T00:00:00Z',
    ...overrides,
  }
}

function makeDependency(overrides: Partial<ModDependency> = {}): ModDependency {
  return {
    id: 'd1',
    mod_id: 'm1',
    dep_kind: 'required',
    target_mod_id: 'm2',
    target_mod_name: 'Cloth Config',
    version_note: '1.17+',
    note: '必需前置。',
    source_url: 'https://modrinth.com/mod/cloth-config',
    verified: true,
    created_at: '2026-10-18T00:00:00Z',
    updated_at: '2026-10-18T00:00:00Z',
    target_mod: {
      slug: 'cloth-config',
      name_zh: 'Cloth Config',
      name_en: 'Cloth Config',
      loaders: ['fabric', 'forge'],
      url: '/mods/cloth-config',
    },
    ...overrides,
  }
}

function makeModpack(overrides: Partial<Modpack> = {}): Modpack {
  return {
    id: 'p1',
    slug: 'fabulously-optimized',
    name_zh: 'Fabulously Optimized',
    name_en: 'Fabulously Optimized',
    aliases: ['FO'],
    summary: '以性能优化为主的 Fabric 整合包。',
    description: '面向低配设备优化的整合包。',
    mc_version: '1.21.1',
    loader: 'fabric',
    author: 'Robotkoer',
    homepage_url: 'https://modrinth.com/modpack/fabulously-optimized',
    source_url: 'https://github.com/Fabulously-Optimized/fabulously-optimized',
    license: 'MIT',
    license_note: '',
    data_version: 'curated-2026-10',
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

function makeModpackMod(overrides: Partial<ModpackMod> = {}): ModpackMod {
  return {
    id: 'pm1',
    modpack_id: 'p1',
    mod_id: 'm1',
    mod_name: 'Fabric API',
    version: '0.92.2',
    note: '',
    sort_order: 1,
    created_at: '2026-10-18T00:00:00Z',
    mod_ref: {
      slug: 'fabric-api',
      name_zh: 'Fabric API',
      name_en: 'Fabric API',
      loaders: ['fabric'],
      url: '/mods/fabric-api',
    },
    ...overrides,
  }
}

beforeEach(() => {
  vi.clearAllMocks()
  setActivePinia(createPinia())
  resetVersionCache()
  ;(listVersions as ReturnType<typeof vi.fn>).mockResolvedValue({
    versions: [{ id: '1.21.1' }, { id: '1.21' }, { id: '1.20.1' }, { id: '1.20' }],
  })
})

async function push(router: Router, fullPath: string) {
  await router.push(fullPath)
  await router.isReady()
}

describe('§16 Mod 展示辅助', () => {
  it('授权下载入口：仅「官方授权入口 + 已说明授权」同时满足才返回链接', () => {
    const version = makeVersion()
    // 有授权协议 → 展示链接
    expect(authorizedDownloadUrl(version, { license: 'MIT', license_note: '' })).toBe(version.download_url)
    // 无授权协议但有授权说明 → 展示链接
    expect(authorizedDownloadUrl(version, { license: '', license_note: '见官方仓库 LICENSE' })).toBe(
      version.download_url,
    )
    // 无任何授权说明 → 不给链接
    expect(authorizedDownloadUrl(version, { license: '', license_note: '' })).toBeNull()
    // 非 http(s) 入口 → 不给链接
    expect(
      authorizedDownloadUrl(
        makeVersion({ download_url: 'javascript:alert(1)' }),
        { license: 'MIT', license_note: '' },
      ),
    ).toBeNull()
    expect(authorizedDownloadUrl(makeVersion({ download_url: null }), { license: 'MIT', license_note: '' })).toBeNull()
  })

  it('整合包清单：已收录优先中文名并给链接，未收录回退纯文本且无链接', () => {
    const listed = makeModpackMod()
    expect(modpackModLabel(listed)).toBe('Fabric API')
    expect(modpackModUrl(listed)).toBe('/mods/fabric-api')

    const unlisted = makeModpackMod({ mod_ref: null, mod_name: 'unlisted-mod' })
    expect(modpackModLabel(unlisted)).toBe('unlisted-mod')
    expect(modpackModUrl(unlisted)).toBeNull()
  })

  it('依赖目标展示名：已收录优先中文名，未收录回退纯文本名', () => {
    expect(dependencyTargetLabel(makeDependency())).toBe('Cloth Config')
    expect(
      dependencyTargetLabel(makeDependency({ target_mod: null, target_mod_name: '某不兼容 Mod' })),
    ).toBe('某不兼容 Mod')
  })

  it('外链判定只接受 http(s)，避免生成无效链接', () => {
    expect(isExternalUrl('https://modrinth.com/mod/fabric-api')).toBe(true)
    expect(isExternalUrl('')).toBe(false)
    expect(isExternalUrl(null)).toBe(false)
    expect(isExternalUrl('javascript:alert(1)')).toBe(false)
  })

  it('加载器与依赖类型展示名覆盖后端枚举', () => {
    expect(MOD_LOADER_LABELS.neoforge).toBe('NeoForge')
    expect(MOD_DEPENDENCY_KIND_LABELS.incompatible).toBe('不兼容')
  })
})

describe('§16 Mod 列表页', () => {
  it('Mod tab：从分享链接还原关键词 / 加载器 / 类别 / 版本 / 页码并据此查询', async () => {
    ;(listMods as ReturnType<typeof vi.fn>).mockResolvedValue({
      items: [makeMod()],
      total: 1,
      totalPages: 1,
    })
    const router = makeRouter()
    await push(router, '/mods?q=Fabric&loader=fabric&category=library&version=1.20&page=2')

    const wrapper = mount(ModsView, { global: { plugins: [router] } })
    await flushPromises()

    expect(listMods).toHaveBeenCalledWith({
      q: 'Fabric',
      loader: 'fabric',
      category: 'library',
      version: '1.20',
      page: 2,
      page_size: 12,
    })
    expect((wrapper.find('input[type="search"]').element as HTMLInputElement).value).toBe('Fabric')
    expect(wrapper.text()).toContain('Fabric API')
    expect(wrapper.text()).toContain('前置库')
  })

  it('整合包 tab：从 URL 的 tab 参数进入，按加载器与目标版本查询', async () => {
    ;(listModpacks as ReturnType<typeof vi.fn>).mockResolvedValue({
      items: [makeModpack()],
      total: 1,
      totalPages: 1,
    })
    const router = makeRouter()
    await push(router, '/mods?tab=modpacks&q=优化&loader=fabric&version=1.21.1')

    const wrapper = mount(ModsView, { global: { plugins: [router] } })
    await flushPromises()

    expect(listModpacks).toHaveBeenCalledWith({
      q: '优化',
      loader: 'fabric',
      version: '1.21.1',
      page: 1,
      page_size: 12,
    })
    expect(wrapper.text()).toContain('Fabulously Optimized')
    expect(wrapper.text()).toContain('Robotkoer')
  })

  it('切换 tab 后 URL 更新、页码回到第 1 页且清空跨 tab 语义不同的版本筛选', async () => {
    ;(listMods as ReturnType<typeof vi.fn>).mockResolvedValue({ items: [], total: 0, totalPages: 1 })
    ;(listModpacks as ReturnType<typeof vi.fn>).mockResolvedValue({ items: [], total: 0, totalPages: 1 })
    const router = makeRouter()
    await push(router, '/mods?q=Fabric&version=1.20&page=3')

    const wrapper = mount(ModsView, { global: { plugins: [router] } })
    await flushPromises()

    const packTab = wrapper.findAll('button').find((b) => b.text() === '整合包')
    expect(packTab).toBeTruthy()
    await packTab!.trigger('click')
    await flushPromises()

    expect(router.currentRoute.value.query).toMatchObject({ tab: 'modpacks', q: 'Fabric' })
    expect(router.currentRoute.value.query.page).toBeUndefined()
    expect(router.currentRoute.value.query.version).toBeUndefined()
  })

  it('清除全部筛选后链接保持干净（无冗余参数）', async () => {
    ;(listMods as ReturnType<typeof vi.fn>).mockResolvedValue({ items: [], total: 0, totalPages: 1 })
    const router = makeRouter()
    await push(router, '/mods?q=Fabric&loader=fabric&category=library&version=1.20')

    const wrapper = mount(ModsView, { global: { plugins: [router] } })
    await flushPromises()

    const clear = wrapper.findAll('button').find((b) => b.text().includes('清除全部筛选'))
    await clear!.trigger('click')
    await flushPromises()

    expect(router.currentRoute.value.query).toEqual({})
  })
})

describe('§16 Mod 详情页', () => {
  const detail: ModDetail = {
    ...makeMod(),
    versions: [
      makeVersion(),
      makeVersion({
        id: 'v2',
        version: '0.91.0+1.20.1',
        release_type: 'beta',
        download_url: null,
        download_note: '该版本未提供授权下载入口，请前往官方页面获取。',
        verified: false,
        verify_note: '入口未核验。',
      }),
    ],
    dependencies: [
      makeDependency(),
      makeDependency({
        id: 'd2',
        dep_kind: 'incompatible',
        target_mod_id: null,
        target_mod_name: '某不兼容 Mod',
        version_note: '全部版本',
        note: '已记录的明确不兼容。',
        source_url: 'https://example.com/incompatible',
        verified: false,
        target_mod: null,
      }),
    ],
    dependents: [
      {
        dep_kind: 'required',
        mod: {
          slug: 'sodium',
          name_zh: 'Sodium',
          name_en: 'Sodium',
          loaders: ['fabric'],
          url: '/mods/sodium',
        },
        version_note: '1.17+',
        note: '依赖 Fabric API。',
        source_url: 'https://modrinth.com/mod/sodium',
        verified: true,
      },
    ],
  }

  it('展示发布版本表、依赖分组与反向依赖', async () => {
    ;(getMod as ReturnType<typeof vi.fn>).mockResolvedValue(detail)
    const router = makeRouter()
    await push(router, '/mods/fabric-api')

    const wrapper = mount(ModDetailView, { global: { plugins: [router] } })
    await flushPromises()

    expect(getMod).toHaveBeenCalledWith('fabric-api')
    expect(wrapper.text()).toContain('发布版本')
    expect(wrapper.text()).toContain('0.92.2+1.20.1')
    expect(wrapper.text()).toContain('依赖与冲突')
    expect(wrapper.text()).toContain('必需前置')
    expect(wrapper.text()).toContain('不兼容')
    expect(wrapper.text()).toContain('谁依赖了它')
    expect(wrapper.text()).toContain('Sodium')
    expect(wrapper.text()).toContain('已验证')
    expect(wrapper.text()).toContain('未验证')
  })

  it('已收录依赖目标给出链接，未收录回退纯文本（不生成死链）', async () => {
    ;(getMod as ReturnType<typeof vi.fn>).mockResolvedValue(detail)
    const router = makeRouter()
    await push(router, '/mods/fabric-api')

    const wrapper = mount(ModDetailView, { global: { plugins: [router] } })
    await flushPromises()

    const hrefs = wrapper.findAll('a').map((a) => a.attributes('href') ?? '')
    expect(hrefs).toContain('/mods/cloth-config')
    expect(hrefs).toContain('/mods/sodium')
    // 未收录的「某不兼容 Mod」不得生成死链，但需展示纯文本名
    expect(hrefs.some((h) => h.includes('某不兼容'))).toBe(false)
    expect(wrapper.text()).toContain('某不兼容 Mod')
  })

  it('版权红线：有授权说明才给下载链接，未提供授权入口的版本回退文字说明', async () => {
    ;(getMod as ReturnType<typeof vi.fn>).mockResolvedValue(detail)
    const router = makeRouter()
    await push(router, '/mods/fabric-api')

    const wrapper = mount(ModDetailView, { global: { plugins: [router] } })
    await flushPromises()

    const hrefs = wrapper.findAll('a').map((a) => a.attributes('href') ?? '')
    expect(hrefs).toContain('https://modrinth.com/mod/fabric-api')
    expect(wrapper.text()).toContain('前往下载（官方 / 授权入口）')
    expect(wrapper.text()).toContain('该版本未提供授权下载入口，请前往官方页面获取。')
  })

  it('版权红线：无任何授权说明时不给下载链接', async () => {
    ;(getMod as ReturnType<typeof vi.fn>).mockResolvedValue({
      ...detail,
      license: '',
      license_note: '',
    })
    const router = makeRouter()
    await push(router, '/mods/fabric-api')

    const wrapper = mount(ModDetailView, { global: { plugins: [router] } })
    await flushPromises()

    const hrefs = wrapper.findAll('a').map((a) => a.attributes('href') ?? '')
    expect(hrefs).not.toContain('https://modrinth.com/mod/fabric-api')
    expect(wrapper.text()).not.toContain('前往下载（官方 / 授权入口）')
  })
})

describe('§16 整合包详情页', () => {
  const detail: ModpackDetail = {
    ...makeModpack(),
    mods: [
      makeModpackMod(),
      makeModpackMod({
        id: 'pm2',
        mod_id: null,
        mod_name: 'unlisted-mod',
        version: '1.0.0',
        note: '未收录，仅作清单展示。',
        mod_ref: null,
      }),
    ],
  }

  it('展示目标版本、作者与包内 Mod 清单', async () => {
    ;(getModpack as ReturnType<typeof vi.fn>).mockResolvedValue(detail)
    const router = makeRouter()
    await push(router, '/modpacks/fabulously-optimized')

    const wrapper = mount(ModpackDetailView, { global: { plugins: [router] } })
    await flushPromises()

    expect(getModpack).toHaveBeenCalledWith('fabulously-optimized')
    expect(wrapper.text()).toContain('包内 Mod 清单')
    expect(wrapper.text()).toContain('Fabric API')
    expect(wrapper.text()).toContain('Robotkoer')
    expect(wrapper.text()).toContain('1.21.1')
  })

  it('已收录清单项给出 Mod 链接，未收录回退纯文本（不生成死链）', async () => {
    ;(getModpack as ReturnType<typeof vi.fn>).mockResolvedValue(detail)
    const router = makeRouter()
    await push(router, '/modpacks/fabulously-optimized')

    const wrapper = mount(ModpackDetailView, { global: { plugins: [router] } })
    await flushPromises()

    const hrefs = wrapper.findAll('a').map((a) => a.attributes('href') ?? '')
    expect(hrefs).toContain('/mods/fabric-api')
    expect(hrefs.some((h) => h.includes('unlisted'))).toBe(false)
    expect(wrapper.text()).toContain('unlisted-mod')
  })
})
