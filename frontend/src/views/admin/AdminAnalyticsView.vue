<script setup lang="ts">
// 运营看板（TODO §13）
// 仅 admin 角色可见（侧边栏过滤 + 后端 403 兜底）。
// 数据为按天聚合的计数与比率：不含 IP / 会话 / 用户标识 / 具体请求参数，
// API 指标只保留路由模板（如 /api/v1/resources/{slug}）。
import { computed, onMounted, reactive, ref } from 'vue'
import { RouterLink } from 'vue-router'
import BaseChart from '@/components/common/BaseChart.vue'
import {
  fetchApiUsage,
  fetchResourceUsage,
  fetchToolUsage,
  fetchUsageOverview,
  fetchUsageTrend,
} from '@/api/admin-analytics'
import { ApiError } from '@/types/api'
import { buildApiTrendOption, buildTrendOption } from '@/utils/analytics-chart'
import type {
  AnalyticsRangeQuery,
  ApiUsageStat,
  ResourceUsageStat,
  ToolUsageStat,
  UsageOverview,
  UsageTrendPoint,
} from '@/types/analytics'

// 预设区间（天数，含当天）。落库周期默认 60s，故最近 1 天数据可能仍在内存中。
const presets = [
  { label: '近 7 天', days: 7 },
  { label: '近 30 天', days: 30 },
  { label: '近 90 天', days: 90 },
]

const filters = reactive<{ days: number | null; from: string; to: string }>({
  days: 30,
  from: '',
  to: '',
})

const overview = ref<UsageOverview | null>(null)
const trend = ref<UsageTrendPoint[]>([])
const tools = ref<ToolUsageStat[]>([])
const resources = ref<ResourceUsageStat[]>([])
const apis = ref<ApiUsageStat[]>([])

const loading = ref(true)
const error = ref('')

/** 当前请求参数：自定义 from/to 优先，其次预设天数。 */
const query = computed<AnalyticsRangeQuery>(() =>
  filters.from || filters.to
    ? { from: filters.from || undefined, to: filters.to || undefined, limit: 50 }
    : { days: filters.days ?? 30, limit: 50 },
)

/** 范围内是否存在任何数据（用于空态提示）。 */
const isEmpty = computed(() => {
  const o = overview.value
  if (!o) return true
  return (
    o.tool_visits === 0 &&
    o.resource_views === 0 &&
    o.search_queries === 0 &&
    o.api_total === 0
  )
})

function applyPreset(days: number) {
  filters.days = days
  filters.from = ''
  filters.to = ''
  void load()
}

function applyCustom() {
  filters.days = null
  void load()
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const params = query.value
    const [o, t, toolRows, resRows, apiRows] = await Promise.all([
      fetchUsageOverview(params),
      fetchUsageTrend(params),
      fetchToolUsage(params),
      fetchResourceUsage(params),
      fetchApiUsage(params),
    ])
    overview.value = o
    trend.value = t
    tools.value = toolRows ?? []
    resources.value = resRows ?? []
    apis.value = apiRows ?? []
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '统计数据加载失败，请稍后重试'
  } finally {
    loading.value = false
  }
}

onMounted(load)

// ===== 展示格式化 =====

function num(v: number | undefined): string {
  if (v === undefined || v === null) return '—'
  return v.toLocaleString('zh-CN')
}

function pct(v: number | undefined): string {
  if (v === undefined || v === null) return '—'
  return `${(v * 100).toFixed(1)}%`
}

function latency(v: number | undefined): string {
  if (v === undefined || v === null) return '—'
  return `${Math.round(v)} ms`
}

/** 严重度配色：失败率 / 错误率越高越醒目。 */
function rateClass(v: number, warn: number, danger: number): string {
  if (v >= danger) return 'text-danger-mc'
  if (v >= warn) return 'text-amber-mc'
  return 'text-text-secondary'
}

// ===== 概览卡片 =====
interface Card {
  label: string
  value: string
  hint?: string
  hintClass?: string
}
const cards = computed<Card[]>(() => {
  const o = overview.value
  if (!o) return []
  return [
    { label: '工具访问量', value: num(o.tool_visits), hint: `${num(o.active_tools)} 个工具有访问` },
    { label: '工具使用量', value: num(o.tool_uses), hint: `失败 ${num(o.tool_failures)} 次` },
    {
      label: '工具失败率',
      value: pct(o.tool_failure_rate),
      hint: '失败数 / 使用数',
      hintClass: rateClass(o.tool_failure_rate, 0.1, 0.3),
    },
    { label: '资源详情访问', value: num(o.resource_views), hint: `${num(o.active_resources)} 个资源` },
    {
      label: '搜索次数',
      value: num(o.search_queries),
      hint: `无结果 ${num(o.search_zero_result)} / 失败 ${num(o.search_failed)}`,
    },
    {
      label: 'API 请求量',
      value: num(o.api_total),
      hint: `错误率 ${pct(o.api_error_rate)}`,
      hintClass: rateClass(o.api_error_rate, 0.01, 0.05),
    },
    {
      label: 'P95 延迟',
      value: latency(o.p95_latency_ms),
      hint: `平均 ${latency(o.avg_latency_ms)}（分桶近似）`,
    },
    {
      label: '限流触发 / 失效链接',
      value: `${num(o.api_rate_limited)} / ${num(o.broken_links)}`,
      hint: '429 次数 / 当前失效链接数',
    },
  ]
})

