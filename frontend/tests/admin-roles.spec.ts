// 后台「角色管理」页测试：
//   1) 列表渲染（内置 / 自定义、权限点数量、删除按钮对内置角色禁用）；
//   2) 新建角色：分组全选 + 保存，校验提交载荷；
//   3) 非法角色标识在前端拦截，不发请求；
//   4) 编辑内置 admin：权限点勾选被禁用（后端亦拒绝削权）；
//   5) 删除自定义角色：二次确认后调用删除接口；
//   6) 加载失败展示错误态。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import AdminRolesView from '@/views/admin/AdminRolesView.vue'
import { ApiError } from '@/types/api'
import type { PermissionGroup, RoleRow } from '@/types/admin'

const api = vi.hoisted(() => ({
  listRoles: vi.fn(),
  listPermissions: vi.fn(),
  createRole: vi.fn(),
  updateRole: vi.fn(),
  deleteRole: vi.fn(),
  getRoleImpact: vi.fn(),
}))
vi.mock('@/api/admin-roles', () => api)

const GROUPS: PermissionGroup[] = [
  {
    key: 'content',
    label: '内容管理',
    permissions: [
      { key: 'content.resources', label: '资源管理' },
      { key: 'content.tools', label: '工具管理' },
    ],
  },
  {
    key: 'ops',
    label: '运营',
    permissions: [{ key: 'ops.mcsrc', label: 'MC 源码库' }],
  },
]

function role(partial: Partial<RoleRow> & { name: string }): RoleRow {
  return {
    label: partial.name,
    description: '',
    permissions: [],
    is_system: false,
    created_at: '2026-10-01T00:00:00Z',
    updated_at: '2026-10-01T00:00:00Z',
    ...partial,
  }
}

const ROLES: RoleRow[] = [
  role({
    name: 'admin',
    label: '超级管理员',
    description: '内置全量权限',
    is_system: true,
    permissions: ['content.resources', 'content.tools', 'ops.mcsrc'],
  }),
  role({ name: 'reviewer', label: '内容审核员', description: '只看内容', permissions: ['content.tools'] }),
]

function buttonByText(wrapper: ReturnType<typeof mount>, text: string) {
  return wrapper.findAll('button').find((b) => b.text().includes(text))
}

async function mountView() {
  setActivePinia(createPinia())
  const wrapper = mount(AdminRolesView, {
    global: { stubs: { teleport: true } },
  })
  await flushPromises()
  return wrapper
}

