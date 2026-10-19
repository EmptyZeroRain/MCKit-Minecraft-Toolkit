<script setup lang="ts">
// 统一的「版本不支持 / 版本不确定」提示（TODO §2）。
// 由 tool-versions.ts 的结论驱动，各工具页共用同一视觉与措辞。
import type { SupportLevel } from '@/utils/tool-versions'

const props = withDefaults(
  defineProps<{
    level: SupportLevel
    message: string
    /** 自定义标题；缺省按级别取「版本不支持」/「版本不确定」 */
    title?: string
  }>(),
  { title: '' },
)

const DEFAULT_TITLE: Record<SupportLevel, string> = {
  supported: '',
  unsupported: '版本不支持',
  unknown: '版本不确定',
}

function heading(): string {
  return props.title || DEFAULT_TITLE[props.level]
}
</script>

<template>
  <p
    v-if="level !== 'supported' && message"
    role="status"
    class="flex gap-2 rounded-md border px-3 py-2 text-sm"
    :class="
      level === 'unsupported'
        ? 'border-amber-500/40 bg-amber-500/10 text-amber-200'
        : 'border-border-light bg-ink-800 text-text-secondary'
    "
  >
    <span class="shrink-0 font-medium">{{ heading() }}：</span>
    <span class="min-w-0">{{ message }}</span>
  </p>
</template>
