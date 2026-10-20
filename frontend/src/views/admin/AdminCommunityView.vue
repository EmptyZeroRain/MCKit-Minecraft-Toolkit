<script setup lang="ts">
// 社区管理（需 content.community 权限）：帖子隐藏 / 恢复 / 删除 + 留言清理。
import { onMounted, reactive, ref } from 'vue'
import ListShell from '@/components/admin/list-shell.vue'
import StatusBadge from '@/components/common/StatusBadge.vue'
import Pagination from '@/components/common/Pagination.vue'
import { useUiStore } from '@/stores/ui'
import { ApiError } from '@/types/api'
import {
  adminDeleteCommunityComment,
  adminDeleteCommunityPost,
  adminListCommunityComments,
  adminListCommunityPosts,
  adminSetCommunityCommentStatus,
  adminSetCommunityPostStatus,
} from '@/api/admin-community'
import {
  COMMUNITY_CATEGORIES,
  COMMUNITY_CATEGORY_LABELS,
  COMMUNITY_STATUS_LABELS,
  communityAuthorName,
  type CommunityCategory,
  type CommunityComment,
  type CommunityPost,
} from '@/types/community'
import { formatDateTime } from '@/utils/format'

const ui = useUiStore()

const filters = reactive<{
  status: '' | 'published' | 'hidden'
  category: CommunityCategory | ''
  q: string
  page: number
  pageSize: number
}>({ status: '', category: '', q: '', page: 1, pageSize: 15 })

const items = ref<CommunityPost[]>([])
const total = ref(0)
const totalPages = ref(1)
const loading = ref(false)
const error = ref('')

/** 展开的帖子留言（按帖子 id 缓存） */
const comments = reactive<Record<string, CommunityComment[]>>({})
const commentsLoading = reactive<Record<string, boolean>>({})
const commentTotals = reactive<Record<string, number>>({})

async function load() {
  loading.value = true
  error.value = ''
  try {
    const res = await adminListCommunityPosts({
      status: filters.status || undefined,
      category: filters.category || undefined,
      q: filters.q.trim() || undefined,
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

async function setStatus(item: CommunityPost, status: 'published' | 'hidden') {
  try {
    await adminSetCommunityPostStatus(item.id, status)
    ui.success(status === 'hidden' ? '帖子已隐藏' : '帖子已恢复')
    load()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '操作失败')
  }
}

async function removePost(item: CommunityPost) {
  if (!window.confirm(`确定删除帖子「${item.title}」？留言与点赞将一并删除。`)) return
  try {
    await adminDeleteCommunityPost(item.id)
    ui.success('帖子已删除')
    delete comments[item.id]
    load()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '删除失败')
  }
}

async function loadComments(postID: string) {
  commentsLoading[postID] = true
  try {
    const res = await adminListCommunityComments({ post_id: postID, page: 1, page_size: 50 })
    comments[postID] = res.items
    commentTotals[postID] = res.total
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '留言加载失败')
  } finally {
    commentsLoading[postID] = false
  }
}

async function toggleComments(item: CommunityPost) {
  if (comments[item.id]) {
    delete comments[item.id]
    return
  }
  await loadComments(item.id)
}

async function setCommentStatus(comment: CommunityComment) {
  const next = comment.status === 'hidden' ? 'published' : 'hidden'
  try {
    await adminSetCommunityCommentStatus(comment.id, next)
    comment.status = next
    ui.success(next === 'hidden' ? '留言已隐藏' : '留言已恢复')
    // 留言可见性会影响帖子的留言计数，刷新当前列表。
    load()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '操作失败')
  }
}

async function removeComment(postID: string, comment: CommunityComment) {
  if (!window.confirm('确定删除该留言？')) return
  try {
    await adminDeleteCommunityComment(comment.id)
    ui.success('留言已删除')
    // 删除后强制重新加载留言，避免缓存中仍显示已删内容。
    delete comments[postID]
    await loadComments(postID)
    load()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '删除失败')
  }
}

onMounted(load)
</script>

