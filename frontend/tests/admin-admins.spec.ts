// 管理员账号管理 API 契约测试（需求 6.1 / 6.2）
// 校验前端调用路径、请求体与分页解析，避免与后端动作子路径契约漂移。
import { describe, expect, it, vi, beforeEach } from 'vitest'

const { post } = vi.hoisted(() => ({ post: vi.fn() }))

vi.mock('@/api/http', () => ({
  default: { post: (...args: unknown[]) => post(...args) },
  toPaged: (res: {
    data: { data: unknown[]; meta?: { total?: number; page?: number; page_size?: number; total_pages?: number } }
  }) => {
    const meta = res.data.meta ?? {}
    return {
      items: res.data.data ?? [],
      total: meta.total ?? 0,
      page: meta.page ?? 1,
      pageSize: meta.page_size ?? 0,
      totalPages: meta.total_pages ?? 0,
    }
  },
}))

import {
  listAdmins,
  createAdmin,
  updateAdmin,
  resetAdminPassword,
  deleteAdmin,
} from '@/api/admin-admins'

beforeEach(() => {
  post.mockReset()
})

describe('管理员账号 API', () => {
  it('listAdmins 调用 /admin/admins/list 并解析分页', async () => {
    post.mockResolvedValue({
      data: {
        data: [{ id: '1', username: 'ops' }],
        meta: { total: 1, page: 1, page_size: 15, total_pages: 1 },
      },
    })
    const res = await listAdmins({ q: 'ops', page: 1, page_size: 15 })
    expect(post).toHaveBeenCalledWith('/admin/admins/list', { q: 'ops', page: 1, page_size: 15 })
    expect(res.items).toHaveLength(1)
    expect(res.total).toBe(1)
  })

  it('createAdmin 提交 username/password/role', async () => {
    post.mockResolvedValue({ data: { data: { id: '1' } } })
    await createAdmin({ username: 'newbie', password: 'abcdefghijkl', role: 'editor' })
    expect(post).toHaveBeenCalledWith('/admin/admins/create', {
      username: 'newbie',
      password: 'abcdefghijkl',
      role: 'editor',
    })
  })

  it('updateAdmin 仅提交白名单字段（角色/启用状态）', async () => {
    post.mockResolvedValue({ data: { data: { id: '1' } } })
    await updateAdmin({ id: '1', is_active: false })
    expect(post).toHaveBeenCalledWith('/admin/admins/update', { id: '1', is_active: false })
  })

  it('resetAdminPassword 提交 id 与新密码', async () => {
    post.mockResolvedValue({ data: { data: null } })
    await resetAdminPassword({ id: '1', new_password: 'abcdefghijkl' })
    expect(post).toHaveBeenCalledWith('/admin/admins/reset-password', {
      id: '1',
      new_password: 'abcdefghijkl',
    })
  })

  it('deleteAdmin 提交 id', async () => {
    post.mockResolvedValue({ data: { data: null } })
    await deleteAdmin('1')
    expect(post).toHaveBeenCalledWith('/admin/admins/delete', { id: '1' })
  })
})
