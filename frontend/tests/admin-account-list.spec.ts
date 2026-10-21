// 统一账号管理列表测试（普通用户 + 后台账号同一列表）：
//   1) 双权限：渲染两类账号与角色下拉，默认请求全部范围；
//   2) 仅 accounts.users：隐藏类型筛选、角色只读、请求收敛为 scope=user；
//   3) 新建：普通用户 8 位密码、后台角色 12 位密码；
//   4) 改角色 / 启停 / 删除预览后删除 / 导出。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import AdminAccountListView from '@/views/admin/AdminAccountListView.vue'
import { useAuthStore } from '@/stores/auth'
import type { AccountDeletePreview, AccountDetail, AccountRow } from '@/api/admin-accounts'
import type { RoleRow } from '@/types/admin'

const api = vi.hoisted(() => ({
  listAccounts: vi.fn(),
  createAccount: vi.fn(),
  getAccountDetail: vi.fn(),
  updateAccount: vi.fn(),
  resetAccountPassword: vi.fn(),
  deleteAccount: vi.fn(),
  deleteAccountPreview: vi.fn(),
  resetAccountMFA: vi.fn(),
  exportAccountsCsv: vi.fn(),
}))
vi.mock('@/api/admin-accounts', () => api)

const rolesApi = vi.hoisted(() => ({ listRoles: vi.fn() }))
vi.mock('@/api/admin-roles', () => rolesApi)

const roles: RoleRow[] = [
  {
    name: 'user',
    label: '普通用户',
    description: '前台用户',
    is_system: true,
    permissions: [],
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  },
  {
    name: 'admin',
    label: '超级管理员',
    description: '全部权限',
    is_system: true,
    permissions: [],
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  },
  {
    name: 'editor',
    label: '内容编辑',
    description: '内容权限',
    is_system: true,
    permissions: [],
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  },
]

function makeRows(): AccountRow[] {
  return [
    {
      id: 'u1',
      user_no: 'UAB12CD34E',
      username: 'alice',
      nickname: '爱丽丝',
      email: 'alice@example.com',
      role: 'user',
      is_active: true,
      last_login_at: null,
      created_at: '2026-10-01T00:00:00Z',
      gift_coins: 120,
      level: 3,
    },
    {
      id: 'a1',
      user_no: 'UZZ98YX76W',
      username: 'root',
      nickname: '',
      email: 'root@example.com',
      role: 'admin',
      is_active: true,
      last_login_at: '2026-10-02T00:00:00Z',
      created_at: '2026-09-01T00:00:00Z',
      gift_coins: 0,
      level: 0,
    },
  ]
}

const detail: AccountDetail = {
  ...makeRows()[0],
  avatar_url: '',
  updated_at: '2026-10-01T00:00:00Z',
  favorite_count: 1,
  recent_count: 0,
  seed_count: 0,
  makeup_cards: 2,
  favorites: [
    {
      id: 'f1',
      kind: 'tool',
      object_id: 'redstone',
      label: '红石资料',
      url: '/tools/redstone',
      created_at: '2026-10-01T00:00:00Z',
    },
  ],
  recents: [],
  seeds: [],
}

const preview: AccountDeletePreview = {
  account_id: 'a1',
  username: 'root',
  role: 'admin',
  is_active: true,
  is_self: false,
  is_last_admin: false,
  favorites: 0,
  recents: 0,
  seeds: 0,
  submissions: 1,
  active_sessions: 2,
  reversible: false,
  notes: [],
}

function makeRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/admin/accounts', component: { template: '<div />' } }],
  })
}

function buttonByText(wrapper: ReturnType<typeof mount>, text: string) {
  return wrapper.findAll('button').find((b) => b.text().includes(text))
}

async function mountView(permissions: string[]) {
  const pinia = createPinia()
  setActivePinia(pinia)
  const auth = useAuthStore()
  auth.user = {
    id: 'actor-1',
    username: 'actor',
    nickname: '操作者',
    role: 'admin',
    permissions,
    is_active: true,
    last_login_at: null,
    created_at: '2026-01-01T00:00:00Z',
  }
  const router = makeRouter()
  await router.push('/admin/accounts')
  await router.isReady()
  const wrapper = mount(AdminAccountListView, {
    global: { plugins: [pinia, router], stubs: { teleport: true } },
  })
  await flushPromises()
  return wrapper
}

