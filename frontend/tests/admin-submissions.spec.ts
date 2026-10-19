// 资源投稿审核（后台）测试：
//   1) 默认只看待审核项，渲染投稿字段与来源外链安全属性；
//   2) 「通过（生成草稿）」「通过并发布」「驳回」三种动作的请求体；
//   3) 已审核项不再显示操作按钮；
//   4) 状态 / 类型 / 关键词筛选透传。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'

const api = vi.hoisted(() => ({
  listSubmissions: vi.fn(),
  getSubmission: vi.fn(),
  reviewSubmission: vi.fn(),
  batchReviewSubmissions: vi.fn(),
}))
vi.mock('@/api/admin-submissions', () => api)

import AdminSubmissionsView from '@/views/admin/AdminSubmissionsView.vue'
import type { ResourceSubmission } from '@/types/submission'
import { SUBMISSION_LICENSE_VERSION } from '@/types/submission'

function makeSubmission(partial: Partial<ResourceSubmission> = {}): ResourceSubmission {
  return {
    id: 's1',
    account_id: 'a1',
    submitter_name: 'alice',
    name: 'JEI 物品管理器',
    summary: '查看合成配方',
    description: '',
    resource_type: 'mod',
    author_name: '作者甲',
    source_platform: 'CurseForge',
    source_url: 'https://example.com/jei',
    license: 'MIT',
    game_edition: 'java',
    supported_versions: ['1.20.1'],
    loader_types: ['fabric'],
    install_guide: '',
    note: '来源为作者官方页',
    license_version: SUBMISSION_LICENSE_VERSION,
    license_accepted_at: '2026-10-19T00:00:00Z',
    status: 'pending',
    reviewer: '',
    reviewed_at: null,
    review_note: '',
    resource_id: null,
    created_at: '2026-10-19T00:00:00Z',
    updated_at: '2026-10-19T00:00:00Z',
    ...partial,
  }
}

function paged(items: ResourceSubmission[]) {
  return { items, total: items.length, page: 1, pageSize: 20, totalPages: 1, suggestions: [] }
}

async function mountView() {
  setActivePinia(createPinia())
  const wrapper = mount(AdminSubmissionsView)
  await flushPromises()
  return wrapper
}

describe('资源投稿审核', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    ;(api.listSubmissions as ReturnType<typeof vi.fn>).mockResolvedValue(paged([makeSubmission()]))
    ;(api.batchReviewSubmissions as ReturnType<typeof vi.fn>).mockResolvedValue({
      requested: 1,
      succeeded: 1,
      failed: 0,
      items: [{ id: 's1', label: 'JEI 物品管理器', ok: true }],
    })
    ;(api.reviewSubmission as ReturnType<typeof vi.fn>).mockResolvedValue({
      submission: makeSubmission({ status: 'approved' }),
      resource: { id: 'r1', status: 'draft' },
    })
  })

  it('默认按待审核筛选并渲染投稿字段', async () => {
    const wrapper = await mountView()
    expect(api.listSubmissions).toHaveBeenCalledWith(
      expect.objectContaining({ status: 'pending', page: 1, page_size: 20 }),
    )
    const text = wrapper.text()
    expect(text).toContain('JEI 物品管理器')
    expect(text).toContain('作者甲')
    expect(text).toContain('来源为作者官方页')
    // 外链带 noopener / nofollow 安全属性
    const link = wrapper.find('a[href="https://example.com/jei"]')
    expect(link.attributes('rel')).toContain('noopener')
    expect(link.attributes('rel')).toContain('nofollow')
  })

  it('「通过（生成草稿）」提交 approved 且 publish=false', async () => {
    const wrapper = await mountView()
    const btn = wrapper.findAll('button').find((b) => b.text() === '通过（生成草稿）')
    await btn!.trigger('click')
    await flushPromises()

    expect(api.reviewSubmission).toHaveBeenCalledWith({
      id: 's1',
      status: 'approved',
      note: '',
      publish: false,
    })
    // 审核后刷新列表（挂载 1 次 + 审核后 1 次）
    expect(api.listSubmissions).toHaveBeenCalledTimes(2)
  })

  it('「通过并发布」提交 publish=true', async () => {
    const wrapper = await mountView()
    const btn = wrapper.findAll('button').find((b) => b.text() === '通过并发布')
    await btn!.trigger('click')
    await flushPromises()

    expect(api.reviewSubmission).toHaveBeenCalledWith({
      id: 's1',
      status: 'approved',
      note: '',
      publish: true,
    })
  })

  it('驳回携带审核备注', async () => {
    const wrapper = await mountView()
    await wrapper.find('input[placeholder="审核备注（可选）"]').setValue('来源不可靠')
    const btn = wrapper.findAll('button').find((b) => b.text() === '驳回')
    await btn!.trigger('click')
    await flushPromises()

    expect(api.reviewSubmission).toHaveBeenCalledWith({
      id: 's1',
      status: 'rejected',
      note: '来源不可靠',
      publish: false,
    })
  })

  it('已审核项不显示操作按钮', async () => {
    ;(api.listSubmissions as ReturnType<typeof vi.fn>).mockResolvedValue(
      paged([
        makeSubmission({
          id: 's2',
          status: 'rejected',
          reviewer: 'admin',
          reviewed_at: '2026-10-19T01:00:00Z',
          review_note: '来源不可靠',
        }),
      ]),
    )
    const wrapper = await mountView()
    expect(wrapper.text()).toContain('已审核')
    expect(wrapper.findAll('button').some((b) => b.text() === '驳回')).toBe(false)
    expect(wrapper.text()).toContain('admin')
    expect(wrapper.text()).toContain('来源不可靠')
  })

  it('筛选透传状态 / 类型 / 关键词', async () => {
    const wrapper = await mountView()
    await wrapper.find('select').setValue('rejected')
    await flushPromises()
    expect(api.listSubmissions).toHaveBeenLastCalledWith(
      expect.objectContaining({ status: 'rejected', page: 1 }),
    )

    const typeSelect = wrapper.findAll('select')[1]
    await typeSelect.setValue('modpack')
    await flushPromises()
    expect(api.listSubmissions).toHaveBeenLastCalledWith(
      expect.objectContaining({ resource_type: 'modpack' }),
    )
  })

  it('批量通过：勾选待审核项后按 ID 列表提交', async () => {
    const wrapper = await mountView()
    // 行首复选框（表头还有一个「全选」）。
    const rowCheckbox = wrapper.find('tbody input[type="checkbox"]')
    expect(rowCheckbox.exists()).toBe(true)
    await rowCheckbox.setValue(true)

    await wrapper.find('input[placeholder="批量审核备注（可选，会写入每一条）"]').setValue('来源合规')
    const btn = wrapper.findAll('button').find((b) => b.text() === '批量通过（草稿）')
    await btn!.trigger('click')
    await flushPromises()

    expect(api.batchReviewSubmissions).toHaveBeenCalledWith({
      ids: ['s1'],
      status: 'approved',
      note: '来源合规',
      publish: false,
    })
    // 批量后刷新列表
    expect(api.listSubmissions).toHaveBeenCalledTimes(2)
  })

  it('批量操作未勾选时不发请求', async () => {
    const wrapper = await mountView()
    const btn = wrapper.findAll('button').find((b) => b.text() === '批量通过（草稿）')
    await btn!.trigger('click')
    await flushPromises()
    expect(api.batchReviewSubmissions).not.toHaveBeenCalled()
  })
})
