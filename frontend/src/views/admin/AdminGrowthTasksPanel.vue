<script setup lang="ts">
// 每日任务定义管理（jingyan.md §10.2）。
//
// 任务每天按业务日期独立记录，无需定时清空；修改任务后保留已生成的当日任务快照，
// 不允许通过编辑任务将已有完成记录重复发奖（后端以快照 + 幂等键保证）。
import { onMounted, reactive, ref } from 'vue'
import AdminTable from '@/components/admin/AdminTable.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import { ApiError } from '@/types/api'
import type { AdminTableColumn } from '@/types/admin'
import {
  createDailyTaskDef,
  deleteDailyTaskDef,
  fetchDailyTaskDefs,
  toggleDailyTaskDef,
  updateDailyTaskDef,
} from '@/api/growth'
import {
  GROWTH_EVENT_LABELS,
  GROWTH_EVENT_TYPES,
  type GrowthTaskDefinition,
  type GrowthTaskInput,
} from '@/types/growth'

const loading = ref(true)
const error = ref('')
const notice = ref('')
const tasks = ref<GrowthTaskDefinition[]>([])
const busyId = ref('')

const toDelete = ref<GrowthTaskDefinition | null>(null)
const deleting = ref(false)

const showForm = ref(false)
const saving = ref(false)
const formError = ref('')
const editingId = ref('')

const inputClass =
  'w-full rounded-md border border-border-dark bg-ink-900 px-3 py-2 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none'

const form = reactive<{
  code: string
  name: string
  description: string
  event_type: string
  target_count: number
  reward_xp: number
  enabled: boolean
  sort_order: number
}>({
  code: '',
  name: '',
  description: '',
  event_type: GROWTH_EVENT_TYPES[0],
  target_count: 1,
  reward_xp: 0,
  enabled: true,
  sort_order: 0,
})

const columns: AdminTableColumn[] = [
  { key: 'sort_order', label: '排序', width: '70px' },
  { key: 'code', label: '编码' },
  { key: 'name', label: '名称' },
  { key: 'event_type', label: '事件类型' },
  { key: 'target_count', label: '目标次数', width: '90px' },
  { key: 'reward_xp', label: '奖励经验', width: '90px' },
  { key: 'version', label: '版本', width: '70px' },
  { key: 'enabled', label: '状态', width: '80px' },
  { key: 'action', label: '操作', align: 'right', width: '200px' },
]

function eventLabel(t: string): string {
  return GROWTH_EVENT_LABELS[t] ?? t
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    tasks.value = await fetchDailyTaskDefs()
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '任务加载失败'
  } finally {
    loading.value = false
  }
}

onMounted(load)

function openCreate() {
  editingId.value = ''
  formError.value = ''
  Object.assign(form, {
    code: '',
    name: '',
    description: '',
    event_type: GROWTH_EVENT_TYPES[0],
    target_count: 1,
    reward_xp: 0,
    enabled: true,
    sort_order: 0,
  })
  showForm.value = true
}

function openEdit(task: GrowthTaskDefinition) {
  editingId.value = task.id
  formError.value = ''
  Object.assign(form, {
    code: task.code,
    name: task.name,
    description: task.description,
    event_type: task.event_type,
    target_count: task.target_count,
    reward_xp: task.reward_xp,
    enabled: task.enabled,
    sort_order: task.sort_order,
  })
  showForm.value = true
}

function buildInput(): GrowthTaskInput {
  return {
    code: form.code.trim(),
    name: form.name.trim(),
    description: form.description.trim(),
    event_type: form.event_type,
    target_count: Number(form.target_count) || 1,
    reward_xp: Number(form.reward_xp) || 0,
    enabled: form.enabled,
    sort_order: Number(form.sort_order) || 0,
  }
}

async function save() {
  formError.value = ''
  notice.value = ''
  if (!form.name.trim()) {
    formError.value = '名称不能为空'
    return
  }
  if (!editingId.value && !form.code.trim()) {
    formError.value = '编码不能为空'
    return
  }
  if (Number(form.target_count) <= 0) {
    formError.value = '目标次数必须大于 0'
    return
  }
  saving.value = true
  try {
    if (editingId.value) {
      await updateDailyTaskDef(editingId.value, buildInput())
      notice.value = '任务已更新（已产生的当日进度保留快照）。'
    } else {
      await createDailyTaskDef(buildInput())
      notice.value = '任务已创建。'
    }
    showForm.value = false
    await load()
  } catch (e) {
    formError.value = e instanceof ApiError ? e.message : '保存失败'
  } finally {
    saving.value = false
  }
}

async function toggle(task: GrowthTaskDefinition) {
  busyId.value = task.id
  error.value = ''
  try {
    await toggleDailyTaskDef(task.id, !task.enabled)
    await load()
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '操作失败'
  } finally {
    busyId.value = ''
  }
}

async function confirmDelete() {
  if (!toDelete.value) return
  deleting.value = true
  error.value = ''
  try {
    await deleteDailyTaskDef(toDelete.value.id)
    toDelete.value = null
    await load()
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '删除失败'
  } finally {
    deleting.value = false
  }
}
</script>

