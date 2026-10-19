// 数据质量监控测试（TODO §2 / §4）
// 覆盖两层：展示辅助纯函数 + 概览视图（检查项 / 汇总 / 导入任务）。
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import {
  actionableChecks,
  categoryLabel,
  groupChecksByCategory,
  isJobFailed,
  problemRatio,
  severityClass,
  severityLabel,
} from '@/utils/data-quality'
import type { DataImportJob, DataQualityCheck } from '@/types/data-quality'

// ===== 纯函数 =====

function check(overrides: Partial<DataQualityCheck> = {}): DataQualityCheck {
  return {
    key: 'items_missing_source',
    label: '缺少来源 URL',
    category: 'items',
    severity: 'warning',
    count: 3,
    total: 12,
    hint: '补齐 source_url。',
    ...overrides,
  }
}

describe('数据质量展示辅助函数', () => {
  it('严重级别文案与样式', () => {
    expect(severityLabel('error')).toBe('严重')
    expect(severityLabel('warning')).toBe('提醒')
    expect(severityLabel('info')).toBe('统计')
    expect(severityClass('error')).toContain('danger-mc')
  })

  it('问题占比处理边界，不产生 NaN 或越界', () => {
    expect(problemRatio(3, 12)).toBe(25)
    expect(problemRatio(0, 12)).toBe(0)
    expect(problemRatio(5, 0)).toBe(0)
    expect(problemRatio(-1, 10)).toBe(0)
    expect(problemRatio(20, 10)).toBe(100)
  })

  it('仅筛出命中数大于 0 的严重 / 提醒项，且严重在前', () => {
    const result = actionableChecks([
      check({ key: 'a', severity: 'info', count: 5 }),
      check({ key: 'b', severity: 'warning', count: 1 }),
      check({ key: 'c', severity: 'error', count: 2 }),
      check({ key: 'd', severity: 'warning', count: 0 }),
    ])
    expect(result.map((c) => c.key)).toEqual(['c', 'b'])
  })

  it('按类别分组，未知类别回退为原文', () => {
    const groups = groupChecksByCategory([
      check({ key: 'a', category: 'items' }),
      check({ key: 'b', category: 'recipes' }),
      check({ key: 'c', category: 'items' }),
    ])
    expect(groups.map((g) => g.category)).toEqual(['items', 'recipes'])
    expect(groups[0].checks).toHaveLength(2)
    expect(categoryLabel('items')).toBe('物品资料库')
    expect(categoryLabel('unknown')).toBe('unknown')
  })

  it('导入任务失败判定只依赖失败行数', () => {
    const base = { failed_count: 0 } as Pick<DataImportJob, 'failed_count'>
    expect(isJobFailed(base)).toBe(false)
    expect(isJobFailed({ failed_count: 2 })).toBe(true)
  })
})

// ===== 视图 =====

vi.mock('@/api/admin-data-quality', () => ({
  fetchDataQualityOverview: vi.fn(),
  listDataImportJobs: vi.fn(),
  listQualityAssignees: vi.fn(),
  updateQualityIssue: vi.fn(),
}))

import {
  fetchDataQualityOverview,
  listDataImportJobs,
  listQualityAssignees,
  updateQualityIssue,
} from '@/api/admin-data-quality'
import AdminDataQualityView from '@/views/admin/AdminDataQualityView.vue'

const OVERVIEW = {
  generated_at: '2026-01-15T00:00:00Z',
  error_checks: 1,
  warning_checks: 1,
  checks: [
    check({ key: 'items_total', label: '物品总数', category: 'items', severity: 'info', count: 12, total: 12 }),
    check({ key: 'items_missing_source', severity: 'warning', count: 3 }),
    check({ key: 'recipes_orphan_result', label: '结果物品不存在于物品库', category: 'recipes', severity: 'error', count: 1, total: 5, hint: '会导致断链。' }),
  ],
}

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

async function mountView(overview = OVERVIEW, jobs = [JOB]) {
  setActivePinia(createPinia())
  ;(fetchDataQualityOverview as ReturnType<typeof vi.fn>).mockResolvedValue(overview)
  ;(listDataImportJobs as ReturnType<typeof vi.fn>).mockResolvedValue({
    items: jobs,
    total: jobs.length,
    page: 1,
    pageSize: 15,
    totalPages: 1,
  })
  ;(listQualityAssignees as ReturnType<typeof vi.fn>).mockResolvedValue([
    { id: 'a1', name: '质量负责人' },
  ])
  ;(updateQualityIssue as ReturnType<typeof vi.fn>).mockResolvedValue({
    status: 'assigned',
    assignee_id: 'a1',
    assignee_name: '质量负责人',
    note: '',
    snapshot_count: 3,
    updated_by: 'tester',
  })
  const wrapper = mount(AdminDataQualityView, {
    global: { stubs: { Pagination: true, RouterLink: true } },
  })
  await flushPromises()
  return wrapper
}

describe('数据质量监控页', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('加载时请求概览与导入任务', async () => {
    await mountView()
    expect(fetchDataQualityOverview).toHaveBeenCalledTimes(1)
    expect(listDataImportJobs).toHaveBeenCalledTimes(1)
  })

  it('展示严重 / 提醒汇总与需要关注的检查项', async () => {
    const wrapper = await mountView()
    const text = wrapper.text()
    expect(text).toContain('严重问题项')
    expect(text).toContain('结果物品不存在于物品库')
    expect(text).toContain('会导致断链。')
    // 统计类不应出现在「需要关注」区域，但仍应出现在全量表格中
    expect(text).toContain('物品总数')
  })

  it('渲染导入任务行并标注失败数与来源', async () => {
    const wrapper = await mountView()
    const text = wrapper.text()
    expect(text).toContain('seeds/items_demo.json')
    expect(text).toContain('items')
    expect(text).toContain('tester')
  })

  it('无问题时给出明确的无问题提示', async () => {
    const wrapper = await mountView({ generated_at: '2026-01-15T00:00:00Z', error_checks: 0, warning_checks: 0, checks: [check({ severity: 'info', count: 5 })] }, [])
    expect(wrapper.text()).toContain('未发现需要处理的数据质量问题')
    expect(wrapper.text()).toContain('暂无导入任务记录')
  })

  it('指派负责人时提交 check_key / 状态与实时快照条数', async () => {
    const wrapper = await mountView()
    const select = wrapper.find('select[aria-label="问题负责人"]')
    expect(select.exists()).toBe(true)
    await select.setValue('a1')
    await flushPromises()

    // 需要关注区按「严重在前」排序，第一个负责人下拉属于 recipes 检查项。
    expect(updateQualityIssue).toHaveBeenCalledWith(
      expect.objectContaining({
        check_key: 'recipes_orphan_result',
        status: 'assigned',
        assignee_id: 'a1',
        snapshot_count: 1,
      }),
    )
    // 保存后刷新概览
    expect(fetchDataQualityOverview).toHaveBeenCalledTimes(2)
  })

  it('可标记已解决，已关闭项显示重新打开', async () => {
    const wrapper = await mountView({
      ...OVERVIEW,
      checks: [
        check({
          key: 'items_missing_source',
          issue: {
            status: 'resolved',
            assignee_name: '质量负责人',
            note: '已补齐',
            snapshot_count: 3,
            updated_by: 'tester',
            updated_at: '2026-01-15T01:00:00Z',
          },
        }),
      ],
    })
    expect(wrapper.text()).toContain('已解决')
    expect(wrapper.text()).toContain('重新打开')
    expect(wrapper.findAll('button').some((b) => b.text() === '标记已解决')).toBe(false)
  })
})
