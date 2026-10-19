// 内容历史版本 API 模块测试（gj.md §15 / §16.2）：
// 验证内容类型 → 路由段映射、请求体字段与分页解析。
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

import { getRevisionDiff, listRevisions, rollbackRevision } from '@/api/admin-revisions'

describe('admin-revisions API', () => {
  beforeEach(() => {
    httpMock.post.mockReset()
  })

  it('内容类型映射到对应路由段，并携带正确请求体', async () => {
    httpMock.post.mockResolvedValue({
      data: { data: [], meta: { total: 0, page: 1, page_size: 20, total_pages: 0 } },
    })
    await listRevisions('resource', 'r1', 2, 10)
    expect(httpMock.post).toHaveBeenCalledWith('/admin/revisions/resources/list', {
      content_id: 'r1',
      page: 2,
      page_size: 10,
    })

    httpMock.post.mockResolvedValue({ data: { data: { changes: [] } } })
    await getRevisionDiff('tool', 't1', 3)
    expect(httpMock.post).toHaveBeenCalledWith('/admin/revisions/tools/diff', {
      content_id: 't1',
      revision_no: 3,
    })

    httpMock.post.mockResolvedValue({ data: { data: { revision_no: 4 } } })
    await rollbackRevision('wiki', 'w1', 1, '回滚原因')
    expect(httpMock.post).toHaveBeenCalledWith('/admin/revisions/wiki/rollback', {
      content_id: 'w1',
      revision_no: 1,
      note: '回滚原因',
    })
  })

  it('分页解析回退到安全默认值', async () => {
    httpMock.post.mockResolvedValue({ data: { data: [{ revision_no: 1 }] } })
    const res = await listRevisions('tool', 't1')
    expect(res.items).toHaveLength(1)
    expect(res.total).toBe(0)
  })
})
