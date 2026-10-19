// 搜索结果体验测试（TODO §5）
// 覆盖：无结果时的可尝试关键词、排序切换、建议词点击重搜。
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory } from 'vue-router'

vi.mock('@/api/public', () => ({ search: vi.fn() }))

import { search } from '@/api/public'
import SearchResultsView from '@/views/SearchResultsView.vue'

function makeRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: { template: '<div />' } },
      { path: '/search', name: 'search', component: { template: '<div />' } },
      { path: '/resources', component: { template: '<div />' } },
    ],
  })
}

interface MockPage {
  items: unknown[]
  total: number
  page?: number
  pageSize?: number
  totalPages?: number
  suggestions?: string[]
}

async function mountView(page: MockPage, query: Record<string, string> = { q: '红石机器' }) {
  setActivePinia(createPinia())
  ;(search as ReturnType<typeof vi.fn>).mockResolvedValue({
    page: 1,
    pageSize: 15,
    totalPages: 0,
    suggestions: [],
    ...page,
  })
  const router = makeRouter()
  await router.push({ path: '/search', query })
  await router.isReady()
  const wrapper = mount(SearchResultsView, {
    global: { plugins: [router], stubs: { Pagination: true } },
  })
  await flushPromises()
  return { wrapper, router }
}

describe('搜索结果页 · 无结果引导', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('无结果时展示后端建议的关键词', async () => {
    const { wrapper } = await mountView({ items: [], total: 0, suggestions: ['红石', '红石粉'] })
    const text = wrapper.text()
    expect(text).toContain('没有找到相关结果')
    expect(text).toContain('换个关键词试试')
    expect(text).toContain('红石粉')
  })

  it('没有建议时不渲染建议区块', async () => {
    const { wrapper } = await mountView({ items: [], total: 0, suggestions: [] })
    expect(wrapper.text()).not.toContain('换个关键词试试')
  })

  it('点击建议词会以该关键词重新搜索并同步 URL', async () => {
    const { wrapper, router } = await mountView({ items: [], total: 0, suggestions: ['红石'] })
    const btn = wrapper.findAll('button').find((b) => b.text() === '红石')
    expect(btn).toBeTruthy()
    await btn!.trigger('click')
    await flushPromises()

    expect(search).toHaveBeenLastCalledWith(expect.objectContaining({ q: '红石', page: 1 }))
    expect(router.currentRoute.value.query.q).toBe('红石')
  })
})

describe('搜索结果页 · 排序', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('默认按相关度请求', async () => {
    await mountView({ items: [], total: 0 })
    expect(search).toHaveBeenLastCalledWith(expect.objectContaining({ sort: 'relevance' }))
  })

  it('切换到「最近更新」后按 updated_desc 请求并写入 URL', async () => {
    const { wrapper, router } = await mountView({ items: [], total: 0 })
    const btn = wrapper.findAll('button').find((b) => b.text() === '最近更新')
    expect(btn).toBeTruthy()
    await btn!.trigger('click')
    await flushPromises()

    expect(search).toHaveBeenLastCalledWith(expect.objectContaining({ sort: 'updated_desc' }))
    expect(router.currentRoute.value.query.sort).toBe('updated_desc')
  })

  it('URL 中的 sort 参数会被还原为对应排序', async () => {
    await mountView({ items: [], total: 0 }, { q: '石头', sort: 'updated_desc' })
    expect(search).toHaveBeenLastCalledWith(expect.objectContaining({ sort: 'updated_desc' }))
  })
})

describe('搜索结果页 · 版本筛选', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('默认不限版本（不传 game_version）', async () => {
    await mountView({ items: [], total: 0 })
    expect(search).toHaveBeenLastCalledWith(expect.objectContaining({ game_version: undefined }))
  })

  it('选择版本后按该版本查询并写入 URL', async () => {
    const { wrapper, router } = await mountView({ items: [], total: 0 })
    const select = wrapper.find('#search-version')
    expect(select.exists()).toBe(true)
    await select.setValue('1.20')
    await flushPromises()

    expect(search).toHaveBeenLastCalledWith(expect.objectContaining({ game_version: '1.20' }))
    expect(router.currentRoute.value.query.game_version).toBe('1.20')
  })

  it('URL 中的 game_version 参数会被还原', async () => {
    await mountView({ items: [], total: 0 }, { q: '石头', game_version: '1.19' })
    expect(search).toHaveBeenLastCalledWith(expect.objectContaining({ game_version: '1.19' }))
  })
})
