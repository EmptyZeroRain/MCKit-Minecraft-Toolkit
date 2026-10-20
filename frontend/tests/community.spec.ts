// 玩家社区前端测试：
//   1) 广场列表渲染作者 user_no，未登录点击发帖跳转登录；
//   2) 登录后发帖调用接口并跳转详情；
//   3) 详情页点赞、发表留言；
//   4) 后台社区管理：恢复帖子、展开/删除留言。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import type { CommunityComment, CommunityPost } from '@/types/community'

const api = vi.hoisted(() => ({
  listCommunityPosts: vi.fn(),
  fetchCommunityPost: vi.fn(),
  listCommunityComments: vi.fn(),
  createCommunityPost: vi.fn(),
  deleteCommunityPost: vi.fn(),
  toggleCommunityLike: vi.fn(),
  createCommunityComment: vi.fn(),
  deleteCommunityComment: vi.fn(),
}))
vi.mock('@/api/community', () => api)

const adminApi = vi.hoisted(() => ({
  adminListCommunityPosts: vi.fn(),
  adminSetCommunityPostStatus: vi.fn(),
  adminDeleteCommunityPost: vi.fn(),
  adminListCommunityComments: vi.fn(),
  adminSetCommunityCommentStatus: vi.fn(),
  adminDeleteCommunityComment: vi.fn(),
}))
vi.mock('@/api/admin-community', () => adminApi)

const CommunityView = (await import('@/views/CommunityView.vue')).default
const CommunityPostView = (await import('@/views/CommunityPostView.vue')).default
const AdminCommunityView = (await import('@/views/admin/AdminCommunityView.vue')).default
const { useUserAuthStore } = await import('@/stores/user-auth')

const POST: CommunityPost = {
  id: 'p1',
  category: 'chat',
  title: '我的第一个红石装置',
  body: '分享一个简单的红石灯控制电路。',
  status: 'published',
  author: {
    user_no: 'U23456789AB',
    username: 'alice',
    nickname: '爱丽丝',
  },
  like_count: 1,
  comment_count: 1,
  liked: false,
  created_at: '2026-10-01T00:00:00Z',
  updated_at: '2026-10-01T00:00:00Z',
}

const COMMENT: CommunityComment = {
  id: 'c1',
  post_id: 'p1',
  body: '这个电路很实用！',
  status: 'published',
  author: {
    user_no: 'U3456789ABC',
    username: 'bob',
    nickname: '鲍勃',
  },
  created_at: '2026-10-01T01:00:00Z',
  updated_at: '2026-10-01T01:00:00Z',
}

function paged<T>(items: T[]) {
  return { items, total: items.length, page: 1, pageSize: 20, totalPages: 1 }
}

function makeRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: { template: '<div />' } },
      { path: '/community', name: 'community', component: { template: '<div />' } },
      {
        path: '/community/:id',
        name: 'community-post',
        component: { template: '<div />' },
      },
      { path: '/login', name: 'login', component: { template: '<div />' } },
      { path: '/admin/community', name: 'admin-community', component: { template: '<div />' } },
    ],
  })
}

async function mountWithPinia(component: typeof CommunityView, path: string, loggedIn = false) {
  const pinia = createPinia()
  setActivePinia(pinia)
  const auth = useUserAuthStore()
  auth.initialized = true
  auth.loaded = true
  if (loggedIn) {
    auth.user = { id: 'u1', user_no: 'U23456789AB', username: 'alice', nickname: '爱丽丝' }
  }
  const router = makeRouter()
  await router.push(path)
  await router.isReady()
  const wrapper = mount(component, {
    global: { plugins: [pinia, router], stubs: { teleport: true } },
  })
  await flushPromises()
  return { wrapper, router }
}

beforeEach(() => {
  vi.clearAllMocks()
  api.listCommunityPosts.mockResolvedValue(paged([POST]))
  api.fetchCommunityPost.mockResolvedValue({ ...POST })
  api.listCommunityComments.mockResolvedValue(paged([COMMENT]))
  api.createCommunityPost.mockResolvedValue({ ...POST, id: 'p2' })
  api.toggleCommunityLike.mockResolvedValue({ liked: true, like_count: 2 })
  api.createCommunityComment.mockResolvedValue(COMMENT)
  api.deleteCommunityComment.mockResolvedValue(undefined)
  api.deleteCommunityPost.mockResolvedValue(undefined)
  adminApi.adminListCommunityPosts.mockResolvedValue(paged([POST]))
  adminApi.adminSetCommunityPostStatus.mockResolvedValue(undefined)
  adminApi.adminDeleteCommunityPost.mockResolvedValue(undefined)
  adminApi.adminListCommunityComments.mockResolvedValue(paged([COMMENT]))
  adminApi.adminSetCommunityCommentStatus.mockResolvedValue(undefined)
  adminApi.adminDeleteCommunityComment.mockResolvedValue(undefined)
})

