// 红石机器设计管理端审核（TODO §15）测试：
//   1) 默认只看待审核项，渲染设计字段与外链安全属性；
//   2) 审核备注随状态一并提交（通过 / 驳回），提交后刷新列表；
//   3) 审核状态徽章按审核语义展示。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import AdminRedstoneView from '@/views/admin/AdminRedstoneView.vue'
import type { RedstoneDesign } from '@/types/redstone'

const api = vi.hoisted(() => ({
  listRedstoneDesignsAdmin: vi.fn(),
  reviewRedstoneDesign: vi.fn(),
}))
vi.mock('@/api/admin-redstone', () => api)

function makeDesign(partial: Partial<RedstoneDesign> = {}): RedstoneDesign {
  return {
    id: 'd1',
    slug: 'item-sorter',
    name_zh: '物品分类机',
    name_en: 'Item Sorter',
    aliases: [],
    purpose: 'sorter',
    difficulty: 'medium',
    summary: '按物品类型自动分流的红石分类机。',
    steps_md: '1. 铺设漏斗链',
    materials: [
      { item_id: 'minecraft:hopper', name_zh: '漏斗', count_min: 4, count_max: 4 },
    ],
    author: '社区作者',
    author_url: null,
    screenshot_url: null,
    video_url: null,
    world_download_url: null,
    world_download_note: '未提供存档下载，请按步骤自行搭建。',
    license: 'CC BY-SA 4.0',
    license_note: '作者授权转载。',
    supported_versions: ['1.14+'],
    game_edition: 'java',
    source_url: 'https://minecraft.wiki/w/Redstone_circuit',
    data_version: 'wiki-2026-10',
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

function paged(items: RedstoneDesign[]) {
  return { items, total: items.length, page: 1, pageSize: 20, totalPages: 1, suggestions: [] }
}

beforeEach(() => {
  vi.clearAllMocks()
  setActivePinia(createPinia())
  api.listRedstoneDesignsAdmin.mockResolvedValue(paged([makeDesign()]))
})

describe('§15 红石设计管理端审核', () => {
  it('默认只看待审核项，渲染设计字段与外链安全属性', async () => {
    const wrapper = mount(AdminRedstoneView)
    await flushPromises()

    expect(api.listRedstoneDesignsAdmin).toHaveBeenCalledWith(
      expect.objectContaining({ review_status: 'pending', page: 1, page_size: 20 }),
    )
    expect(wrapper.text()).toContain('物品分类机')
    expect(wrapper.text()).toContain('item-sorter')
    expect(wrapper.text()).toContain('分类机')
    expect(wrapper.text()).toContain('中等')
    expect(wrapper.text()).toContain('CC BY-SA 4.0')
    expect(wrapper.text()).toContain('待审核')

    const link = wrapper.find('a[href="https://minecraft.wiki/w/Redstone_circuit"]')
    expect(link.exists()).toBe(true)
    expect(link.attributes('rel')).toContain('noopener')
  })

  it('填写备注后点「通过」提交 published 并刷新列表', async () => {
    api.reviewRedstoneDesign.mockResolvedValue(makeDesign({ review_status: 'published', reviewer: 'admin' }))

    const wrapper = mount(AdminRedstoneView)
    await flushPromises()

    await wrapper.find('input[placeholder="审核备注（可选）"]').setValue('内容与来源核验无误')
    await wrapper.findAll('button').find((b) => b.text() === '通过')!.trigger('click')
    await flushPromises()

    expect(api.reviewRedstoneDesign).toHaveBeenCalledWith('d1', 'published', '内容与来源核验无误')
    // 审核后重新拉取列表
    expect(api.listRedstoneDesignsAdmin).toHaveBeenCalledTimes(2)
  })

  it('点「驳回」提交 rejected 状态', async () => {
    api.reviewRedstoneDesign.mockResolvedValue(makeDesign({ review_status: 'rejected', reviewer: 'admin' }))

    const wrapper = mount(AdminRedstoneView)
    await flushPromises()

    await wrapper.findAll('button').find((b) => b.text() === '驳回')!.trigger('click')
    await flushPromises()

    expect(api.reviewRedstoneDesign).toHaveBeenCalledWith('d1', 'rejected', '')
  })

  it('已通过项展示审核人与审核时间', async () => {
    api.listRedstoneDesignsAdmin.mockResolvedValue(
      paged([
        makeDesign({
          review_status: 'published',
          reviewer: 'admin',
          reviewed_at: '2026-10-18T03:00:00Z',
          review_note: '已核验',
        }),
      ]),
    )

    const wrapper = mount(AdminRedstoneView)
    await flushPromises()

    expect(wrapper.text()).toContain('已通过')
    expect(wrapper.text()).toContain('admin')
    expect(wrapper.text()).toContain('已核验')
  })
})
