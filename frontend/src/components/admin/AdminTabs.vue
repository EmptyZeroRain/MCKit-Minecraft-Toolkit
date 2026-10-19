<script setup lang="ts">
// 后台合并页的 Tab 栏 + 无权限兜底（配合 useAdminTabs 使用）。
// 统一交互与无障碍语义（role=tablist/tab + aria-selected），各合并页只关心内容。
import EmptyState from '@/components/common/EmptyState.vue'
import type { AdminTab } from '@/composables/useAdminTabs'

defineProps<{
  tabs: AdminTab[]
  active: string
  ariaLabel?: string
  /** 无任何权限时的说明文案 */
  emptyDescription?: string
}>()

defineEmits<{ select: [key: string] }>()
</script>

<template>
  <div>
    <div
      v-if="tabs.length"
      class="mb-5 flex gap-1 border-b border-border-dark"
      role="tablist"
      :aria-label="ariaLabel ?? '标签页'"
    >
      <button
        v-for="t in tabs"
        :key="t.key"
        type="button"
        role="tab"
        :aria-selected="active === t.key"
        class="-mb-px border-b-2 px-4 py-2 text-sm transition focus:outline-none"
        :class="
          active === t.key
            ? 'border-grass-500 text-grass-300'
            : 'border-transparent text-text-secondary hover:text-text-primary'
        "
        @click="$emit('select', t.key)"
      >
        {{ t.label }}
      </button>
    </div>

    <EmptyState
      v-if="!tabs.length"
      title="无权限访问"
      :description="emptyDescription ?? '当前账号没有访问该页面的权限，请联系管理员分配。'"
    />
    <slot v-else />
  </div>
</template>
