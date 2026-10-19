// ===== 运营看板图表配置（纯函数，TODO §13）=====
// 抽出为纯函数的原因：
//   1) 视图只负责取数与排版，图表配置可独立单测（含真实 ECharts 渲染校验）；
//   2) ECharts 的 option 体积大且易错，集中在处便于维护。
// 配色为「明暗主题下都可读」的中性色板，因此无需监听主题切换。
import type { EChartsCoreOption } from 'echarts/core'
import type { UsageTrendPoint } from '@/types/analytics'

/** 工具与内容趋势配色 */
export const TREND_PALETTE = ['#5eb845', '#63b3ed', '#f6ad55', '#b794f4', '#fc8181']
/** API 趋势配色（请求量 / 5xx / 限流） */
export const API_PALETTE = ['#63b3ed', '#fc8181', '#f6ad55']

const AXIS_COLOR = '#8b98b3'
const GRID_LINE = 'rgba(139, 152, 179, 0.18)'

/** 日期轴标签只显示 MM-DD，避免长区间挤压。 */
function shortDate(date: string): string {
  return date.slice(5)
}

/** 折线图公共骨架（直角坐标系 + 图例 + 坐标轴样式） */
function lineBase(dates: string[], colors: string[]): EChartsCoreOption {
  return {
    color: colors,
    tooltip: { trigger: 'axis' },
    legend: { top: 0, textStyle: { color: AXIS_COLOR } },
    grid: { left: 8, right: 16, top: 40, bottom: 8, containLabel: true },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: dates,
      axisLabel: { color: AXIS_COLOR },
      axisLine: { lineStyle: { color: GRID_LINE } },
    },
    yAxis: {
      type: 'value',
      axisLabel: { color: AXIS_COLOR },
      splitLine: { lineStyle: { color: GRID_LINE } },
    },
    series: [],
  }
}

/** 工具 / 资源 / 搜索按天趋势。 */
export function buildTrendOption(points: UsageTrendPoint[]): EChartsCoreOption {
  const option = lineBase(
    points.map((p) => shortDate(p.date)),
    TREND_PALETTE,
  ) as Record<string, unknown>
  option.series = [
    { name: '工具访问', type: 'line', smooth: true, data: points.map((p) => p.tool_visits) },
    { name: '工具使用', type: 'line', smooth: true, data: points.map((p) => p.tool_uses) },
    { name: '工具失败', type: 'line', smooth: true, data: points.map((p) => p.tool_failures) },
    { name: '资源访问', type: 'line', smooth: true, data: points.map((p) => p.resource_views) },
    { name: '搜索次数', type: 'line', smooth: true, data: points.map((p) => p.search_queries) },
  ]
  return option
}

/** API 请求量 / 5xx / 限流按天趋势。 */
export function buildApiTrendOption(points: UsageTrendPoint[]): EChartsCoreOption {
  const option = lineBase(
    points.map((p) => shortDate(p.date)),
    API_PALETTE,
  ) as Record<string, unknown>
  option.series = [
    { name: '请求量', type: 'line', smooth: true, data: points.map((p) => p.api_total) },
    { name: '5xx', type: 'line', smooth: true, data: points.map((p) => p.api_errors) },
    { name: '限流 429', type: 'line', smooth: true, data: points.map((p) => p.api_rate_limited) },
  ]
  return option
}
