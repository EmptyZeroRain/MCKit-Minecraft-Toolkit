<script setup lang="ts" generic="T">
// 管理端通用表格（TODO §4.3）：统一表头、加载态、错误态与空态，行内容由
// `cell-<列 key>` 具名插槽提供（未提供插槽时按字段名兜底渲染）。
//
// 设计取舍：
//   - 只负责「外壳 + 状态分支」，不接管排序 / 选择等业务行为，避免各页被迫改造；
//   - 列定义是纯数据（label / align / width），列宽与对齐风格因此天然统一；
//   - 兜底渲染对空值统一输出「—」，避免各页各写一套 `|| '—'`；
//   - 泛型 `T` 由 `rows` 推导，插槽里的 `row` 因此有精确类型。
//
// 与 `list-shell.vue` 的关系：外壳负责标题 / 加载 / 错误，本组件在需要时也能独立
// 承担这三件事；两者同时使用时不要重复传 `loading` / `error`。
import LoadingSkeleton from '@/components/common/LoadingSkeleton.vue'
import ErrorState from '@/components/common/ErrorState.vue'
import type { AdminTableColumn } from '@/types/admin'

const props = withDefaults(
  defineProps<{
    columns: AdminTableColumn[]
    rows: readonly T[]
    /** 加载中（展示骨架屏）；外层已有 ListShell 时不要重复传 */
    loading?: boolean
    /** 错误信息（展示可重试错误态）；外层已有 ListShell 时不要重复传 */
    error?: string
    /** 数据为空时的提示文案 */
    emptyText?: string
    /** 行 key 取值字段名；不传时回退数组下标 */
    rowKey?: string
    /** 表格最小宽度（窄屏横向滚动，如 `760px`） */
    minWidth?: string
    /** 外层已有卡片容器时置 true，避免出现卡片套卡片 */
    plain?: boolean
  }>(),
  { loading: false, error: '', emptyText: '暂无数据', rowKey: '', minWidth: '', plain: false },
)

defineEmits<{ retry: [] }>()

/** 对齐类名。 */
function alignClass(align?: AdminTableColumn['align']): string {
  if (align === 'center') return 'text-center'
  if (align === 'right') return 'text-right'
  return 'text-left'
}

/** 取单元格原始值（无对应插槽时的兜底渲染）。 */
function cellValue(row: T, key: string): string {
  const value = (row as Record<string, unknown> | null)?.[key]
  if (value === null || value === undefined || value === '') return '—'
  return String(value)
}

/** 行 key（未配置 rowKey 时用下标）。 */
function keyFor(row: T, index: number): string | number {
  if (!props.rowKey) return index
  const value = (row as Record<string, unknown> | null)?.[props.rowKey]
  return value === null || value === undefined ? index : String(value)
}
</script>

<template>
  <LoadingSkeleton v-if="loading" :count="5" :class="plain ? 'p-2' : 'card p-2'" />
  <ErrorState v-else-if="error" :message="error" retryable @retry="$emit('retry')" />
  <div
    v-else-if="!rows.length"
    class="p-10 text-center text-sm text-text-muted"
    :class="plain ? '' : 'card'"
  >
    {{ emptyText }}
  </div>
  <div v-else :class="plain ? 'overflow-x-auto' : 'card overflow-x-auto'">
    <table
      class="w-full text-left text-sm"
      :style="minWidth ? { minWidth } : undefined"
    >
      <thead class="border-b border-border-dark bg-ink-900 text-xs text-text-muted">
        <tr>
          <th
            v-for="col in columns"
            :key="col.key"
            scope="col"
            class="px-4 py-3 font-medium"
            :class="alignClass(col.align)"
            :style="col.width ? { width: col.width } : undefined"
          >
            {{ col.label }}
          </th>
        </tr>
      </thead>
      <tbody class="divide-y divide-border-dark">
        <tr v-for="(row, index) in rows" :key="keyFor(row, index)" class="hover:bg-ink-800/60">
          <td
            v-for="col in columns"
            :key="col.key"
            class="px-4 py-2.5"
            :class="alignClass(col.align)"
          >
            <slot :name="`cell-${col.key}`" :row="row" :index="index">
              {{ cellValue(row, col.key) }}
            </slot>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
