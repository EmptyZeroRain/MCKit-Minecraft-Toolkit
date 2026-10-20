// 资源管理操作测试：已发布资源必须先下架，删除与下架必须是两个独立动作。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { useUiStore } from '@/stores/ui'

const api = vi.hoisted(() => ({
  listResources: vi.fn(),
  unpublishResource: vi.fn(),
  deleteResource: vi.fn(),
  checkResourceLink: vi.fn(),
  exportResourcesCsv: vi.fn(),
}))
vi.mock('@/api/admin-resources', () => api)

import AdminResourcesView from '@/views/admin/AdminResourcesView.vue'

function resource(status: 'published' | 'hidden') {
  return {
    id: 'r1',
    slug: 'jei',
    name: 'JEI',
    resource_type: 'mod',
    status,
    link_status: 'ok',
    updated_at: '2026-10-01T00:00:00Z',
  }
}

async function mountView(status: 'published' | 'hidden') {
  setActivePinia(createPinia())
  ;(api.listResources as ReturnType<typeof vi.fn>).mockResolvedValue({
    items: [resource(status)],
    total: 1,
    page: 1,
    pageSize: 15,
    totalPages: 1,
  })
  const wrapper = mount(AdminResourcesView, {
    attachTo: document.body,
    global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } },
  })
  await flushPromises()
  return wrapper
}

function button(wrapper: ReturnType<typeof mount>, text: string) {
  return wrapper.findAll('button').find((item) => item.text() === text)
}

describe('资源管理 · 下架与删除', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    document.body.innerHTML = ''
    ;(api.unpublishResource as ReturnType<typeof vi.fn>).mockResolvedValue(undefined)
    ;(api.deleteResource as ReturnType<typeof vi.fn>).mockResolvedValue(undefined)
  })

  it('已发布资源显示下架按钮，点击后调用下架接口', async () => {
    const wrapper = await mountView('published')
    expect(button(wrapper, '下架')).toBeTruthy()

    await button(wrapper, '下架')!.trigger('click')
    await flushPromises()

    expect(api.unpublishResource).toHaveBeenCalledWith('r1')
    expect(api.deleteResource).not.toHaveBeenCalled()
  })

  it('已发布资源点击删除时先提示下架，不弹出删除确认', async () => {
    const wrapper = await mountView('published')
    const ui = useUiStore()

    await button(wrapper, '删除')!.trigger('click')
    await flushPromises()

    expect(ui.toasts.at(-1)?.message).toBe('请先下架该资源，再执行删除')
    expect(api.deleteResource).not.toHaveBeenCalled()
    expect(document.querySelector('[role="dialog"]')).toBeNull()
  })

  it('已下架资源确认后可以删除', async () => {
    const wrapper = await mountView('hidden')

    await button(wrapper, '删除')!.trigger('click')
    await flushPromises()
    expect(document.querySelector('[role="dialog"]')).not.toBeNull()

    const confirm = Array.from(document.querySelectorAll('button')).find(
      (item) => item.textContent?.trim() === '确认删除',
    )
    confirm!.click()
    await flushPromises()

    expect(api.deleteResource).toHaveBeenCalledWith('r1')
  })
})
