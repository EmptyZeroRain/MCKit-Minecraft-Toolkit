<script setup lang="ts">
// 等级阈值管理（jingyan.md §5 / §10.4 相关）。
//
// 等级仅作展示，不构成任何权限。阈值必须全局唯一、按等级严格递增，Lv.1 固定为 0。
// 等级定义可追加新等级或删除最高级；修改后可用「重算等级」修正历史用户展示等级。
import { computed, onMounted, reactive, ref } from 'vue'
import AdminTable from '@/components/admin/AdminTable.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import { ApiError } from '@/types/api'
import type { AdminTableColumn } from '@/types/admin'
import {
  adjustUserLevel,
  createGrowthLevel,
  deleteGrowthLevel,
  fetchGrowthLevels,
  recalculateGrowthLevels,
  updateGrowthLevel,
} from '@/api/growth'
import type { GrowthLevel } from '@/types/growth'

const loading = ref(true)
const error = ref('')
const notice = ref('')
const levels = ref<GrowthLevel[]>([])
const busyLevel = ref<number | null>(null)

const maxLevel = computed(() => levels.value.reduce((max, l) => Math.max(max, l.level), 0))
const nextLevel = computed(() => maxLevel.value + 1)
const lastLevel = computed(() => levels.value.find((l) => l.level === maxLevel.value) ?? null)

const showCreate = ref(false)
const creating = ref(false)
const createError = ref('')
const createForm = reactive({ threshold_xp: 0, title: '', enabled: true })

const recalcOpen = ref(false)
const recalculating = ref(false)

// 单用户等级调整：只改等级偏移，不修改等级定义本身。
const levelAdjust = reactive({ target_user_id: '', delta_level: 1, reason: '' })
const adjusting = ref(false)
const adjustError = ref('')
const adjustNotice = ref('')

const inputClass =
  'w-full rounded-md border border-border-dark bg-ink-900 px-2 py-1 text-sm text-text-primary focus:border-grass-500 focus:outline-none'

/** 行内编辑草稿：level -> { threshold_xp, title, enabled } */
const drafts = reactive<Record<number, { threshold_xp: number; title: string; enabled: boolean }>>({})

const columns: AdminTableColumn[] = [
  { key: 'level', label: '等级', width: '80px' },
  { key: 'threshold_xp', label: '所需经验', width: '160px' },
  { key: 'title', label: '称号' },
  { key: 'enabled', label: '启用', width: '80px' },
  { key: 'action', label: '操作', align: 'right', width: '180px' },
]

async function load() {
  loading.value = true
  error.value = ''
  try {
    levels.value = await fetchGrowthLevels()
    for (const l of levels.value) {
      drafts[l.level] = { threshold_xp: l.threshold_xp, title: l.title, enabled: l.enabled }
    }
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '等级加载失败'
  } finally {
    loading.value = false
  }
}

onMounted(load)

function openCreate() {
  createError.value = ''
  createForm.title = ''
  createForm.enabled = true
  createForm.threshold_xp = lastLevel.value ? lastLevel.value.threshold_xp + 100 : 0
  showCreate.value = true
}

async function submitCreate() {
  createError.value = ''
  const title = createForm.title.trim()
  const threshold = Number(createForm.threshold_xp)
  if (!title) {
    createError.value = '请填写等级称号'
    return
  }
  if (!Number.isInteger(threshold) || threshold < 0) {
    createError.value = '所需经验必须是非负整数'
    return
  }
  if (lastLevel.value && threshold <= lastLevel.value.threshold_xp) {
    createError.value = `所需经验必须大于 Lv.${lastLevel.value.level} 的 ${lastLevel.value.threshold_xp}`
    return
  }
  const level = nextLevel.value
  creating.value = true
  error.value = ''
  notice.value = ''
  try {
    await createGrowthLevel({ threshold_xp: threshold, title, enabled: createForm.enabled })
    notice.value = `已新增 Lv.${level}。`
    showCreate.value = false
    await load()
  } catch (e) {
    createError.value = e instanceof ApiError ? e.message : '新增等级失败'
  } finally {
    creating.value = false
  }
}

function canDelete(row: GrowthLevel): boolean {
  return row.level > 1 && row.level === maxLevel.value
}

