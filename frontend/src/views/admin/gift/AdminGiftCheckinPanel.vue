<script setup lang="ts">
// 签到设置面板：签到礼物币、签到经验算法、断签惩罚、补签卡与连续签到里程碑。
import { onMounted, ref } from 'vue'
import { ApiError } from '@/types/api'
import { fetchGiftSettings, updateGiftSettings } from '@/api/gift'
import type { GiftCheckinMilestoneInput } from '@/types/gift'

const inputClass =
  'w-full rounded-md border border-border-dark bg-ink-900 px-3 py-2 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none'

const loading = ref(true)
const saving = ref(false)
const error = ref('')
const notice = ref('')

const checkinReward = ref(1)
const baseXP = ref(10)
const streakStepXP = ref(2)
const streakBonusCap = ref(100)
const milestoneBonusCap = ref(500)
const breakPenaltyXP = ref(5)
const makeupCardPrice = ref(10)
const milestones = ref<GiftCheckinMilestoneInput[]>([])

function applySettings(settings: Awaited<ReturnType<typeof fetchGiftSettings>>) {
  checkinReward.value = Number(settings.checkin_reward ?? 0)
  baseXP.value = Number(settings.base_xp ?? 0)
  streakStepXP.value = Number(settings.streak_step_xp ?? 0)
  streakBonusCap.value = Number(settings.streak_bonus_cap ?? 0)
  milestoneBonusCap.value = Number(settings.milestone_bonus_cap ?? 0)
  breakPenaltyXP.value = Number(settings.break_penalty_xp ?? 0)
  makeupCardPrice.value = Number(settings.makeup_card_price ?? 1)
  milestones.value = (settings.milestones ?? []).map((m) => ({
    streak_days: m.streak_days,
    bonus_xp: m.bonus_xp,
    enabled: m.enabled,
  }))
}

async function loadSettings() {
  loading.value = true
  error.value = ''
  try {
    applySettings(await fetchGiftSettings())
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '签到配置加载失败'
  } finally {
    loading.value = false
  }
}

function isNonNegativeInt(value: number): boolean {
  return Number.isInteger(Number(value)) && Number(value) >= 0
}

function validate(): string {
  const values: Array<[string, number, number]> = [
    ['签到奖励', checkinReward.value, 0],
    ['基础经验', baseXP.value, 0],
    ['连续递增经验', streakStepXP.value, 0],
    ['连续奖励上限', streakBonusCap.value, 0],
    ['里程碑奖励上限', milestoneBonusCap.value, 0],
    ['断签惩罚经验', breakPenaltyXP.value, 0],
    ['补签卡价格', makeupCardPrice.value, 1],
  ]
  for (const [label, value, min] of values) {
    if (!isNonNegativeInt(value) || Number(value) < min) {
      return `${label}必须是大于等于 ${min} 的整数`
    }
  }
  const seen = new Set<number>()
  for (const m of milestones.value) {
    if (!Number.isInteger(Number(m.streak_days)) || Number(m.streak_days) < 1) {
      return '里程碑连续天数必须是大于等于 1 的整数'
    }
    if (!isNonNegativeInt(m.bonus_xp)) {
      return '里程碑奖励经验必须是大于等于 0 的整数'
    }
    const days = Number(m.streak_days)
    if (seen.has(days)) {
      return `里程碑连续天数 ${days} 重复`
    }
    seen.add(days)
  }
  return ''
}

async function saveSettings() {
  error.value = ''
  notice.value = ''
  const validationError = validate()
  if (validationError) {
    error.value = validationError
    return
  }
  saving.value = true
  try {
    const settings = await updateGiftSettings({
      checkin_reward: Number(checkinReward.value),
      base_xp: Number(baseXP.value),
      streak_step_xp: Number(streakStepXP.value),
      streak_bonus_cap: Number(streakBonusCap.value),
      milestone_bonus_cap: Number(milestoneBonusCap.value),
      break_penalty_xp: Number(breakPenaltyXP.value),
      makeup_card_price: Number(makeupCardPrice.value),
      milestones: milestones.value.map((m) => ({
        streak_days: Number(m.streak_days),
        bonus_xp: Number(m.bonus_xp),
        enabled: Boolean(m.enabled),
      })),
    })
    applySettings(settings)
    notice.value = '签到配置已保存；新的签到从下一次签到开始按新配置计算。'
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '保存失败'
  } finally {
    saving.value = false
  }
}

function addMilestone() {
  const used = new Set(milestones.value.map((m) => Number(m.streak_days)))
  let days = 7
  while (used.has(days)) days += 1
  milestones.value.push({ streak_days: days, bonus_xp: 10, enabled: true })
}

function removeMilestone(index: number) {
  milestones.value.splice(index, 1)
}

onMounted(loadSettings)
</script>

