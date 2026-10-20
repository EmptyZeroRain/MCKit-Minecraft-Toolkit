// 后台用户评论 Tab：跨帖子列表、状态筛选与隐藏 / 恢复。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import type { CommunityComment } from '@/types/community'

const api = vi.hoisted(() => ({
  adminListAllCommunityComments: vi.fn(),
  adminSetCommunityCommentStatus: vi.fn(),
  adminDeleteCommunityComment: vi.fn(),
}))
vi.mock('@/api/admin-community', () => api)

const AdminCommentsView = (await import('@/views/admin/AdminCommentsView.vue')).default

const COMMENT: CommunityComment = {
  id: 'c1',
  post_id: 'p1',
  post_title: '红石入门',
  body: '这个装置很实用',
  status: 'published',
  author: { user_no: 'U23456789AB', username: 'alice', nickname: '爱丽丝' },
  created_at: '2026-10-01T01:00:00Z',
  updated_at: '2026-10-01T01:00:00Z',
}

function paged<T>(items: T[]) {
  return { items, total: items.length, page: 1, pageSize: 20, totalPages: 1 }
}

function mountView() {
  const pinia = createPinia()
  setActivePinia(pinia)
  return mount(AdminCommentsView, { global: { plugins: [pinia] } })
}

beforeEach(() => {
  vi.clearAllMocks()
  api.adminListAllCommunityComments.mockResolvedValue(paged([COMMENT]))
  api.adminSetCommunityCommentStatus.mockResolvedValue(undefined)
  api.adminDeleteCommunityComment.mockResolvedValue(undefined)
})

describe('后台用户评论', () => {
  it('加载跨帖子评论并展示所属帖子与作者', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(api.adminListAllCommunityComments).toHaveBeenCalledWith(
      expect.objectContaining({ page: 1, page_size: 20 }),
    )
    expect(wrapper.text()).toContain('这个装置很实用')
    expect(wrapper.text()).toContain('红石入门')
    expect(wrapper.text()).toContain('U23456789AB')
  })

  it('隐藏评论后原地更新状态', async () => {
    const wrapper = mountView()
    await flushPromises()

    const hide = wrapper.findAll('button').find((b) => b.text() === '隐藏')
    await hide!.trigger('click')
    await flushPromises()

    expect(api.adminSetCommunityCommentStatus).toHaveBeenCalledWith('c1', 'hidden')
    expect(wrapper.text()).toContain('恢复')
  })
})
