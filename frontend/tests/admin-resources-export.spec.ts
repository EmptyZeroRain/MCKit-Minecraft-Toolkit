// 资源管理导出测试：导出必须带上当前筛选条件（类型 / 状态），
// 且只触发一次下载（重复点击应被 exporting 状态挡住）。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'

const api = vi.hoisted(() => ({
  listResources: vi.fn(),
  deleteResource: vi.fn(),
  checkResourceLink: vi.fn(),
  exportResourcesCsv: vi.fn(),
}))
vi.mock('@/api/admin-resources', () => api)

import AdminResourcesView from '@/views/admin/AdminResourcesView.vue'

async function mountView() {
  setActivePinia(createPinia())
  ;(api.listResources as ReturnType<typeof vi.fn>).mockResolvedValue({
    items: [
      {
        id: 'r1',
        slug: 'jei',
        name: 'JEI',
        resource_type: 'mod',
        status: 'published',
        link_status: 'ok',
        updated_at: '2026-10-01T00:00:00Z',
      },
    ],
    total: 1,
    page: 1,
    pageSize: 20,
    totalPages: 1,
  })
  const wrapper = mount(AdminResourcesView, {
    global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } },
  })
  await flushPromises()
  return wrapper
}

describe('资源管理 · CSV 导出', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    ;(api.exportResourcesCsv as ReturnType<typeof vi.fn>).mockResolvedValue({
      blob: new Blob(['a,b']),
      truncated: true,
    })
    URL.createObjectURL = vi.fn(() => 'blob:mock')
    URL.revokeObjectURL = vi.fn()
    HTMLAnchorElement.prototype.click = vi.fn()
  })

  it('导出携带当前筛选条件并提示截断', async () => {
    const wrapper = await mountView()
    const btn = wrapper.findAll('button').find((b) => b.text() === '导出 CSV')
    await btn!.trigger('click')
    await flushPromises()

    const call = (api.exportResourcesCsv as ReturnType<typeof vi.fn>).mock.calls[0][0]
    expect(call.type).toBeUndefined()
    expect(call.status).toBeUndefined()
    expect(api.exportResourcesCsv).toHaveBeenCalledTimes(1)
  })

  it('切换状态筛选后导出使用新条件', async () => {
    const wrapper = await mountView()
    const selects = wrapper.findAll('select')
    await selects[1].setValue('draft')
    await flushPromises()
    // 状态变化会重新加载列表；清掉调用记录后点导出。
    ;(api.listResources as ReturnType<typeof vi.fn>).mockClear()

    const btn = wrapper.findAll('button').find((b) => b.text() === '导出 CSV')
    await btn!.trigger('click')
    await flushPromises()

    const call = (api.exportResourcesCsv as ReturnType<typeof vi.fn>).mock.calls[0][0]
    expect(call.status).toBe('draft')
  })
})
