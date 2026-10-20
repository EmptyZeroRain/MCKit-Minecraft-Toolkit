<script setup lang="ts">
// 等级曲线与称号规则（jingyan.md §5 / §10.4 相关）。
//
// 等级只作展示，不参与权限。Lv.0 是访客；Lv.1 - Lv.9999 由曲线自动生成：
//   达到 Lv.N 所需经验 = step_xp * N * (N-1) / 2
// 后台不再逐级创建 / 删除等级，只维护曲线和称号规则：
//   * 区间称号：min_level < max_level，启用区间之间不能重叠；
//   * 指定等级称号：min_level === max_level，优先级高于区间。
import { computed, onMounted, reactive, ref } from 'vue'
import AdminTable from '@/components/admin/AdminTable.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import { ApiError } from '@/types/api'
import type { AdminTableColumn } from '@/types/admin'
import {
  adjustUserLevel,
  createGrowthLevelTitle,
  deleteGrowthLevelTitle,
  fetchGrowthLevelConfig,
  fetchGrowthLevelTitles,
  recalculateGrowthLevels,
  updateGrowthLevelConfig,
  updateGrowthLevelTitle,
} from '@/api/growth'
import type {
  GrowthLevelConfig,
  GrowthLevelTitle,
  GrowthLevelTitleInput,
} from '@/types/growth'

const inputClass =
  'w-full rounded-md border border-border-dark bg-ink-900 px-2 py-1 text-sm text-text-primary focus:border-grass-500 focus:outline-none'

const loading = ref(true)
const error = ref('')
const notice = ref('')

const config = ref<GrowthLevelConfig>({ max_level: 9999, step_xp: 100 })
const configForm = reactive({ max_level: 9999, step_xp: 100 })
const savingConfig = ref(false)
const configError = ref('')
const configNotice = ref('')

const titles = ref<GrowthLevelTitle[]>([])
const drafts = reactive<Record<string, GrowthLevelTitleInput>>({})
const busyTitleID = ref('')

const rangeForm = reactive({ min_level: 1, max_level: 10, title: '', enabled: true })
const exactForm = reactive({ level: 100, title: '', enabled: true })
const createError = ref('')
const creating = ref(false)

const recalcOpen = ref(false)
const recalculating = ref(false)

const levelAdjust = reactive({ target_user_id: '', target_level: 1, reason: '' })
const adjusting = ref(false)
const adjustError = ref('')
const adjustNotice = ref('')

const titleColumns: AdminTableColumn[] = [
  { key: 'range', label: '等级范围', width: '150px' },
  { key: 'kind', label: '类型', width: '100px' },
  { key: 'title', label: '称号' },
  { key: 'enabled', label: '启用', width: '80px' },
  { key: 'action', label: '操作', align: 'right', width: '170px' },
]

/** 区间称号：min < max。 */
const rangeTitles = computed(() => titles.value.filter((t) => t.min_level !== t.max_level))
/** 指定等级称号：min === max。 */
const exactTitles = computed(() => titles.value.filter((t) => t.min_level === t.max_level))

/** 曲线预览（关键等级，避免 9999 行铺满页面）。 */
const previewRows = computed(() => {
  const max = Math.max(1, Math.trunc(Number(configForm.max_level) || 1))
  const candidates = [1, 2, 10, 100, 1000, max]
  const levels = [...new Set(candidates.filter((level) => level >= 1 && level <= max))]
  levels.sort((a, b) => a - b)
  return levels.map((level) => ({ level, xp: thresholdXP(level) }))
})

/** 计算等级门槛；使用 BigInt 避免高 step_xp 时超出 JS 安全整数。 */
function thresholdXP(level: number): bigint {
  if (level <= 1) return 0n
  const step = BigInt(Math.max(0, Math.trunc(Number(configForm.step_xp) || 0)))
  const n = BigInt(level)
  return (step * n * (n - 1n)) / 2n
}

function formatXP(value: bigint): string {
  return value.toLocaleString('zh-CN')
}

