// 后台「普通用户管理」页测试：
//   1) 默认按第 1 页加载，渲染用户名 / 状态 / 个人数据计数 / 操作列；
//   2) 新建用户：非法用户名 / 弱密码被前端拦截，合法提交调用 createUser 并刷新；
//   3) 状态筛选触发带 status 的重新加载（并重置到第 1 页）；
//   4) 禁用 / 启用：调用 updateUser 并就地更新状态；
//   5) 重置密码：弱密码前端拦截，合法密码调用接口；
//   6) 删除：二次确认后调用删除接口并刷新；
//   7) 详情：打开抽屉并渲染收藏 / 最近使用 / 常用种子；
//   8) 导出 CSV：调用导出接口。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory } from 'vue-router'
import AdminUsersView from '@/views/admin/AdminUsersView.vue'
import type { PlatformUserDetail, PlatformUserRow } from '@/types/admin'

const api = vi.hoisted(() => ({
  listUsers: vi.fn(),
  createUser: vi.fn(),
  getUserDetail: vi.fn(),
  updateUser: vi.fn(),
  resetUserPassword: vi.fn(),
  deleteUser: vi.fn(),
  exportUsersCsv: vi.fn(),
}))
vi.mock('@/api/admin-users', () => api)

function makeRows(): PlatformUserRow[] {
  return [
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
}

const detail: PlatformUserDetail = {
  id: 'u1',
  username: 'alice',
  is_active: true,
  last_login_at: null,
  created_at: '2026-10-01T00:00:00Z',
  updated_at: '2026-10-01T00:00:00Z',
  favorite_count: 1,
  recent_count: 1,
  seed_count: 1,
  favorites: [
    { id: 'f1', kind: 'tool', object_id: 'redstone', label: '红石资料', url: '/tools/redstone', created_at: '2026-10-01T00:00:00Z' },
  ],
  recents: [
    { id: 'r1', kind: 'item', object_id: 'minecraft:stone', label: '石头', url: '/items/minecraft:stone', visited_at: '2026-10-02T00:00:00Z' },
  ],
  seeds: [
    { id: 's1', name: '测试种子', seed: '123456789', note: '备注', created_at: '2026-10-01T00:00:00Z', updated_at: '2026-10-01T00:00:00Z' },
  ],
}

function makeRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/admin/users', component: { template: '<div />' } }],
  })
}

function buttonByText(wrapper: ReturnType<typeof mount>, text: string) {
  return wrapper.findAll('button').find((b) => b.text().includes(text))
}

async function mountView() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const router = makeRouter()
  await router.push('/admin/users')
  await router.isReady()
  const wrapper = mount(AdminUsersView, {
    global: { plugins: [pinia, router], stubs: { teleport: true } },
  })
  await flushPromises()
  return wrapper
}