describe('玩家社区', () => {
  it('列表展示作者 user_no；未登录点击发帖跳转登录', async () => {
    const { wrapper, router } = await mountWithPinia(CommunityView, '/community')

    expect(api.listCommunityPosts).toHaveBeenCalled()
    expect(wrapper.text()).toContain('我的第一个红石装置')
    expect(wrapper.text()).toContain('U23456789AB')
    expect(wrapper.text()).toContain('爱丽丝')

    const publish = wrapper.findAll('button').find((b) => b.text() === '发布帖子')
    await publish!.trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/login')
    expect(router.currentRoute.value.query.redirect).toBe('/community')
  })

  it('登录后发帖调用接口并跳转详情', async () => {
    const { wrapper, router } = await mountWithPinia(CommunityView, '/community', true)

    const publish = wrapper.findAll('button').find((b) => b.text() === '发布帖子')
    await publish!.trigger('click')
    await flushPromises()

    await wrapper.find('input[placeholder="用一句话说明主题"]').setValue('新帖子标题')
    await wrapper.find('textarea[placeholder*="请勿发布个人隐私"]').setValue('新帖子正文')
    const submit = wrapper.findAll('button').find((b) => b.text() === '发布')
    await submit!.trigger('click')
    await flushPromises()

    expect(api.createCommunityPost).toHaveBeenCalledWith({
      category: 'chat',
      title: '新帖子标题',
      body: '新帖子正文',
    })
    expect(router.currentRoute.value.path).toBe('/community/p2')
  })
})

describe('社区帖子详情', () => {
  it('点赞与发表留言调用接口并刷新', async () => {
    const { wrapper } = await mountWithPinia(CommunityPostView, '/community/p1', true)

    expect(wrapper.text()).toContain('我的第一个红石装置')
    expect(wrapper.text()).toContain('这个电路很实用！')

    const like = wrapper.findAll('button').find((b) => b.text().includes('点赞'))
    await like!.trigger('click')
    await flushPromises()
    expect(api.toggleCommunityLike).toHaveBeenCalledWith('p1')
    expect(wrapper.text()).toContain('已点赞')
    expect(wrapper.text()).toContain('（2）')

    await wrapper.find('textarea[placeholder*="友善发言"]').setValue('  新留言  ')
    const submit = wrapper.findAll('button').find((b) => b.text() === '发表留言')
    await submit!.trigger('click')
    await flushPromises()

    expect(api.createCommunityComment).toHaveBeenCalledWith({ post_id: 'p1', body: '新留言' })
    expect(api.fetchCommunityPost).toHaveBeenCalledTimes(2)
    expect(api.listCommunityComments).toHaveBeenCalledTimes(2)
  })
})

describe('后台社区管理', () => {
  it('恢复帖子并展开 / 删除留言', async () => {
    adminApi.adminListCommunityPosts.mockResolvedValue(
      paged([{ ...POST, status: 'hidden' }]),
    )
    vi.spyOn(window, 'confirm').mockReturnValue(true)

    const { wrapper } = await mountWithPinia(AdminCommunityView, '/admin/community')

    expect(wrapper.text()).toContain('我的第一个红石装置')
    expect(wrapper.text()).toContain('U23456789AB')
    expect(wrapper.text()).toContain('已隐藏')

    const restore = wrapper.findAll('button').find((b) => b.text() === '恢复')
    await restore!.trigger('click')
    await flushPromises()
    expect(adminApi.adminSetCommunityPostStatus).toHaveBeenCalledWith('p1', 'published')

    const showComments = wrapper.findAll('button').find((b) => b.text() === '查看留言')
    await showComments!.trigger('click')
    await flushPromises()
    expect(adminApi.adminListCommunityComments).toHaveBeenCalledWith({
      post_id: 'p1',
      page: 1,
      page_size: 50,
    })
    expect(wrapper.text()).toContain('这个电路很实用！')

    const hide = wrapper.findAll('button').find((b) => b.text() === '隐藏')
    await hide!.trigger('click')
    await flushPromises()
    expect(adminApi.adminSetCommunityCommentStatus).toHaveBeenCalledWith('c1', 'hidden')
    expect(wrapper.text()).toContain('已隐藏')

    const remove = wrapper.findAll('button').find((b) => b.text() === '删除')
    await remove!.trigger('click')
    await flushPromises()
    expect(adminApi.adminDeleteCommunityComment).toHaveBeenCalledWith('c1')
    expect(adminApi.adminListCommunityComments).toHaveBeenCalledTimes(2)
  })
})
