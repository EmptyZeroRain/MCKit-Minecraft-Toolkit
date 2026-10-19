<script setup lang="ts">
// ECharts 薄封装（TODO §13）。
// 只注册用到的图表与组件以控制打包体积；不做主题注入，改用明暗主题下都可读的
// 中性配色（由调用方在 option 中给出），因此无需监听主题切换。
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { init, use, type ECharts, type EChartsCoreOption } from 'echarts/core'
import { BarChart, LineChart } from 'echarts/charts'
import { GridComponent, LegendComponent, TooltipComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'

use([BarChart, LineChart, GridComponent, LegendComponent, TooltipComponent, CanvasRenderer])

const props = withDefaults(
  defineProps<{
    option: EChartsCoreOption
    height?: string
    /** 无障碍描述（图表本身对读屏不可读，需文字替代） */
    ariaLabel?: string
  }>(),
  { height: '18rem', ariaLabel: '数据图表' },
)

const el = ref<HTMLDivElement | null>(null)
let chart: ECharts | null = null
let resizeObserver: ResizeObserver | null = null

function render() {
  // notMerge=true：区间或数据源切换时避免残留上一次的系列。
  chart?.setOption(props.option, true)
}

onMounted(() => {
  if (!el.value) return
  chart = init(el.value)
  render()
  if (typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver(() => chart?.resize())
    resizeObserver.observe(el.value)
  }
})

watch(() => props.option, render, { deep: true })

onBeforeUnmount(() => {
  resizeObserver?.disconnect()
  resizeObserver = null
  chart?.dispose()
  chart = null
})
</script>

<template>
  <div ref="el" :style="{ height }" class="w-full" role="img" :aria-label="ariaLabel" />
</template>
