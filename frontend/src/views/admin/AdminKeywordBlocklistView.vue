<script setup lang="ts">
// 关键字屏蔽（内容健康 / 需 ops.dataquality 权限）。
//
// 规则在服务端匹配：用户发帖、发表评论时若正文 / 标题命中启用中的关键字，
// 接口直接拒绝。本页只维护规则，不会批量扫描或删除历史内容。
import { onMounted, reactive, ref } from 'vue'
import AdminTable from '@/components/admin/AdminTable.vue'
import ListShell from '@/components/admin/list-shell.vue'
import { useUiStore } from '@/stores/ui'
import { ApiError } from '@/types/api'
import type { AdminTableColumn } from '@/types/admin'
import {
  createContentKeyword,
  deleteContentKeyword,
  listContentKeywords,
  updateContentKeyword,
} from '@/api/admin-content-filter'
import type { ContentKeyword } from '@/types/content-filter'
import { formatDateTime } from '@/utils/format'

const ui = useUiStore()

const inputClass =
  'w-full rounded-md border border-border-dark bg-ink-900 px-3 py-2 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none'

const loading = ref(false)
const error = ref('')
const items = ref<ContentKeyword[]>([])
const saving = ref(false)
const busyId = ref('')
const editingId = ref('')
const formError = ref('')
const form = reactive({ keyword: '', note: '', enabled: true })

const columns: AdminTableColumn[] = [
  { key: 'keyword', label: '关键字' },
  { key: 'note', label: '备注' },
  { key: 'enabled', label: '状态', width: '90px' },
  { key: 'created_at', label: '创建时间', width: '170px' },
  { key: 'action', label: '操作', align: 'right', width: '150px' },
]

async function load() {
  loading.value = true
  error.value = ''
  try {
    items.value = await listContentKeywords()
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '关键字加载失败'
  } finally {
    loading.value = false
  }
}

function resetForm() {
  editingId.value = ''
  formError.value = ''
  form.keyword = ''
  form.note = ''
  form.enabled = true
}

function edit(item: ContentKeyword) {
  editingId.value = item.id
  formError.value = ''
  form.keyword = item.keyword
  form.note = item.note
  form.enabled = item.enabled
}

async function save() {
  formError.value = ''
  if (!form.keyword.trim()) {
    formError.value = '关键字不能为空'
    return
  }
  saving.value = true
  try {
    if (editingId.value) {
      await updateContentKeyword({
        id: editingId.value,
        keyword: form.keyword.trim(),
        note: form.note.trim(),
        enabled: form.enabled,
      })
      ui.success('关键字已更新')
    } else {
      await createContentKeyword(form.keyword.trim(), form.note.trim())
      ui.success('关键字已新增')
    }
    resetForm()
    await load()
  } catch (err) {
    formError.value = err instanceof ApiError ? err.message : '保存失败'
  } finally {
    saving.value = false
  }
}

async function toggle(item: ContentKeyword) {
  busyId.value = item.id
  try {
    const updated = await updateContentKeyword({
      id: item.id,
      keyword: item.keyword,
      note: item.note,
      enabled: !item.enabled,
    })
    const index = items.value.findIndex((row) => row.id === item.id)
    if (index >= 0) items.value[index] = updated
    ui.success(updated.enabled ? '关键字已启用' : '关键字已停用')
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '操作失败')
  } finally {
    busyId.value = ''
  }
}

async function remove(item: ContentKeyword) {
  if (!window.confirm(`确定删除关键字「${item.keyword}」？`)) return
  busyId.value = item.id
  try {
    await deleteContentKeyword(item.id)
    ui.success('关键字已删除')
    if (editingId.value === item.id) resetForm()
    await load()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '删除失败')
  } finally {
    busyId.value = ''
  }
}

onMounted(load)
</script>

<template>
  <div class="space-y-4">
    <section class="card p-4">
      <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
        <div>
          <h2 class="text-sm font-medium text-text-primary">
            {{ editingId ? '编辑屏蔽关键字' : '新增屏蔽关键字' }}
          </h2>
          <p class="mt-0.5 text-xs text-text-muted">
            启用后，帖子标题 / 正文或用户评论命中关键字时会被拒绝提交；不追溯删除历史内容。
          </p>
        </div>
        <button
          v-if="editingId"
          type="button"
          class="rounded border border-border-light px-3 py-1.5 text-xs text-text-secondary transition hover:bg-ink-700"
          @click="resetForm"
        >
          取消编辑
        </button>
      </div>

      <form class="grid gap-3 sm:grid-cols-[minmax(0,1fr)_minmax(0,1.5fr)_auto_auto]" @submit.prevent="save">
        <input
          v-model="form.keyword"
          :class="inputClass"
          type="text"
          maxlength="100"
          placeholder="关键字，如：外挂 / 代练"
        />
        <input v-model="form.note" :class="inputClass" type="text" maxlength="255" placeholder="备注（可选）" />
        <label class="flex items-center gap-2 text-xs text-text-secondary">
          <input v-model="form.enabled" type="checkbox" class="h-4 w-4 rounded border-border-dark" />
          启用
        </label>
        <button
          type="submit"
          :disabled="saving"
          class="rounded-md border border-grass-500/60 bg-grass-500/10 px-4 py-2 text-sm text-grass-300 transition hover:bg-grass-500/20 disabled:opacity-50"
        >
          {{ saving ? '保存中…' : editingId ? '保存' : '新增' }}
        </button>
      </form>
      <p v-if="formError" class="mt-2 text-xs text-danger-mc" role="alert">{{ formError }}</p>
    </section>

    <ListShell title="屏蔽关键字" :loading="loading" :error="error" @retry="load">
      <template #toolbar>
        <span class="text-xs text-text-muted">共 {{ items.length }} 条，启用 {{ items.filter((i) => i.enabled).length }} 条</span>
      </template>

      <AdminTable :columns="columns" :rows="items" row-key="id" empty-text="暂无屏蔽关键字">
        <template #cell-keyword="{ row }">
          <span class="font-mono text-sm text-text-primary">{{ row.keyword }}</span>
        </template>
        <template #cell-note="{ row }">
          <span class="text-xs text-text-secondary">{{ row.note || '—' }}</span>
        </template>
        <template #cell-enabled="{ row }">
          <span
            class="rounded-full px-2 py-0.5 text-xs"
            :class="row.enabled ? 'bg-grass-500/15 text-grass-300' : 'bg-ink-700 text-text-muted'"
          >
            {{ row.enabled ? '已启用' : '已停用' }}
          </span>
        </template>
        <template #cell-created_at="{ row }">
          <span class="text-xs text-text-muted">{{ formatDateTime(row.created_at) }}</span>
        </template>
        <template #cell-action="{ row }">
          <div class="flex justify-end gap-3 text-xs">
            <button type="button" class="text-text-secondary hover:underline" @click="edit(row)">编辑</button>
            <button
              type="button"
              class="text-text-secondary hover:underline disabled:opacity-50"
              :disabled="busyId === row.id"
              @click="toggle(row)"
            >
              {{ row.enabled ? '停用' : '启用' }}
            </button>
            <button type="button" class="text-red-300 hover:underline" @click="remove(row)">删除</button>
          </div>
        </template>
      </AdminTable>
    </ListShell>
  </div>
</template>
