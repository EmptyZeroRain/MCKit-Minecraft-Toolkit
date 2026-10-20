<script setup lang="ts">
// 签到设置面板：配置每日签到发放的签到礼物币。
import { onMounted, ref } from 'vue'
import { ApiError } from '@/types/api'
import { fetchGiftSettings, updateGiftSettings } from '@/api/gift'

const inputClass =
  'w-full rounded-md border border-border-dark bg-ink-900 px-3 py-2 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none'

const loading = ref(true)
const saving = ref(false)
const error = ref('')
const notice = ref('')
const checkinReward = ref(1)

async function loadSettings() {
  loading.value = true
  error.value = ''
  try {
    const settings = await fetchGiftSettings()
    checkinReward.value = settings.checkin_reward
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '签到配置加载失败'
  } finally {
    loading.value = false
  }
}

async function saveSettings() {
  error.value = ''
  notice.value = ''
  if (!Number.isInteger(Number(checkinReward.value)) || Number(checkinReward.value) < 0) {
    error.value = '签到奖励必须是大于等于 0 的整数'
    return
  }
  saving.value = true
  try {
    const settings = await updateGiftSettings(Number(checkinReward.value))
    checkinReward.value = settings.checkin_reward
    notice.value = `签到奖励已更新为 ${settings.checkin_reward} 签到礼物币/天。`
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '保存失败'
  } finally {
    saving.value = false
  }
}

onMounted(loadSettings)
</script>

<template>
  <div>
    <p v-if="error" class="mb-3 rounded-card border border-danger-mc/40 bg-danger-mc/10 px-4 py-2 text-sm text-danger-mc" role="alert">{{ error }}</p>
    <p v-if="notice" class="mb-3 rounded-card border border-grass-500/40 bg-grass-500/10 px-4 py-2 text-sm text-grass-400" role="status">{{ notice }}</p>

    <form class="card grid gap-3 p-4 sm:grid-cols-4" @submit.prevent="saveSettings">
      <h2 class="text-sm font-medium text-text-secondary sm:col-span-4">每日签到奖励</h2>
      <label class="text-xs text-text-muted sm:col-span-2">
        每天签到发放的签到礼物币
        <input v-model.number="checkinReward" :class="inputClass" type="number" min="0" step="1" />
      </label>
      <div class="flex items-end sm:col-span-2">
        <button
          type="submit"
          class="rounded-md bg-grass-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-grass-500 disabled:opacity-60"
          :disabled="saving || loading"
        >
          保存签到奖励
        </button>
      </div>
      <p class="text-[11px] text-text-muted sm:col-span-4">
        签到币只能兑换标记为「签到币可兑」的礼物；允许时优先扣签到币，不足部分再扣通用礼物币。
      </p>
    </form>
  </div>
</template>
