<script setup lang="ts">
// Mod 与整合包内容审核（TODO §16，admin 专属）
// 两个 tab：Mod / 整合包，均默认只看待审核项，逐条「通过 / 驳回」并留备注。
// 仅 admin 角色可见（侧边栏过滤 + 后端 403 兜底）。
import { onMounted, reactive, ref } from 'vue'
import ListShell from '@/components/admin/list-shell.vue'
import StatusBadge from '@/components/common/StatusBadge.vue'
import Pagination from '@/components/common/Pagination.vue'
import { useUiStore } from '@/stores/ui'
import { listModpacksAdmin, listModsAdmin, reviewMod, reviewModpack } from '@/api/admin-mods'
import { ApiError } from '@/types/api'
import {
  MOD_CATEGORY_LABELS,
  MOD_LOADERS,
  MOD_LOADER_LABELS,
  isExternalUrl,
  type Mod,
  type ModLoader,
  type Modpack,
  type ModReviewStatus,
} from '@/types/mods'
import { formatDateTime, joinList } from '@/utils/format'

type Tab = 'mods' | 'modpacks'

const ui = useUiStore()

// 审核状态中文标签：审核语义与公开侧「已发布」不同，这里统一为「已通过」。
const REVIEW_STATUS_LABELS: Record<ModReviewStatus, string> = {
  pending: '待审核',
  published: '已通过',
  rejected: '已驳回',
}

// 复用 StatusBadge 调色板：文字由 label 覆盖为审核语义，仅借用颜色。
const REVIEW_BADGE_STATUS: Record<ModReviewStatus, string> = {
  pending: 'processing',
  published: 'published',
  rejected: 'removed',
}

const activeTab = ref<Tab>('mods')
const filters = reactive<{
  reviewStatus: ModReviewStatus | ''
  loader: ModLoader | ''
  page: number
  pageSize: number
}>({
  reviewStatus: 'pending',
  loader: '',
  page: 1,
  pageSize: 20,
})

const mods = ref<Mod[]>([])
const modpacks = ref<Modpack[]>([])
const total = ref(0)
const totalPages = ref(1)
const loading = ref(false)
const error = ref('')
/** 每条记录的审核备注草稿（审核时随状态一并提交） */
const notes = reactive<Record<string, string>>({})
const reviewingId = ref('')

async function load() {
  loading.value = true
  error.value = ''
  try {
    const params = {
      review_status: filters.reviewStatus || undefined,
      loader: filters.loader || undefined,
      page: filters.page,
      page_size: filters.pageSize,
    }
    if (activeTab.value === 'mods') {
      const res = await listModsAdmin(params)
      mods.value = res.items
      total.value = res.total
      totalPages.value = res.totalPages || Math.max(1, Math.ceil(res.total / filters.pageSize))
    } else {
      const res = await listModpacksAdmin(params)
      modpacks.value = res.items
      total.value = res.total
      totalPages.value = res.totalPages || Math.max(1, Math.ceil(res.total / filters.pageSize))
    }
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '加载失败'
  } finally {
    loading.value = false
  }
}

async function review(id: string, status: ModReviewStatus, name: string) {
  reviewingId.value = id
  try {
    if (activeTab.value === 'mods') {
      await reviewMod(id, status, notes[id] ?? '')
    } else {
      await reviewModpack(id, status, notes[id] ?? '')
    }
    ui.success(`已${REVIEW_STATUS_LABELS[status]}：${name}`)
    notes[id] = ''
    await load()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '审核失败')
  } finally {
    reviewingId.value = ''
  }
}

function switchTab(tab: Tab) {
  if (activeTab.value === tab) return
  activeTab.value = tab
  filters.page = 1
  load()
}

function onFilter() {
  filters.page = 1
  load()
}

function onPageChange(p: number) {
  filters.page = p
  load()
}

onMounted(load)
</script>