function fillDrafts(items: GrowthLevelTitle[]) {
  for (const key of Object.keys(drafts)) delete drafts[key]
  for (const item of items) {
    drafts[item.id] = {
      min_level: item.min_level,
      max_level: item.max_level,
      title: item.title,
      enabled: item.enabled,
    }
  }
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [cfg, items] = await Promise.all([fetchGrowthLevelConfig(), fetchGrowthLevelTitles()])
    config.value = cfg
    configForm.max_level = cfg.max_level
    configForm.step_xp = cfg.step_xp
    titles.value = items
    fillDrafts(items)
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '等级配置加载失败'
  } finally {
    loading.value = false
  }
}

onMounted(load)

function validateTitle(input: GrowthLevelTitleInput): string {
  if (!Number.isInteger(input.min_level) || !Number.isInteger(input.max_level)) {
    return '等级必须是整数'
  }
  if (input.min_level < 0 || input.max_level < input.min_level) {
    return '等级范围不合法：最小值不能小于 0，最大值不能小于最小值'
  }
  if (input.enabled && input.max_level > config.value.max_level) {
    return `启用的称号范围不能超过当前等级上限 Lv.${config.value.max_level}；可先停用再保存`
  }
  if (!input.title.trim()) return '请填写称号'
  if (input.title.trim().length > 100) return '称号不能超过 100 个字符'
  return ''
}

async function submitConfig() {
  configError.value = ''
  configNotice.value = ''
  const maxLevel = Number(configForm.max_level)
  const stepXP = Number(configForm.step_xp)
  if (!Number.isInteger(maxLevel) || maxLevel < 1 || maxLevel > 9999) {
    configError.value = '等级上限必须是 1 - 9999 的整数'
    return
  }
  if (!Number.isInteger(stepXP) || stepXP < 1 || stepXP > 1_000_000_000) {
    configError.value = '每级递增值必须是 1 - 1,000,000,000 的整数'
    return
  }
  savingConfig.value = true
  try {
    const fixed = await updateGrowthLevelConfig({ max_level: maxLevel, step_xp: stepXP })
    config.value = { ...config.value, max_level: maxLevel, step_xp: stepXP }
    configNotice.value = `曲线已保存，重算修正 ${fixed} 个用户等级。`
    notice.value = ''
    // 上限可能降低，称号规则本身不删，仅刷新列表以展示最新状态。
    titles.value = await fetchGrowthLevelTitles()
    fillDrafts(titles.value)
  } catch (e) {
    const detail = e instanceof ApiError ? e.details?.[0]?.message : ''
    configError.value = detail || (e instanceof ApiError ? e.message : '保存等级曲线失败')
  } finally {
    savingConfig.value = false
  }
}

async function submitRangeTitle() {
  createError.value = ''
  const input: GrowthLevelTitleInput = {
    min_level: Number(rangeForm.min_level),
    max_level: Number(rangeForm.max_level),
    title: rangeForm.title.trim(),
    enabled: rangeForm.enabled,
  }
  const invalid = validateTitle(input)
  if (invalid) {
    createError.value = invalid
    return
  }
  if (input.min_level === input.max_level) {
    createError.value = '区间称号的最大等级必须大于最小等级；单级请使用右侧指定等级称号'
    return
  }
  creating.value = true
  try {
    await createGrowthLevelTitle(input)
    notice.value = `已新增区间称号 Lv.${input.min_level}-${input.max_level}「${input.title}」。`
    rangeForm.title = ''
    titles.value = await fetchGrowthLevelTitles()
    fillDrafts(titles.value)
  } catch (e) {
    const detail = e instanceof ApiError ? e.details?.[0]?.message : ''
    createError.value = detail || (e instanceof ApiError ? e.message : '新增区间称号失败')
  } finally {
    creating.value = false
  }
}

