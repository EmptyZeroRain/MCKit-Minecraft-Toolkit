<script setup lang="ts">
// 等级曲线与单用户等级调整（jingyan.md §5 / §10.4 相关）。
//
// 等级只作展示，不参与权限。Lv.0 是访客；Lv.1 起由曲线自动生成（默认不设上限）：
//   达到 Lv.N 所需经验 = step_xp * N * (N-1) / 2
// 后台不再逐级创建 / 删除等级，只维护曲线和人工覆盖。
// 称号规则（区间 / 指定等级）已独立到「称号」Tab。
import { computed, onMounted, reactive, ref } from 'vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import { ApiError } from '@/types/api'
import {
  adjustUserLevel,
  fetchGrowthLevelConfig,
  recalculateGrowthLevels,
  updateGrowthLevelConfig,
} from '@/api/growth'
import type { GrowthLevelConfig } from '@/types/growth'

const inputClass =
  'w-full rounded-md border border-border-dark bg-ink-900 px-2 py-1 text-sm text-text-primary focus:border-grass-500 focus:outline-none'

const loading = ref(true)
const error = ref('')
const notice = ref('')

const config = ref<GrowthLevelConfig>({ max_level: 0, step_xp: 100 })
const configForm = reactive({ max_level: 0, step_xp: 100 })
const savingConfig = ref(false)
const configError = ref('')
const configNotice = ref('')

const recalcOpen = ref(false)
const recalculating = ref(false)

const levelAdjust = reactive({ target_user_id: '', target_level: 1, reason: '' })
const adjusting = ref(false)
const adjustError = ref('')
const adjustNotice = ref('')

/** 0 表示不设上限；HTML 输入仍需要一个技术上的最大整数。 */
const maxLevelInput = computed(() =>
  config.value.max_level > 0 ? config.value.max_level : 2147483647,
)
const levelLimitText = computed(() =>
  config.value.max_level > 0 ? `0 - ${config.value.max_level}` : '0 - 无限',
)

/** 曲线预览（关键等级）。 */
const previewRows = computed(() => {
  const max = Math.trunc(Number(configForm.max_level) || 0)
  const candidates = [1, 2, 10, 100, 1000, 10000]
  if (max > 0) candidates.push(max)
  const levels = [...new Set(candidates.filter((level) => level >= 1 && (max <= 0 || level <= max)))]
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

async function load() {
  loading.value = true
  error.value = ''
  try {
    const cfg = await fetchGrowthLevelConfig()
    config.value = cfg
    configForm.max_level = cfg.max_level
    configForm.step_xp = cfg.step_xp
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '等级配置加载失败'
  } finally {
    loading.value = false
  }
}

onMounted(load)

async function submitConfig() {
  configError.value = ''
  configNotice.value = ''
  const maxLevel = Number(configForm.max_level)
  const stepXP = Number(configForm.step_xp)
  if (!Number.isInteger(maxLevel) || maxLevel < 0 || maxLevel > 2147483647) {
    configError.value = '等级上限必须是非负整数（0 表示无限）'
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
  } catch (e) {
    const detail = e instanceof ApiError ? e.details?.[0]?.message : ''
    configError.value = detail || (e instanceof ApiError ? e.message : '保存等级曲线失败')
  } finally {
    savingConfig.value = false
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
  if (config.value.max_level > 0 && targetLevel > config.value.max_level) {
    adjustError.value = `目标等级必须在 ${levelLimitText.value} 之间`
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
        <h2 class="text-sm font-medium text-text-secondary">等级曲线</h2>
        <p class="mt-1 text-xs text-text-muted">
          Lv.0 为访客；累计满 7 个活跃日后进入 Lv.1，之后按经验自动升级
          {{ config.max_level > 0 ? `到 Lv.${config.max_level}` : '，不设等级上限' }}。
          等级仅展示，不影响权限；称号规则请见「称号」Tab。
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
        等级上限（0 = 无限）
        <input v-model.number="configForm.max_level" :class="inputClass" type="number" min="0" max="2147483647" step="1" />
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

    <form data-test="level-adjust-form" class="card mt-5 grid gap-3 p-4 sm:grid-cols-4" @submit.prevent="submitLevelAdjust">
      <h3 class="text-sm font-medium text-text-secondary sm:col-span-4">单用户等级调整</h3>
      <p class="text-xs text-text-muted sm:col-span-4">
        直接覆盖展示等级，不修改等级曲线；覆盖后经验继续累积，但不会自动改级，直到下次人工调整。
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
        目标等级（{{ levelLimitText }}）
        <input
          v-model.number="levelAdjust.target_level"
          :class="inputClass"
          type="number"
          min="0"
          :max="maxLevelInput"
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
      message="将依据当前等级曲线重算所有用户的展示等级。此操作不改变经验与权限，仅修正展示值；若设置了等级上限，人工覆盖等级会被限制在该上限内。"
      confirm-text="重算"
      :confirm-disabled="recalculating"
      @confirm="confirmRecalculate"
      @cancel="recalcOpen = false"
    />
  </div>
</template>