<template>
  <div class="space-y-6">
    <header>
      <h1 class="text-xl font-bold text-text-primary md:text-2xl">Mod 审核</h1>
      <p class="mt-1 text-sm text-text-secondary">
        审核 Mod 与整合包投稿：默认展示待审核项，通过后才会对公开接口可见；驳回需说明原因。审核人取自登录会话，
        审核状态与备注完整留痕并写入审计日志。
      </p>
    </header>

    <div class="flex gap-2">
      <button
        type="button"
        class="rounded-md border px-4 py-2 text-sm transition"
        :class="
          activeTab === 'mods'
            ? 'border-grass-500 bg-grass-500/15 text-grass-300'
            : 'border-border-dark text-text-secondary hover:bg-ink-700'
        "
        @click="switchTab('mods')"
      >
        Mod
      </button>
      <button
        type="button"
        class="rounded-md border px-4 py-2 text-sm transition"
        :class="
          activeTab === 'modpacks'
            ? 'border-grass-500 bg-grass-500/15 text-grass-300'
            : 'border-border-dark text-text-secondary hover:bg-ink-700'
        "
        @click="switchTab('modpacks')"
      >
        整合包
      </button>
    </div>

    <div class="card p-4">
      <div class="flex flex-wrap items-end gap-3">
        <select
          v-model="filters.reviewStatus"
          class="h-9 rounded-md border border-border-dark bg-ink-800 px-2 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
          @change="onFilter"
        >
          <option value="">全部审核状态</option>
          <option value="pending">{{ REVIEW_STATUS_LABELS.pending }}</option>
          <option value="published">{{ REVIEW_STATUS_LABELS.published }}</option>
          <option value="rejected">{{ REVIEW_STATUS_LABELS.rejected }}</option>
        </select>
        <select
          v-model="filters.loader"
          class="h-9 rounded-md border border-border-dark bg-ink-800 px-2 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
          @change="onFilter"
        >
          <option value="">全部加载器</option>
          <option v-for="l in MOD_LOADERS" :key="l" :value="l">{{ MOD_LOADER_LABELS[l] }}</option>
        </select>
      </div>
    </div>

    <ListShell title="审核列表" :loading="loading" :error="error" @retry="load">
      <!-- Mod tab -->
      <div v-if="activeTab === 'mods'" class="card overflow-hidden">
        <div class="overflow-x-auto">
          <table class="w-full text-left text-sm">
            <thead class="border-b border-border-dark bg-ink-900 text-xs text-text-muted">
              <tr>
                <th scope="col" class="px-4 py-3 font-medium">Mod</th>
                <th scope="col" class="px-4 py-3 font-medium">加载器 / 类别</th>
                <th scope="col" class="px-4 py-3 font-medium">适用版本</th>
                <th scope="col" class="px-4 py-3 font-medium">来源 / 许可</th>
                <th scope="col" class="px-4 py-3 font-medium">审核状态</th>
                <th scope="col" class="px-4 py-3 font-medium">审核操作</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border-dark">
              <tr v-for="item in mods" :key="item.id" class="align-top hover:bg-ink-800/60">
                <td class="px-4 py-3">
                  <div class="font-medium text-text-primary">{{ item.name_zh || '（未命名）' }}</div>
                  <div class="mt-0.5 font-mono text-xs text-text-muted">{{ item.slug }}</div>
                </td>
                <td class="px-4 py-3 text-text-secondary">
                  {{ joinList(item.loaders.map((l) => MOD_LOADER_LABELS[l])) }}
                  <div class="mt-0.5 text-xs text-text-muted">
                    {{ joinList(item.categories.map((c) => MOD_CATEGORY_LABELS[c])) }}
                  </div>
                </td>
                <td class="px-4 py-3 text-xs text-text-secondary">{{ joinList(item.mc_versions) }}</td>
                <td class="px-4 py-3">
                  <a
                    v-if="isExternalUrl(item.source_url)"
                    :href="item.source_url || ''"
                    target="_blank"
                    rel="noopener noreferrer nofollow"
                    class="break-all font-mono text-xs text-grass-300 hover:underline"
                  >
                    {{ item.source_url }}
                  </a>
                  <span v-else class="text-xs text-text-muted">—</span>
                  <div class="mt-0.5 text-xs text-text-muted">许可：{{ item.license || '未标注' }}</div>
                </td>
                <td class="px-4 py-3">
                  <StatusBadge
                    :status="REVIEW_BADGE_STATUS[item.review_status]"
                    :label="REVIEW_STATUS_LABELS[item.review_status]"
                  />
                  <div v-if="item.reviewed_at" class="mt-1 text-xs text-text-muted">
                    最近审核：{{ item.reviewer || '（未记录审核人）' }} · {{ formatDateTime(item.reviewed_at) }}
                    <template v-if="item.review_note"> · {{ item.review_note }}</template>
                  </div>
                </td>
                <td class="px-4 py-3">
                  <input
                    v-model="notes[item.id]"
                    type="text"
                    placeholder="审核备注（可选）"
                    class="mb-2 w-full rounded-md border border-border-dark bg-ink-900 px-2 py-1 text-xs text-text-primary placeholder:text-text-muted"
                  />
                  <div class="flex flex-wrap gap-2">
                    <button
                      type="button"
                      class="rounded border border-grass-500/50 bg-grass-500/10 px-2 py-1 text-xs text-grass-300 transition hover:bg-grass-500/20 disabled:opacity-50"
                      :disabled="reviewingId === item.id"
                      @click="review(item.id, 'published', item.name_zh || item.slug)"
                    >
                      通过
                    </button>
                    <button
                      type="button"
                      class="rounded border border-danger-mc/50 bg-danger-mc/10 px-2 py-1 text-xs text-danger-mc transition hover:bg-danger-mc/20 disabled:opacity-50"
                      :disabled="reviewingId === item.id"
                      @click="review(item.id, 'rejected', item.name_zh || item.slug)"
                    >
                      驳回
                    </button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <p v-if="!mods.length" class="px-4 py-8 text-center text-sm text-text-muted">
          暂无 Mod——当前筛选下没有待审核的 Mod。
        </p>
      </div>

      <!-- 整合包 tab -->
      <div v-else class="card overflow-hidden">
        <div class="overflow-x-auto">
          <table class="w-full text-left text-sm">
            <thead class="border-b border-border-dark bg-ink-900 text-xs text-text-muted">
              <tr>
                <th scope="col" class="px-4 py-3 font-medium">整合包</th>
                <th scope="col" class="px-4 py-3 font-medium">MC 版本</th>
                <th scope="col" class="px-4 py-3 font-medium">加载器 / 作者</th>
                <th scope="col" class="px-4 py-3 font-medium">来源 / 许可</th>
                <th scope="col" class="px-4 py-3 font-medium">审核状态</th>
                <th scope="col" class="px-4 py-3 font-medium">审核操作</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border-dark">
              <tr v-for="item in modpacks" :key="item.id" class="align-top hover:bg-ink-800/60">
                <td class="px-4 py-3">
                  <div class="font-medium text-text-primary">{{ item.name_zh || '（未命名）' }}</div>
                  <div class="mt-0.5 font-mono text-xs text-text-muted">{{ item.slug }}</div>
                </td>
                <td class="px-4 py-3 text-xs text-text-secondary">{{ item.mc_version || '—' }}</td>
                <td class="px-4 py-3 text-text-secondary">
                  {{ item.loader ? MOD_LOADER_LABELS[item.loader] : '—' }}
                  <div class="mt-0.5 text-xs text-text-muted">{{ item.author || '—' }}</div>
                </td>
                <td class="px-4 py-3">
                  <a
                    v-if="isExternalUrl(item.source_url)"
                    :href="item.source_url || ''"
                    target="_blank"
                    rel="noopener noreferrer nofollow"
                    class="break-all font-mono text-xs text-grass-300 hover:underline"
                  >
                    {{ item.source_url }}
                  </a>
                  <span v-else class="text-xs text-text-muted">—</span>
                  <div class="mt-0.5 text-xs text-text-muted">许可：{{ item.license || '未标注' }}</div>
                </td>
                <td class="px-4 py-3">
                  <StatusBadge
                    :status="REVIEW_BADGE_STATUS[item.review_status]"
                    :label="REVIEW_STATUS_LABELS[item.review_status]"
                  />
                  <div v-if="item.reviewed_at" class="mt-1 text-xs text-text-muted">
                    最近审核：{{ item.reviewer || '（未记录审核人）' }} · {{ formatDateTime(item.reviewed_at) }}
                    <template v-if="item.review_note"> · {{ item.review_note }}</template>
                  </div>
                </td>
                <td class="px-4 py-3">
                  <input
                    v-model="notes[item.id]"
                    type="text"
                    placeholder="审核备注（可选）"
                    class="mb-2 w-full rounded-md border border-border-dark bg-ink-900 px-2 py-1 text-xs text-text-primary placeholder:text-text-muted"
                  />
                  <div class="flex flex-wrap gap-2">
                    <button
                      type="button"
                      class="rounded border border-grass-500/50 bg-grass-500/10 px-2 py-1 text-xs text-grass-300 transition hover:bg-grass-500/20 disabled:opacity-50"
                      :disabled="reviewingId === item.id"
                      @click="review(item.id, 'published', item.name_zh || item.slug)"
                    >
                      通过
                    </button>
                    <button
                      type="button"
                      class="rounded border border-danger-mc/50 bg-danger-mc/10 px-2 py-1 text-xs text-danger-mc transition hover:bg-danger-mc/20 disabled:opacity-50"
                      :disabled="reviewingId === item.id"
                      @click="review(item.id, 'rejected', item.name_zh || item.slug)"
                    >
                      驳回
                    </button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <p v-if="!modpacks.length" class="px-4 py-8 text-center text-sm text-text-muted">
          暂无整合包——当前筛选下没有待审核的整合包。
        </p>
      </div>

      <Pagination
        class="mt-4"
        :page="filters.page"
        :page-size="filters.pageSize"
        :total="total"
        :total-pages="totalPages"
        @change="onPageChange"
      />
    </ListShell>
  </div>
</template>
