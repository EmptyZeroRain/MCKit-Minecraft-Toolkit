// 数据导入任务测试（内容健康独立 Tab）
// 从「数据质量监控」拆出后，导入历史（来源文件 / 失败行 / 过滤 / 分页）独立验证。
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import type { DataImportJob } from '@/types/data-quality'

vi.mock('@/api/admin-data-quality', () => ({
  listDataImportJobs: vi.fn(),
}))

import { listDataImportJobs } from '@/api/admin-data-quality'
import AdminDataImportJobsView from '@/views/admin/AdminDataImportJobsView.vue'

const JOB: DataImportJob = {
  id: 'j1',
  data_type: 'items',
  source_file: 'seeds/items_demo.json',
  source_version: '',
  dry_run: false,
  total_count: 12,
  created_count: 12,
  updated_count: 0,
  skipped_count: 0,
  failed_count: 1,
  error_summary: ['第 4 行：item_category 非法'],
  operator: 'tester',
  started_at: '2026-01-15T00:00:00Z',
  finished_at: '2026-01-15T00:00:01Z',
  created_at: '2026-01-15T00:00:00Z',
}

async function mountView(jobs: DataImportJob[] = [JOB]) {
  setActivePinia(createPinia())
  ;(listDataImportJobs as ReturnType<typeof vi.fn>).mockResolvedValue({
    items: jobs,
    total: jobs.length,
    page: 1,
    pageSize: 15,
    totalPages: 1,
  })
  const wrapper = mount(AdminDataImportJobsView, {
    global: { stubs: { Pagination: true } },
  })
  await flushPromises()
  return wrapper
}

describe('数据导入任务页', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('加载时按默认分页请求导入任务', async () => {
    await mountView()
    expect(listDataImportJobs).toHaveBeenCalledWith({ data_type: undefined, page: 1, page_size: 15 })
  })

  it('渲染任务行：来源文件、执行者与失败数', async () => {
    const wrapper = await mountView()
    const text = wrapper.text()
    expect(text).toContain('seeds/items_demo.json')
    expect(text).toContain('items')
    expect(text).toContain('tester')
    expect(text).toContain('12 / 0')
  })

  it('可按数据类型过滤并回到第一页', async () => {
    const wrapper = await mountView()
    await wrapper.find('select[aria-label="按数据类型过滤"]').setValue('recipes')
    await flushPromises()
    expect(listDataImportJobs).toHaveBeenLastCalledWith(
      expect.objectContaining({ data_type: 'recipes', page: 1 }),
    )
  })

  it('展开错误明细，再次点击收起', async () => {
    const wrapper = await mountView()
    const toggle = wrapper.findAll('button').find((b) => b.text().startsWith('错误'))
    expect(toggle?.attributes('aria-expanded')).toBe('false')
    expect(wrapper.text()).not.toContain('第 4 行：item_category 非法')

    await toggle!.trigger('click')
    expect(wrapper.text()).toContain('第 4 行：item_category 非法')
    expect(wrapper.findAll('button').some((b) => b.text() === '收起')).toBe(true)

    const collapse = wrapper.findAll('button').find((b) => b.text() === '收起')
    await collapse!.trigger('click')
    expect(wrapper.text()).not.toContain('第 4 行：item_category 非法')
  })

  it('无记录时给出空态提示', async () => {
    const wrapper = await mountView([])
    expect(wrapper.text()).toContain('暂无导入任务记录')
  })

  it('请求失败时展示错误信息', async () => {
    setActivePinia(createPinia())
    ;(listDataImportJobs as ReturnType<typeof vi.fn>).mockRejectedValue(new Error('boom'))
    const wrapper = mount(AdminDataImportJobsView, { global: { stubs: { Pagination: true } } })
    await flushPromises()
    expect(wrapper.text()).toContain('加载失败')
  })
})
