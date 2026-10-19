<script setup lang="ts">
// 反馈管理（需求 5.10）
import { onMounted, reactive, ref } from 'vue'
import ListShell from '@/components/admin/list-shell.vue'
import StatusBadge from '@/components/common/StatusBadge.vue'
import Pagination from '@/components/common/Pagination.vue'
import { useUiStore } from '@/stores/ui'
import { listFeedback, listFeedbackReviews, updateFeedbackStatus } from '@/api/admin-system'
import type { Feedback, FeedbackReview } from '@/types/feedback'
import { FEEDBACK_TARGET_KIND_LABELS, FEEDBACK_TYPES, FEEDBACK_TYPE_LABELS, FEEDBACK_STATUSES, FEEDBACK_STATUS_LABELS, type FeedbackType, type FeedbackStatus } from '@/types/common'
import { ApiError } from '@/types/api'
import { formatDateTime } from '@/utils/format'

const ui = useUiStore()
const filters = reactive<{
  status: FeedbackStatus | ''
  type: FeedbackType | ''
  page: number
  pageSize: number
}>({ status: '', type: '', page: 1, pageSize: 15 })
const items = ref<Feedback[]>([])
const total = ref(0)
const totalPages = ref(1)
const loading = ref(false)
const error = ref('')
/** 每条反馈的审核备注输入（审核时随状态一并提交） */
const notes = reactive<Record<string, string>>({})
/** 已展开的审核历史（按反馈 id 缓存） */
const reviews = reactive<Record<string, FeedbackReview[]>>({})
const reviewsLoading = reactive<Record<string, boolean>>({})

async function load() {
  loading.value = true
  error.value = ''
  try {
    const res = await listFeedback({
      status: filters.status || undefined,
      feedback_type: filters.type || undefined,
      page: filters.page,
      page_size: filters.pageSize,
    })
    items.value = res.items
    total.value = res.total
    totalPages.value = res.totalPages || Math.max(1, Math.ceil(res.total / filters.pageSize))
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '加载失败'
  } finally {
    loading.value = false
  }
}

function onPageChange(p: number) {
  filters.page = p
  load()
}

async function setStatus(item: Feedback, status: (typeof FEEDBACK_STATUSES)[number]) {
  try {
    await updateFeedbackStatus({ id: item.id, status, note: notes[item.id]?.trim() || undefined })
    ui.success('审核结果已记录')
    notes[item.id] = ''
    delete reviews[item.id]
    load()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '更新失败')
  }
}

async function toggleHistory(item: Feedback) {
  if (reviews[item.id]) {
    delete reviews[item.id]
    return
  }
  reviewsLoading[item.id] = true
  try {
    reviews[item.id] = await listFeedbackReviews(item.id)
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '加载审核历史失败')
  } finally {
    reviewsLoading[item.id] = false
  }
}

onMounted(load)
</script>

