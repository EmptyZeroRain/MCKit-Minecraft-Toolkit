// 内容建设任务 API 模块测试（gj.md §8.3）：
// 验证动作子路径、请求体字段与分页解析，避免前端与后端接口漂移。
import { beforeEach, describe, expect, it, vi } from 'vitest'

const httpMock = vi.hoisted(() => ({ post: vi.fn() }))
vi.mock('@/api/http', () => ({
  default: httpMock,
  toPaged: (res: { data: { data?: unknown[]; meta?: Record<string, number> } }) => ({
    items: res.data.data ?? [],
    total: res.data.meta?.total ?? 0,
    page: res.data.meta?.page ?? 1,
    pageSize: res.data.meta?.page_size ?? 0,
    totalPages: res.data.meta?.total_pages ?? 0,
    suggestions: [],
  }),
}))

import {
  createContentTask,
  deleteContentTask,
  listContentTaskCandidates,
  listContentTasks,
  recheckContentTask,
  updateContentTask,
} from '@/api/admin-content-tasks'

describe('admin-content-tasks API', () => {
  beforeEach(() => {
    httpMock.post.mockReset()
  })

  it('列表走 /list 并透传筛选与分页', async () => {
    httpMock.post.mockResolvedValue({
      data: { data: [], meta: { total: 0, page: 2, page_size: 10, total_pages: 0 } },
    })
    const res = await listContentTasks({ status: 'open', q: '红石', page: 2, page_size: 10 })
    expect(httpMock.post).toHaveBeenCalledWith('/admin/content-tasks/list', {
      status: 'open',
      q: '红石',
      page: 2,
      page_size: 10,
    })
    expect(res.page).toBe(2)
  })

  it('候选默认取 20 条', async () => {
    httpMock.post.mockResolvedValue({ data: { data: [{ term: '远古残骸' }] } })
    const res = await listContentTaskCandidates()
    expect(httpMock.post).toHaveBeenCalledWith('/admin/content-tasks/candidates', { limit: 20 })
    expect(res).toHaveLength(1)
  })

  it('创建 / 更新 / 重新检测 / 删除的动作子路径与请求体正确', async () => {
    httpMock.post.mockResolvedValue({ data: { data: { id: 't1' } } })
    await createContentTask({ keyword: '红石', assignee_id: 'a1' })
    expect(httpMock.post).toHaveBeenCalledWith('/admin/content-tasks/create', {
      keyword: '红石',
      assignee_id: 'a1',
    })

    await updateContentTask({ id: 't1', status: 'done' })
    expect(httpMock.post).toHaveBeenCalledWith('/admin/content-tasks/update', {
      id: 't1',
      status: 'done',
    })

    await recheckContentTask('t1')
    expect(httpMock.post).toHaveBeenCalledWith('/admin/content-tasks/recheck', { id: 't1' })

    await deleteContentTask('t1')
    expect(httpMock.post).toHaveBeenCalledWith('/admin/content-tasks/delete', { id: 't1' })
  })
})
