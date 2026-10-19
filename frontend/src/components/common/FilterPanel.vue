<script setup lang="ts">
// 筛选面板：通用单选/多选筛选
export interface FilterOption {
  label: string
  value: string
}

const props = defineProps<{
  title: string
  options: FilterOption[]
  modelValue: string
  /** 是否允许取消选择（点击当前项清除） */
  clearable?: boolean
  /** '全部' 的展示文本 */
  allLabel?: string
}>()

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

function select(value: string) {
  if (props.clearable && value === props.modelValue) {
    emit('update:modelValue', '')
    return
  }
  emit('update:modelValue', value)
}
</script>

<template>
  <div class="card p-4">
    <h3 class="mb-3 text-sm font-semibold text-text-primary">{{ title }}</h3>
    <div class="flex flex-wrap gap-2">
      <button
        type="button"
        class="rounded-full border px-3 py-1 text-xs transition"
        :class="
          modelValue === ''
            ? 'border-grass-500 bg-grass-500/15 text-grass-300'
            : 'border-border-dark bg-ink-700 text-text-secondary hover:bg-ink-600'
        "
        @click="select('')"
      >
        {{ allLabel ?? '全部' }}
      </button>
      <button
        v-for="opt in options"
        :key="opt.value"
        type="button"
        class="rounded-full border px-3 py-1 text-xs transition"
        :class="
          modelValue === opt.value
            ? 'border-grass-500 bg-grass-500/15 text-grass-300'
            : 'border-border-dark bg-ink-700 text-text-secondary hover:bg-ink-600'
        "
        @click="select(opt.value)"
      >
        {{ opt.label }}
      </button>
    </div>
  </div>
</template>