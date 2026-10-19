// 内容历史版本面板测试（gj.md §15 / §16.2）：
// 列表渲染、展开字段级差异、回滚二次确认与事件回调。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import ContentRevisionsPanel from '@/components/admin/ContentRevisionsPanel.vue'
import type { RevisionListItem } from '@/types/revision'

const api = vi.hoisted(() => ({
  listRevisions: vi.fn(),
  getRevisionDiff: vi.fn(),
  rollbackRevision: vi.fn(),
}))
vi.mock('@/api/admin-revisions', () => api)

function item(partial: Partial<RevisionListItem> & { revision_no: number }): RevisionListItem {
  return {
    id: `rev-${partial.revision_no}`,
    content_type: 'tool',
    content_id: 't1',
    action: 'update',
    change_summary: [],
    actor_name: 'admin',
    note: '',
    created_at: '2026-10-01T00:00:00Z',
    ...partial,
  }
}

function paged(items: RevisionListItem[]) {
  return { items, total: items.length, page: 1, pageSize: 50, totalPages: 1, suggestions: [] }
}

function buttonByText(wrapper: ReturnType<typeof mount>, text: string) {
  const btn = wrapper.findAll('button').find((b) => b.text().includes(text))
  if (!btn) throw new Error(`未找到按钮: ${text}`)
  return btn
}

describe('ContentRevisionsPanel', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    api.listRevisions.mockReset()
    api.getRevisionDiff.mockReset()
    api.rollbackRevision.mockReset()
    vi.restoreAllMocks()
  })

  it('渲染历史列表，并可按版本查看字段级差异', async () => {
    api.listRevisions.mockResolvedValue(
      paged([
        item({ revision_no: 2, change_summary: ['name'], action: 'update' }),
        item({ revision_no: 1, action: 'create' }),
      ]),
    )
    api.getRevisionDiff.mockResolvedValue({
      revision: { revision_no: 2, snapshot: {} },
      previous: { revision_no: 1, snapshot: {} },
      changes: [{ field: 'name', before: '旧名称', after: '新名称' }],
    })

    const wrapper = mount(ContentRevisionsPanel, {
      props: { contentType: 'tool', contentId: 't1' },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('#2')
    expect(wrapper.text()).toContain('#1')
    // 变更字段以中文标签展示
    expect(wrapper.text()).toContain('名称')

    await buttonByText(wrapper, '查看差异').trigger('click')
    await flushPromises()

    expect(api.getRevisionDiff).toHaveBeenCalledWith('tool', 't1', 2)
    expect(wrapper.text()).toContain('旧名称')
    expect(wrapper.text()).toContain('新名称')
  })

  it('回滚需二次确认，成功后刷新列表并通知父组件', async () => {
    api.listRevisions.mockResolvedValue(paged([item({ revision_no: 1, action: 'create' })]))
    api.rollbackRevision.mockResolvedValue({ revision_no: 2 })
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true)

    const wrapper = mount(ContentRevisionsPanel, {
      props: { contentType: 'resource', contentId: 'r1' },
    })
    await flushPromises()

    await buttonByText(wrapper, '回滚').trigger('click')
    await flushPromises()

    expect(confirmSpy).toHaveBeenCalled()
    expect(api.rollbackRevision).toHaveBeenCalledWith('resource', 'r1', 1)
    expect(wrapper.emitted('rolled-back')).toBeTruthy()
    // 回滚后重新拉取列表
    expect(api.listRevisions).toHaveBeenCalledTimes(2)
  })

  it('取消二次确认时不发起回滚', async () => {
    api.listRevisions.mockResolvedValue(paged([item({ revision_no: 1, action: 'create' })]))
    vi.spyOn(window, 'confirm').mockReturnValue(false)

    const wrapper = mount(ContentRevisionsPanel, {
      props: { contentType: 'wiki', contentId: 'w1' },
    })
    await flushPromises()

    await buttonByText(wrapper, '回滚').trigger('click')
    await flushPromises()

    expect(api.rollbackRevision).not.toHaveBeenCalled()
    expect(wrapper.emitted('rolled-back')).toBeFalsy()
  })
})