<template>
  <div class="space-y-4">
    <ListShell title="社区管理" :loading="loading" :error="error" @retry="load">
      <template #toolbar>
        <span class="text-xs text-text-muted">共 {{ total }} 个帖子</span>
      </template>

      <div class="mb-4 flex flex-wrap gap-3">
        <input
          v-model="filters.q"
          type="search"
          placeholder="搜索标题或正文"
          class="h-9 w-56 rounded-md border border-border-dark bg-ink-800 px-3 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
          @change="filters.page = 1; load()"
        />
        <select
          v-model="filters.status"
          class="h-9 rounded-md border border-border-dark bg-ink-800 px-2 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
          @change="filters.page = 1; load()"
        >
          <option value="">全部状态</option>
          <option value="published">已发布</option>
          <option value="hidden">已隐藏</option>
        </select>
        <select
          v-model="filters.category"
          class="h-9 rounded-md border border-border-dark bg-ink-800 px-2 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
          @change="filters.page = 1; load()"
        >
          <option value="">全部分类</option>
          <option v-for="c in COMMUNITY_CATEGORIES" :key="c" :value="c">
            {{ COMMUNITY_CATEGORY_LABELS[c] }}
          </option>
        </select>
      </div>

      <div v-if="items.length" class="space-y-3">
        <article v-for="item in items" :key="item.id" class="card p-4">
          <div class="mb-2 flex flex-wrap items-center gap-2 text-xs">
            <StatusBadge :status="item.status" />
            <span class="rounded border border-border-light bg-ink-700 px-1.5 py-0.5 text-text-secondary">
              {{ COMMUNITY_CATEGORY_LABELS[item.category] }}
            </span>
            <span class="text-text-muted">
              {{ communityAuthorName(item.author) }} · <span class="font-mono">{{ item.author.user_no }}</span>
            </span>
            <span class="text-text-muted">{{ formatDateTime(item.created_at) }}</span>
          </div>

          <h3 class="mb-1 font-medium text-text-primary">{{ item.title }}</h3>
          <p class="mb-3 line-clamp-3 whitespace-pre-wrap text-sm text-text-secondary">{{ item.body }}</p>

          <div class="mb-3 flex flex-wrap gap-4 text-xs text-text-muted">
            <span>👍 {{ item.like_count }}</span>
            <span>💬 {{ item.comment_count }}</span>
            <span>{{ COMMUNITY_STATUS_LABELS[item.status] }}</span>
          </div>

          <div class="flex flex-wrap gap-2">
            <button
              v-if="item.status === 'published'"
              type="button"
              class="rounded border border-border-light px-2 py-1 text-xs text-text-secondary transition hover:bg-ink-700"
              @click="setStatus(item, 'hidden')"
            >
              隐藏
            </button>
            <button
              v-else
              type="button"
              class="rounded border border-grass-500/60 px-2 py-1 text-xs text-grass-300 transition hover:bg-grass-500/10"
              @click="setStatus(item, 'published')"
            >
              恢复
            </button>
            <button
              type="button"
              class="rounded border border-border-light px-2 py-1 text-xs text-text-secondary transition hover:bg-ink-700"
              @click="toggleComments(item)"
            >
              {{ comments[item.id] ? '收起留言' : '查看留言' }}
            </button>
            <button
              type="button"
              class="ml-auto rounded border border-red-500/40 px-2 py-1 text-xs text-red-300 transition hover:bg-red-500/10"
              @click="removePost(item)"
            >
              删除帖子
            </button>
          </div>

          <div v-if="commentsLoading[item.id]" class="mt-3 text-xs text-text-muted">留言加载中…</div>
          <div v-else-if="comments[item.id]" class="mt-3 space-y-2 border-t border-border-dark pt-3">
            <p v-if="!comments[item.id].length" class="text-xs text-text-muted">暂无留言。</p>
            <div
              v-for="comment in comments[item.id]"
              :key="comment.id"
              class="rounded border border-border-dark bg-ink-900/60 p-2.5"
            >
              <div class="mb-1 flex flex-wrap items-center gap-2 text-xs text-text-muted">
                <StatusBadge v-if="comment.status === 'hidden'" status="hidden" />
                <span>{{ communityAuthorName(comment.author) }}</span>
                <span class="font-mono">{{ comment.author.user_no }}</span>
                <span>{{ formatDateTime(comment.created_at) }}</span>
                <button
                  type="button"
                  class="ml-auto text-text-secondary hover:underline"
                  @click="setCommentStatus(comment)"
                >
                  {{ comment.status === 'hidden' ? '恢复' : '隐藏' }}
                </button>
                <button
                  type="button"
                  class="text-red-300 hover:underline"
                  @click="removeComment(item.id, comment)"
                >
                  删除
                </button>
              </div>
              <p class="whitespace-pre-wrap text-sm text-text-secondary">{{ comment.body }}</p>
            </div>
          </div>
        </article>
      </div>

      <p v-else class="text-sm text-text-muted">暂无符合条件的帖子。</p>

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
