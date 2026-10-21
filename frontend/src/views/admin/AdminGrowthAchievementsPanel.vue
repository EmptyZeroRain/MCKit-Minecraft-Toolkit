<script setup lang="ts">
// 成就定义管理（jingyan.md §10.3）。
//
// 条件类型必须在后端允许列表内；图标仅允许受控的 HTTPS URL 或站内 / 路径。
// 「历史补算」用于新增 / 调整成就后对已有行为补发解锁。
import { onMounted, reactive, ref } from 'vue'
import AdminTable from '@/components/admin/AdminTable.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import { ApiError } from '@/types/api'
import type { AdminTableColumn } from '@/types/admin'
import {
  createAchievementDef,
  fetchAchievementDefs,
  fetchAdminUserAchievements,
  grantUserAchievement,
  recalculateAchievements,
  revokeUserAchievement,
  toggleAchievementDef,
  updateAchievementDef,
} from '@/api/growth'
import {
  ACHIEVEMENT_CONDITION_LABELS,
  ACHIEVEMENT_CONDITION_TYPES,
  GROWTH_EVENT_LABELS,
  GROWTH_EVENT_TYPES,
  RARITY_LABELS,
  type AdminUserAchievement,
  type GrowthAchievementDefinition,
  type GrowthAchievementInput,
} from '@/types/growth'

const loading = ref(true)
const error = ref('')
const notice = ref('')
const defs = ref<GrowthAchievementDefinition[]>([])
const busyId = ref('')

const recalcOpen = ref(false)
const recalcUserId = ref('')
const recalculating = ref(false)

// 单用户成就授予 / 撤销。撤销为软删除，自动补算不会重新解锁。
const userAchievements = ref<AdminUserAchievement[]>([])
const userAchieveUserId = ref('')
// 记录当前表格对应的目标用户，避免改了输入框后误操作上一批成就。
const userAchieveLoadedFor = ref('')
const userAchieveLoading = ref(false)
const userAchieveError = ref('')
const userAchieveNotice = ref('')
const userAchieveBusy = ref('')
const achievementActionReason = ref('')

const showForm = ref(false)
const saving = ref(false)
const formError = ref('')
const editingId = ref('')

const inputClass =
  'w-full rounded-md border border-border-dark bg-ink-900 px-3 py-2 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none'

const form = reactive<{
  name: string
  description: string
  category: string
  rarity: string
  icon_url: string
  condition_type: string
  condition_event_type: string
  condition_subject_type: string
  target_value: number
  reward_xp: number
  is_hidden: boolean
  enabled: boolean
  sort_order: number
}>({
  name: '',
  description: '',
  category: '探索',
  rarity: 'common',
  icon_url: '',
  condition_type: ACHIEVEMENT_CONDITION_TYPES[0],
  condition_event_type: GROWTH_EVENT_TYPES[0],
  condition_subject_type: '',
  target_value: 1,
  reward_xp: 0,
  is_hidden: false,
  enabled: true,
  sort_order: 0,
})

const rarityOptions = Object.keys(RARITY_LABELS)

const columns: AdminTableColumn[] = [
  { key: 'sort_order', label: '排序', width: '70px' },
  { key: 'name', label: '名称' },
  { key: 'category', label: '分类', width: '90px' },
  { key: 'rarity', label: '稀有度', width: '80px' },
  { key: 'condition', label: '条件' },
  { key: 'reward_xp', label: '奖励', width: '70px' },
  { key: 'version', label: '版本', width: '70px' },
  { key: 'enabled', label: '状态', width: '80px' },
  { key: 'action', label: '操作', align: 'right', width: '160px' },
]

const userColumns: AdminTableColumn[] = [
  { key: 'name', label: '成就' },
  { key: 'reward_xp', label: '奖励', width: '70px' },
  { key: 'status', label: '状态', width: '110px' },
  { key: 'action', label: '操作', align: 'right', width: '150px' },
]

function rarityLabel(r: string): string {
  return RARITY_LABELS[r] ?? r
}

/** 条件说明预览（与后端 evaluateAchievements 语义一致）。 */
function conditionText(def: GrowthAchievementDefinition): string {
  const type = ACHIEVEMENT_CONDITION_LABELS[def.condition_type] ?? def.condition_type
  if (def.condition_type === 'level_reached') return `${type} Lv.${def.target_value}`
  if (def.condition_type === 'daily_task_days') return `${type}达到 ${def.target_value} 天`
  const ev = GROWTH_EVENT_LABELS[def.condition_event_type] ?? def.condition_event_type
  if (def.condition_type === 'distinct_subject_count') {
    return `${ev} 去重目标数达到 ${def.target_value}`
  }
  return `${ev} 累计 ${def.target_value} 次`
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    defs.value = await fetchAchievementDefs()
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '成就加载失败'
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
    description: '',
    category: '探索',
    rarity: 'common',
    icon_url: '',
    condition_type: ACHIEVEMENT_CONDITION_TYPES[0],
    condition_event_type: GROWTH_EVENT_TYPES[0],
    condition_subject_type: '',
    target_value: 1,
    reward_xp: 0,
    is_hidden: false,
    enabled: true,
    sort_order: 0,
  })
  showForm.value = true
}

