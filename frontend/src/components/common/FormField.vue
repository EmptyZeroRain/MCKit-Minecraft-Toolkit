<script setup lang="ts">
// 表单字段包装：可见标签 + 错误提示 + 必填星号（需求 4.2）
import { useId } from 'vue'

defineProps<{
  label: string
  required?: boolean
  error?: string
  hint?: string
}>()

const uid = useId()
</script>

<template>
  <div class="space-y-1.5">
    <label :for="uid" class="block text-sm font-medium text-text-primary">
      {{ label }}
      <span v-if="required" class="text-danger-mc" aria-hidden="true"> *</span>
    </label>
    <slot :id="uid" />
    <p v-if="hint && !error" class="text-xs text-text-muted">{{ hint }}</p>
    <p v-if="error" class="text-xs text-danger-mc" role="alert">{{ error }}</p>
  </div>
</template>