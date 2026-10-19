<script setup lang="ts">
// 资源管理列表（需求 6.3 / 5.8 链接检查）
import { onMounted, reactive, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
import ListShell from '@/components/admin/list-shell.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import StatusBadge from '@/components/common/StatusBadge.vue'
import ResourceTypeBadge from '@/components/badges/ResourceTypeBadge.vue'
import Pagination from '@/components/common/Pagination.vue'
import { useUiStore } from '@/stores/ui'
import {
  listResources,
  deleteResource,
  checkResourceLink,
  exportResourcesCsv,
} from '@/api/admin-resources'
import type { Resource } from '@/types/resource'
import {
  RESOURCE_TYPES,
  RESOURCE_TYPE_LABELS,
  RESOURCE_STATUSES,
  type ResourceType,
  type ResourceStatus,
} from '@/types/common'
import { ApiError } from '@/types/api'
import { timeAgo } from '@/utils/format'

const ui = useUiStore()

const filters = reactive<{
  q: string
  type: ResourceType | ''
  status: ResourceStatus | ''
  page: number
  pageSize: number
}>({
  q: '',
  type: '',
  status: '',
  page: 1,
  pageSize: 15,
})

const items = ref<Resource[]>([])
const total = ref(0)
const totalPages = ref(1)
const loading = ref(false)
const exporting = ref(false)
const error = ref('')

const toDelete = ref<Resource | null>(null)
const deleting = ref(false)
const checkingId = ref<string | null>(null)

async function load() {
  loading.value = true
  error.value = ''
  try {
    const res = await listResources({
      q: filters.q || undefined,
      type: filters.type || undefined,
      status: filters.status || undefined,
      page: filters.page,
      page_size: filters.pageSize,
    })
    items.value = res.items
    total.value = res.total
    totalPages.value = res.totalPages || Math.max(1, Math.ceil(res.total / filters.pageSize))
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '加载失败'
  } finally {
    loading.value = false
  }
}

watch([() => filters.type, () => filters.status], () => {
  filters.page = 1
  load()
})

function onPageChange(p: number) {
  filters.page = p
  load()
}

async function confirmDelete() {
  if (!toDelete.value) return
  deleting.value = true
  try {
    await deleteResource(toDelete.value.id)
    ui.success('已删除/下架资源')
    toDelete.value = null
    load()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '删除失败')
  } finally {
    deleting.value = false
  }
}

async function handleCheckLink(item: Resource) {
  checkingId.value = item.id
  try {
    const result = await checkResourceLink(item.id)
    ui.success(`链接检查完成：${result.status}`)
    load()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '链接检查失败')
  } finally {
    checkingId.value = null
  }
}

/** 按当前筛选导出 CSV（只读、后端最多 5000 条）。 */
async function handleExport() {
  if (exporting.value) return
  exporting.value = true
  try {
    const { blob, truncated } = await exportResourcesCsv({
      q: filters.q || undefined,
      type: filters.type || undefined,
      status: filters.status || undefined,
    })
    triggerDownload(blob)
    ui.success(truncated ? '已导出（超出上限，仅含前 5000 条）' : '已导出 CSV')
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '导出失败')
  } finally {
    exporting.value = false
  }
}

function triggerDownload(blob: Blob) {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `resources-${new Date().toISOString().slice(0, 10)}.csv`
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}

onMounted(load)
</script>