describe('统一账号管理列表', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    URL.createObjectURL = vi.fn(() => 'blob:mock')
    URL.revokeObjectURL = vi.fn()
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    rolesApi.listRoles.mockResolvedValue(roles)
    api.listAccounts.mockResolvedValue({
      items: makeRows(),
      total: 2,
      page: 1,
      pageSize: 15,
      totalPages: 1,
      suggestions: [],
    })
    api.getAccountDetail.mockResolvedValue(detail)
    api.deleteAccountPreview.mockResolvedValue(preview)
    api.createAccount.mockResolvedValue(makeRows()[0])
    api.updateAccount.mockImplementation(async (payload: { id: string; role?: string; is_active?: boolean }) => {
      const base = makeRows().find((r) => r.id === payload.id)!
      return { ...base, ...payload }
    })
    api.resetAccountPassword.mockResolvedValue(undefined)
    api.deleteAccount.mockResolvedValue(undefined)
    api.resetAccountMFA.mockResolvedValue(undefined)
    api.exportAccountsCsv.mockResolvedValue({ blob: new Blob(['x']), truncated: false })
  })

  it('双权限：同一列表渲染普通用户与后台账号，并请求全部范围', async () => {
    const wrapper = await mountView(['accounts.admins', 'accounts.users'])

    expect(api.listAccounts).toHaveBeenCalledWith(
      expect.objectContaining({ scope: '', page: 1, page_size: 15 }),
    )
    expect(wrapper.find('select[aria-label="按账号类型筛选"]').exists()).toBe(true)

    const cells = wrapper.findAll('tbody tr').map((tr) => tr.findAll('td').map((td) => td.text()))
    expect(cells[0][0]).toBe('UAB12CD34E')
    expect(cells[0][1]).toContain('alice')
    expect(cells[1][1]).toContain('root')
    expect(cells[0][2]).toContain('普通用户')
    expect(cells[1][2]).toContain('超级管理员')
    // 列表聚焦账号本身：邮箱 / 礼物币 / 等级，不再展示收藏等个人内容计数。
    expect(cells[0][6]).toBe('alice@example.com')
    expect(cells[0][7]).toBe('120')
    expect(cells[0][8]).toBe('Lv.3')
    expect(wrapper.findAll('thead th').map((th) => th.text())).toEqual([
      '用户 ID',
      '用户名',
      '角色',
      '状态',
      '最后登录',
      '注册',
      '邮箱',
      '礼物币',
      '等级',
      '操作',
    ])

    // 双权限下每行都有角色下拉。
    expect(wrapper.find('select[aria-label="修改 alice 的角色"]').exists()).toBe(true)
    expect(wrapper.find('select[aria-label="修改 root 的角色"]').exists()).toBe(true)
  })

  it('仅 accounts.users：隐藏类型筛选，角色只读，请求收敛为普通用户', async () => {
    const wrapper = await mountView(['accounts.users'])

    expect(api.listAccounts).toHaveBeenCalledWith(expect.objectContaining({ scope: 'user' }))
    expect(wrapper.find('select[aria-label="按账号类型筛选"]').exists()).toBe(false)
    expect(wrapper.find('select[aria-label="修改 alice 的角色"]').exists()).toBe(false)
    // 后台账号行不可操作。
    const rootRow = wrapper.findAll('tbody tr')[1]
    expect(rootRow.findAll('button').find((b) => b.text() === '详情')!.attributes('disabled')).toBeDefined()
  })

  it('新建普通用户：用户名与 8 位密码校验后调用统一创建接口', async () => {
    const wrapper = await mountView(['accounts.admins', 'accounts.users'])
    api.listAccounts.mockClear()

    const name = wrapper.find('input[aria-label="新账号用户名"]')
    const pwd = wrapper.find('input[aria-label="新账号初始密码"]')

    await name.setValue('1bad')
    await pwd.setValue('good-password-123')
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(api.createAccount).not.toHaveBeenCalled()

    await name.setValue('carol')
    await pwd.setValue('short')
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(api.createAccount).not.toHaveBeenCalled()

    await pwd.setValue('good-password-123')
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(api.createAccount).toHaveBeenCalledWith({
      username: 'carol',
      password: 'good-password-123',
      role: 'user',
    })
    expect(api.listAccounts).toHaveBeenCalled()
  })

  it('新建后台角色：密码至少 12 位', async () => {
    const wrapper = await mountView(['accounts.admins', 'accounts.users'])
    const name = wrapper.find('input[aria-label="新账号用户名"]')
    const pwd = wrapper.find('input[aria-label="新账号初始密码"]')
    const roleSelect = wrapper.find('form select')

    await name.setValue('ops')
    await roleSelect.setValue('editor')
    await pwd.setValue('eleven-pass')
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(api.createAccount).not.toHaveBeenCalled()

    await pwd.setValue('twelve-pass-1')
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(api.createAccount).toHaveBeenCalledWith({
      username: 'ops',
      password: 'twelve-pass-1',
      role: 'editor',
    })
  })

  it('修改角色：确认后调用 update 接口并就地更新', async () => {
    const wrapper = await mountView(['accounts.admins', 'accounts.users'])
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(true)

    const select = wrapper.find('select[aria-label="修改 alice 的角色"]')
    await select.setValue('editor')
    await flushPromises()

    expect(confirm).toHaveBeenCalled()
    expect(api.updateAccount).toHaveBeenCalledWith({ id: 'u1', role: 'editor' })
    expect(
      (wrapper.find('select[aria-label="修改 alice 的角色"]').element as HTMLSelectElement).value,
    ).toBe('editor')
  })

  it('启停：调用 update 接口并就地更新状态', async () => {
    const wrapper = await mountView(['accounts.admins', 'accounts.users'])
    const row = wrapper.findAll('tbody tr')[0]
    await row.findAll('button').find((b) => b.text() === '停用')!.trigger('click')
    await flushPromises()

    expect(api.updateAccount).toHaveBeenCalledWith({ id: 'u1', is_active: false })
    expect(
      wrapper.findAll('tbody tr')[0].findAll('button').find((b) => b.text() === '启用'),
    ).toBeTruthy()
  })

  it('删除：先拉取影响预览，确认后调用删除并刷新', async () => {
    const wrapper = await mountView(['accounts.admins', 'accounts.users'])
    api.listAccounts.mockClear()

    const row = wrapper.findAll('tbody tr')[1]
    await row.findAll('button').find((b) => b.text() === '删除')!.trigger('click')
    await flushPromises()

    expect(api.deleteAccountPreview).toHaveBeenCalledWith('a1')
    expect(wrapper.text()).toContain('撤销 2 个登录会话')
    await buttonByText(wrapper, '确认删除')!.trigger('click')
    await flushPromises()

    expect(api.deleteAccount).toHaveBeenCalledWith('a1')
    expect(api.listAccounts).toHaveBeenCalled()
  })

  it('详情展示补签卡持有量', async () => {
    const wrapper = await mountView(['accounts.admins', 'accounts.users'])

    const row = wrapper.findAll('tbody tr')[0]
    await row.findAll('button').find((b) => b.text() === '详情')!.trigger('click')
    await flushPromises()

    expect(api.getAccountDetail).toHaveBeenCalledWith('u1')
    expect(wrapper.text()).toContain('礼物与签到')
    expect(wrapper.text()).toContain('补签卡')
    expect(wrapper.text()).toContain('2 张')
  })

  it('导出：按当前筛选调用 CSV 导出', async () => {
    const wrapper = await mountView(['accounts.admins', 'accounts.users'])
    await wrapper.find('select[aria-label="按状态筛选"]').setValue('disabled')
    await flushPromises()
    api.exportAccountsCsv.mockClear()

    await buttonByText(wrapper, '导出 CSV')!.trigger('click')
    await flushPromises()

    expect(api.exportAccountsCsv).toHaveBeenCalledWith(
      expect.objectContaining({ status: 'disabled', scope: '' }),
    )
  })
})
