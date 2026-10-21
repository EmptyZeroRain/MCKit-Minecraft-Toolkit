<script setup lang="ts">
// 经验规则管理（jingyan.md §10.4）。
//
// 规则版本化：修改会生成新版本（version+1），历史版本保留，已发奖流水的
// rule_version 仍可追溯。停用某规则时后端会停用其全部版本。
import { computed, onMounted, reactive, ref } from 'vue'
import AdminTable from '@/components/admin/AdminTable.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import { ApiError } from '@/types/api'
import type { AdminTableColumn } from '@/types/admin'
import {
  createGrowthRule,
  deleteGrowthRule,
  fetchGrowthRules,
  toggleGrowthRule,
  updateGrowthRule,
} from '@/api/growth'
import {
  GROWTH_EVENT_LABELS,
  GROWTH_EVENT_TYPES,
  type GrowthRule,
  type GrowthRuleInput,
} from '@/types/growth'

const loading = ref(true)
const error = ref('')
const rules = ref<GrowthRule[]>([])
const busyId = ref('')
const notice = ref('')

const toDelete = ref<GrowthRule | null>(null)
const deleting = ref(false)

const showForm = ref(false)
const saving = ref(false)
const formError = ref('')
const editingId = ref('')

const inputClass =
  'w-full rounded-md border border-border-dark bg-ink-900 px-3 py-2 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none'

const form = reactive<{
  name: string
  event_type: string
  xp_amount: number
  daily_limit: string
  per_subject_limit: string
  enabled: boolean
  effective_from: string
  effective_until: string
}>({
  name: '',
  event_type: GROWTH_EVENT_TYPES[0],
  xp_amount: 0,
  daily_limit: '',
  per_subject_limit: '',
  enabled: true,
  effective_from: '',
  effective_until: '',
})

const columns: AdminTableColumn[] = [
  { key: 'name', label: '名称' },
  { key: 'event_type', label: '事件类型' },
  { key: 'xp_amount', label: '经验', width: '80px' },
  { key: 'limits', label: '每日 / 单对象上限', width: '150px' },
  { key: 'version', label: '版本', width: '70px' },
  { key: 'enabled', label: '状态', width: '80px' },
  { key: 'action', label: '操作', align: 'right', width: '200px' },
]

function eventLabel(t: string): string {
  return GROWTH_EVENT_LABELS[t] ?? t
}

/** 预估「按典型行为」每日可获得的经验（口径：xp_amount × 每日上限，未设上限则按单次）。 */
const previewXP = computed(() => {
  const xp = Number(form.xp_amount) || 0
  const daily = form.daily_limit === '' ? null : Number(form.daily_limit)
  if (daily && daily > 0) return xp * daily
  return xp
})

async function load() {
  loading.value = true
  error.value = ''
  try {
    rules.value = await fetchGrowthRules()
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '规则加载失败'
  } finally {
    loading.value = false
  }
}

onMounted(load)

function openCreate() {
  editingId.value = ''
  formError.value = ''
  Object.assign(form, {
    name: '',
    event_type: GROWTH_EVENT_TYPES[0],
    xp_amount: 0,
    daily_limit: '',
    per_subject_limit: '',
    enabled: true,
    effective_from: '',
    effective_until: '',
  })
  showForm.value = true
}

function openEdit(rule: GrowthRule) {
  editingId.value = rule.id
  formError.value = ''
  Object.assign(form, {
    name: rule.name,
    event_type: rule.event_type,
    xp_amount: rule.xp_amount,
    daily_limit: rule.daily_limit === null ? '' : String(rule.daily_limit),
    per_subject_limit: rule.per_subject_limit === null ? '' : String(rule.per_subject_limit),
    enabled: rule.enabled,
    effective_from: rule.effective_from ? rule.effective_from.slice(0, 16) : '',
    effective_until: rule.effective_until ? rule.effective_until.slice(0, 16) : '',
  })
  showForm.value = true
}

function buildInput(): GrowthRuleInput {
  return {
    name: form.name.trim(),
    event_type: form.event_type,
    xp_amount: Number(form.xp_amount) || 0,
    daily_limit: form.daily_limit === '' ? null : Number(form.daily_limit),
    per_subject_limit: form.per_subject_limit === '' ? null : Number(form.per_subject_limit),
    enabled: form.enabled,
    effective_from: form.effective_from ? new Date(form.effective_from).toISOString() : null,
    effective_until: form.effective_until ? new Date(form.effective_until).toISOString() : null,
  }
}