<template>
  <div class="space-y-4">
    <ListShell title="资源管理" :loading="loading" :error="error" @retry="load">
      <div class="mb-4 flex flex-wrap items-center gap-3">
        <div class="relative min-w-52 flex-1">
          <input
            v-model="filters.q"
            type="search"
            aria-label="搜索资源"
            placeholder="搜索名称 / 作者 / URL"
            class="h-9 w-full rounded-md border border-border-dark bg-ink-800 px-3 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
          />
        </div>
        <select
          v-model="filters.type"
          class="h-9 rounded-md border border-border-dark bg-ink-800 px-2 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
        >
          <option value="">全部类型</option>
          <option v-for="t in RESOURCE_TYPES" :key="t" :value="t">{{ RESOURCE_TYPE_LABELS[t] }}</option>
        </select>
        <select
          v-model="filters.status"
          class="h-9 rounded-md border border-border-dark bg-ink-800 px-2 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
        >
          <option value="">全部状态</option>
          <option v-for="s in RESOURCE_STATUSES" :key="s" :value="s">{{ s }}</option>
        </select>
        <button
          type="button"
          class="h-9 rounded-md border border-border-dark px-3 text-sm text-text-secondary transition hover:bg-ink-700 disabled:opacity-50"
          :disabled="exporting || loading"
          @click="handleExport"
        >
          {{ exporting ? '导出中…' : '导出 CSV' }}
        </button>
        <RouterLink
          to="/admin/resources/new"
          class="rounded-md bg-grass-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-grass-500"
        >
          + 新建资源
        </RouterLink>
      </div>

      <div v-if="items.length" class="card overflow-x-auto">
        <table class="w-full min-w-[720px] text-left text-sm">
          <thead class="border-b border-border-dark bg-ink-900 text-xs text-text-muted">
            <tr>
              <th scope="col" class="px-4 py-2.5">名称</th>
              <th scope="col" class="px-4 py-2.5">类型</th>
              <th scope="col" class="px-4 py-2.5">状态</th>
              <th scope="col" class="px-4 py-2.5">链接</th>
              <th scope="col" class="px-4 py-2.5">更新</th>
              <th scope="col" class="px-4 py-2.5 text-right">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-border-dark">
            <tr v-for="item in items" :key="item.id" class="hover:bg-ink-800/60">
              <td class="px-4 py-3">
                <div class="font-medium text-text-primary">{{ item.name }}</div>
                <div class="text-xs text-text-muted">{{ item.slug }}</div>
              </td>
              <td class="px-4 py-3">
                <ResourceTypeBadge :type="item.resource_type" />
              </td>
              <td class="px-4 py-3"><StatusBadge :status="item.status" /></td>
              <td class="px-4 py-3">
                <div class="flex items-center gap-1.5 text-xs">
                  <span
                    class="inline-block h-1.5 w-1.5 rounded-full"
                    :class="{
                      'bg-grass-400': item.link_status === 'ok',
                      'bg-amber-mc': item.link_status === 'redirected',
                      'bg-danger-mc': ['broken', 'error'].includes(item.link_status),
                      'bg-text-muted': item.link_status === 'unchecked' || item.link_status === 'blocked',
                    }"
                  />
                  {{ item.link_status }}
                </div>
              </td>
              <td class="px-4 py-3 text-xs text-text-muted">{{ timeAgo(item.updated_at) }}</td>
              <td class="px-4 py-3">
                <div class="flex justify-end gap-1.5 text-xs">
                  <button
                    type="button"
                    class="rounded border border-border-light px-2 py-1 text-text-secondary hover:bg-ink-700"
                    :disabled="checkingId === item.id"
                    @click="handleCheckLink(item)"
                  >
                    {{ checkingId === item.id ? '检查中…' : '检查链接' }}
                  </button>
                  <RouterLink
                    :to="`/admin/resources/${item.id}/edit`"
                    class="rounded border border-border-light px-2 py-1 text-grass-400 hover:bg-ink-700"
                  >
                    编辑
                  </RouterLink>
                  <button
                    type="button"
                    class="rounded border border-danger-mc/40 px-2 py-1 text-danger-mc hover:bg-danger-mc/10"
                    @click="toDelete = item"
                  >
                    删除
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <div v-else-if="!loading" class="card p-10 text-center text-sm text-text-muted">
        暂无资源，点击右上角“新建资源”添加。
      </div>
    </ListShell>

    <Pagination
      :page="filters.page"
      :page-size="filters.pageSize"
      :total="total"
      :total-pages="totalPages"
      @change="onPageChange"
    />

    <ConfirmDialog
      :open="!!toDelete"
      title="删除资源"
      :message="`确定删除「${toDelete?.name}」吗？此操作将软删除或下架该资源。`"
      confirm-text="确认删除"
      dangerous
      :confirm-disabled="deleting"
      @confirm="confirmDelete"
      @cancel="toDelete = null"
    />
  </div>
</template>