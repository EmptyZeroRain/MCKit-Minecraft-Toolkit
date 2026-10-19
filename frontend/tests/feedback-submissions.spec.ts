// 用户投稿与纠错（TODO §17）测试：
//   1) 类型契约：投稿类型、需授权条款的类型、关联资料类型与标签完整；
//   2) 投稿表单：资料纠错必须先接受授权条款，提交时携带授权版本与关联资料；
//   3) 普通反馈（bug）无需授权条款，且不携带 license_version；
//   4) 管理端审核：状态 + 备注一并提交，并可展开审核变更历史。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter, type Router } from 'vue-router'

const publicApi = vi.hoisted(() => ({ submitFeedback: vi.fn() }))
vi.mock('@/api/public', () => publicApi)

const adminApi = vi.hoisted(() => ({
  listFeedback: vi.fn(),
  updateFeedbackStatus: vi.fn(),
  listFeedbackReviews: vi.fn(),
}))
vi.mock('@/api/admin-system', () => adminApi)

import FeedbackView from '@/views/FeedbackView.vue'
import AdminFeedbackView from '@/views/admin/AdminFeedbackView.vue'
import type { Feedback, FeedbackReview } from '@/types/feedback'
import {
  FEEDBACK_LICENSE_TYPES,
  FEEDBACK_LICENSE_VERSION,
  FEEDBACK_TARGET_KINDS,
  FEEDBACK_TARGET_KIND_LABELS,
  FEEDBACK_TYPES,
  FEEDBACK_TYPE_LABELS,
} from '@/types/common'

function makeRouter(): Router {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', name: 'home', component: { template: '<div />' } },
      { path: '/feedback', name: 'feedback', component: { template: '<div />' } },
    ],
  })
}

function makeFeedback(partial: Partial<Feedback> = {}): Feedback {
  return {
    id: 'f1',
    feedback_type: 'data_correction',
    title: '牛的生命值有误',
    content: '应为 10 而非 20。',
    related_url: null,
    contact: null,
    status: 'new',
    target_kind: 'mob',
    target_ref: 'cow',
    reviewer: '',
    reviewed_at: null,
    review_note: '',
    license_version: FEEDBACK_LICENSE_VERSION,
    license_accepted_at: '2026-10-18T00:00:00Z',
    created_at: '2026-10-18T00:00:00Z',
    updated_at: '2026-10-18T00:00:00Z',
    ...partial,
  }
}

function paged(items: Feedback[]) {
  return { items, total: items.length, page: 1, pageSize: 15, totalPages: 1, suggestions: [] }
}

beforeEach(() => {
  vi.clearAllMocks()
  setActivePinia(createPinia())
})

describe('§17 投稿类型契约', () => {
  it('包含资料纠错与资源补充，且标签完整', () => {
    expect(FEEDBACK_TYPES).toContain('data_correction')
    expect(FEEDBACK_TYPES).toContain('resource_supplement')
    for (const t of FEEDBACK_TYPES) {
      expect(FEEDBACK_TYPE_LABELS[t]).toBeTruthy()
    }
    expect(FEEDBACK_TYPE_LABELS.data_correction).toBe('资料纠错')
  })

  it('需授权条款的类型与关联资料类型枚举与后端契约一致', () => {
    expect([...FEEDBACK_LICENSE_TYPES]).toEqual(['data_correction', 'resource_supplement'])
    expect([...FEEDBACK_TARGET_KINDS]).toEqual([
      'resource',
      'item',
      'dev_doc',
      'mob',
      'villager',
      'potion',
      'tool',
      'other',
    ])
    for (const k of FEEDBACK_TARGET_KINDS) {
      expect(FEEDBACK_TARGET_KIND_LABELS[k]).toBeTruthy()
    }
  })
})

