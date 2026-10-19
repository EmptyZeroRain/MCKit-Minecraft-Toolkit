// 后台任务页测试：
//   1) 状态 / 类型标签渲染与失败原因展示；
//   2) 失败任务可重试、排队任务可取消；
//   3) 详情展开显示 payload / result；
//   4) 内置表单入队（链接巡检）与通用 JSON 参数校验；
//   5) describeBatchResult 的部分失败文案。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'

const api = vi.hoisted(() => ({
  listJobTypes: vi.fn(),
  listJobs: vi.fn(),
  getJob: vi.fn(),
  enqueueJob: vi.fn(),
  retryJob: vi.fn(),
  cancelJob: vi.fn(),
}))
vi.mock('@/api/admin-jobs', () => api)

import AdminJobsView from '@/views/admin/AdminJobsView.vue'
import type { AdminJob } from '@/types/jobs'
import { describeBatchResult } from '@/types/batch'

function makeJob(partial: Partial<AdminJob> = {}): AdminJob {
  return {
    id: '11111111-1111-1111-1111-111111111111',
    type: 'linkhealth.run',
    status: 'succeeded',
    payload: { only_pending: true, limit: 100 },
    result: { checked: 12, broken: 1 },
    attempts: 1,
    max_attempts: 3,
    last_error: '',
    created_by: 'admin',
    next_run_at: '2026-10-20T00:00:00Z',
    started_at: '2026-10-20T00:00:01Z',
    finished_at: '2026-10-20T00:00:03Z',
    created_at: '2026-10-20T00:00:00Z',
    updated_at: '2026-10-20T00:00:03Z',
    ...partial,
  }
}

function paged(items: AdminJob[]) {
  return { items, total: items.length, page: 1, pageSize: 20, totalPages: 1, suggestions: [] }
}

async function mountView() {
  setActivePinia(createPinia())
  const wrapper = mount(AdminJobsView)
  await flushPromises()
  return wrapper
}

describe('后台任务', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    ;(api.listJobTypes as ReturnType<typeof vi.fn>).mockResolvedValue([
      { type: 'linkhealth.run', desc: '链接健康巡检' },
    ])
    ;(api.listJobs as ReturnType<typeof vi.fn>).mockResolvedValue(paged([makeJob()]))
    ;(api.retryJob as ReturnType<typeof vi.fn>).mockResolvedValue(makeJob({ status: 'pending', id: 'j2' }))
    ;(api.cancelJob as ReturnType<typeof vi.fn>).mockResolvedValue(makeJob({ status: 'cancelled' }))
    ;(api.enqueueJob as ReturnType<typeof vi.fn>).mockResolvedValue(makeJob({ status: 'pending', id: 'j3' }))
  })

  it('渲染任务状态、类型与结果摘要', async () => {
    const wrapper = await mountView()
    const text = wrapper.text()
    expect(text).toContain('链接健康巡检')
    expect(text).toContain('已成功')
    expect(text).toContain('第 1 / 3 次')
    expect(text).toContain('checked=12')
  })

  it('失败任务展示失败原因并可重试', async () => {
    ;(api.listJobs as ReturnType<typeof vi.fn>).mockResolvedValue(
      paged([
        makeJob({
          status: 'failed',
          attempts: 3,
          last_error: '链接巡检执行失败，请查看服务日志',
          result: {},
        }),
      ]),
    )
    const wrapper = await mountView()
    expect(wrapper.text()).toContain('链接巡检执行失败')

    const btn = wrapper.findAll('button').find((b) => b.text() === '重试')
    await btn!.trigger('click')
    await flushPromises()
    expect(api.retryJob).toHaveBeenCalledWith('11111111-1111-1111-1111-111111111111')
  })

  it('排队中的任务可取消，执行中的任务不提供取消', async () => {
    ;(api.listJobs as ReturnType<typeof vi.fn>).mockResolvedValue(
      paged([makeJob({ status: 'pending' }), makeJob({ id: 'j9', status: 'running' })]),
    )
    const wrapper = await mountView()
    const cancelButtons = wrapper.findAll('button').filter((b) => b.text() === '取消')
    expect(cancelButtons).toHaveLength(1)
  })

  it('详情展开显示入参与结果 JSON', async () => {
    const wrapper = await mountView()
    const btn = wrapper.findAll('button').find((b) => b.text() === '详情')
    await btn!.trigger('click')
    await flushPromises()
    const text = wrapper.text()
    expect(text).toContain('任务参数')
    expect(text).toContain('"only_pending": true')
    expect(text).toContain('"checked": 12')
  })

  it('内置表单入队：链接巡检参数按勾选与条数上限提交', async () => {
    const wrapper = await mountView()
    const limitInput = wrapper.find('input[type="number"]')
    await limitInput.setValue(50)
    const btn = wrapper.findAll('button').find((b) => b.text() === '入队')
    await btn!.trigger('click')
    await flushPromises()

    expect(api.enqueueJob).toHaveBeenCalledWith('linkhealth.run', {
      only_pending: true,
      limit: 50,
    })
  })
})

describe('describeBatchResult', () => {
  it('全部成功时只报成功条数', () => {
    expect(
      describeBatchResult({ requested: 2, succeeded: 2, failed: 0, items: [] }, '批量通过'),
    ).toBe('批量通过成功：2 条')
  })

  it('部分失败时列出最多 3 条原因并汇总剩余条数', () => {
    const msg = describeBatchResult(
      {
        requested: 5,
        succeeded: 1,
        failed: 4,
        items: [
          { id: '1', label: '甲', ok: false, error: '已被审核，已跳过' },
          { id: '2', label: '乙', ok: false, error: '已被审核，已跳过' },
          { id: '3', label: '丙', ok: false, error: '已被审核，已跳过' },
          { id: '4', label: '丁', ok: false, error: '已被审核，已跳过' },
        ],
      },
      '批量驳回',
    )
    expect(msg).toContain('成功 1 条，失败 4 条')
    expect(msg).toContain('甲（已被审核，已跳过）')
    expect(msg).toContain('等 4 条')
    expect(msg).not.toContain('丁')
  })
})