<template>
  <div class="space-y-4">
    <ListShell title="反馈管理" :loading="loading" :error="error" @retry="load">
      <div class="mb-4 flex flex-wrap gap-3">
        <select
          v-model="filters.type"
          class="h-9 rounded-md border border-border-dark bg-ink-800 px-2 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
          @change="filters.page = 1; load()"
        >
          <option value="">全部类型</option>
          <option v-for="t in FEEDBACK_TYPES" :key="t" :value="t">{{ FEEDBACK_TYPE_LABELS[t] }}</option>
        </select>
        <select
          v-model="filters.status"
          class="h-9 rounded-md border border-border-dark bg-ink-800 px-2 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
          @change="filters.page = 1; load()"
        >
          <option value="">全部状态</option>
          <option v-for="s in FEEDBACK_STATUSES" :key="s" :value="s">{{ FEEDBACK_STATUS_LABELS[s] }}</option>
        </select>
      </div>

      <div v-if="items.length" class="space-y-3">
        <article v-for="item in items" :key="item.id" class="card p-4">
          <div class="mb-2 flex flex-wrap items-center gap-2">
            <StatusBadge :status="item.status" />
            <span class="rounded border border-border-light bg-ink-700 px-1.5 py-0.5 text-xs text-text-secondary">
              {{ FEEDBACK_TYPE_LABELS[item.feedback_type] }}
            </span>
            <span class="text-xs text-text-muted">{{ formatDateTime(item.created_at) }}</span>
          </div>
          <h3 class="mb-1 font-medium text-text-primary">{{ item.title }}</h3>
          <p class="mb-3 whitespace-pre-wrap text-sm text-text-secondary">{{ item.content }}</p>
          <div class="mb-3 flex flex-wrap gap-4 text-xs text-text-muted">
            <span v-if="item.target_kind !== 'other' || item.target_ref">
              关联资料：{{ FEEDBACK_TARGET_KIND_LABELS[item.target_kind] }}<template v-if="item.target_ref"> · {{ item.target_ref }}</template>
            </span>
            <span v-if="item.related_url">关联：<a :href="item.related_url" target="_blank" rel="noopener noreferrer nofollow" class="text-grass-400 hover:underline">{{ item.related_url }}</a></span>
            <span v-if="item.contact">联系：{{ item.contact }}</span>
            <span v-if="item.license_version">授权条款：{{ item.license_version }}</span>
          </div>
          <p v-if="item.reviewed_at" class="mb-3 text-xs text-text-muted">
            最近审核：{{ item.reviewer || '（未记录审核人）' }} · {{ formatDateTime(item.reviewed_at) }}
            <template v-if="item.review_note"> · {{ item.review_note }}</template>
          </p>
          <div class="mb-3 flex flex-wrap items-center gap-2">
            <input
              v-model="notes[item.id]"
              type="text"
              placeholder="审核备注（可选，随状态一并记录）"
              maxlength="500"
              class="h-8 min-w-[16rem] flex-1 rounded-md border border-border-dark bg-ink-900 px-2 text-xs text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
            />
            <div class="flex gap-2">
              <button
                v-for="s in FEEDBACK_STATUSES"
                :key="s"
                type="button"
                class="rounded border px-2 py-1 text-xs transition"
                :class="
                  item.status === s
                    ? 'border-grass-500 bg-grass-500/15 text-grass-300'
                    : 'border-border-light text-text-secondary hover:bg-ink-700'
                "
                @click="setStatus(item, s)"
              >
                {{ FEEDBACK_STATUS_LABELS[s] }}
              </button>
            </div>
            <button
              type="button"
              class="rounded border border-border-light px-2 py-1 text-xs text-text-secondary transition hover:bg-ink-700"
              @click="toggleHistory(item)"
            >
              {{ reviews[item.id] ? '收起审核历史' : '审核历史' }}
            </button>
          </div>
          <div v-if="reviewsLoading[item.id]" class="text-xs text-text-muted">加载中…</div>
          <ol v-else-if="reviews[item.id]" class="space-y-1 border-l border-border-light pl-3 text-xs text-text-secondary">
            <li v-for="rv in reviews[item.id]" :key="rv.id">
              <span class="text-text-muted">{{ formatDateTime(rv.created_at) }}</span>
              · {{ FEEDBACK_STATUS_LABELS[rv.from_status] }} → {{ FEEDBACK_STATUS_LABELS[rv.to_status] }}
              · {{ rv.reviewer || '（未记录审核人）' }}
              <template v-if="rv.note"> · {{ rv.note }}</template>
            </li>
            <li v-if="!reviews[item.id].length" class="text-text-muted">暂无审核记录。</li>
          </ol>
        </article>
      </div>
      <div v-else-if="!loading" class="card p-10 text-center text-sm text-text-muted">暂无反馈。</div>
    </ListShell>

    <Pagination
      :page="filters.page"
      :page-size="filters.pageSize"
      :total="total"
      :total-pages="totalPages"
      @change="onPageChange"
    />
  </div>
</template>