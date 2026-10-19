// 链接健康视图测试（TODO §6）
// 覆盖：默认只看失败项的加载、状态/复核状态渲染、后台复查汇总、
// 人工确认（确认可用 / 确认失效）后的行内更新。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import AdminLinkHealthView from '@/views/admin/AdminLinkHealthView.vue'
import type { LinkHealthItem, LinkHealthRunSummary } from '@/types/link-health'

const api = vi.hoisted(() => ({
  runLinkHealthCheck: vi.fn(),
  listLinkHealth: vi.fn(),
  confirmLinkHealth: vi.fn(),
}))
vi.mock('@/api/admin-link-health', () => api)

function makeItem(partial: Partial<LinkHealthItem> = {}): LinkHealthItem {
  return {
    id: 'c1',
    resource_id: 'r1',
    resource_name: '示例 Mod',
    resource_slug: 'demo-mod',
    url: 'https://example.com/download',
    status: 'broken',
    status_code: 404,
    final_url: '',
    trigger_source: 'auto',
    review_state: 'pending',
    reviewed_by: '',
    reviewed_at: null,
    review_note: '',
    checked_at: '2026-10-18T00:00:00Z',
    created_at: '2026-10-18T00:00:00Z',
    updated_at: '2026-10-18T00:00:00Z',
    ...partial,
  }
}

function paged(items: LinkHealthItem[]) {
  return { items, total: items.length, page: 1, pageSize: 20, totalPages: 1, suggestions: [] }
}

beforeEach(() => {
  vi.clearAllMocks()
  setActivePinia(createPinia())
  api.listLinkHealth.mockResolvedValue(paged([makeItem()]))
})

describe('§6 链接健康视图', () => {
  it('默认只看失败项，渲染状态与复核状态', async () => {
    const wrapper = mount(AdminLinkHealthView)
    await flushPromises()

    expect(api.listLinkHealth).toHaveBeenCalledWith(
      expect.objectContaining({ failed_only: true, page: 1, page_size: 20 }),
    )
    expect(wrapper.text()).toContain('示例 Mod')
    expect(wrapper.text()).toContain('已失效')
    expect(wrapper.text()).toContain('HTTP 404')
    expect(wrapper.text()).toContain('待确认')

    const link = wrapper.find('a[href="https://example.com/download"]')
    expect(link.exists()).toBe(true)
    expect(link.attributes('rel')).toContain('noopener')
  })

  it('后台复查后展示汇总并刷新列表', async () => {
    const summary: LinkHealthRunSummary = {
      checked: 12,
      ok: 8,
      redirected: 1,
      broken: 2,
      blocked: 1,
      error: 0,
      concurrency: 4,
      trigger_source: 'manual',
    }
    api.runLinkHealthCheck.mockResolvedValue(summary)

    const wrapper = mount(AdminLinkHealthView)
    await flushPromises()

    const runBtn = wrapper.findAll('button').find((b) => b.text().includes('开始后台复查'))
    expect(runBtn).toBeTruthy()
    await runBtn!.trigger('click')
    await flushPromises()

    expect(api.runLinkHealthCheck).toHaveBeenCalledWith({ only_pending: true })
    expect(wrapper.text()).toContain('本次检查 12 条')
    expect(wrapper.text()).toContain('并发上限 4')
    // 复查后重新拉取列表
    expect(api.listLinkHealth).toHaveBeenCalledTimes(2)
  })

  it('确认可用后行内复核状态更新为已确认可用', async () => {
    api.confirmLinkHealth.mockResolvedValue(
      makeItem({ review_state: 'confirmed_ok', reviewed_by: 'admin', reviewed_at: '2026-10-18T01:00:00Z' }),
    )

    const wrapper = mount(AdminLinkHealthView)
    await flushPromises()

    const okBtn = wrapper.findAll('button').find((b) => b.text() === '确认可用')
    expect(okBtn).toBeTruthy()
    await okBtn!.trigger('click')
    await flushPromises()

    expect(api.confirmLinkHealth).toHaveBeenCalledWith('c1', 'confirm_ok', '')
    expect(wrapper.text()).toContain('已确认可用')
  })

  it('确认失效时提交备注', async () => {
    api.confirmLinkHealth.mockResolvedValue(makeItem({ review_state: 'confirmed_broken', review_note: '来源已下架' }))

    const wrapper = mount(AdminLinkHealthView)
    await flushPromises()

    await wrapper.find('input[placeholder="备注（可选）"]').setValue('来源已下架')
    const brokenBtn = wrapper.findAll('button').find((b) => b.text() === '确认失效')
    await brokenBtn!.trigger('click')
    await flushPromises()

    expect(api.confirmLinkHealth).toHaveBeenCalledWith('c1', 'confirm_broken', '来源已下架')
    expect(wrapper.text()).toContain('已确认失效')
  })
})
