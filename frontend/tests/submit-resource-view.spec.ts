// 资源投稿页交互测试：
//   1) 必填校验（名称 / 简介 / 来源链接）与授权条款勾选；
//   2) 提交契约：版本串解析为数组、加载器多选、携带当前授权版本；
//   3) 提交后刷新「我的投稿」；
//   4) 我的投稿列表渲染审核状态与驳回备注；
//   5) 未登录时禁用提交并给出登录引导。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'

const api = vi.hoisted(() => ({
  submitResource: vi.fn(),
  listMySubmissions: vi.fn(),
}))
vi.mock('@/api/user', () => ({
  ...api,
  fetchCaptcha: vi.fn(),
  register: vi.fn(),
  login: vi.fn(),
  logout: vi.fn(),
  me: vi.fn(),
  changePassword: vi.fn(),
  deleteAccount: vi.fn(),
  listFavorites: vi.fn(),
  toggleFavorite: vi.fn(),
  removeFavorite: vi.fn(),
  listRecents: vi.fn(),
  recordRecent: vi.fn(),
  clearRecents: vi.fn(),
  listSeeds: vi.fn(),
  saveSeed: vi.fn(),
  deleteSeed: vi.fn(),
}))

import SubmitResourceView from '@/views/SubmitResourceView.vue'
import { useUserAuthStore } from '@/stores/user-auth'
import type { ResourceSubmission } from '@/types/submission'
import { SUBMISSION_LICENSE_VERSION } from '@/types/submission'

function makeSubmission(partial: Partial<ResourceSubmission> = {}): ResourceSubmission {
  return {
    id: 's1',
    account_id: 'a1',
    submitter_name: 'alice',
    name: 'JEI 物品管理器',
    summary: '查看合成配方',
    description: '',
    resource_type: 'mod',
    author_name: '',
    source_platform: '',
    source_url: 'https://example.com/jei',
    license: '',
    game_edition: 'java',
    supported_versions: ['1.20.1'],
    loader_types: ['fabric'],
    install_guide: '',
    note: '',
    license_version: SUBMISSION_LICENSE_VERSION,
    license_accepted_at: null,
    status: 'pending',
    reviewer: '',
    reviewed_at: null,
    review_note: '',
    resource_id: null,
    created_at: '2026-10-19T00:00:00Z',
    updated_at: '2026-10-19T00:00:00Z',
    ...partial,
  }
}

function paged(items: ResourceSubmission[]) {
  return { items, total: items.length, page: 1, pageSize: 10, totalPages: 1 }
}

async function mountView(loggedIn = true) {
  setActivePinia(createPinia())
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: { template: '<div />' } },
      { path: '/login', component: { template: '<div />' } },
      { path: '/submit-resource', component: { template: '<div />' } },
    ],
  })
  if (loggedIn) {
    useUserAuthStore().user = { id: 'a1', username: 'alice' }
  }
  const wrapper = mount(SubmitResourceView, { global: { plugins: [router] } })
  await flushPromises()
  return wrapper
}

describe('资源投稿页', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    ;(api.listMySubmissions as ReturnType<typeof vi.fn>).mockResolvedValue(paged([]))
    ;(api.submitResource as ReturnType<typeof vi.fn>).mockResolvedValue(makeSubmission())
  })

  it('缺少必填字段时不提交并提示', async () => {
    const wrapper = await mountView()
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(api.submitResource).not.toHaveBeenCalled()
    const text = wrapper.text()
    expect(text).toContain('请填写资源名称')
    expect(text).toContain('请填写一句话简介')
    expect(text).toContain('请填写资源原始来源链接')
  })

  it('未勾选授权条款时不提交', async () => {
    const wrapper = await mountView()
    await wrapper.find('input[placeholder="如：JEI 物品管理器"]').setValue('钠')
    await wrapper.find('input[placeholder="用一句话说明这个资源的作用"]').setValue('性能优化')
    await wrapper
      .find('input[placeholder="https://…"]')
      .setValue('https://example.com/sodium')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(api.submitResource).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('请先阅读并接受投稿授权条款')
  })

  it('提交时解析版本串、携带加载器与授权版本，并刷新我的投稿', async () => {
    const wrapper = await mountView()
    await wrapper.find('input[placeholder="如：JEI 物品管理器"]').setValue('钠')
    await wrapper.find('input[placeholder="用一句话说明这个资源的作用"]').setValue('性能优化')
    await wrapper
      .find('input[placeholder="https://…"]')
      .setValue('https://example.com/sodium')
    await wrapper.find('input[placeholder="1.20.1 1.20.4"]').setValue('1.20.1, 1.20.4 1.20.1')
    // 选中 Fabric 加载器
    const loaderBtn = wrapper.findAll('button').find((b) => b.text() === 'Fabric')
    await loaderBtn!.trigger('click')
    // 勾选授权条款（表单内唯一 checkbox）
    await wrapper.find('input[type="checkbox"]').setValue(true)
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(api.submitResource).toHaveBeenCalledTimes(1)
    const payload = (api.submitResource as ReturnType<typeof vi.fn>).mock.calls[0][0]
    expect(payload.name).toBe('钠')
    expect(payload.source_url).toBe('https://example.com/sodium')
    // 版本串去重
    expect(payload.supported_versions).toEqual(['1.20.1', '1.20.4'])
    expect(payload.loader_types).toEqual(['fabric'])
    expect(payload.license_version).toBe(SUBMISSION_LICENSE_VERSION)
    // 提交后重新拉取列表（挂载时 1 次 + 提交后 1 次）
    expect(api.listMySubmissions).toHaveBeenCalledTimes(2)
  })

  it('渲染我的投稿的审核状态与驳回备注', async () => {
    ;(api.listMySubmissions as ReturnType<typeof vi.fn>).mockResolvedValue(
      paged([
        makeSubmission({ id: 's1', name: '待审资源', status: 'pending' }),
        makeSubmission({
          id: 's2',
          name: '被驳回资源',
          status: 'rejected',
          review_note: '来源不可靠',
          reviewed_at: '2026-10-19T01:00:00Z',
        }),
        makeSubmission({ id: 's3', name: '已通过资源', status: 'approved', resource_id: 'r1' }),
      ]),
    )
    const wrapper = await mountView()
    const text = wrapper.text()
    expect(text).toContain('待审资源')
    expect(text).toContain('被驳回资源')
    expect(text).toContain('来源不可靠')
    expect(text).toContain('已通过资源')
    expect(text).toContain('待审核 1 / 10')
  })

  it('未登录时禁用提交并给出登录引导', async () => {
    const wrapper = await mountView(false)
    expect(wrapper.text()).toContain('投稿需要登录')
    const submit = wrapper.find('button[type="submit"]')
    expect(submit.attributes('disabled')).toBeDefined()
  })
})
