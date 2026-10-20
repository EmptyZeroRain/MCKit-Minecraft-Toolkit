<script setup lang="ts">
// 等级阈值管理（jingyan.md §5 / §10.4 相关）。
//
// 等级仅作展示，不构成任何权限。阈值必须全局唯一、按等级严格递增，Lv.1 固定为 0。
// 等级定义可指定等级号新增（允许跳号）或删除任意非 Lv.1 等级；修改后可用「重算等级」修正历史用户展示等级。
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
const createForm = reactive({ level: 1, threshold_xp: 0, title: '', enabled: true })

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
  createForm.level = nextLevel.value
  createForm.title = ''
  createForm.enabled = true
  createForm.threshold_xp = lastLevel.value ? lastLevel.value.threshold_xp + 100 : 0
  showCreate.value = true
}

async function submitCreate() {
  createError.value = ''
  const title = createForm.title.trim()
  const level = Number(createForm.level)
  const threshold = Number(createForm.threshold_xp)
  if (!Number.isInteger(level) || level < 1) {
    createError.value = '等级号必须是大于等于 1 的整数'
    return
  }
  if (!title) {
    createError.value = '请填写等级称号'
    return
  }
  if (!Number.isInteger(threshold) || threshold < 0) {
    createError.value = '所需经验必须是非负整数'
    return
  }
  if (levels.value.some((l) => l.level === level)) {
    createError.value = `Lv.${level} 已存在，请使用其他等级号`
    return
  }
  if (level === 1 && threshold !== 0) {
    createError.value = 'Lv.1 的所需经验必须为 0'
    return
  }
  if (levels.value.length === 0 && (level !== 1 || threshold !== 0)) {
    createError.value = '第一个等级必须是 Lv.1，且所需经验为 0'
    return
  }
  const lower = levels.value.filter((l) => l.level < level)
  const higher = levels.value.filter((l) => l.level > level)
  const lowerMax = lower.reduce<GrowthLevel | null>(
    (max, l) => (!max || l.threshold_xp > max.threshold_xp ? l : max),
    null,
  )
  const higherMin = higher.reduce<GrowthLevel | null>(
    (min, l) => (!min || l.threshold_xp < min.threshold_xp ? l : min),
    null,
  )
  if (lowerMax && threshold <= lowerMax.threshold_xp) {
    createError.value = `所需经验必须大于 Lv.${lowerMax.level} 的 ${lowerMax.threshold_xp}`
    return
  }
  if (higherMin && threshold >= higherMin.threshold_xp) {
    createError.value = `所需经验必须小于 Lv.${higherMin.level} 的 ${higherMin.threshold_xp}`
    return
  }
  creating.value = true
  error.value = ''
  notice.value = ''
  try {
    const created = await createGrowthLevel({
      level,
      threshold_xp: threshold,
      title,
      enabled: createForm.enabled,
    })
    notice.value = `已新增 Lv.${created.level}。`
    showCreate.value = false
    await load()
  } catch (e) {
    createError.value = e instanceof ApiError ? e.message : '新增等级失败'
  } finally {
    creating.value = false
  }
}

function canDelete(row: GrowthLevel): boolean {
  return row.level > 1
}

async function removeLevel(row: GrowthLevel) {
  if (!canDelete(row) || busyLevel.value !== null) return
  if (
    !window.confirm(
      `确定删除 Lv.${row.level}「${row.title}」吗？删除后所有更高等级号会减 1，用户等级会重算，且不可恢复。`,
    )
  ) {
    return
  }
  busyLevel.value = row.level
  error.value = ''
  notice.value = ''
  try {
    await deleteGrowthLevel(row.level)
    notice.value = `已删除 Lv.${row.level}，后续等级已重排，用户等级已重算。`
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
  const targetId = levelAdjust.target_user_id.trim().toUpperCase()
  if (!targetId) {
    adjustError.value = '请填写目标用户 ID'
    return
  }
  // 后端只接受对外 user_no；这里提前拦截内部 UUID，避免落到通用的 404/422 文案。
  if (/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(targetId)) {
    adjustError.value = '目标用户 ID 只支持对外用户 ID（如 U7K3M2QX9F），不支持内部 UUID'
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
      target_user_id: targetId,
      delta_level: Number(levelAdjust.delta_level),
      reason: levelAdjust.reason.trim(),
    })
    adjustNotice.value = `调整完成：Lv.${result.level} ${result.level_title}（基础 Lv.${result.base_level}，偏移 ${result.level_offset > 0 ? '+' : ''}${result.level_offset}）。`
    levelAdjust.target_user_id = ''
    levelAdjust.delta_level = 1
    levelAdjust.reason = ''
  } catch (e) {
    // 后端字段级错误（如对外 ID 不存在）优先展示，避免只看到通用错误文案。
    const detail = e instanceof ApiError ? e.details?.[0]?.message : ''
    adjustError.value = detail || (e instanceof ApiError ? e.message : '等级调整失败')
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
        只调整展示等级偏移，不修改等级定义本身；经验仍按阈值正常累积。目标只填用户列表中的对外 ID，
        不要填内部 UUID 或用户名；普通用户与后台账号均可调整。
      </p>
      <label class="text-xs text-text-muted sm:col-span-2">
        目标用户 ID（对外 ID）
        <input
          v-model="levelAdjust.target_user_id"
          :class="inputClass"
          type="text"
          maxlength="12"
          autocomplete="off"
          autocapitalize="characters"
          spellcheck="false"
          placeholder="U7K3M2QX9F"
        />
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
        阈值必须唯一且严格递增，Lv.1 固定为 0 经验。等级仅展示，不影响权限；新增可指定等级号并允许跳号，删除任意非 Lv.1 等级会自动重排后续等级。
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

    <form v-if="showCreate" class="card mb-4 grid gap-3 p-4 sm:grid-cols-5" @submit.prevent="submitCreate">
      <h2 class="text-sm font-medium text-text-secondary sm:col-span-5">新增等级（默认 Lv.{{ nextLevel }}）</h2>
      <p class="text-xs text-text-muted sm:col-span-5">
        可指定等级号并允许跳号（如 99、999）；阈值必须与等级号一起严格递增，Lv.1 固定为 0。
      </p>
      <label class="text-xs text-text-muted">
        等级号
        <input
          v-model.number="createForm.level"
          :class="inputClass"
          type="number"
          min="1"
          step="1"
          placeholder="如 99"
        />
      </label>
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
      <p v-if="createError" class="text-xs text-danger-mc sm:col-span-5" role="alert">{{ createError }}</p>
      <div class="flex gap-2 sm:col-span-5">
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
            :title="canDelete(row) ? '删除该等级并自动重排后续等级' : 'Lv.1 不可删除'"
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
