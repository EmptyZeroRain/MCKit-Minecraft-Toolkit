// 后台「普通用户管理」列表页测试：
//   1) 默认按第 1 页加载，渲染用户名 / 状态 / 个人数据计数；
//   2) 状态筛选触发带 status 的重新加载（并重置到第 1 页）。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory } from 'vue-router'
import AdminUsersView from '@/views/admin/AdminUsersView.vue'
import type { PlatformUserRow } from '@/types/admin'

const api = vi.hoisted(() => ({ listUsers: vi.fn() }))
vi.mock('@/api/admin-users', () => api)

const rows: PlatformUserRow[] = [
  {
    id: 'u1',
    username: 'alice',
    is_active: true,
    last_login_at: null,
    created_at: '2026-10-01T00:00:00Z',
    favorite_count: 2,
    recent_count: 1,
    seed_count: 3,
  },
  {
    id: 'u2',
    username: 'bob',
    is_active: false,
    last_login_at: '2026-10-02T00:00:00Z',
    created_at: '2026-10-01T00:00:00Z',
    favorite_count: 0,
    recent_count: 0,
    seed_count: 0,
  },
]

function makeRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/admin/users', component: { template: '<div />' } }],
  })
}

async function mountView() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const router = makeRouter()
  await router.push('/admin/users')
  await router.isReady()
  const wrapper = mount(AdminUsersView, { global: { plugins: [pinia, router] } })
  await flushPromises()
  return wrapper
}

describe('后台普通用户列表', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    api.listUsers.mockReset()
    api.listUsers.mockResolvedValue({
      items: rows,
      total: 2,
      page: 1,
      pageSize: 15,
      totalPages: 1,
      suggestions: [],
    })
  })

  it('默认加载并渲染用户名、状态与个人数据计数', async () => {
    const wrapper = await mountView()

    expect(api.listUsers).toHaveBeenCalledWith(
      expect.objectContaining({ q: undefined, status: undefined, page: 1, page_size: 15 }),
    )
    const text = wrapper.text()
    expect(text).toContain('alice')
    expect(text).toContain('bob')
    expect(text).toContain('启用')
    expect(text).toContain('禁用')

    const cells = wrapper.findAll('tbody tr').map((tr) =>
      tr.findAll('td').map((td) => td.text()),
    )
    expect(cells[0]).toEqual(['alice', '启用', expect.any(String), '从未登录', '2', '1', '3'])
    expect(cells[1]).toEqual(['bob', '禁用', expect.any(String), expect.any(String), '0', '0', '0'])
  })

  it('切换状态筛选时带上 status 并重置到第 1 页', async () => {
    const wrapper = await mountView()
    api.listUsers.mockClear()

    await wrapper.find('select').setValue('disabled')
    await flushPromises()

    expect(api.listUsers).toHaveBeenCalledWith(
      expect.objectContaining({ status: 'disabled', page: 1 }),
    )
  })
})
