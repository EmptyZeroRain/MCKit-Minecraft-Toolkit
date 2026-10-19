// 资源投稿 API 模块测试：
//   1) 用户端：/user/submissions/submit 契约（含授权版本）、我的投稿分页解析；
//   2) 管理端：/admin/submissions/* 动作子路径、审核请求体与返回结构。
// 目的：防止前后端接口漂移（字段名 / 路径 / 分页 meta 解析）。
import { beforeEach, describe, expect, it, vi } from 'vitest'

const userHttpMock = vi.hoisted(() => ({ post: vi.fn() }))
vi.mock('@/api/user-http', () => ({
  default: userHttpMock,
  setUserCsrfToken: vi.fn(),
}))

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

import { listMySubmissions, submitResource } from '@/api/user'
import { getSubmission, listSubmissions, reviewSubmission } from '@/api/admin-submissions'
import { SUBMISSION_LICENSE_VERSION } from '@/types/submission'

const payload = {
  name: 'JEI 物品管理器',
  summary: '查看合成配方',
  resource_type: 'mod' as const,
  source_url: 'https://example.com/jei',
  game_edition: 'java' as const,
  supported_versions: ['1.20.1'],
  loader_types: ['fabric' as const],
  license_version: SUBMISSION_LICENSE_VERSION,
}

describe('资源投稿（用户端 API）', () => {
  beforeEach(() => {
    userHttpMock.post.mockReset()
    httpMock.post.mockReset()
  })

  it('提交投稿走 /user/submissions/submit 并透传结构化字段', async () => {
    userHttpMock.post.mockResolvedValue({ data: { data: { id: 's1', status: 'pending' } } })
    const res = await submitResource(payload)
    expect(userHttpMock.post).toHaveBeenCalledWith('/user/submissions/submit', payload)
    expect(res.id).toBe('s1')
  })

  it('我的投稿解析分页 meta', async () => {
    userHttpMock.post.mockResolvedValue({
      data: {
        data: [{ id: 's1' }],
        meta: { total: 3, page: 2, page_size: 10, total_pages: 1 },
      },
    })
    const res = await listMySubmissions({ page: 2, page_size: 10 })
    expect(userHttpMock.post).toHaveBeenCalledWith('/user/submissions/list', {
      page: 2,
      page_size: 10,
    })
    expect(res.total).toBe(3)
    expect(res.page).toBe(2)
    expect(res.pageSize).toBe(10)
    expect(res.items).toHaveLength(1)
  })
})

describe('资源投稿（管理端 API）', () => {
  beforeEach(() => {
    userHttpMock.post.mockReset()
    httpMock.post.mockReset()
  })

  it('审核列表走 /admin/submissions/list', async () => {
    httpMock.post.mockResolvedValue({
      data: { data: [], meta: { total: 0, page: 1, page_size: 20, total_pages: 0 } },
    })
    const res = await listSubmissions({ status: 'pending', q: '红石' })
    expect(httpMock.post).toHaveBeenCalledWith('/admin/submissions/list', {
      status: 'pending',
      q: '红石',
    })
    expect(res.items).toHaveLength(0)
  })

  it('投稿详情走 /admin/submissions/detail', async () => {
    httpMock.post.mockResolvedValue({ data: { data: { id: 's1' } } })
    const res = await getSubmission('s1')
    expect(httpMock.post).toHaveBeenCalledWith('/admin/submissions/detail', { id: 's1' })
    expect(res?.id).toBe('s1')
  })

  it('审核走 /admin/submissions/review 并返回生成的资源', async () => {
    httpMock.post.mockResolvedValue({
      data: {
        data: {
          submission: { id: 's1', status: 'approved' },
          resource: { id: 'r1', status: 'draft' },
        },
      },
    })
    const res = await reviewSubmission({ id: 's1', status: 'approved', note: '字段完整', publish: false })
    expect(httpMock.post).toHaveBeenCalledWith('/admin/submissions/review', {
      id: 's1',
      status: 'approved',
      note: '字段完整',
      publish: false,
    })
    expect(res.resource?.status).toBe('draft')
  })
})