async function removeLevel(row: GrowthLevel) {
  if (!canDelete(row) || busyLevel.value !== null) return
  if (
    !window.confirm(
      `确定删除 Lv.${row.level}「${row.title}」吗？删除后用户等级会按剩余阈值重算，且不可恢复。`,
    )
  ) {
    return
  }
  busyLevel.value = row.level
  error.value = ''
  notice.value = ''
  try {
    await deleteGrowthLevel(row.level)
    notice.value = `已删除 Lv.${row.level}，用户等级已重算。`
    await load()
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '删除等级失败'
  } finally {
    busyLevel.value = null
  }
}

async function save(level: GrowthLevel) {
  const draft = drafts[level.level]
  if (!draft || busyLevel.value !== null) return
  busyLevel.value = level.level
  error.value = ''
  notice.value = ''
  try {
    await updateGrowthLevel({
      level: level.level,
      threshold_xp: Number(draft.threshold_xp) || 0,
      title: draft.title.trim(),
      enabled: draft.enabled,
    })
    notice.value = `Lv.${level.level} 已更新。`
    await load()
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '保存失败'
  } finally {
    busyLevel.value = null
  }
}

async function submitLevelAdjust() {
  adjustError.value = ''
  adjustNotice.value = ''
  if (!levelAdjust.target_user_id.trim()) {
    adjustError.value = '请填写目标用户 ID'
    return
  }
  if (!levelAdjust.delta_level) {
    adjustError.value = '调整值不能为 0'
    return
  }
  if (!levelAdjust.reason.trim()) {
    adjustError.value = '请填写调整原因'
    return
  }
  adjusting.value = true
  try {
    const result = await adjustUserLevel({
      target_user_id: levelAdjust.target_user_id.trim(),
      delta_level: Number(levelAdjust.delta_level),
      reason: levelAdjust.reason.trim(),
    })
    adjustNotice.value = `调整完成：Lv.${result.level} ${result.level_title}（基础 Lv.${result.base_level}，偏移 ${result.level_offset > 0 ? '+' : ''}${result.level_offset}）。`
    levelAdjust.target_user_id = ''
    levelAdjust.delta_level = 1
    levelAdjust.reason = ''
  } catch (e) {
    adjustError.value = e instanceof ApiError ? e.message : '等级调整失败'
  } finally {
    adjusting.value = false
  }
}

async function confirmRecalculate() {
  recalculating.value = true
  error.value = ''
  try {
    const fixed = await recalculateGrowthLevels()
    notice.value = `已重算，修正 ${fixed} 个用户等级。`
    recalcOpen.value = false
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '重算失败'
  } finally {
    recalculating.value = false
  }
}
</script>