function openEdit(def: GrowthAchievementDefinition) {
  editingId.value = def.id
  formError.value = ''
  Object.assign(form, {
    name: def.name,
    description: def.description,
    category: def.category,
    rarity: def.rarity,
    icon_url: def.icon_url,
    condition_type: def.condition_type,
    condition_event_type: def.condition_event_type || GROWTH_EVENT_TYPES[0],
    condition_subject_type: def.condition_subject_type,
    target_value: def.target_value,
    reward_xp: def.reward_xp,
    is_hidden: def.is_hidden,
    enabled: def.enabled,
    sort_order: def.sort_order,
  })
  showForm.value = true
}

function buildInput(): GrowthAchievementInput {
  return {
    name: form.name.trim(),
    description: form.description.trim(),
    category: form.category.trim(),
    rarity: form.rarity,
    icon_url: form.icon_url.trim(),
    condition_type: form.condition_type,
    condition_event_type: form.condition_event_type,
    condition_subject_type: form.condition_subject_type.trim(),
    target_value: Number(form.target_value) || 1,
    reward_xp: Number(form.reward_xp) || 0,
    is_hidden: form.is_hidden,
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
  if (!form.category.trim()) {
    formError.value = '分类不能为空'
    return
  }
  if (Number(form.target_value) <= 0) {
    formError.value = '目标值必须大于 0'
    return
  }
  saving.value = true
  try {
    if (editingId.value) {
      await updateAchievementDef(editingId.value, buildInput())
      notice.value = '成就已更新。'
    } else {
      await createAchievementDef(buildInput())
      notice.value = '成就已创建。'
    }
    showForm.value = false
    await load()
  } catch (e) {
    formError.value = e instanceof ApiError ? e.message : '保存失败'
  } finally {
    saving.value = false
  }
}

async function toggle(def: GrowthAchievementDefinition) {
  busyId.value = def.id
  error.value = ''
  try {
    await toggleAchievementDef(def.id, !def.enabled)
    await load()
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '操作失败'
  } finally {
    busyId.value = ''
  }
}

async function loadUserAchievements() {
  userAchieveError.value = ''
  userAchieveNotice.value = ''
  userAchieveLoadedFor.value = ''
  const target = userAchieveUserId.value.trim()
  if (!target) {
    userAchieveError.value = '请填写目标用户 ID'
    return
  }
  userAchieveLoading.value = true
  try {
    userAchievements.value = await fetchAdminUserAchievements(target)
    userAchieveLoadedFor.value = target
  } catch (e) {
    userAchievements.value = []
    userAchieveError.value = e instanceof ApiError ? e.message : '用户成就加载失败'
  } finally {
    userAchieveLoading.value = false
  }
}

async function grantAchievement(item: AdminUserAchievement) {
  userAchieveError.value = ''
  userAchieveNotice.value = ''
  if (userAchieveLoadedFor.value !== userAchieveUserId.value.trim()) {
    userAchieveError.value = '请先查询目标用户成就，再执行授予'
    return
  }
  if (!achievementActionReason.value.trim()) {
    userAchieveError.value = '请填写授予原因'
    return
  }
  if (!window.confirm(`确定为该用户授予「${item.name}」吗？会按定义补发一次性奖励经验。`)) {
    return
  }
  userAchieveBusy.value = item.achievement_id
  try {
    await grantUserAchievement(
      userAchieveUserId.value.trim(),
      item.achievement_id,
      achievementActionReason.value.trim(),
    )
    await loadUserAchievements()
    userAchieveNotice.value = `已授予「${item.name}」。`
  } catch (e) {
    userAchieveError.value = e instanceof ApiError ? e.message : '授予失败'
  } finally {
    userAchieveBusy.value = ''
  }
}

async function revokeAchievement(item: AdminUserAchievement) {
  userAchieveError.value = ''
  userAchieveNotice.value = ''
  if (userAchieveLoadedFor.value !== userAchieveUserId.value.trim()) {
    userAchieveError.value = '请先查询目标用户成就，再执行撤销'
    return
  }
  if (!achievementActionReason.value.trim()) {
    userAchieveError.value = '请填写撤销原因'
    return
  }
  if (!window.confirm(`确定撤销「${item.name}」吗？不会回收已发放的经验，自动补算也不会重新解锁。`)) {
    return
  }
  userAchieveBusy.value = item.achievement_id
  try {
    await revokeUserAchievement(
      userAchieveUserId.value.trim(),
      item.achievement_id,
      achievementActionReason.value.trim(),
    )
    await loadUserAchievements()
    userAchieveNotice.value = `已撤销「${item.name}」。`
  } catch (e) {
    userAchieveError.value = e instanceof ApiError ? e.message : '撤销失败'
  } finally {
    userAchieveBusy.value = ''
  }
}

async function confirmRecalculate() {
  recalculating.value = true
  error.value = ''
  try {
    const unlocked = await recalculateAchievements(recalcUserId.value.trim())
    notice.value = `补算完成，新解锁 ${unlocked} 项成就。`
    recalcOpen.value = false
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '补算失败'
  } finally {
    recalculating.value = false
  }
}
</script>

<template>
  <div>
    <div class="card mb-5 grid gap-3 p-4 sm:grid-cols-4">
      <h2 class="text-sm font-medium text-text-secondary sm:col-span-4">单用户成就授予 / 撤销</h2>
      <p class="text-xs text-text-muted sm:col-span-4">
        授予会按定义补发一次性奖励经验；撤销为软删除，不回收已发经验，自动补算不会重新解锁。
      </p>
      <label class="text-xs text-text-muted sm:col-span-2">
        目标用户 ID（内部 UUID 或对外 ID）
        <input
          v-model="userAchieveUserId"
          :class="inputClass"
          type="text"
          placeholder="UUID / U7K3M2QX9F"
          @keyup.enter="loadUserAchievements"
        />
      </label>
      <label class="text-xs text-text-muted sm:col-span-2">
        操作原因（授予 / 撤销必填）
        <input
          v-model="achievementActionReason"
          :class="inputClass"
          type="text"
          maxlength="500"
          placeholder="如 活动补发 / 违规撤销"
        />
      </label>
      <div class="flex items-center gap-2 sm:col-span-4">
        <button
          type="button"
          class="rounded-md border border-border-light bg-ink-700 px-3 py-1.5 text-sm text-text-secondary transition hover:bg-ink-600"
          :disabled="userAchieveLoading"
          @click="loadUserAchievements"
        >
          查询用户成就
        </button>
        <span v-if="userAchieveLoading" class="text-xs text-text-muted">加载中…</span>
      </div>
      <p v-if="userAchieveError" class="text-xs text-danger-mc sm:col-span-4" role="alert">{{ userAchieveError }}</p>
      <p v-if="userAchieveNotice" class="text-xs text-grass-400 sm:col-span-4" role="status">{{ userAchieveNotice }}</p>
    </div>

    <AdminTable
      v-if="userAchievements.length && userAchieveLoadedFor === userAchieveUserId.trim()"
      class="mb-5"
      :columns="userColumns"
      :rows="userAchievements"
      :loading="userAchieveLoading"
      row-key="achievement_id"
      empty-text="暂无成就定义"
    >
      <template #cell-reward_xp="{ row }">+{{ row.reward_xp }}</template>
      <template #cell-status="{ row }">
        <span v-if="row.revoked" class="text-danger-mc">已撤销</span>
        <span v-else-if="row.unlocked" class="text-grass-300">已持有</span>
        <span v-else class="text-text-muted">未解锁</span>
      </template>
      <template #cell-action="{ row }">
        <div class="flex justify-end gap-2">
          <button
            v-if="!row.unlocked"
            type="button"
            class="rounded-md bg-grass-600 px-2.5 py-1 text-xs font-medium text-white transition hover:bg-grass-500 disabled:opacity-50"
            :disabled="userAchieveBusy === row.achievement_id"
            @click="grantAchievement(row)"
          >
            {{ row.revoked ? '重新授予' : '授予' }}
          </button>
          <button
            v-else
            type="button"
            class="rounded-md border border-danger-mc/40 px-2.5 py-1 text-xs text-danger-mc transition hover:bg-danger-mc/10 disabled:opacity-50"
            :disabled="userAchieveBusy === row.achievement_id"
            @click="revokeAchievement(row)"
          >
            撤销
          </button>
        </div>
      </template>
    </AdminTable>

    <div class="mb-4 flex items-center justify-between">
      <p class="text-xs text-text-muted">
        图标仅允许受控 HTTPS URL 或站内 / 路径；停用不影响已解锁记录。
      </p>
      <div class="flex shrink-0 items-center gap-2">
        <input
          v-model="recalcUserId"
          :class="inputClass"
          class="w-56"
          type="text"
          placeholder="补算指定用户 ID（可留空）"
        />
        <button type="button" class="rounded-md border border-border-light bg-ink-700 px-3 py-1.5 text-sm text-text-secondary transition hover:bg-ink-600" @click="recalcOpen = true">
          历史补算
        </button>
        <button type="button" class="rounded-md bg-grass-600 px-3 py-1.5 text-sm font-medium text-white transition hover:bg-grass-500" @click="openCreate">
          新建成就
        </button>
      </div>
    </div>

    <p v-if="error" class="mb-3 rounded-card border border-danger-mc/40 bg-danger-mc/10 px-4 py-2 text-sm text-danger-mc" role="alert">{{ error }}</p>
    <p v-if="notice" class="mb-3 rounded-card border border-grass-500/40 bg-grass-500/10 px-4 py-2 text-sm text-grass-400" role="status">{{ notice }}</p>

    <form v-if="showForm" class="card mb-5 grid gap-3 p-4 sm:grid-cols-2" @submit.prevent="save">
      <h2 class="text-sm font-medium text-text-secondary sm:col-span-2">
        {{ editingId ? '编辑成就' : '新建成就' }}
      </h2>
      <label class="text-xs text-text-muted">
        名称
        <input v-model="form.name" :class="inputClass" type="text" placeholder="如 工具达人" />
      </label>
      <label class="text-xs text-text-muted sm:col-span-2">
        描述
        <input v-model="form.description" :class="inputClass" type="text" placeholder="如 使用 10 种不同工具" />
      </label>
      <label class="text-xs text-text-muted">
        分类
        <input v-model="form.category" :class="inputClass" type="text" maxlength="50" placeholder="如 探索" />
      </label>
      <label class="text-xs text-text-muted">
        稀有度
        <select v-model="form.rarity" :class="inputClass">
          <option v-for="r in rarityOptions" :key="r" :value="r">{{ rarityLabel(r) }}</option>
        </select>
      </label>
      <label class="text-xs text-text-muted sm:col-span-2">
        图标地址（HTTPS 或 / 开头，可留空）
        <input v-model="form.icon_url" :class="inputClass" type="text" placeholder="https://… 或 /images/…" />
      </label>
      <label class="text-xs text-text-muted">
        条件类型
        <select v-model="form.condition_type" :class="inputClass">
          <option v-for="t in ACHIEVEMENT_CONDITION_TYPES" :key="t" :value="t">
            {{ ACHIEVEMENT_CONDITION_LABELS[t] }}
          </option>
        </select>
      </label>
      <label class="text-xs text-text-muted">
        目标值
        <input v-model.number="form.target_value" :class="inputClass" type="number" min="1" />
      </label>
      <label
        v-if="form.condition_type === 'event_count' || form.condition_type === 'distinct_subject_count'"
        class="text-xs text-text-muted"
      >
        统计事件类型
        <select v-model="form.condition_event_type" :class="inputClass">
          <option v-for="t in GROWTH_EVENT_TYPES" :key="t" :value="t">{{ GROWTH_EVENT_LABELS[t] }}</option>
        </select>
      </label>
      <label v-if="form.condition_type === 'distinct_subject_count'" class="text-xs text-text-muted">
        目标对象类型
        <input v-model="form.condition_subject_type" :class="inputClass" type="text" placeholder="如 resource / tool" />
      </label>
      <label class="text-xs text-text-muted">
        奖励经验
        <input v-model.number="form.reward_xp" :class="inputClass" type="number" min="0" />
      </label>
      <label class="text-xs text-text-muted">
        排序
        <input v-model.number="form.sort_order" :class="inputClass" type="number" />
      </label>
      <div class="flex items-center gap-4 text-xs text-text-muted sm:col-span-2">
        <label class="flex items-center gap-2"><input v-model="form.is_hidden" type="checkbox" /> 隐藏成就</label>
        <label class="flex items-center gap-2"><input v-model="form.enabled" type="checkbox" /> 启用</label>
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

    <AdminTable :columns="columns" :rows="defs" :loading="loading" row-key="id" empty-text="暂无成就定义">
      <template #cell-name="{ row }">
        <span>{{ row.name }}</span>
        <span v-if="row.is_hidden" class="ml-1 rounded bg-ink-700 px-1.5 py-0.5 text-[10px] text-text-muted">隐藏</span>
      </template>
      <template #cell-rarity="{ row }">{{ rarityLabel(row.rarity) }}</template>
      <template #cell-condition="{ row }">
        <span class="text-xs text-text-muted">{{ conditionText(row) }}</span>
      </template>
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
        </div>
      </template>
    </AdminTable>

    <ConfirmDialog
      :open="recalcOpen"
      title="历史补算成就"
      message="将按当前成就定义对已有行为补算解锁。留空用户 ID 表示对全部有成长记录的用户执行，耗时可能较长。"
      confirm-text="开始补算"
      :confirm-disabled="recalculating"
      @confirm="confirmRecalculate"
      @cancel="recalcOpen = false"
    />
  </div>
</template>
