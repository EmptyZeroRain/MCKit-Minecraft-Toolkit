<script setup lang="ts">
// 等级阈值管理（jingyan.md §5 / §10.4 相关）。
//
// 等级仅作展示，不构成任何权限。阈值必须全局唯一、按等级严格递增，Lv.1 固定为 0。
// 修改阈值后可「重算等级」修正历史用户展示等级（不改变经验与权限）。
import { onMounted, reactive, ref } from 'vue'
import AdminTable from '@/components/admin/AdminTable.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import { ApiError } from '@/types/api'
import type { AdminTableColumn } from '@/types/admin'
import { fetchGrowthLevels, recalculateGrowthLevels, updateGrowthLevel } from '@/api/growth'
import type { GrowthLevel } from '@/types/growth'

const loading = ref(true)
const error = ref('')
const notice = ref('')
const levels = ref<GrowthLevel[]>([])
const busyLevel = ref<number | null>(null)

const recalcOpen = ref(false)
const recalculating = ref(false)

const inputClass =
  'w-full rounded-md border border-border-dark bg-ink-900 px-2 py-1 text-sm text-text-primary focus:border-grass-500 focus:outline-none'

/** 行内编辑草稿：level -> { threshold_xp, title, enabled } */
const drafts = reactive<Record<number, { threshold_xp: number; title: string; enabled: boolean }>>({})

const columns: AdminTableColumn[] = [
  { key: 'level', label: '等级', width: '80px' },
  { key: 'threshold_xp', label: '所需经验', width: '160px' },
  { key: 'title', label: '称号' },
  { key: 'enabled', label: '启用', width: '80px' },
  { key: 'action', label: '操作', align: 'right', width: '100px' },
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

async function save(level: GrowthLevel) {
  const draft = drafts[level.level]
  if (!draft) return
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
    <div class="mb-4 flex items-center justify-between">
      <p class="text-xs text-text-muted">
        阈值必须唯一且严格递增，Lv.1 固定为 0 经验。等级仅展示，不影响权限。
      </p>
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
        <button
          type="button"
          class="rounded-md bg-grass-600 px-3 py-1 text-xs font-medium text-white transition hover:bg-grass-500 disabled:opacity-50"
          :disabled="busyLevel === row.level"
          @click="save(row)"
        >
          保存
        </button>
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