async function save() {
  formError.value = ''
  notice.value = ''
  if (!form.name.trim()) {
    formError.value = '名称不能为空'
    return
  }
  saving.value = true
  try {
    if (editingId.value) {
      await updateGrowthRule(editingId.value, buildInput())
      notice.value = '规则已更新为新版本。'
    } else {
      await createGrowthRule(buildInput())
      notice.value = '规则已创建。'
    }
    showForm.value = false
    await load()
  } catch (e) {
    formError.value = e instanceof ApiError ? e.message : '保存失败'
  } finally {
    saving.value = false
  }
}

async function toggle(rule: GrowthRule) {
  busyId.value = rule.id
  error.value = ''
  try {
    await toggleGrowthRule(rule.id, !rule.enabled)
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
    await deleteGrowthRule(toDelete.value.id)
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
        修改规则会生成新版本；停用规则会停用其全部版本；已被账本引用的规则不可删除。
      </p>
      <button
        type="button"
        class="shrink-0 rounded-md bg-grass-600 px-3 py-1.5 text-sm font-medium text-white transition hover:bg-grass-500"
        @click="openCreate"
      >
        新建规则
      </button>
    </div>

    <p v-if="error" class="mb-3 rounded-card border border-danger-mc/40 bg-danger-mc/10 px-4 py-2 text-sm text-danger-mc" role="alert">
      {{ error }}
    </p>
    <p v-if="notice" class="mb-3 rounded-card border border-grass-500/40 bg-grass-500/10 px-4 py-2 text-sm text-grass-400" role="status">
      {{ notice }}
    </p>

    <!-- 表单 -->
    <form v-if="showForm" class="card mb-5 grid gap-3 p-4 sm:grid-cols-2" @submit.prevent="save">
      <h2 class="text-sm font-medium text-text-secondary sm:col-span-2">
        {{ editingId ? '编辑规则（生成新版本）' : '新建规则' }}
      </h2>
      <label class="text-xs text-text-muted">
        名称
        <input v-model="form.name" :class="inputClass" type="text" placeholder="如 投稿审核通过" />
      </label>
      <label class="text-xs text-text-muted">
        事件类型
        <select v-model="form.event_type" :class="inputClass">
          <option v-for="t in GROWTH_EVENT_TYPES" :key="t" :value="t">{{ eventLabel(t) }}（{{ t }}）</option>
        </select>
      </label>
      <label class="text-xs text-text-muted">
        经验值
        <input v-model.number="form.xp_amount" :class="inputClass" type="number" min="0" />
      </label>
      <label class="text-xs text-text-muted">
        每日上限（留空为不限）
        <input v-model="form.daily_limit" :class="inputClass" type="number" min="1" placeholder="不限" />
      </label>
      <label class="text-xs text-text-muted">
        单对象上限（留空为不限）
        <input v-model="form.per_subject_limit" :class="inputClass" type="number" min="1" placeholder="不限" />
      </label>
      <label class="text-xs text-text-muted">
        生效开始（可选）
        <input v-model="form.effective_from" :class="inputClass" type="datetime-local" />
      </label>
      <label class="text-xs text-text-muted">
        生效结束（可选）
        <input v-model="form.effective_until" :class="inputClass" type="datetime-local" />
      </label>
      <label class="flex items-center gap-2 text-xs text-text-muted">
        <input v-model="form.enabled" type="checkbox" /> 启用
      </label>
      <p class="text-xs text-text-muted sm:col-span-2">
        预估每日经验（按典型行为）：<span class="font-semibold text-grass-300">{{ previewXP }}</span>
      </p>
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

    <AdminTable :columns="columns" :rows="rules" :loading="loading" row-key="id" empty-text="暂无经验规则">
      <template #cell-event_type="{ row }">{{ eventLabel(row.event_type) }}</template>
      <template #cell-xp_amount="{ row }">+{{ row.xp_amount }}</template>
      <template #cell-limits="{ row }">
        <span class="text-xs text-text-muted">
          {{ row.daily_limit ?? '不限' }} / {{ row.per_subject_limit ?? '不限' }}
        </span>
      </template>
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
      title="删除经验规则"
      :message="toDelete ? `确定删除规则「${toDelete.name}」的全部版本吗？已被账本引用的规则将被拒绝。` : ''"
      confirm-text="删除"
      dangerous
      :confirm-disabled="deleting"
      @confirm="confirmDelete"
      @cancel="toDelete = null"
    />
  </div>
</template>
