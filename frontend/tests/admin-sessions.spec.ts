// 系统管理 / 会话管理：全站会话列表、当前会话保护与踢下线。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import type { AdminSessionItem } from '@/api/admin-sessions'

const api = vi.hoisted(() => ({
  listAdminSessions: vi.fn(),
  revokeAdminSession: vi.fn(),
}))
vi.mock('@/api/admin-sessions', () => api)

const AdminSessionsView = (await import('@/views/admin/AdminSessionsView.vue')).default

const CURRENT: AdminSessionItem = {
  id: 'aaaaaaaaaaaaaaaa',
  username: 'admin',
  role: 'admin',
  account_type: 'admin',
  created_at: '2026-10-01T00:00:00Z',
  last_seen_at: '2026-10-01T01:00:00Z',
  expires_at: '2026-10-04T00:00:00Z',
  current: true,
}

const USER_SESSION: AdminSessionItem = {
  id: 'bbbbbbbbbbbbbbbb',
  username: 'alice',
  role: 'user',
  account_type: 'user',
  created_at: '2026-10-01T02:00:00Z',
  last_seen_at: '2026-10-01T03:00:00Z',
  expires_at: '2026-10-04T02:00:00Z',
  current: false,
}

function paged(items: AdminSessionItem[]) {
  return { items, total: items.length, page: 1, pageSize: 20, totalPages: 1 }
}

function mountView() {
  const pinia = createPinia()
  setActivePinia(pinia)
  return mount(AdminSessionsView, { global: { plugins: [pinia] } })
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.stubGlobal('confirm', vi.fn(() => true))
  api.listAdminSessions.mockResolvedValue(paged([CURRENT, USER_SESSION]))
  api.revokeAdminSession.mockResolvedValue(undefined)
})

describe('后台会话管理', () => {
  it('同时展示管理员与普通用户会话，当前会话禁止踢下线', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(api.listAdminSessions).toHaveBeenCalledWith(
      expect.objectContaining({ page: 1, page_size: 20, account_type: 'all' }),
    )
    const text = wrapper.text()
    expect(text).toContain('admin')
    expect(text).toContain('alice')
    expect(text).toContain('当前会话')
    expect(text).toContain('普通用户')

    const currentButton = wrapper.findAll('button').find((b) => b.text() === '当前会话')!
    expect(currentButton.attributes('disabled')).toBeDefined()
  })

  it('踢下线普通用户会话后刷新列表', async () => {
    const wrapper = mountView()
    await flushPromises()

    const kick = wrapper.findAll('button').find((b) => b.text() === '踢下线')!
    await kick.trigger('click')
    await flushPromises()

    expect(api.revokeAdminSession).toHaveBeenCalledWith('bbbbbbbbbbbbbbbb')
    expect(api.listAdminSessions).toHaveBeenCalledTimes(2)
  })

  it('按管理员类型筛选并携带关键字', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.find('input[type="search"]').setValue('admin')
    await wrapper.find('select').setValue('admin')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(api.listAdminSessions).toHaveBeenLastCalledWith(
      expect.objectContaining({ q: 'admin', account_type: 'admin', page: 1 }),
    )
  })
})
