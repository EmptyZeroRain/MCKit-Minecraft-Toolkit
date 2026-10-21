<script setup lang="ts">
// 等级称号规则（jingyan.md §5 / §10.4 相关）。
//
// 称号只做展示，不参与权限：
//   * 区间称号：min_level < max_level，启用区间之间不能重叠；
//   * 指定等级称号：min_level === max_level，优先级高于区间。
// 称号规则列表默认每页 5 条，避免规则变多后把新增表单挤出视口。
import { computed, onMounted, reactive, ref, watch } from 'vue'
import AdminTable from '@/components/admin/AdminTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import { ApiError } from '@/types/api'
import type { AdminTableColumn } from '@/types/admin'
import {
  createGrowthLevelTitle,
  deleteGrowthLevelTitle,
  fetchGrowthLevelConfig,
  fetchGrowthLevelTitles,
  updateGrowthLevelTitle,
} from '@/api/growth'
import type {
  GrowthLevelConfig,
  GrowthLevelTitle,
  GrowthLevelTitleInput,
} from '@/types/growth'

const PAGE_SIZE = 5

const inputClass =
  'w-full rounded-md border border-border-dark bg-ink-900 px-2 py-1 text-sm text-text-primary focus:border-grass-500 focus:outline-none'

const loading = ref(true)
const error = ref('')
const notice = ref('')

const config = ref<GrowthLevelConfig>({ max_level: 0, step_xp: 100 })
const titles = ref<GrowthLevelTitle[]>([])
const drafts = reactive<Record<string, GrowthLevelTitleInput>>({})
const busyTitleID = ref('')

const rangeForm = reactive({ min_level: 1, max_level: 10, title: '', enabled: true })
const exactForm = reactive({ level: 100, title: '', enabled: true })
const createError = ref('')
const creating = ref(false)

const page = ref(1)

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

const totalPages = computed(() => Math.ceil(titles.value.length / PAGE_SIZE))
const pagedTitles = computed(() => {
  const start = (page.value - 1) * PAGE_SIZE
  return titles.value.slice(start, start + PAGE_SIZE)
})

/** 0 表示不设上限；HTML 输入仍需要一个技术上的最大整数。 */
const titleMaxLevel = computed(() =>
  config.value.max_level > 0 ? config.value.max_level : 2147483647,
)

/** 删除或重新加载后页数可能减少，自动回落到最后一页。 */
watch(totalPages, (total) => {
  if (total > 0 && page.value > total) page.value = total
  if (total === 0) page.value = 1
})

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
    titles.value = items
    fillDrafts(items)
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '称号规则加载失败'
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
  if (input.max_level > 2147483647) {
    return '等级不能超过 2147483647'
  }
  if (input.enabled && config.value.max_level > 0 && input.max_level > config.value.max_level) {
    return `启用的称号范围不能超过当前等级上限 Lv.${config.value.max_level}；可先停用再保存`
  }
  if (!input.title.trim()) return '请填写称号'
  if (input.title.trim().length > 100) return '称号不能超过 100 个字符'
  return ''
}

async function reloadTitles() {
  titles.value = await fetchGrowthLevelTitles()
  fillDrafts(titles.value)
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
    await reloadTitles()
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
    await reloadTitles()
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
    await reloadTitles()
  } catch (e) {
    const detail = e instanceof ApiError ? e.details?.[0]?.message : ''
    createError.value = detail || (e instanceof ApiError ? e.message : '保存称号规则失败')
  } finally {
    busyTitleID.value = ''
  }
}

function titleLabel(row: GrowthLevelTitle): string {
  return row.min_level === row.max_level
    ? `Lv.${row.min_level}`
    : `Lv.${row.min_level}-${row.max_level}`
}

async function removeTitle(row: GrowthLevelTitle) {
  if (!window.confirm(`确定删除称号规则 ${titleLabel(row)}「${row.title}」？删除后该等级会回退为默认 Lv.N。`)) {
    return
  }
  busyTitleID.value = row.id
  try {
    await deleteGrowthLevelTitle(row.id)
    notice.value = `已删除称号规则 ${titleLabel(row)}。`
    await reloadTitles()
  } catch (e) {
    createError.value = e instanceof ApiError ? e.message : '删除称号规则失败'
  } finally {
    busyTitleID.value = ''
  }
}
</script>

<template>
  <div>
    <div class="mb-4">
      <h2 class="text-sm font-medium text-text-secondary">等级称号规则</h2>
      <p class="mt-1 text-xs text-text-muted">
        指定等级称号优先于区间称号；启用区间之间不能重叠。
        称号仅展示，不影响权限；等级曲线与单用户等级调整请见「等级」Tab。
      </p>
    </div>

    <p v-if="error" class="mb-3 rounded-card border border-danger-mc/40 bg-danger-mc/10 px-4 py-2 text-sm text-danger-mc" role="alert">{{ error }}</p>
    <p v-if="notice" class="mb-3 rounded-card border border-grass-500/40 bg-grass-500/10 px-4 py-2 text-sm text-grass-400" role="status">{{ notice }}</p>

    <div class="card mb-5 p-4">
      <div class="grid gap-4 lg:grid-cols-2">
        <form data-test="range-title-form" class="grid gap-2 rounded-md border border-border-dark p-3 sm:grid-cols-2" @submit.prevent="submitRangeTitle">
          <h3 class="text-sm font-medium text-text-secondary sm:col-span-2">新增区间称号</h3>
          <p class="text-xs text-text-muted sm:col-span-2">例如 Lv.1-10「初来乍到」；启用区间之间不能重叠。</p>
          <label class="text-xs text-text-muted">
            最小等级
            <input v-model.number="rangeForm.min_level" :class="inputClass" type="number" min="0" :max="titleMaxLevel" step="1" />
          </label>
          <label class="text-xs text-text-muted">
            最大等级
            <input v-model.number="rangeForm.max_level" :class="inputClass" type="number" min="0" :max="titleMaxLevel" step="1" />
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
            <input v-model.number="exactForm.level" :class="inputClass" type="number" min="0" :max="titleMaxLevel" step="1" />
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
    <AdminTable :columns="titleColumns" :rows="pagedTitles" :loading="loading" row-key="id" empty-text="暂无称号规则，所有等级将展示默认 Lv.N">
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

    <Pagination
      v-if="!loading && !error"
      class="mt-3"
      :page="page"
      :page-size="PAGE_SIZE"
      :total="titles.length"
      :total-pages="totalPages"
      @change="page = $event"
    />
  </div>
</template>