async function submitExactTitle() {
  createError.value = ''
  const level = Number(exactForm.level)
  const input: GrowthLevelTitleInput = {
    min_level: level,
    max_level: level,
    title: exactForm.title.trim(),
    enabled: exactForm.enabled,
  }
  const invalid = validateTitle(input)
  if (invalid) {
    createError.value = invalid
    return
  }
  creating.value = true
  try {
    await createGrowthLevelTitle(input)
    notice.value = `已新增 Lv.${level} 指定称号「${input.title}」。`
    exactForm.title = ''
    titles.value = await fetchGrowthLevelTitles()
    fillDrafts(titles.value)
  } catch (e) {
    const detail = e instanceof ApiError ? e.details?.[0]?.message : ''
    createError.value = detail || (e instanceof ApiError ? e.message : '新增指定等级称号失败')
  } finally {
    creating.value = false
  }
}

async function saveTitle(row: GrowthLevelTitle) {
  createError.value = ''
  const draft = drafts[row.id]
  if (!draft) return
  const input: GrowthLevelTitleInput = {
    min_level: Number(draft.min_level),
    max_level: Number(draft.max_level),
    title: draft.title.trim(),
    enabled: draft.enabled,
  }
  const invalid = validateTitle(input)
  if (invalid) {
    createError.value = invalid
    return
  }
  busyTitleID.value = row.id
  try {
    await updateGrowthLevelTitle(row.id, input)
    notice.value = `已保存称号规则 Lv.${input.min_level}${input.min_level === input.max_level ? '' : `-${input.max_level}`}。`
    titles.value = await fetchGrowthLevelTitles()
    fillDrafts(titles.value)
  } catch (e) {
    const detail = e instanceof ApiError ? e.details?.[0]?.message : ''
    createError.value = detail || (e instanceof ApiError ? e.message : '保存称号规则失败')
  } finally {
    busyTitleID.value = ''
  }
}

