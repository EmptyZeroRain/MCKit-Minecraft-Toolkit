// @vitest-environment node
// 图表配置测试（TODO §13）
// 用真实 ECharts 引擎（SSR + SVG 渲染器）渲染 utils/analytics-chart.ts 产出的 option，
// 确认配置合法、图例与坐标轴可实际输出——jsdom 无 canvas，故本文件使用 node 环境。
// 这样即使没有浏览器，也能验证「页面图表在真实引擎下不会因配置错误而失败」。
import { describe, expect, it } from 'vitest'
import { init, use } from 'echarts/core'
import { LineChart } from 'echarts/charts'
import { GridComponent, LegendComponent, TooltipComponent } from 'echarts/components'
import { SVGRenderer } from 'echarts/renderers'
import { buildApiTrendOption, buildTrendOption } from '@/utils/analytics-chart'
import type { UsageTrendPoint } from '@/types/analytics'

use([LineChart, GridComponent, LegendComponent, TooltipComponent, SVGRenderer])

const points: UsageTrendPoint[] = [
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
    tool_visits: 70,
    tool_uses: 45,
    tool_failures: 3,
    resource_views: 25,
    search_queries: 18,
    api_total: 500,
    api_errors: 2,
    api_rate_limited: 4,
  },
]

/** 用真实 ECharts 渲染指定 option，返回 SVG 字符串。 */
function renderToSvg(option: Parameters<ReturnType<typeof init>['setOption']>[0]) {
  const chart = init(null, null, { renderer: 'svg', ssr: true, width: 800, height: 320 })
  chart.setOption(option)
  const svg = chart.renderToSVGString()
  chart.dispose()
  return svg
}

describe('运营看板图表配置', () => {
  it('工具趋势：5 条折线、日期轴为 MM-DD、数据按天对齐', () => {
    const option = buildTrendOption(points) as {
      series: Array<{ name: string; data: number[] }>
      xAxis: { data: string[] }
    }
    expect(option.series.map((s) => s.name)).toEqual([
      '工具访问',
      '工具使用',
      '工具失败',
      '资源访问',
      '搜索次数',
    ])
    expect(option.series[0].data).toEqual([60, 70])
    expect(option.series[3].data).toEqual([20, 25])
    expect(option.xAxis.data).toEqual(['10-16', '10-17'])
  })

  it('API 趋势：请求量 / 5xx / 限流三条线', () => {
    const option = buildApiTrendOption(points) as {
      series: Array<{ name: string; data: number[] }>
    }
    expect(option.series.map((s) => s.name)).toEqual(['请求量', '5xx', '限流 429'])
    expect(option.series[2].data).toEqual([1, 4])
  })

  it('空数据时不抛错，仍产出合法 option（可渲染空坐标轴）', () => {
    const option = buildTrendOption([])
    const svg = renderToSvg(option)
    expect(svg).toContain('<svg')
  })

  it('真实 ECharts（SSR/SVG）能渲染两张图并输出图例文字', () => {
    const trendSvg = renderToSvg(buildTrendOption(points))
    expect(trendSvg).toContain('<svg')
    expect(trendSvg).toContain('工具访问')
    expect(trendSvg).toContain('搜索次数')
    expect(trendSvg).toContain('10-16')

    const apiSvg = renderToSvg(buildApiTrendOption(points))
    expect(apiSvg).toContain('请求量')
    expect(apiSvg).toContain('限流 429')
    expect(apiSvg).toContain('10-17')
  })
})