<template>
  <div>
    <form class="card mb-5 grid gap-3 p-4 sm:grid-cols-4" @submit.prevent="submitLevelAdjust">
      <h2 class="text-sm font-medium text-text-secondary sm:col-span-4">单用户等级调整</h2>
      <p class="text-xs text-text-muted sm:col-span-4">
        只调整展示等级偏移，不修改等级定义本身；经验仍按阈值正常累积。
      </p>
      <label class="text-xs text-text-muted sm:col-span-2">
        目标用户 ID（内部 UUID 或对外 ID）
        <input v-model="levelAdjust.target_user_id" :class="inputClass" type="text" placeholder="UUID / U7K3M2QX9F" />
      </label>
      <label class="text-xs text-text-muted">
        调整值（可负）
        <input v-model.number="levelAdjust.delta_level" :class="inputClass" type="number" />
      </label>
      <label class="text-xs text-text-muted">
        原因（必填）
        <input v-model="levelAdjust.reason" :class="inputClass" type="text" placeholder="如 活动补偿" />
      </label>
      <p v-if="adjustError" class="text-xs text-danger-mc sm:col-span-4" role="alert">{{ adjustError }}</p>
      <p v-if="adjustNotice" class="text-xs text-grass-400 sm:col-span-4" role="status">{{ adjustNotice }}</p>
      <button
        type="submit"
        class="rounded-md bg-grass-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-grass-500 disabled:opacity-60 sm:col-span-4 sm:w-auto sm:justify-self-start"
        :disabled="adjusting"
      >
        提交等级调整
      </button>
    </form>

    <div class="mb-4 flex flex-wrap items-center justify-between gap-3">
      <p class="text-xs text-text-muted">
        阈值必须唯一且严格递增，Lv.1 固定为 0 经验。等级仅展示，不影响权限；新增会追加到最高级之后，删除仅限最高级且 Lv.1 不可删除。
      </p>
      <div class="flex shrink-0 gap-2">
        <button
          type="button"
          class="rounded-md bg-grass-600 px-3 py-1.5 text-sm font-medium text-white transition hover:bg-grass-500 disabled:opacity-60"
          :disabled="loading"
          @click="openCreate"
        >
          新增等级
        </button>
        <button
          type="button"
          class="rounded-md border border-border-light bg-ink-700 px-3 py-1.5 text-sm text-text-secondary transition hover:bg-ink-600"
          @click="recalcOpen = true"
        >
          重算等级
        </button>
      </div>
    </div>

    <p v-if="error" class="mb-3 rounded-card border border-danger-mc/40 bg-danger-mc/10 px-4 py-2 text-sm text-danger-mc" role="alert">{{ error }}</p>
    <p v-if="notice" class="mb-3 rounded-card border border-grass-500/40 bg-grass-500/10 px-4 py-2 text-sm text-grass-400" role="status">{{ notice }}</p>

    <form v-if="showCreate" class="card mb-4 grid gap-3 p-4 sm:grid-cols-4" @submit.prevent="submitCreate">
      <h2 class="text-sm font-medium text-text-secondary sm:col-span-4">新增 Lv.{{ nextLevel }}</h2>
      <p class="text-xs text-text-muted sm:col-span-4">
        等级号由服务端自动追加；所需经验必须大于当前最高级
        <template v-if="lastLevel"> Lv.{{ lastLevel.level }} 的 {{ lastLevel.threshold_xp }}</template>
        。
      </p>
      <label class="text-xs text-text-muted">
        所需经验
        <input
          v-model.number="createForm.threshold_xp"
          :class="inputClass"
          type="number"
          min="0"
          step="1"
          placeholder="如 8000"
        />
      </label>
      <label class="text-xs text-text-muted sm:col-span-2">
        称号
        <input
          v-model="createForm.title"
          :class="inputClass"
          type="text"
          maxlength="100"
          placeholder="如 红石大师"
        />
      </label>
      <label class="flex items-end gap-2 pb-2 text-xs text-text-muted">
        <input v-model="createForm.enabled" type="checkbox" />
        启用
      </label>
      <p v-if="createError" class="text-xs text-danger-mc sm:col-span-4" role="alert">{{ createError }}</p>
      <div class="flex gap-2 sm:col-span-4">
        <button
          type="submit"
          class="rounded-md bg-grass-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-grass-500 disabled:opacity-60"
          :disabled="creating"
        >
          {{ creating ? '创建中…' : '创建等级' }}
        </button>
        <button
          type="button"
          class="rounded-md border border-border-light bg-ink-700 px-4 py-2 text-sm text-text-secondary transition hover:bg-ink-600"
          :disabled="creating"
          @click="showCreate = false"
        >
          取消
        </button>
      </div>
    </form>

    <AdminTable :columns="columns" :rows="levels" :loading="loading" row-key="level" empty-text="暂无等级配置">
      <template #cell-level="{ row }">Lv.{{ row.level }}</template>
      <template #cell-threshold_xp="{ row }">
        <input v-model.number="drafts[row.level].threshold_xp" :class="inputClass" type="number" min="0" :disabled="row.level === 1" />
      </template>
      <template #cell-title="{ row }">
        <input v-model="drafts[row.level].title" :class="inputClass" type="text" maxlength="100" />
      </template>
      <template #cell-enabled="{ row }">
        <input v-model="drafts[row.level].enabled" type="checkbox" />
      </template>
      <template #cell-action="{ row }">
        <div class="flex justify-end gap-2">
          <button
            type="button"
            class="rounded-md bg-grass-600 px-3 py-1 text-xs font-medium text-white transition hover:bg-grass-500 disabled:opacity-50"
            :disabled="busyLevel !== null"
            @click="save(row)"
          >
            保存
          </button>
          <button
            type="button"
            class="rounded-md border border-danger-mc/40 px-3 py-1 text-xs font-medium text-danger-mc transition hover:bg-danger-mc/10 disabled:cursor-not-allowed disabled:opacity-30"
            :disabled="!canDelete(row) || busyLevel !== null"
            :title="canDelete(row) ? '删除最高等级' : '仅可删除最高等级（Lv.1 不可删除）'"
            @click="removeLevel(row)"
          >
            删除
          </button>
        </div>
      </template>
    </AdminTable>

    <ConfirmDialog
      :open="recalcOpen"
      title="重算用户等级"
      message="将依据当前阈值表重算所有用户的展示等级。此操作不改变经验与权限，仅修正展示值。"
      confirm-text="重算"
      :confirm-disabled="recalculating"
      @confirm="confirmRecalculate"
      @cancel="recalcOpen = false"
    />
  </div>
</template>