describe('后台普通用户管理', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    // jsdom 未实现 URL.createObjectURL，导出下载需要打桩。
    URL.createObjectURL = vi.fn(() => 'blob:mock')
    URL.revokeObjectURL = vi.fn()
    // 避免临时 <a> 的 click 触发 jsdom 导航告警。
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    api.listUsers.mockResolvedValue({
      items: makeRows(),
      total: 2,
      page: 1,
      pageSize: 15,
      totalPages: 1,
      suggestions: [],
    })
    api.getUserDetail.mockResolvedValue(detail)
    api.createUser.mockResolvedValue(makeRows()[0])
    api.updateUser.mockResolvedValue({ ...makeRows()[0], is_active: false })
    api.resetUserPassword.mockResolvedValue(undefined)
    api.deleteUser.mockResolvedValue(undefined)
    api.exportUsersCsv.mockResolvedValue({ blob: new Blob(['x']), truncated: false })
  })

  it('默认加载并渲染用户名、状态、计数与操作列', async () => {
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
    expect(cells[0].slice(0, 7)).toEqual([
      'alice',
      '启用',
      expect.any(String),
      '从未登录',
      '2',
      '1',
      '3',
    ])
    expect(cells[0][7]).toContain('详情')
    expect(cells[0][7]).toContain('禁用')
    expect(cells[0][7]).toContain('重置密码')
    expect(cells[0][7]).toContain('删除')
    expect(cells[1].slice(0, 7)).toEqual([
      'bob',
      '禁用',
      expect.any(String),
      expect.any(String),
      '0',
      '0',
      '0',
    ])
    expect(cells[1][7]).toContain('启用')
  })

  it('新建用户：非法用户名或弱密码被拦截，合法提交调用 createUser 并刷新', async () => {
    const wrapper = await mountView()
    api.listUsers.mockClear()

    const nameInput = wrapper.find('input[aria-label="新用户用户名"]')
    const pwdInput = wrapper.find('input[aria-label="新用户初始密码"]')

    // 非法用户名（数字开头）被前端拦截。
    await nameInput.setValue('1bad')
    await pwdInput.setValue('good-password-123')
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(api.createUser).not.toHaveBeenCalled()

    // 弱密码被前端拦截。
    await nameInput.setValue('carol')
    await pwdInput.setValue('short')
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(api.createUser).not.toHaveBeenCalled()

    // 合法提交并刷新列表。
    await pwdInput.setValue('good-password-123')
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(api.createUser).toHaveBeenCalledWith({ username: 'carol', password: 'good-password-123' })
    expect(api.listUsers).toHaveBeenCalled()
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

  it('禁用用户：调用 updateUser 并就地更新状态', async () => {
    const wrapper = await mountView()
    const row = wrapper.findAll('tbody tr')[0]
    await row.findAll('button').find((b) => b.text() === '禁用')!.trigger('click')
    await flushPromises()

    expect(api.updateUser).toHaveBeenCalledWith({ id: 'u1', is_active: false })
    const toggled = wrapper
      .findAll('tbody tr')[0]
      .findAll('button')
      .find((b) => b.text() === '启用')
    expect(toggled).toBeTruthy()
  })

  it('重置密码：弱密码拦截，合法密码调用接口', async () => {
    const wrapper = await mountView()
    const row = wrapper.findAll('tbody tr')[0]
    await row.findAll('button').find((b) => b.text() === '重置密码')!.trigger('click')
    await flushPromises()

    const input = wrapper.find('[role="dialog"] input[type="password"]')
    await input.setValue('short')
    await buttonByText(wrapper, '确认重置')!.trigger('click')
    await flushPromises()
    expect(api.resetUserPassword).not.toHaveBeenCalled()

    await input.setValue('good-password-123')
    await buttonByText(wrapper, '确认重置')!.trigger('click')
    await flushPromises()
    expect(api.resetUserPassword).toHaveBeenCalledWith({ id: 'u1', new_password: 'good-password-123' })
  })

  it('删除用户需二次确认，确认后调用删除接口并刷新', async () => {
    const wrapper = await mountView()
    api.listUsers.mockClear()
    const row = wrapper.findAll('tbody tr')[1]
    await row.findAll('button').find((b) => b.text() === '删除')!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('确定删除「bob」吗')
    await buttonByText(wrapper, '确认删除')!.trigger('click')
    await flushPromises()

    expect(api.deleteUser).toHaveBeenCalledWith('u2')
    expect(api.listUsers).toHaveBeenCalled()
  })

  it('详情：打开抽屉并渲染个人数据明细', async () => {
    const wrapper = await mountView()
    const row = wrapper.findAll('tbody tr')[0]
    await row.findAll('button').find((b) => b.text() === '详情')!.trigger('click')
    await flushPromises()

    expect(api.getUserDetail).toHaveBeenCalledWith('u1')
    const text = wrapper.text()
    expect(text).toContain('红石资料')
    expect(text).toContain('石头')
    expect(text).toContain('测试种子')
    expect(text).toContain('123456789')
  })

  it('导出 CSV：调用导出接口', async () => {
    const wrapper = await mountView()
    await buttonByText(wrapper, '导出 CSV')!.trigger('click')
    await flushPromises()

    expect(api.exportUsersCsv).toHaveBeenCalledWith({ q: undefined, status: undefined })
  })
})