// ===== 图表配置（中性配色，明暗主题下均可读；见 utils/analytics-chart.ts）=====
const trendOption = computed(() => buildTrendOption(trend.value))
const apiOption = computed(() => buildApiTrendOption(trend.value))

/** 工具明细的失败率进度条宽度（上限 100%）。 */
const failBarWidth = (v: number) => `${Math.min(100, Math.round(v * 100))}%`
</script>

<template>
  <div>
    <div class="mb-6 flex flex-wrap items-start justify-between gap-4">
      <div>
        <h1 class="text-xl font-semibold text-text-primary">运营看板</h1>
        <p class="mt-1 text-sm text-text-secondary">
          按天聚合的使用统计：不含 IP、会话与用户标识；API 指标只保留路由模板，延迟为分桶近似值。
        </p>
      </div>
      <RouterLink
        to="/admin/search-terms"
        class="shrink-0 rounded-md border border-border-light bg-ink-700 px-3 py-2 text-xs text-text-primary hover:bg-ink-600"
      >
        查看高频无结果词
      </RouterLink>
    </div>

    <!-- 时间范围 -->
    <div class="card mb-5 flex flex-wrap items-end gap-3 p-4">
      <div class="flex flex-wrap gap-2">
        <button
          v-for="p in presets"
          :key="p.days"
          type="button"
          class="rounded-md border px-3 py-1.5 text-xs transition"
          :class="
            filters.days === p.days && !filters.from && !filters.to
              ? 'border-grass-500 bg-grass-500/15 text-grass-300'
              : 'border-border-light text-text-secondary hover:text-text-primary'
          "
          @click="applyPreset(p.days)"
        >
          {{ p.label }}
        </button>
      </div>
      <div class="flex flex-wrap items-end gap-2">
        <label class="text-xs text-text-secondary">
          起
          <input
            v-model="filters.from"
            type="date"
            class="mt-1 block h-9 rounded-md border border-border-dark bg-ink-800 px-2 text-xs text-text-primary focus:border-grass-500 focus:outline-none"
          />
        </label>
        <label class="text-xs text-text-secondary">
          止
          <input
            v-model="filters.to"
            type="date"
            class="mt-1 block h-9 rounded-md border border-border-dark bg-ink-800 px-2 text-xs text-text-primary focus:border-grass-500 focus:outline-none"
          />
        </label>
        <button
          type="button"
          class="h-9 rounded-md border border-border-light bg-ink-700 px-4 text-xs text-text-primary hover:bg-ink-600"
          @click="applyCustom"
        >
          应用
        </button>
      </div>
      <span v-if="overview" class="ml-auto text-xs text-text-muted">
        统计范围 {{ overview.from }} ~ {{ overview.to }}
      </span>
    </div>

    <div
      v-if="error"
      role="alert"
      class="mb-5 rounded-card border border-danger-mc/40 bg-danger-mc/10 p-4 text-sm text-danger-mc"
    >
      {{ error }}
    </div>

    <div v-if="loading" class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
      <div v-for="i in 8" :key="i" class="card h-24 animate-pulse" />
    </div>

    <template v-else>
      <div v-if="isEmpty" class="mb-5 rounded-card border border-border-light bg-ink-800/60 p-4 text-sm text-text-secondary">
        所选范围内暂无统计数据。统计为访问时采集、内存聚合后按周期落库
        （默认 60 秒），刚产生的访问可能尚未出现在这里。
      </div>

      <!-- 概览卡片 -->
      <div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <div v-for="c in cards" :key="c.label" class="card p-5">
          <div class="text-sm text-text-secondary">{{ c.label }}</div>
          <div class="mt-2 text-3xl font-bold text-text-primary">{{ c.value }}</div>
          <div v-if="c.hint" class="mt-1 text-xs" :class="c.hintClass ?? 'text-text-muted'">
            {{ c.hint }}
          </div>
        </div>
      </div>

      <!-- 趋势图 -->
      <div class="mt-6 grid gap-4 xl:grid-cols-2">
        <div class="card p-5">
          <h2 class="mb-2 font-medium text-text-primary">工具与内容趋势</h2>
          <BaseChart :option="trendOption" height="18rem" aria-label="工具访问、使用、失败与资源访问、搜索次数按天趋势" />
        </div>
        <div class="card p-5">
          <h2 class="mb-2 font-medium text-text-primary">API 请求与错误趋势</h2>
          <BaseChart :option="apiOption" height="18rem" aria-label="API 请求量、5xx 与限流次数按天趋势" />
        </div>
      </div>

      <!-- 工具明细 -->
      <div class="card mt-6 overflow-hidden">
        <h2 class="border-b border-border-dark px-5 py-3 font-medium text-text-primary">工具使用明细</h2>
        <div v-if="tools.length === 0" class="px-5 py-6 text-sm text-text-muted">范围内暂无工具使用记录。</div>
        <div v-else class="overflow-x-auto">
          <table class="w-full text-left text-sm">
            <thead class="text-xs text-text-muted">
              <tr class="border-b border-border-dark">
                <th scope="col" class="px-5 py-2 font-normal">工具 slug</th>
                <th scope="col" class="px-3 py-2 text-right font-normal">访问</th>
                <th scope="col" class="px-3 py-2 text-right font-normal">使用</th>
                <th scope="col" class="px-3 py-2 text-right font-normal">失败</th>
                <th scope="col" class="px-5 py-2 font-normal">失败率</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="t in tools" :key="t.slug" class="border-b border-border-dark/60 last:border-0">
                <td class="px-5 py-2 font-mono text-xs text-text-primary">{{ t.slug }}</td>
                <td class="px-3 py-2 text-right text-text-secondary">{{ num(t.visits) }}</td>
                <td class="px-3 py-2 text-right text-text-secondary">{{ num(t.uses) }}</td>
                <td class="px-3 py-2 text-right text-text-secondary">{{ num(t.failures) }}</td>
                <td class="px-5 py-2">
                  <div class="flex items-center gap-2">
                    <div class="h-1.5 w-24 overflow-hidden rounded-full bg-ink-600">
                      <div
                        class="h-full rounded-full"
                        :class="t.fail_rate >= 0.3 ? 'bg-danger-mc' : t.fail_rate >= 0.1 ? 'bg-amber-mc' : 'bg-grass-500'"
                        :style="{ width: failBarWidth(t.fail_rate) }"
                      />
                    </div>
                    <span :class="rateClass(t.fail_rate, 0.1, 0.3)">{{ pct(t.fail_rate) }}</span>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <div class="mt-6 grid gap-4 xl:grid-cols-2">
        <!-- 资源明细 -->
        <div class="card overflow-hidden">
          <h2 class="border-b border-border-dark px-5 py-3 font-medium text-text-primary">资源详情访问 Top</h2>
          <div v-if="resources.length === 0" class="px-5 py-6 text-sm text-text-muted">范围内暂无资源访问记录。</div>
          <div v-else class="overflow-x-auto">
            <table class="w-full text-left text-sm">
              <thead class="text-xs text-text-muted">
                <tr class="border-b border-border-dark">
                  <th scope="col" class="px-5 py-2 font-normal">资源 slug</th>
                  <th scope="col" class="px-5 py-2 text-right font-normal">访问量</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="r in resources" :key="r.slug" class="border-b border-border-dark/60 last:border-0">
                  <td class="px-5 py-2 font-mono text-xs text-text-primary">{{ r.slug }}</td>
                  <td class="px-5 py-2 text-right text-text-secondary">{{ num(r.views) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>

        <!-- API 明细 -->
        <div class="card overflow-hidden">
          <h2 class="border-b border-border-dark px-5 py-3 font-medium text-text-primary">关键 API 指标</h2>
          <div v-if="apis.length === 0" class="px-5 py-6 text-sm text-text-muted">范围内暂无 API 调用记录。</div>
          <div v-else class="overflow-x-auto">
            <table class="w-full text-left text-sm">
              <thead class="text-xs text-text-muted">
                <tr class="border-b border-border-dark">
                  <th scope="col" class="px-5 py-2 font-normal">路由模板</th>
                  <th scope="col" class="px-3 py-2 text-right font-normal">请求</th>
                  <th scope="col" class="px-3 py-2 text-right font-normal">5xx</th>
                  <th scope="col" class="px-3 py-2 text-right font-normal">429</th>
                  <th scope="col" class="px-5 py-2 text-right font-normal">P95</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="a in apis" :key="`${a.method} ${a.route}`" class="border-b border-border-dark/60 last:border-0">
                  <td class="px-5 py-2">
                    <span class="mr-2 rounded bg-ink-600 px-1.5 py-0.5 font-mono text-[10px] text-text-secondary">
                      {{ a.method }}
                    </span>
                    <span class="font-mono text-xs text-text-primary">{{ a.route }}</span>
                  </td>
                  <td class="px-3 py-2 text-right text-text-secondary">{{ num(a.total) }}</td>
                  <td class="px-3 py-2 text-right" :class="a.errors > 0 ? 'text-danger-mc' : 'text-text-secondary'">
                    {{ num(a.errors) }}
                  </td>
                  <td class="px-3 py-2 text-right" :class="a.rate_limited > 0 ? 'text-amber-mc' : 'text-text-secondary'">
                    {{ num(a.rate_limited) }}
                  </td>
                  <td class="px-5 py-2 text-right text-text-secondary">{{ a.p95_ms }} ms</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>
