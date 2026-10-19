// 搜索词统计测试（TODO §5）
// 覆盖两层：展示辅助纯函数 + 管理页（热门 / 无结果词）。
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { isHighNoResult, noResultRatio } from '@/utils/search-term'
import type { SearchTermStat } from '@/types/search-term'

// ===== 纯函数 =====

describe('搜索词统计展示辅助函数', () => {
  it('无结果占比处理边界，不产生 NaN 或越界', () => {
    expect(noResultRatio({ hits: 4, no_result_hits: 1 })).toBe(25)
    expect(noResultRatio({ hits: 0, no_result_hits: 0 })).toBe(0)
    expect(noResultRatio({ hits: 2, no_result_hits: 5 })).toBe(100)
  })

  it('高频无结果词按阈值判定', () => {
    expect(isHighNoResult({ no_result_hits: 3 })).toBe(true)
    expect(isHighNoResult({ no_result_hits: 2 })).toBe(false)
    expect(isHighNoResult({ no_result_hits: 2 }, 2)).toBe(true)
  })
})

// ===== 视图 =====

vi.mock('@/api/admin-search-terms', () => ({ listSearchTerms: vi.fn() }))

import { listSearchTerms } from '@/api/admin-search-terms'
import AdminSearchTermsView from '@/views/admin/AdminSearchTermsView.vue'

const TERMS: SearchTermStat[] = [
  {
    id: 't1',
    term: '红石',
    hits: 5,
    no_result_hits: 5,
    failed_hits: 0,
    last_result_count: 0,
    first_seen_at: '2026-01-15T00:00:00Z',
    last_seen_at: '2026-01-15T01:00:00Z',
  },
  {
    id: 't2',
    term: '钻石',
    hits: 9,
    no_result_hits: 0,
    failed_hits: 2,
    last_result_count: 3,
    first_seen_at: '2026-01-15T00:00:00Z',
    last_seen_at: '2026-01-15T02:00:00Z',
  },
]

async function mountView(reply: { items: SearchTermStat[]; total: number }) {
  setActivePinia(createPinia())
  ;(listSearchTerms as ReturnType<typeof vi.fn>).mockResolvedValue({
    items: reply.items,
    total: reply.total,
    page: 1,
    pageSize: 20,
    totalPages: 1,
    suggestions: [],
  })
  const wrapper = mount(AdminSearchTermsView, {
    global: { stubs: { Pagination: true } },
  })
  await flushPromises()
  return wrapper
}

describe('搜索词统计页', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('默认请求热门搜索（kind=hot）', async () => {
    await mountView({ items: TERMS, total: 2 })
    expect(listSearchTerms).toHaveBeenCalledWith(expect.objectContaining({ kind: 'hot', page: 1 }))
  })

  it('展示关键词、次数与无结果占比', async () => {
    const wrapper = await mountView({ items: TERMS, total: 2 })
    const text = wrapper.text()
    expect(text).toContain('红石')
    expect(text).toContain('钻石')
    expect(text).toContain('100%')
  })

  it('切到「高频无结果词」后按 kind=no_result 重新请求', async () => {
    const wrapper = await mountView({ items: TERMS, total: 2 })
    const btn = wrapper.findAll('button').find((b) => b.text() === '高频无结果词')
    expect(btn).toBeTruthy()
    await btn!.trigger('click')
    await flushPromises()

    expect(listSearchTerms).toHaveBeenLastCalledWith(
      expect.objectContaining({ kind: 'no_result', page: 1 }),
    )
  })

  it('切到「查询失败词」后按 kind=failed 重新请求，并展示失败次数', async () => {
    const wrapper = await mountView({ items: TERMS, total: 2 })
    const btn = wrapper.findAll('button').find((b) => b.text() === '查询失败词')
    expect(btn).toBeTruthy()
    await btn!.trigger('click')
    await flushPromises()

    expect(listSearchTerms).toHaveBeenLastCalledWith(
      expect.objectContaining({ kind: 'failed', page: 1 }),
    )
    expect(wrapper.text()).toContain('失败次数')
  })

  it('无记录时给出空态提示', async () => {
    const wrapper = await mountView({ items: [], total: 0 })
    expect(wrapper.text()).toContain('暂无搜索记录')
  })
})
