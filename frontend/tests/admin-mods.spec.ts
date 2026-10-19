// Mod 与整合包管理端审核（TODO §16）测试：
//   1) 默认 Mod tab 只看待审核项，渲染加载器/类别/来源外链安全属性；
//   2) 审核备注随状态提交（通过 / 驳回），提交后刷新列表；
//   3) 切换整合包 tab 拉取整合包并支持审核；
//   4) 已通过项展示审核人 / 审核时间 / 备注。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import AdminModsView from '@/views/admin/AdminModsView.vue'
import type { Mod, Modpack } from '@/types/mods'

const api = vi.hoisted(() => ({
  listModsAdmin: vi.fn(),
  reviewMod: vi.fn(),
  listModpacksAdmin: vi.fn(),
  reviewModpack: vi.fn(),
}))
vi.mock('@/api/admin-mods', () => api)

function makeMod(partial: Partial<Mod> = {}): Mod {
  return {
    id: 'm1',
    slug: 'sodium',
    name_zh: '钠',
    name_en: 'Sodium',
    aliases: [],
    summary: '面向 Fabric 的高性能渲染优化 Mod。',
    description: '通过重写渲染管线提升帧率。',
    categories: ['optimization'],
    loaders: ['fabric'],
    mc_versions: ['1.20-1.21'],
    homepage_url: null,
    source_url: 'https://github.com/CaffeineMC/sodium',
    license: 'LGPL-3.0',
    license_note: '开源许可，可自由分发。',
    game_edition: 'java',
    data_version: 'modrinth-2026-10',
    last_verified_at: '2026-10-18T00:00:00Z',
    review_status: 'pending',
    reviewer: '',
    reviewed_at: null,
    review_note: '',
    sort_order: 1,
    created_at: '2026-10-18T00:00:00Z',
    updated_at: '2026-10-18T00:00:00Z',
    ...partial,
  }
}

function makeModpack(partial: Partial<Modpack> = {}): Modpack {
  return {
    id: 'p1',
    slug: 'fabulously-optimized',
    name_zh: '极致优化整合包',
    name_en: 'Fabulously Optimized',
    aliases: [],
    summary: '以性能优化为核心的 Fabric 整合包。',
    description: '开箱即用的优化整合包。',
    mc_version: '1.21',
    loader: 'fabric',
    author: '社区作者',
    homepage_url: null,
    source_url: 'https://github.com/Fabulously-Optimized/fabulously-optimized',
    license: 'MIT',
    license_note: '整合包清单开放许可。',
    data_version: 'modrinth-2026-10',
    last_verified_at: '2026-10-18T00:00:00Z',
    review_status: 'pending',
    reviewer: '',
    reviewed_at: null,
    review_note: '',
    sort_order: 1,
    created_at: '2026-10-18T00:00:00Z',
    updated_at: '2026-10-18T00:00:00Z',
    ...partial,
  }
}

function paged<T>(items: T[]) {
  return { items, total: items.length, page: 1, pageSize: 20, totalPages: 1, suggestions: [] }
}

beforeEach(() => {
  vi.clearAllMocks()
  setActivePinia(createPinia())
  api.listModsAdmin.mockResolvedValue(paged([makeMod()]))
  api.listModpacksAdmin.mockResolvedValue(paged([makeModpack()]))
})

describe('§16 Mod 管理端审核', () => {
  it('默认 Mod tab 只看待审核项，渲染加载器/类别/来源外链', async () => {
    const wrapper = mount(AdminModsView)
    await flushPromises()

    expect(api.listModsAdmin).toHaveBeenCalledWith(
      expect.objectContaining({ review_status: 'pending', page: 1, page_size: 20 }),
    )
    expect(api.listModpacksAdmin).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('钠')
    expect(wrapper.text()).toContain('sodium')
    expect(wrapper.text()).toContain('Fabric')
    expect(wrapper.text()).toContain('优化')
    expect(wrapper.text()).toContain('1.20-1.21')
    expect(wrapper.text()).toContain('LGPL-3.0')
    expect(wrapper.text()).toContain('待审核')

    const link = wrapper.find('a[href="https://github.com/CaffeineMC/sodium"]')
    expect(link.exists()).toBe(true)
    expect(link.attributes('rel')).toContain('noopener')
  })

  it('填写备注后点「通过」提交 published 并刷新列表', async () => {
    api.reviewMod.mockResolvedValue(makeMod({ review_status: 'published', reviewer: 'admin' }))

    const wrapper = mount(AdminModsView)
    await flushPromises()

    await wrapper.find('input[placeholder="审核备注（可选）"]').setValue('来源与许可核验通过')
    await wrapper.findAll('button').find((b) => b.text() === '通过')!.trigger('click')
    await flushPromises()

    expect(api.reviewMod).toHaveBeenCalledWith('m1', 'published', '来源与许可核验通过')
    expect(api.listModsAdmin).toHaveBeenCalledTimes(2)
  })

  it('点「驳回」提交 rejected 状态', async () => {
    api.reviewMod.mockResolvedValue(makeMod({ review_status: 'rejected', reviewer: 'admin' }))

    const wrapper = mount(AdminModsView)
    await flushPromises()

    await wrapper.findAll('button').find((b) => b.text() === '驳回')!.trigger('click')
    await flushPromises()

    expect(api.reviewMod).toHaveBeenCalledWith('m1', 'rejected', '')
  })

  it('切到整合包 tab 拉取整合包并支持审核', async () => {
    api.reviewModpack.mockResolvedValue(makeModpack({ review_status: 'published', reviewer: 'admin' }))

    const wrapper = mount(AdminModsView)
    await flushPromises()

    await wrapper.findAll('button').find((b) => b.text() === '整合包')!.trigger('click')
    await flushPromises()

    expect(api.listModpacksAdmin).toHaveBeenCalledWith(
      expect.objectContaining({ review_status: 'pending' }),
    )
    expect(wrapper.text()).toContain('极致优化整合包')
    expect(wrapper.text()).toContain('社区作者')

    await wrapper.findAll('button').find((b) => b.text() === '通过')!.trigger('click')
    await flushPromises()

    expect(api.reviewModpack).toHaveBeenCalledWith('p1', 'published', '')
    expect(api.reviewMod).not.toHaveBeenCalled()
  })

  it('已通过项展示审核人 / 审核时间 / 备注', async () => {
    api.listModsAdmin.mockResolvedValue(
      paged([
        makeMod({
          review_status: 'published',
          reviewer: 'admin',
          reviewed_at: '2026-10-18T03:00:00Z',
          review_note: '已核验来源',
        }),
      ]),
    )

    const wrapper = mount(AdminModsView)
    await flushPromises()

    expect(wrapper.text()).toContain('已通过')
    expect(wrapper.text()).toContain('admin')
    expect(wrapper.text()).toContain('已核验来源')
  })
})
