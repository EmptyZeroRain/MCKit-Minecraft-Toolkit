// 运营看板视图测试（TODO §13）
// 覆盖：概览卡片渲染、趋势图 series 接线、明细表格、空态、自定义时间范围、
// 错误态。图表组件以 stub 替换（jsdom 无 canvas，真实 ECharts 无法初始化）。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import AdminAnalyticsView from '@/views/admin/AdminAnalyticsView.vue'
import { ApiError } from '@/types/api'
import type { UsageOverview, UsageTrendPoint } from '@/types/analytics'

const api = vi.hoisted(() => ({
  fetchUsageOverview: vi.fn(),
  fetchUsageTrend: vi.fn(),
  fetchToolUsage: vi.fn(),
  fetchResourceUsage: vi.fn(),
  fetchApiUsage: vi.fn(),
}))
vi.mock('@/api/admin-analytics', () => api)

function overview(partial: Partial<UsageOverview> = {}): UsageOverview {
  return {
    from: '2026-10-11',
    to: '2026-10-17',
    tool_visits: 120,
    tool_uses: 80,
    tool_failures: 4,
    tool_failure_rate: 0.05,
    active_tools: 3,
    resource_views: 45,
    active_resources: 2,
    broken_links: 1,
    search_queries: 30,
    search_zero_result: 6,
    search_failed: 1,
    api_total: 900,
    api_errors: 2,
    api_client_errors: 12,
    api_rate_limited: 3,
    api_error_rate: 2 / 900,
    avg_latency_ms: 12.5,
    p95_latency_ms: 200,
    ...partial,
  }
}

const trend: UsageTrendPoint[] = [
  {
    date: '2026-10-16',
    tool_visits: 60,
    tool_uses: 40,
    tool_failures: 2,
    resource_views: 20,
    search_queries: 15,
    api_total: 400,
    api_errors: 1,
    api_rate_limited: 1,
  },
  {
    date: '2026-10-17',
    tool_visits: 60,
    tool_uses: 40,
    tool_failures: 2,
    resource_views: 25,
    search_queries: 15,
    api_total: 500,
    api_errors: 1,
    api_rate_limited: 2,
  },
]

const stubs = {
  BaseChart: { name: 'BaseChart', props: ['option', 'height', 'ariaLabel'], template: '<div class="chart-stub" />' },
  RouterLink: { template: '<a><slot /></a>' },
}

async function mountView() {
  setActivePinia(createPinia())
  const wrapper = mount(AdminAnalyticsView, { global: { stubs } })
  await flushPromises()
  return wrapper
}

function mockSuccess(ov: UsageOverview = overview(), tr: UsageTrendPoint[] = trend) {
  api.fetchUsageOverview.mockResolvedValue(ov)
  api.fetchUsageTrend.mockResolvedValue(tr)
  api.fetchToolUsage.mockResolvedValue([
    { slug: 'region-inspect', visits: 100, uses: 70, failures: 3, fail_rate: 0.04 },
    { slug: 'seed-tools', visits: 20, uses: 10, failures: 4, fail_rate: 0.4 },
  ])
  api.fetchResourceUsage.mockResolvedValue([{ slug: 'iris', views: 45 }])
  api.fetchApiUsage.mockResolvedValue([
    {
      route: '/api/v1/resources/{slug}',
      method: 'POST',
      total: 300,
      errors: 0,
      client_errors: 5,
      rate_limited: 1,
      avg_ms: 8.2,
      max_ms: 120,
      p95_ms: 200,
    },
  ])
}

