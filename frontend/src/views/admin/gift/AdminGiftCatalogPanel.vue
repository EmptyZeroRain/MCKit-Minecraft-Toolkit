<script setup lang="ts">
// 礼品目录面板：新建 / 编辑 / 启停 / 排序 / 删除礼物。
//
// 已有兑换记录的礼物只能停用，不能删除，避免改写历史快照。
import { onMounted, reactive, ref } from 'vue'
import AdminTable from '@/components/admin/AdminTable.vue'
import { ApiError } from '@/types/api'
import type { AdminTableColumn } from '@/types/admin'
import {
  createGiftDefinition,
  deleteGiftDefinition,
  fetchGiftDefinitions,
  updateGiftDefinition,
} from '@/api/gift'
import type { GiftDefinition, GiftDefinitionInput } from '@/types/gift'

const inputClass =
  'w-full rounded-md border border-border-dark bg-ink-900 px-3 py-2 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none'

const loading = ref(true)
const error = ref('')
const notice = ref('')
const defs = ref<GiftDefinition[]>([])
const busyId = ref('')

const showForm = ref(false)
const saving = ref(false)
const formError = ref('')
const editingId = ref('')

const form = reactive<{
  name: string
  description: string
  icon: string
  price: number
  stock: number
  allow_checkin: boolean
  sort_order: number
  enabled: boolean
}>({
  name: '',
  description: '',
  icon: '🎁',
  price: 1,
  stock: 0,
  allow_checkin: false,
  sort_order: 0,
  enabled: true,
})

const columns: AdminTableColumn[] = [
  { key: 'icon', label: '图标', width: '60px', align: 'center' },
  { key: 'name', label: '礼物' },
  { key: 'price', label: '价格', width: '90px' },
  { key: 'allow_checkin', label: '签到币可兑', width: '110px', align: 'center' },
  { key: 'stock', label: '库存', width: '90px', align: 'center' },
  { key: 'redeemed_count', label: '已兑换', width: '80px', align: 'center' },
  { key: 'remaining', label: '剩余', width: '80px', align: 'center' },
  { key: 'status', label: '状态', width: '80px', align: 'center' },
  { key: 'action', label: '操作', width: '180px', align: 'right' },
]