<template>
  <div>
    <div class="mb-4 flex items-center justify-between">
      <p class="text-xs text-text-muted">
        任务按业务日期独立记录；达到目标即由服务端自动发奖，不提供手动领取。
      </p>
      <button
        type="button"
        class="shrink-0 rounded-md bg-grass-600 px-3 py-1.5 text-sm font-medium text-white transition hover:bg-grass-500"
        @click="openCreate"
      >
        新建任务
      </button>
    </div>

    <p v-if="error" class="mb-3 rounded-card border border-danger-mc/40 bg-danger-mc/10 px-4 py-2 text-sm text-danger-mc" role="alert">{{ error }}</p>
    <p v-if="notice" class="mb-3 rounded-card border border-grass-500/40 bg-grass-500/10 px-4 py-2 text-sm text-grass-400" role="status">{{ notice }}</p>

    <form v-if="showForm" class="card mb-5 grid gap-3 p-4 sm:grid-cols-2" @submit.prevent="save">
      <h2 class="text-sm font-medium text-text-secondary sm:col-span-2">
        {{ editingId ? '编辑任务' : '新建任务' }}
      </h2>
      <label class="text-xs text-text-muted">
        编码
        <input v-model="form.code" :class="inputClass" type="text" :disabled="!!editingId" placeholder="如 daily_login" />
      </label>
      <label class="text-xs text-text-muted">
        名称
        <input v-model="form.name" :class="inputClass" type="text" placeholder="如 每日登录" />
      </label>
      <label class="text-xs text-text-muted sm:col-span-2">
        说明
        <input v-model="form.description" :class="inputClass" type="text" placeholder="如 今天成功登录一次" />
      </label>
      <label class="text-xs text-text-muted">
        事件类型
        <select v-model="form.event_type" :class="inputClass">
          <option v-for="t in GROWTH_EVENT_TYPES" :key="t" :value="t">{{ eventLabel(t) }}（{{ t }}）</option>
        </select>
      </label>
      <label class="text-xs text-text-muted">
        目标次数
        <input v-model.number="form.target_count" :class="inputClass" type="number" min="1" />
      </label>
      <label class="text-xs text-text-muted">
        奖励经验
        <input v-model.number="form.reward_xp" :class="inputClass" type="number" min="0" />
      </label>
      <label class="text-xs text-text-muted">
        排序
        <input v-model.number="form.sort_order" :class="inputClass" type="number" />
      </label>
      <label class="flex items-center gap-2 text-xs text-text-muted">
        <input v-model="form.enabled" type="checkbox" /> 启用
      </label>
      <p v-if="formError" class="text-xs text-danger-mc sm:col-span-2" role="alert">{{ formError }}</p>
      <div class="flex gap-2 sm:col-span-2">
        <button type="submit" class="rounded-md bg-grass-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-grass-500 disabled:opacity-60" :disabled="saving">
          保存
        </button>
        <button type="button" class="rounded-md border border-border-light bg-ink-700 px-4 py-2 text-sm text-text-secondary transition hover:bg-ink-600" @click="showForm = false">
          取消
        </button>
      </div>
    </form>

    <AdminTable :columns="columns" :rows="tasks" :loading="loading" row-key="id" empty-text="暂无每日任务">
      <template #cell-code="{ row }">
        <span class="font-mono text-xs text-text-primary">{{ row.code }}</span>
      </template>
      <template #cell-event_type="{ row }">{{ eventLabel(row.event_type) }}</template>
      <template #cell-target_count="{ row }">{{ row.target_count }}</template>
      <template #cell-reward_xp="{ row }">+{{ row.reward_xp }}</template>
      <template #cell-version="{ row }">v{{ row.version }}</template>
      <template #cell-enabled="{ row }">
        <span :class="row.enabled ? 'text-grass-300' : 'text-text-muted'">{{ row.enabled ? '启用' : '停用' }}</span>
      </template>
      <template #cell-action="{ row }">
        <div class="flex justify-end gap-2">
          <button type="button" class="rounded-md border border-border-light bg-ink-700 px-2.5 py-1 text-xs text-text-secondary transition hover:bg-ink-600" @click="openEdit(row)">
            编辑
          </button>
          <button type="button" class="rounded-md border border-border-light bg-ink-700 px-2.5 py-1 text-xs text-text-secondary transition hover:bg-ink-600 disabled:opacity-50" :disabled="busyId === row.id" @click="toggle(row)">
            {{ row.enabled ? '停用' : '启用' }}
          </button>
          <button type="button" class="rounded-md border border-danger-mc/40 px-2.5 py-1 text-xs text-danger-mc transition hover:bg-danger-mc/10" @click="toDelete = row">
            删除
          </button>
        </div>
      </template>
    </AdminTable>

    <ConfirmDialog
      :open="!!toDelete"
      title="删除每日任务"
      :message="toDelete ? `确定删除任务「${toDelete.code}」吗？已产生用户进度的任务将被拒绝。` : ''"
      confirm-text="删除"
      dangerous
      :confirm-disabled="deleting"
      @confirm="confirmDelete"
      @cancel="toDelete = null"
    />
  </div>
</template>