describe('AdminAnalyticsView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('默认按近 30 天加载，渲染概览卡片与明细表', async () => {
    mockSuccess()
    const wrapper = await mountView()

    expect(api.fetchUsageOverview).toHaveBeenCalledWith({ days: 30, limit: 50 })
    expect(api.fetchUsageTrend).toHaveBeenCalledWith({ days: 30, limit: 50 })

    const text = wrapper.text()
    expect(text).toContain('工具访问量')
    expect(text).toContain('工具失败率')
    // 失败率与错误率以百分比展示
    expect(text).toContain('5.0%')
    // 明细表：工具 slug、资源 slug、路由模板
    expect(text).toContain('region-inspect')
    expect(text).toContain('iris')
    expect(text).toContain('/api/v1/resources/{slug}')
    expect(text).toContain('200 ms')
    // 概览范围回显
    expect(text).toContain('2026-10-11 ~ 2026-10-17')
  })

  it('趋势图接线正确：5 条工具/内容 series，3 条 API series，x 轴为日期', async () => {
    mockSuccess()
    const wrapper = await mountView()

    const charts = wrapper.findAllComponents({ name: 'BaseChart' })
    expect(charts).toHaveLength(2)

    const trendOption = charts[0].props('option') as {
      series: Array<{ name: string; data: number[] }>
      xAxis: { data: string[] }
    }
    expect(trendOption.series.map((s) => s.name)).toEqual([
      '工具访问',
      '工具使用',
      '工具失败',
      '资源访问',
      '搜索次数',
    ])
    expect(trendOption.series[0].data).toEqual([60, 60])
    expect(trendOption.xAxis.data).toEqual(['10-16', '10-17'])

    const apiOption = charts[1].props('option') as {
      series: Array<{ name: string; data: number[] }>
    }
    expect(apiOption.series.map((s) => s.name)).toEqual(['请求量', '5xx', '限流 429'])
    expect(apiOption.series[2].data).toEqual([1, 2])
  })

  it('切换预设区间会按天数重新请求', async () => {
    mockSuccess()
    const wrapper = await mountView()
    api.fetchUsageOverview.mockClear()

    const button = wrapper.findAll('button').find((b) => b.text() === '近 7 天')
    expect(button).toBeTruthy()
    await button!.trigger('click')
    await flushPromises()

    expect(api.fetchUsageOverview).toHaveBeenCalledWith({ days: 7, limit: 50 })
  })

  it('自定义起止日期时优先于天数，且不发送 days', async () => {
    mockSuccess()
    const wrapper = await mountView()
    api.fetchUsageOverview.mockClear()

    const inputs = wrapper.findAll('input[type="date"]')
    expect(inputs).toHaveLength(2)
    await inputs[0].setValue('2026-09-01')
    await inputs[1].setValue('2026-09-30')
    const applyBtn = wrapper.findAll('button').find((b) => b.text() === '应用')
    await applyBtn!.trigger('click')
    await flushPromises()

    expect(api.fetchUsageOverview).toHaveBeenCalledWith({
      from: '2026-09-01',
      to: '2026-09-30',
      limit: 50,
    })
  })

  it('全零数据时展示空态提示', async () => {
    mockSuccess(
      overview({
        tool_visits: 0,
        tool_uses: 0,
        tool_failures: 0,
        tool_failure_rate: 0,
        active_tools: 0,
        resource_views: 0,
        active_resources: 0,
        search_queries: 0,
        search_zero_result: 0,
        search_failed: 0,
        api_total: 0,
        api_errors: 0,
        api_client_errors: 0,
        api_rate_limited: 0,
        api_error_rate: 0,
        avg_latency_ms: 0,
        p95_latency_ms: 0,
      }),
      [],
    )
    api.fetchToolUsage.mockResolvedValue([])
    api.fetchResourceUsage.mockResolvedValue([])
    api.fetchApiUsage.mockResolvedValue([])

    const wrapper = await mountView()
    expect(wrapper.text()).toContain('所选范围内暂无统计数据')
    expect(wrapper.text()).toContain('范围内暂无工具使用记录')
  })

  it('接口失败时展示错误提示而非崩溃', async () => {
    api.fetchUsageOverview.mockRejectedValue(new ApiError('统计数据不可用', 'INTERNAL_ERROR'))
    api.fetchUsageTrend.mockResolvedValue([])
    api.fetchToolUsage.mockResolvedValue([])
    api.fetchResourceUsage.mockResolvedValue([])
    api.fetchApiUsage.mockResolvedValue([])

    const wrapper = await mountView()
    expect(wrapper.text()).toContain('统计数据不可用')
  })
})
