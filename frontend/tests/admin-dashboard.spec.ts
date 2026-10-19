// 管理仪表盘测试：概览卡片应包含「用户总数」，并来自普通用户列表接口的 total。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory } from 'vue-router'
import AdminDashboardView from '@/views/admin/AdminDashboardView.vue'
import { useAuthStore } from '@/stores/auth'
import type { AdminUser } from '@/types/admin'

const api = vi.hoisted(() => ({
  listResources: vi.fn(),
  listTools: vi.fn(),
  listWiki: vi.fn(),
  listFeedback: vi.fn(),
  listUsers: vi.fn(),
}))
vi.mock('@/api/admin-resources', () => ({ listResources: api.listResources }))
vi.mock('@/api/admin-tools', () => ({ listTools: api.listTools }))
vi.mock('@/api/admin-wiki', () => ({ listWiki: api.listWiki }))
vi.mock('@/api/admin-system', () => ({ listFeedback: api.listFeedback }))
vi.mock('@/api/admin-users', () => ({ listUsers: api.listUsers }))

function paged(total: number) {
  return { items: [], total, page: 1, pageSize: 1, totalPages: total, suggestions: [] }
}

async function mountView() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const auth = useAuthStore()
  auth.user = { id: 'a1', username: 'admin', role: 'admin' } as AdminUser

  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/admin', component: { template: '<div />' } },
      { path: '/admin/resources', component: { template: '<div />' } },
      { path: '/admin/tools', component: { template: '<div />' } },
      { path: '/admin/wiki', component: { template: '<div />' } },
      { path: '/admin/feedback', component: { template: '<div />' } },
      { path: '/admin/users', component: { template: '<div />' } },
      { path: '/admin/resources/new', component: { template: '<div />' } },
      { path: '/admin/tools/new', component: { template: '<div />' } },
      { path: '/admin/wiki/new', component: { template: '<div />' } },
    ],
  })
  await router.push('/admin')
  await router.isReady()

  const wrapper = mount(AdminDashboardView, { global: { plugins: [pinia, router] } })
  await flushPromises()
  return wrapper
}

describe('管理仪表盘 · 概览卡片', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    api.listResources.mockReset().mockResolvedValue(paged(12))
    api.listTools.mockReset().mockResolvedValue(paged(18))
    api.listWiki.mockReset().mockResolvedValue(paged(17))
    api.listFeedback.mockReset().mockResolvedValue(paged(4))
    api.listUsers.mockReset().mockResolvedValue(paged(7))
  })

  it('展示「用户总数」并取普通用户列表的 total', async () => {
    const wrapper = await mountView()

    expect(api.listUsers).toHaveBeenCalledWith({ page: 1, page_size: 1 })
    expect(wrapper.text()).toContain('用户总数')
    expect(wrapper.text()).toContain('7')
    expect(wrapper.text()).toContain('资源总数')
  })
})