describe('§17 投稿表单', () => {
  it('资料纠错未接受授权条款时不提交并提示', async () => {
    const router = makeRouter()
    await router.push('/feedback')
    await router.isReady()
    const wrapper = mount(FeedbackView, { global: { plugins: [router] } })
    await flushPromises()

    // 选择「资料纠错」
    const correction = wrapper.findAll('button').find((b) => b.text() === '资料纠错')
    expect(correction).toBeTruthy()
    await correction!.trigger('click')

    await wrapper.find('input[type="text"]').setValue('牛的生命值有误')
    await wrapper.find('textarea').setValue('应为 10 而非 20。')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(publicApi.submitFeedback).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('请先阅读并接受投稿授权条款')
  })

  it('接受授权条款后提交，携带授权版本与关联资料', async () => {
    publicApi.submitFeedback.mockResolvedValue({ id: 'new-1' })
    const router = makeRouter()
    await router.push('/feedback')
    await router.isReady()
    const wrapper = mount(FeedbackView, { global: { plugins: [router] } })
    await flushPromises()

    await wrapper.findAll('button').find((b) => b.text() === '资料纠错')!.trigger('click')
    await wrapper.find('input[type="text"]').setValue('牛的生命值有误')
    await wrapper.find('textarea').setValue('应为 10 而非 20。')

    // 关联资料类型选择「生物」，标识填 slug
    const selects = wrapper.findAll('select')
    await selects[0].setValue('mob')
    await wrapper.findAll('input[type="text"]')[1].setValue('cow')
    // 勾选授权条款
    await wrapper.find('input[type="checkbox"]').setValue(true)

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(publicApi.submitFeedback).toHaveBeenCalledWith(
      expect.objectContaining({
        feedback_type: 'data_correction',
        title: '牛的生命值有误',
        content: '应为 10 而非 20。',
        target_kind: 'mob',
        target_ref: 'cow',
        license_version: FEEDBACK_LICENSE_VERSION,
      }),
    )
  })

  it('普通错误反馈无需授权条款，且不携带 license_version', async () => {
    publicApi.submitFeedback.mockResolvedValue({ id: 'new-2' })
    const router = makeRouter()
    await router.push('/feedback')
    await router.isReady()
    const wrapper = mount(FeedbackView, { global: { plugins: [router] } })
    await flushPromises()

    // 默认类型为 bug，不出现授权条款勾选框
    expect(wrapper.find('input[type="checkbox"]').exists()).toBe(false)
    await wrapper.find('input[type="text"]').setValue('页面报错')
    await wrapper.find('textarea').setValue('点击后 500。')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    const payload = publicApi.submitFeedback.mock.calls[0][0]
    expect(payload.feedback_type).toBe('bug')
    expect(payload.license_version).toBeUndefined()
  })
})

describe('§17 管理端审核', () => {
  it('审核时提交备注并记录审核人信息', async () => {
    adminApi.listFeedback.mockResolvedValue(paged([makeFeedback()]))
    adminApi.updateFeedbackStatus.mockResolvedValue(
      makeFeedback({ status: 'resolved', reviewer: 'admin', review_note: '已修正为 10' }),
    )

    const wrapper = mount(AdminFeedbackView)
    await flushPromises()

    expect(wrapper.text()).toContain('资料纠错')
    expect(wrapper.text()).toContain('关联资料：生物 · cow')

    await wrapper.find('input[placeholder="审核备注（可选，随状态一并记录）"]').setValue('已修正为 10')
    await wrapper.findAll('button').find((b) => b.text() === '已解决')!.trigger('click')
    await flushPromises()

    expect(adminApi.updateFeedbackStatus).toHaveBeenCalledWith({
      id: 'f1',
      status: 'resolved',
      note: '已修正为 10',
    })
  })

  it('可展开审核变更历史（状态流转 + 审核人）', async () => {
    adminApi.listFeedback.mockResolvedValue(
      paged([makeFeedback({ status: 'resolved', reviewer: 'admin', reviewed_at: '2026-10-18T02:00:00Z' })]),
    )
    const history: FeedbackReview[] = [
      {
        id: 'r1',
        feedback_id: 'f1',
        from_status: 'new',
        to_status: 'processing',
        reviewer: 'admin',
        note: '正在核对',
        created_at: '2026-10-18T01:00:00Z',
      },
      {
        id: 'r2',
        feedback_id: 'f1',
        from_status: 'processing',
        to_status: 'resolved',
        reviewer: 'admin',
        note: '已修正为 10',
        created_at: '2026-10-18T02:00:00Z',
      },
    ]
    adminApi.listFeedbackReviews.mockResolvedValue(history)

    const wrapper = mount(AdminFeedbackView)
    await flushPromises()

    await wrapper.findAll('button').find((b) => b.text() === '审核历史')!.trigger('click')
    await flushPromises()

    expect(adminApi.listFeedbackReviews).toHaveBeenCalledWith('f1')
    expect(wrapper.text()).toContain('新提交 → 处理中')
    expect(wrapper.text()).toContain('处理中 → 已解决')
    expect(wrapper.text()).toContain('已修正为 10')
    expect(wrapper.text()).toContain('正在核对')
  })
})
