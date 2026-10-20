<script setup lang="ts">
// 用户评论（需 content.community 权限）：跨帖子查看全部留言，支持搜索与审核。
//
// 留言在公开侧仅展示 published；后台可查看隐藏内容并恢复 / 删除。
// 删除为物理删除，会同步减少帖子留言计数，操作写审计日志。
import { onMounted, reactive, ref } from 'vue'
import ListShell from '@/components/admin/list-shell.vue'
import StatusBadge from '@/components/common/StatusBadge.vue'
import Pagination from '@/components/common/Pagination.vue'
import { useUiStore } from '@/stores/ui'
import { ApiError } from '@/types/api'
import {
  adminDeleteCommunityComment,
  adminListAllCommunityComments,
  adminSetCommunityCommentStatus,
} from '@/api/admin-community'
import { communityAuthorName, type CommunityComment } from '@/types/community'
import { formatDateTime } from '@/utils/format'

const ui = useUiStore()

const filters = reactive<{
  status: '' | 'published' | 'hidden'
  q: string
  page: number
  pageSize: number
}>({ status: '', q: '', page: 1, pageSize: 20 })

const items = ref<CommunityComment[]>([])
const total = ref(0)
const totalPages = ref(1)
const loading = ref(false)
const error = ref('')

async function load() {
  loading.value = true
  error.value = ''
  try {
    const res = await adminListAllCommunityComments({
      status: filters.status || undefined,
      q: filters.q.trim() || undefined,
      page: filters.page,
      page_size: filters.pageSize,
    })
    items.value = res.items
    total.value = res.total
    totalPages.value = res.totalPages || Math.max(1, Math.ceil(res.total / filters.pageSize))
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '评论加载失败'
  } finally {
    loading.value = false
  }
}

function search() {
  filters.page = 1
  void load()
}

function onPageChange(page: number) {
  filters.page = page
  void load()
}

async function toggleStatus(item: CommunityComment) {
  const next = item.status === 'hidden' ? 'published' : 'hidden'
  try {
    await adminSetCommunityCommentStatus(item.id, next)
    item.status = next
    ui.success(next === 'hidden' ? '评论已隐藏' : '评论已恢复')
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '操作失败')
  }
}

async function remove(item: CommunityComment) {
  if (!window.confirm('确定删除该用户评论？删除后不可恢复。')) return
  try {
    await adminDeleteCommunityComment(item.id)
    ui.success('评论已删除')
    await load()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '删除失败')
  }
}

onMounted(load)
</script>

<template>
  <div class="space-y-4">
    <ListShell title="用户评论" :loading="loading" :error="error" @retry="load">
      <template #toolbar>
        <span class="text-xs text-text-muted">共 {{ total }} 条评论</span>
      </template>

      <form class="mb-4 flex flex-wrap gap-3" @submit.prevent="search">
        <input
          v-model="filters.q"
          type="search"
          placeholder="搜索评论、帖子标题或作者"
          class="h-9 w-72 rounded-md border border-border-dark bg-ink-800 px-3 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
        />
        <select
          v-model="filters.status"
          class="h-9 rounded-md border border-border-dark bg-ink-800 px-2 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
          @change="search"
        >
          <option value="">全部状态</option>
          <option value="published">已发布</option>
          <option value="hidden">已隐藏</option>
        </select>
        <button
          type="submit"
          class="h-9 rounded-md border border-border-light bg-ink-700 px-4 text-sm text-text-secondary transition hover:bg-ink-600"
        >
          查询
        </button>
      </form>

      <div v-if="items.length" class="space-y-3">
        <article v-for="item in items" :key="item.id" class="card p-4">
          <div class="mb-2 flex flex-wrap items-center gap-2 text-xs">
            <StatusBadge :status="item.status" />
            <span class="text-text-muted">
              {{ communityAuthorName(item.author) }} ·
              <span class="font-mono">{{ item.author.user_no }}</span>
            </span>
            <span class="text-text-muted">{{ formatDateTime(item.created_at) }}</span>
          </div>

          <p class="mb-3 whitespace-pre-wrap text-sm text-text-primary">{{ item.body }}</p>

          <div class="flex flex-wrap items-center gap-3 border-t border-border-dark pt-2.5 text-xs">
            <span class="min-w-0 flex-1 truncate text-text-muted">
              所属帖子：{{ item.post_title || item.post_id }}
            </span>
            <button
              type="button"
              class="rounded border border-border-light px-2 py-1 text-text-secondary transition hover:bg-ink-700"
              @click="toggleStatus(item)"
            >
              {{ item.status === 'hidden' ? '恢复' : '隐藏' }}
            </button>
            <button
              type="button"
              class="rounded border border-red-500/40 px-2 py-1 text-red-300 transition hover:bg-red-500/10"
              @click="remove(item)"
            >
              删除
            </button>
          </div>
        </article>
      </div>

      <p v-else class="text-sm text-text-muted">暂无符合条件的用户评论。</p>

      <Pagination
        :page="filters.page"
        :page-size="filters.pageSize"
        :total="total"
        :total-pages="totalPages"
        @change="onPageChange"
      />
    </ListShell>
  </div>
</template>