async function removeTitle(row: GrowthLevelTitle) {
  const label = row.min_level === row.max_level
    ? `Lv.${row.min_level}`
    : `Lv.${row.min_level}-${row.max_level}`
  if (!window.confirm(`确定删除称号规则 ${label}「${row.title}」？删除后该等级会回退为默认 Lv.N。`)) {
    return
  }
  busyTitleID.value = row.id
  try {
    await deleteGrowthLevelTitle(row.id)
    notice.value = `已删除称号规则 ${label}。`
    titles.value = await fetchGrowthLevelTitles()
    fillDrafts(titles.value)
  } catch (e) {
    createError.value = e instanceof ApiError ? e.message : '删除称号规则失败'
  } finally {
    busyTitleID.value = ''
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
  const targetLevel = Number(levelAdjust.target_level)
  if (!Number.isInteger(targetLevel) || targetLevel < 0) {
    adjustError.value = '目标等级必须是非负整数'
    return
  }
  if (targetLevel > config.value.max_level) {
    adjustError.value = `目标等级必须在 0 - ${config.value.max_level} 之间`
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
      target_level: targetLevel,
      reason: levelAdjust.reason.trim(),
    })
    adjustNotice.value = `调整完成：Lv.${result.level} ${result.level_title}（已人工覆盖，经验继续累积但不自动改级）。`
    levelAdjust.target_user_id = ''
    levelAdjust.target_level = 1
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
    <div class="mb-4 flex flex-wrap items-center justify-between gap-3">
      <div>
        <h2 class="text-sm font-medium text-text-secondary">等级曲线与称号</h2>
        <p class="mt-1 text-xs text-text-muted">
          Lv.0 为访客；累计满 7 个活跃日后进入 Lv.1，之后按经验自动升级到 Lv.{{ config.max_level }}。
          等级仅展示，不影响权限。
        </p>
      </div>
      <button
        type="button"
        class="shrink-0 rounded-md border border-border-light bg-ink-700 px-3 py-1.5 text-sm text-text-secondary transition hover:bg-ink-600"
        @click="recalcOpen = true"
      >
        重算等级
      </button>
    </div>

    <p v-if="error" class="mb-3 rounded-card border border-danger-mc/40 bg-danger-mc/10 px-4 py-2 text-sm text-danger-mc" role="alert">{{ error }}</p>
    <p v-if="notice" class="mb-3 rounded-card border border-grass-500/40 bg-grass-500/10 px-4 py-2 text-sm text-grass-400" role="status">{{ notice }}</p>

    <form data-test="level-config-form" class="card mb-5 grid gap-3 p-4 sm:grid-cols-4" @submit.prevent="submitConfig">
      <h3 class="text-sm font-medium text-text-secondary sm:col-span-4">等级曲线</h3>
      <p class="text-xs text-text-muted sm:col-span-4">
        达到 Lv.N 所需经验 = 每级递增值 × N × (N-1) ÷ 2。Lv.0 / Lv.1 均为 0 XP；保存后会立即按新曲线重算全部用户等级。
      </p>
      <label class="text-xs text-text-muted">
        等级上限
        <input v-model.number="configForm.max_level" :class="inputClass" type="number" min="1" max="9999" step="1" />
      </label>
      <label class="text-xs text-text-muted">
        每级递增值（XP）
        <input v-model.number="configForm.step_xp" :class="inputClass" type="number" min="1" max="1000000000" step="1" />
      </label>
      <div class="rounded-md border border-border-dark bg-ink-900 px-3 py-2 text-xs text-text-muted sm:col-span-2">
        <p class="mb-1 font-medium text-text-secondary">曲线预览</p>
        <div v-for="row in previewRows" :key="row.level" class="flex justify-between gap-3">
          <span>Lv.{{ row.level }}</span>
          <span>{{ formatXP(row.xp) }} XP</span>
        </div>
      </div>
      <p v-if="configError" class="text-xs text-danger-mc sm:col-span-4" role="alert">{{ configError }}</p>
      <p v-if="configNotice" class="text-xs text-grass-400 sm:col-span-4" role="status">{{ configNotice }}</p>
      <button
        type="submit"
        class="rounded-md bg-grass-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-grass-500 disabled:opacity-60 sm:col-span-4 sm:w-auto sm:justify-self-start"
        :disabled="savingConfig"
      >
        {{ savingConfig ? '保存中…' : '保存等级曲线' }}
      </button>
    </form>

    <div class="card mb-5 p-4">
      <div class="grid gap-4 lg:grid-cols-2">
        <form data-test="range-title-form" class="grid gap-2 rounded-md border border-border-dark p-3 sm:grid-cols-2" @submit.prevent="submitRangeTitle">
          <h3 class="text-sm font-medium text-text-secondary sm:col-span-2">新增区间称号</h3>
          <p class="text-xs text-text-muted sm:col-span-2">例如 Lv.1-10「初来乍到」；启用区间之间不能重叠。</p>
          <label class="text-xs text-text-muted">
            最小等级
            <input v-model.number="rangeForm.min_level" :class="inputClass" type="number" min="0" :max="config.max_level" step="1" />
          </label>
          <label class="text-xs text-text-muted">
            最大等级
            <input v-model.number="rangeForm.max_level" :class="inputClass" type="number" min="0" :max="config.max_level" step="1" />
          </label>
          <label class="text-xs text-text-muted sm:col-span-2">
            称号
            <input v-model="rangeForm.title" :class="inputClass" type="text" maxlength="100" placeholder="如 初来乍到" />
          </label>
          <label class="flex items-center gap-2 text-xs text-text-muted">
            <input v-model="rangeForm.enabled" type="checkbox" />
            启用
          </label>
          <button
            type="submit"
            class="rounded-md bg-grass-600 px-3 py-1.5 text-sm font-medium text-white transition hover:bg-grass-500 disabled:opacity-60"
            :disabled="creating"
          >
            新增区间称号
          </button>
        </form>

        <form data-test="exact-title-form" class="grid gap-2 rounded-md border border-border-dark p-3 sm:grid-cols-2" @submit.prevent="submitExactTitle">
          <h3 class="text-sm font-medium text-text-secondary sm:col-span-2">新增指定等级称号</h3>
          <p class="text-xs text-text-muted sm:col-span-2">例如 Lv.100「MCkit 传奇」；指定等级称号优先于区间称号。</p>
          <label class="text-xs text-text-muted">
            等级
            <input v-model.number="exactForm.level" :class="inputClass" type="number" min="0" :max="config.max_level" step="1" />
          </label>
          <label class="text-xs text-text-muted">
            称号
            <input v-model="exactForm.title" :class="inputClass" type="text" maxlength="100" placeholder="如 MCkit 传奇" />
          </label>
          <label class="flex items-center gap-2 text-xs text-text-muted">
            <input v-model="exactForm.enabled" type="checkbox" />
            启用
          </label>
          <button
            type="submit"
            class="rounded-md bg-grass-600 px-3 py-1.5 text-sm font-medium text-white transition hover:bg-grass-500 disabled:opacity-60"
            :disabled="creating"
          >
            新增指定称号
          </button>
        </form>
      </div>
      <p v-if="createError" class="mt-3 text-xs text-danger-mc" role="alert">{{ createError }}</p>
    </div>

    <div class="mb-2 flex flex-wrap items-center justify-between gap-2">
      <h3 class="text-sm font-medium text-text-secondary">称号规则</h3>
      <p class="text-xs text-text-muted">区间 {{ rangeTitles.length }} 条 · 指定等级 {{ exactTitles.length }} 条</p>
    </div>
    <AdminTable :columns="titleColumns" :rows="titles" :loading="loading" row-key="id" empty-text="暂无称号规则，所有等级将展示默认 Lv.N">
      <template #cell-range="{ row }">
        <span class="font-mono text-xs">
          {{ row.min_level === row.max_level ? `Lv.${row.min_level}` : `Lv.${row.min_level}-${row.max_level}` }}
        </span>
      </template>
      <template #cell-kind="{ row }">
        <span class="text-xs" :class="row.min_level === row.max_level ? 'text-grass-400' : 'text-text-muted'">
          {{ row.min_level === row.max_level ? '指定等级' : '区间' }}
        </span>
      </template>
      <template #cell-title="{ row }">
        <input v-model="drafts[row.id].title" :class="inputClass" type="text" maxlength="100" />
      </template>
      <template #cell-enabled="{ row }">
        <input v-model="drafts[row.id].enabled" type="checkbox" />
      </template>
      <template #cell-action="{ row }">
        <div class="flex justify-end gap-2">
          <button
            type="button"
            class="rounded-md bg-grass-600 px-3 py-1 text-xs font-medium text-white transition hover:bg-grass-500 disabled:opacity-50"
            :disabled="busyTitleID !== ''"
            @click="saveTitle(row)"
          >
            保存
          </button>
          <button
            type="button"
            class="rounded-md border border-danger-mc/40 px-3 py-1 text-xs font-medium text-danger-mc transition hover:bg-danger-mc/10 disabled:opacity-50"
            :disabled="busyTitleID !== ''"
            @click="removeTitle(row)"
          >
            删除
          </button>
        </div>
      </template>
    </AdminTable>

    <form data-test="level-adjust-form" class="card mt-5 grid gap-3 p-4 sm:grid-cols-4" @submit.prevent="submitLevelAdjust">
      <h3 class="text-sm font-medium text-text-secondary sm:col-span-4">单用户等级调整</h3>
      <p class="text-xs text-text-muted sm:col-span-4">
        直接覆盖展示等级，不修改等级曲线与称号规则；覆盖后经验继续累积，但不会自动改级，直到下次人工调整。
        目标只填用户列表中的对外 ID，不要填内部 UUID 或用户名；普通用户与后台账号均可调整。
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
        目标等级（0 - {{ config.max_level }}）
        <input
          v-model.number="levelAdjust.target_level"
          :class="inputClass"
          type="number"
          min="0"
          :max="config.max_level"
          step="1"
          placeholder="如 3"
        />
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

    <ConfirmDialog
      :open="recalcOpen"
      title="重算用户等级"
      message="将依据当前等级曲线重算所有用户的展示等级。此操作不改变经验与权限，仅修正展示值；人工覆盖等级会被限制在当前等级上限内。"
      confirm-text="重算"
      :confirm-disabled="recalculating"
      @confirm="confirmRecalculate"
      @cancel="recalcOpen = false"
    />
  </div>
</template>
