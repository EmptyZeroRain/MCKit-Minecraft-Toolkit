// 内容建设任务页测试（gj.md §8.3 搜索词闭环）：
// 覆盖候选 → 转任务、状态流转、编辑保存（仅提交变更字段）、重新检测与删除。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'

vi.mock('@/api/admin-content-tasks', () => ({
  listContentTasks: vi.fn(),
  listContentTaskCandidates: vi.fn(),
  createContentTask: vi.fn(),
  updateContentTask: vi.fn(),
  recheckContentTask: vi.fn(),
  deleteContentTask: vi.fn(),
}))
vi.mock('@/api/admin-admins', () => ({ listAdmins: vi.fn() }))

import {
  createContentTask,
  deleteContentTask,
  listContentTaskCandidates,
  listContentTasks,
  recheckContentTask,
  updateContentTask,
} from '@/api/admin-content-tasks'
import { listAdmins } from '@/api/admin-admins'
import AdminContentTasksView from '@/views/admin/AdminContentTasksView.vue'
import type { ContentTask, ContentTaskCandidate } from '@/types/content-task'

const CANDIDATES: ContentTaskCandidate[] = [
  { term: '远古残骸', hits: 5, no_result_hits: 4, last_seen_at: '2026-01-15T01:00:00Z', task_id: '' },
  { term: '钻石矿分布', hits: 9, no_result_hits: 6, last_seen_at: '2026-01-15T02:00:00Z', task_id: 't9' },
]

const TASKS: ContentTask[] = [
  {
    id: 't1',
    keyword: '红石',
    search_term_id: 's1',
    hits: 5,
    no_result_hits: 5,
    last_seen_at: '2026-01-15T01:00:00Z',
    status: 'open',
    assignee_id: null,
    assignee_name: '',
    related_content_type: '',
    related_content_id: null,
    note: '待补充',
    created_by: 'a1',
    created_at: '2026-01-10T00:00:00Z',
    updated_at: '2026-01-15T01:00:00Z',
  },
]

function paged<T>(items: T[]) {
  return { items, total: items.length, page: 1, pageSize: 20, totalPages: 1, suggestions: [] }
}

async function mountView(tasks: ContentTask[] = TASKS) {
  setActivePinia(createPinia())
  ;(listContentTasks as ReturnType<typeof vi.fn>).mockResolvedValue(paged(tasks))
  ;(listContentTaskCandidates as ReturnType<typeof vi.fn>).mockResolvedValue(CANDIDATES)
  ;(listAdmins as ReturnType<typeof vi.fn>).mockResolvedValue(
    paged([{ id: 'a1', username: '阿甲', nickname: '阿甲', role: 'editor', is_active: true, last_login_at: null, created_at: '2026-01-01T00:00:00Z' }]),
  )
  const wrapper = mount(AdminContentTasksView, {
    global: { stubs: { Pagination: true, teleport: true } },
  })
  await flushPromises()
  return wrapper
}

describe('内容建设任务页', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('展示候选缺口与任务列表', async () => {
    const wrapper = await mountView()
    const text = wrapper.text()
    expect(text).toContain('远古残骸')
    expect(text).toContain('红石')
    expect(text).toContain('待处理')
  })

  it('候选仅对尚无任务的关键词提供「转为任务」', async () => {
    const wrapper = await mountView()
    const buttons = wrapper.findAll('button').filter((b) => b.text() === '转为任务')
    expect(buttons).toHaveLength(1)

    ;(createContentTask as ReturnType<typeof vi.fn>).mockResolvedValue({ ...TASKS[0] })
    await buttons[0].trigger('click')
    await flushPromises()
    expect(createContentTask).toHaveBeenCalledWith({ keyword: '远古残骸' })
    // 创建后刷新候选与任务列表
    expect(listContentTasks).toHaveBeenCalledTimes(2)
  })

  it('表格内切换状态调用更新接口', async () => {
    const wrapper = await mountView()
    ;(updateContentTask as ReturnType<typeof vi.fn>).mockResolvedValue({ ...TASKS[0], status: 'in_progress' })
    const select = wrapper.find('[aria-label="修改「红石」状态"]')
    expect(select.exists()).toBe(true)
    await select.setValue('in_progress')
    await flushPromises()
    expect(updateContentTask).toHaveBeenCalledWith({ id: 't1', status: 'in_progress' })
  })

  it('编辑保存只提交发生变化的字段', async () => {
    const wrapper = await mountView()
    const editBtn = wrapper.findAll('button').find((b) => b.text() === '编辑')
    expect(editBtn).toBeTruthy()
    await editBtn!.trigger('click')
    await flushPromises()

    // 仅修改备注
    const textarea = wrapper.find('textarea')
    await textarea.setValue('已补充矿层分布')
    ;(updateContentTask as ReturnType<typeof vi.fn>).mockResolvedValue({ ...TASKS[0] })
    const saveBtn = wrapper.findAll('button').find((b) => b.text() === '保存')
    await saveBtn!.trigger('click')
    await flushPromises()

    expect(updateContentTask).toHaveBeenCalledWith({ id: 't1', note: '已补充矿层分布' })
  })

  it('重新检测有结果时提示并可标记完成', async () => {
    const wrapper = await mountView()
    const editBtn = wrapper.findAll('button').find((b) => b.text() === '编辑')
    await editBtn!.trigger('click')
    await flushPromises()

    ;(recheckContentTask as ReturnType<typeof vi.fn>).mockResolvedValue({
      task_id: 't1',
      keyword: '红石',
      result_count: 3,
      has_results: true,
    })
    const recheckBtn = wrapper.findAll('button').find((b) => b.text() === '重新检测')
    await recheckBtn!.trigger('click')
    await flushPromises()

    expect(recheckContentTask).toHaveBeenCalledWith('t1')
    expect(wrapper.text()).toContain('检测到 3 条搜索结果')
    expect(wrapper.findAll('button').some((b) => b.text() === '标记完成')).toBe(true)
  })

  it('删除需二次确认', async () => {
    const wrapper = await mountView()
    const delBtn = wrapper.findAll('button').find((b) => b.text() === '删除')
    await delBtn!.trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-confirm]').exists()).toBe(true)

    ;(deleteContentTask as ReturnType<typeof vi.fn>).mockResolvedValue(undefined)
    await wrapper.find('[data-confirm]').trigger('click')
    await flushPromises()
    expect(deleteContentTask).toHaveBeenCalledWith('t1')
  })

  it('无任务时给出空态提示', async () => {
    const wrapper = await mountView([])
    expect(wrapper.text()).toContain('暂无内容建设任务')
  })
})