async function loadDefs() {
  loading.value = true
  error.value = ''
  try {
    defs.value = await fetchGiftDefinitions()
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '礼品目录加载失败'
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editingId.value = ''
  formError.value = ''
  Object.assign(form, {
    name: '',
    description: '',
    icon: '🎁',
    price: 1,
    stock: 0,
    allow_checkin: false,
    sort_order: 0,
    enabled: true,
  })
  showForm.value = true
}

function openEdit(def: GiftDefinition) {
  editingId.value = def.id
  formError.value = ''
  Object.assign(form, {
    name: def.name,
    description: def.description,
    icon: def.icon,
    price: def.price,
    stock: def.stock ?? 0,
    allow_checkin: def.allow_checkin,
    sort_order: def.sort_order,
    enabled: def.enabled,
  })
  showForm.value = true
}

function statusText(def: GiftDefinition): string {
  if (!def.enabled) return '停用'
  if (def.remaining !== null && def.remaining <= 0) return '已兑完'
  return '启用'
}

function buildInput(): GiftDefinitionInput {
  return {
    name: form.name.trim(),
    description: form.description.trim(),
    icon: form.icon.trim() || '🎁',
    price: Number(form.price),
    stock: Number(form.stock),
    allow_checkin: form.allow_checkin,
    sort_order: Number(form.sort_order),
    enabled: form.enabled,
  }
}

async function save() {
  formError.value = ''
  notice.value = ''
  if (!form.name.trim()) {
    formError.value = '请填写礼物名称'
    return
  }
  if (!Number.isInteger(Number(form.price)) || Number(form.price) <= 0) {
    formError.value = '礼物价格必须是大于 0 的整数'
    return
  }
  if (!Number.isInteger(Number(form.stock)) || Number(form.stock) < 0) {
    formError.value = '库存必须是大于等于 0 的整数（0 表示不限）'
    return
  }
  saving.value = true
  try {
    if (editingId.value) {
      await updateGiftDefinition(editingId.value, buildInput())
      notice.value = '礼物已更新。'
    } else {
      await createGiftDefinition(buildInput())
      notice.value = '礼物已创建。'
    }
    showForm.value = false
    await loadDefs()
  } catch (e) {
    formError.value = e instanceof ApiError ? e.message : '保存失败'
  } finally {
    saving.value = false
  }
}

async function toggleEnabled(def: GiftDefinition) {
  busyId.value = def.id
  error.value = ''
  notice.value = ''
  try {
    await updateGiftDefinition(def.id, {
      name: def.name,
      description: def.description,
      icon: def.icon,
      price: def.price,
      stock: def.stock ?? 0,
      allow_checkin: def.allow_checkin,
      sort_order: def.sort_order,
      enabled: !def.enabled,
    })
    notice.value = def.enabled ? '礼物已停用。' : '礼物已启用。'
    await loadDefs()
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '状态更新失败'
  } finally {
    busyId.value = ''
  }
}

async function remove(def: GiftDefinition) {
  if (!window.confirm(`确定删除礼物「${def.name}」吗？已有兑换记录的礼物无法删除。`)) {
    return
  }
  busyId.value = def.id
  error.value = ''
  notice.value = ''
  try {
    await deleteGiftDefinition(def.id)
    notice.value = '礼物已删除。'
    await loadDefs()
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '删除失败'
  } finally {
    busyId.value = ''
  }
}

onMounted(loadDefs)
</script>

<template>
  <div>
    <div class="mb-4 flex flex-wrap items-center justify-between gap-3">
      <p class="text-xs text-text-muted">
        配置礼物价格、库存与「是否允许签到礼物币兑换」；库存填 0 表示不限。已有兑换记录的礼物只能停用，不能删除。
      </p>
      <button
        type="button"
        class="rounded-md bg-grass-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-grass-500"
        @click="openCreate"
      >
        新建礼物
      </button>
    </div>

    <p v-if="error" class="mb-3 rounded-card border border-danger-mc/40 bg-danger-mc/10 px-4 py-2 text-sm text-danger-mc" role="alert">{{ error }}</p>
    <p v-if="notice" class="mb-3 rounded-card border border-grass-500/40 bg-grass-500/10 px-4 py-2 text-sm text-grass-400" role="status">{{ notice }}</p>

    <form v-if="showForm" class="card mb-5 grid gap-3 p-4 sm:grid-cols-2" @submit.prevent="save">
      <h2 class="text-sm font-medium text-text-secondary sm:col-span-2">
        {{ editingId ? '编辑礼物' : '新建礼物' }}
      </h2>
      <label class="text-xs text-text-muted">
        名称
        <input v-model="form.name" :class="inputClass" type="text" maxlength="80" placeholder="如 泥土块" />
      </label>
      <label class="text-xs text-text-muted sm:col-span-2">
        描述
        <input v-model="form.description" :class="inputClass" type="text" maxlength="500" placeholder="展示给用户的礼物说明" />
      </label>
      <label class="text-xs text-text-muted">
        图标（Emoji）
        <input v-model="form.icon" :class="inputClass" type="text" maxlength="16" placeholder="🎁" />
      </label>
      <label class="text-xs text-text-muted">
        价格（礼物币 / 个）
        <input v-model.number="form.price" :class="inputClass" type="number" min="1" step="1" />
      </label>
      <label class="text-xs text-text-muted">
        库存（0 = 不限）
        <input v-model.number="form.stock" :class="inputClass" type="number" min="0" step="1" />
      </label>
      <label class="text-xs text-text-muted">
        排序值（越小越靠前）
        <input v-model.number="form.sort_order" :class="inputClass" type="number" step="1" />
      </label>
      <div class="grid gap-2 text-xs text-text-secondary">
        <label class="flex items-center gap-2">
          <input v-model="form.allow_checkin" type="checkbox" />
          允许签到礼物币兑换
        </label>
        <label class="flex items-center gap-2">
          <input v-model="form.enabled" type="checkbox" />
          启用（下架后用户不可见、不可兑换）
        </label>
      </div>
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

    <AdminTable :columns="columns" :rows="defs" :loading="loading" row-key="id" empty-text="暂无礼物，点击右上角新建">
      <template #cell-icon="{ row }">
        <span class="text-lg">{{ row.icon }}</span>
      </template>
      <template #cell-name="{ row }">
        <div class="font-medium text-text-primary">{{ row.name }}</div>
        <div class="text-[11px] text-text-muted">{{ row.description || '—' }}</div>
      </template>
      <template #cell-price="{ row }">
        <span class="font-medium text-grass-300">{{ row.price }}</span>
        <span class="text-[11px] text-text-muted"> 币</span>
      </template>
      <template #cell-allow_checkin="{ row }">
        <span :class="row.allow_checkin ? 'text-grass-300' : 'text-text-muted'">
          {{ row.allow_checkin ? '可兑换' : '仅通用币' }}
        </span>
      </template>
      <template #cell-stock="{ row }">
        <span class="text-text-secondary">{{ row.stock > 0 ? row.stock : '不限' }}</span>
      </template>
      <template #cell-redeemed_count="{ row }">
        <span class="text-text-secondary">{{ row.redeemed_count }}</span>
      </template>
      <template #cell-remaining="{ row }">
        <span :class="row.remaining === 0 ? 'text-danger-mc' : 'text-text-secondary'">
          {{ row.remaining === null ? '不限' : row.remaining }}
        </span>
      </template>
      <template #cell-status="{ row }">
        <span :class="row.enabled ? 'text-grass-300' : 'text-text-muted'">
          {{ statusText(row) }}
        </span>
      </template>
      <template #cell-action="{ row }">
        <div class="flex justify-end gap-2">
          <button type="button" class="text-xs text-grass-400 hover:underline" @click="openEdit(row)">编辑</button>
          <button
            type="button"
            class="text-xs text-text-secondary hover:underline disabled:opacity-50"
            :disabled="busyId === row.id"
            @click="toggleEnabled(row)"
          >
            {{ row.enabled ? '停用' : '启用' }}
          </button>
          <button
            type="button"
            class="text-xs text-danger-mc hover:underline disabled:opacity-50"
            :disabled="busyId === row.id"
            @click="remove(row)"
          >
            删除
          </button>
        </div>
      </template>
    </AdminTable>
  </div>
</template>
