// 内容健康 / 关键字屏蔽：新增、启停与列表展示。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import type { ContentKeyword } from '@/types/content-filter'

const api = vi.hoisted(() => ({
  listContentKeywords: vi.fn(),
  createContentKeyword: vi.fn(),
  updateContentKeyword: vi.fn(),
  deleteContentKeyword: vi.fn(),
}))
vi.mock('@/api/admin-content-filter', () => api)

const AdminKeywordBlocklistView = (
  await import('@/views/admin/AdminKeywordBlocklistView.vue')
).default

const KEYWORD: ContentKeyword = {
  id: 'k1',
  keyword: '外挂',
  enabled: true,
  note: '社区内容安全',
  created_by: null,
  created_at: '2026-10-01T00:00:00Z',
  updated_at: '2026-10-01T00:00:00Z',
}

function mountView() {
  const pinia = createPinia()
  setActivePinia(pinia)
  return mount(AdminKeywordBlocklistView, { global: { plugins: [pinia] } })
}

beforeEach(() => {
  vi.clearAllMocks()
  api.listContentKeywords.mockResolvedValue([KEYWORD])
  api.createContentKeyword.mockResolvedValue({ ...KEYWORD, id: 'k2', keyword: '代练' })
  api.updateContentKeyword.mockResolvedValue({ ...KEYWORD, enabled: false })
  api.deleteContentKeyword.mockResolvedValue(undefined)
})

describe('关键字屏蔽', () => {
  it('加载列表并展示启用状态', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(api.listContentKeywords).toHaveBeenCalled()
    expect(wrapper.text()).toContain('外挂')
    expect(wrapper.text()).toContain('已启用')
  })

  it('新增关键字后重新加载列表', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.find('input[placeholder*="关键字"]').setValue('代练')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(api.createContentKeyword).toHaveBeenCalledWith('代练', '')
    expect(api.listContentKeywords).toHaveBeenCalledTimes(2)
  })

  it('停用关键字调用更新接口', async () => {
    const wrapper = mountView()
    await flushPromises()

    const disable = wrapper.findAll('button').find((b) => b.text() === '停用')
    await disable!.trigger('click')
    await flushPromises()

    expect(api.updateContentKeyword).toHaveBeenCalledWith({
      id: 'k1',
      keyword: '外挂',
      note: '社区内容安全',
      enabled: false,
    })
  })
})