describe('后台角色管理', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.listRoles.mockResolvedValue(ROLES)
    api.listPermissions.mockResolvedValue(GROUPS)
    api.createRole.mockResolvedValue(role({ name: 'reviewer2' }))
    api.updateRole.mockResolvedValue(role({ name: 'reviewer' }))
    api.deleteRole.mockResolvedValue(undefined)
    api.getRoleImpact.mockResolvedValue({
      role: 'reviewer',
      label: '内容审核员',
      is_system: false,
      affected_accounts: 2,
      current_permissions: ['content.tools'],
      next_permissions: ['content.tools', 'ops.mcsrc'],
      added: ['ops.mcsrc'],
      removed: [],
      notes: ['权限变更会立即影响该角色下所有账号的下一次鉴权'],
    })
  })

  it('渲染角色列表、权限点数量与类型徽标', async () => {
    const wrapper = await mountView()

    expect(api.listRoles).toHaveBeenCalled()
    expect(api.listPermissions).toHaveBeenCalled()
    const text = wrapper.text()
    expect(text).toContain('超级管理员')
    expect(text).toContain('内容审核员')
    expect(text).toContain('内置')
    expect(text).toContain('自定义')
    expect(wrapper.findAll('tbody tr').length).toBe(2)
  })

  it('内置角色的删除按钮被禁用，自定义角色可删除', async () => {
    const wrapper = await mountView()
    const rows = wrapper.findAll('tbody tr')

    const builtinDelete = rows[0].findAll('button').find((b) => b.text().includes('删除'))
    const customDelete = rows[1].findAll('button').find((b) => b.text().includes('删除'))
    expect(builtinDelete?.attributes('disabled')).toBeDefined()
    expect(customDelete?.attributes('disabled')).toBeUndefined()
  })

  it('新建角色：分组全选后保存，载荷包含所选权限点', async () => {
    const wrapper = await mountView()

    await buttonByText(wrapper, '新建角色')!.trigger('click')
    await flushPromises()

    const inputs = wrapper.findAll('input[type="text"]')
    await inputs[0].setValue('reviewer2')
    await inputs[1].setValue('二审员')

    // 第二组「运营」全选 -> 只勾上 ops.mcsrc
    await buttonByText(wrapper, '运营（全选）')!.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('已选 1 项')

    await buttonByText(wrapper, '保存')!.trigger('click')
    await flushPromises()

    expect(api.createRole).toHaveBeenCalledWith({
      name: 'reviewer2',
      label: '二审员',
      description: '',
      permissions: ['ops.mcsrc'],
    })
  })

  it('非法角色标识在前端拦截，不发起请求', async () => {
    const wrapper = await mountView()

    await buttonByText(wrapper, '新建角色')!.trigger('click')
    await flushPromises()
    const inputs = wrapper.findAll('input[type="text"]')
    await inputs[0].setValue('Bad-Name')
    await inputs[1].setValue('非法标识')

    await buttonByText(wrapper, '保存')!.trigger('click')
    await flushPromises()

    expect(api.createRole).not.toHaveBeenCalled()
  })

  it('编辑内置 admin：权限点勾选全部禁用并给出说明', async () => {
    const wrapper = await mountView()
    await wrapper.findAll('tbody tr')[0].findAll('button')[0].trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('内置 admin 角色必须保留全部权限')
    const boxes = wrapper.findAll('input[type="checkbox"]')
    expect(boxes.length).toBeGreaterThan(0)
    expect(boxes.every((b) => b.attributes('disabled') !== undefined)).toBe(true)
  })

  it('删除自定义角色需二次确认，确认后调用删除接口', async () => {
    const wrapper = await mountView()
    const rows = wrapper.findAll('tbody tr')
    await rows[1].findAll('button').find((b) => b.text().includes('删除'))!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('确定删除角色「内容审核员」吗')
    const confirm = wrapper.findAll('button').find((b) => b.text().includes('确认删除'))
    await confirm!.trigger('click')
    await flushPromises()

    expect(api.deleteRole).toHaveBeenCalledWith('reviewer')
  })

  it('加载失败展示错误态且可重试', async () => {
    api.listRoles.mockRejectedValue(new ApiError('服务暂不可用', 'UPSTREAM_ERROR'))
    const wrapper = await mountView()

    expect(wrapper.text()).toContain('服务暂不可用')
    api.listRoles.mockResolvedValue(ROLES)
    await buttonByText(wrapper, '重试')!.trigger('click')
    await flushPromises()
    expect(wrapper.findAll('tbody tr').length).toBe(2)
  })
})

describe('后台角色管理 · 权限变更影响预览', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.listRoles.mockResolvedValue(ROLES)
    api.listPermissions.mockResolvedValue(GROUPS)
    api.updateRole.mockResolvedValue(role({ name: 'reviewer' }))
    api.getRoleImpact.mockResolvedValue({
      role: 'reviewer',
      label: '内容审核员',
      is_system: false,
      affected_accounts: 2,
      current_permissions: ['content.tools'],
      next_permissions: ['content.tools', 'ops.mcsrc'],
      added: ['ops.mcsrc'],
      removed: [],
      notes: ['权限变更会立即影响该角色下所有账号的下一次鉴权'],
    })
  })

  it('编辑角色先展示影响预览，确认后才提交更新', async () => {
    const wrapper = await mountView()
    // 第二行是自定义角色 reviewer，第一个按钮为「编辑」
    await wrapper.findAll('tbody tr')[1].findAll('button')[0].trigger('click')
    await flushPromises()

    // 勾选「运营」分组新增 ops.mcsrc
    await buttonByText(wrapper, '运营（全选）')!.trigger('click')
    await flushPromises()

    await buttonByText(wrapper, '保存')!.trigger('click')
    await flushPromises()

    expect(api.getRoleImpact).toHaveBeenCalledWith('reviewer', expect.arrayContaining(['ops.mcsrc']))
    // 预览阶段尚未提交更新
    expect(api.updateRole).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('受影响账号：2 个')
    expect(wrapper.text()).toContain('新增权限：ops.mcsrc')

    await buttonByText(wrapper, '确认保存')!.trigger('click')
    await flushPromises()

    expect(api.updateRole).toHaveBeenCalledWith({
      name: 'reviewer',
      label: expect.any(String),
      description: expect.any(String),
      permissions: expect.arrayContaining(['ops.mcsrc']),
    })
  })
})