<template>
  <div>
    <p v-if="error" class="mb-3 rounded-card border border-danger-mc/40 bg-danger-mc/10 px-4 py-2 text-sm text-danger-mc" role="alert">{{ error }}</p>
    <p v-if="notice" class="mb-3 rounded-card border border-grass-500/40 bg-grass-500/10 px-4 py-2 text-sm text-grass-400" role="status">{{ notice }}</p>

    <form class="space-y-4" @submit.prevent="saveSettings">
      <section class="card grid gap-3 p-4 sm:grid-cols-4">
        <h2 class="text-sm font-medium text-text-secondary sm:col-span-4">每日签到奖励</h2>
        <label class="text-xs text-text-muted sm:col-span-2">
          每天签到发放的签到礼物币
          <input v-model.number="checkinReward" :class="inputClass" type="number" min="0" step="1" />
        </label>
        <p class="text-[11px] text-text-muted sm:col-span-4">
          签到币只能兑换标记为「签到币可兑」的礼物；允许时优先扣签到币，不足部分再扣通用礼物币。
        </p>
      </section>

      <section class="card grid gap-3 p-4 sm:grid-cols-4">
        <h2 class="text-sm font-medium text-text-secondary sm:col-span-4">签到经验算法</h2>
        <label class="text-xs text-text-muted">
          基础经验
          <input v-model.number="baseXP" :class="inputClass" type="number" min="0" step="1" />
          <span class="mt-1 block text-[11px]">每次真实签到 / 补签固定获得</span>
        </label>
        <label class="text-xs text-text-muted">
          连续递增经验
          <input v-model.number="streakStepXP" :class="inputClass" type="number" min="0" step="1" />
          <span class="mt-1 block text-[11px]">连续第 N 天额外增加 (N-1) × 该值</span>
        </label>
        <label class="text-xs text-text-muted">
          连续奖励上限
          <input v-model.number="streakBonusCap" :class="inputClass" type="number" min="0" step="1" />
          <span class="mt-1 block text-[11px]">单次签到连续奖励不超过该值</span>
        </label>
        <label class="text-xs text-text-muted">
          断签惩罚经验
          <input v-model.number="breakPenaltyXP" :class="inputClass" type="number" min="0" step="1" />
          <span class="mt-1 block text-[11px]">漏签后首次真实签到扣除，最多扣到 0</span>
        </label>
        <label class="text-xs text-text-muted sm:col-span-2">
          里程碑奖励上限
          <input v-model.number="milestoneBonusCap" :class="inputClass" type="number" min="0" step="1" />
          <span class="mt-1 block text-[11px]">单次签到所有里程碑奖励合计上限</span>
        </label>
        <label class="text-xs text-text-muted sm:col-span-2">
          补签卡价格（签到礼物币）
          <input v-model.number="makeupCardPrice" :class="inputClass" type="number" min="1" step="1" />
          <span class="mt-1 block text-[11px]">只能使用签到礼物币购买；购买数量不设上限</span>
        </label>
      </section>

      <section class="card p-4">
        <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
          <div>
            <h2 class="text-sm font-medium text-text-secondary">连续签到里程碑</h2>
            <p class="text-[11px] text-text-muted">
              真实签到达到对应连续天数时额外发放经验；补签不触发里程碑。里程碑按整表保存。
            </p>
          </div>
          <button
            type="button"
            class="rounded-md border border-border-dark bg-ink-800 px-3 py-1.5 text-xs text-text-secondary transition hover:bg-ink-700"
            @click="addMilestone"
          >
            新增里程碑
          </button>
        </div>
        <div v-if="milestones.length" class="space-y-2">
          <div
            v-for="(m, index) in milestones"
            :key="index"
            class="grid items-end gap-2 rounded-md border border-border-dark bg-ink-900/50 p-3 sm:grid-cols-12"
          >
            <label class="text-xs text-text-muted sm:col-span-4">
              连续天数
              <input v-model.number="m.streak_days" :class="inputClass" type="number" min="1" step="1" />
            </label>
            <label class="text-xs text-text-muted sm:col-span-4">
              奖励经验
              <input v-model.number="m.bonus_xp" :class="inputClass" type="number" min="0" step="1" />
            </label>
            <label class="flex items-center gap-2 text-xs text-text-muted sm:col-span-2">
              <input v-model="m.enabled" type="checkbox" class="h-4 w-4 accent-grass-500" />
              启用
            </label>
            <div class="sm:col-span-2">
              <button
                type="button"
                class="w-full rounded-md border border-danger-mc/40 px-3 py-2 text-xs text-danger-mc transition hover:bg-danger-mc/10"
                @click="removeMilestone(index)"
              >
                删除
              </button>
            </div>
          </div>
        </div>
        <p v-else class="rounded-md border border-dashed border-border-dark p-4 text-center text-xs text-text-muted">
          暂无里程碑，签到不会获得里程碑奖励。
        </p>
      </section>

      <div class="flex items-center gap-3">
        <button
          type="submit"
          class="rounded-md bg-grass-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-grass-500 disabled:opacity-60"
          :disabled="saving || loading"
        >
          {{ saving ? '保存中…' : '保存签到配置' }}
        </button>
        <span class="text-[11px] text-text-muted">经验写入成长账本，历史签到记录不会被改写。</span>
      </div>
    </form>
  </div>
</template>
