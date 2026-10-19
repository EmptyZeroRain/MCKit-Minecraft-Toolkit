<script setup lang="ts">
// 状态徽章：通用 status -> 颜色映射
import { computed } from 'vue'

const props = defineProps<{
  /** 状态值 */
  status: string
  /** 展示文本；缺省时使用状态值本身 */
  label?: string
}>()

type Palette = 'green' | 'amber' | 'red' | 'blue' | 'gray' | 'purple'

const STATUS_STYLE: Record<string, { text: string; cls: string; palette: Palette }> = {
  published: { text: '已发布', cls: 'border-grass-500/40 bg-grass-500/10 text-grass-300', palette: 'green' },
  draft: { text: '草稿', cls: 'border-amber-mc/40 bg-amber-mc/10 text-amber-mc', palette: 'amber' },
  hidden: { text: '已隐藏', cls: 'border-border-light bg-ink-700 text-text-secondary', palette: 'gray' },
  removed: { text: '已下架', cls: 'border-danger-mc/40 bg-danger-mc/10 text-danger-mc', palette: 'red' },
  new: { text: '新提交', cls: 'border-blue-400/40 bg-blue-400/10 text-blue-300', palette: 'blue' },
  processing: { text: '处理中', cls: 'border-amber-mc/40 bg-amber-mc/10 text-amber-mc', palette: 'amber' },
  resolved: { text: '已解决', cls: 'border-grass-500/40 bg-grass-500/10 text-grass-300', palette: 'green' },
  ignored: { text: '已忽略', cls: 'border-border-light bg-ink-700 text-text-muted', palette: 'gray' },
  success: { text: '成功', cls: 'border-grass-500/40 bg-grass-500/10 text-grass-300', palette: 'green' },
  failure: { text: '失败', cls: 'border-danger-mc/40 bg-danger-mc/10 text-danger-mc', palette: 'red' },
}

const style = computed(() => {
  const s = STATUS_STYLE[props.status]
  if (s) return s.cls
  // 兜底：按首字符颜色
  return 'border-border-light bg-ink-700 text-text-secondary'
})

const text = computed(() => props.label ?? STATUS_STYLE[props.status]?.text ?? props.status)
</script>

<template>
  <span
    class="inline-flex items-center rounded-full border px-2 py-0.5 text-xs font-medium"
    :class="style"
  >
    {{ text }}
  </span>
</template>