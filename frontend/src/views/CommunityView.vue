<script setup lang="ts">
// 社区广场：帖子列表 + 发帖入口。
// 读接口匿名可用；发帖要求普通用户登录（写操作由后端 CSRF / 角色双重校验）。
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import PageContainer from '@/components/layout/PageContainer.vue'
import Breadcrumbs from '@/components/layout/Breadcrumbs.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import ErrorState from '@/components/common/ErrorState.vue'
import LoadingSkeleton from '@/components/common/LoadingSkeleton.vue'
import Pagination from '@/components/common/Pagination.vue'
import { ApiError } from '@/types/api'
import { createCommunityPost, listCommunityPosts } from '@/api/community'
import {
  COMMUNITY_CATEGORIES,
  COMMUNITY_CATEGORY_LABELS,
  communityAuthorName,
  type CommunityCategory,
  type CommunityPost,
} from '@/types/community'
import { useUserAuthStore } from '@/stores/user-auth'
import { formatDateTime } from '@/utils/format'
import { applyPageMeta } from '@/utils/seo'

applyPageMeta({
  title: '玩家社区',
  description: 'Minecraft 玩家社区：交流心得、求助问题、展示作品、分享教程。',
  canonicalPath: '/community',
})

const router = useRouter()
const userAuth = useUserAuthStore()

const filters = reactive<{
  category: CommunityCategory | ''
  sort: 'latest' | 'hot'
  q: string
  page: number
  pageSize: number
}>({ category: '', sort: 'latest', q: '', page: 1, pageSize: 12 })

const posts = ref<CommunityPost[]>([])
const total = ref(0)
const totalPages = ref(1)
const loading = ref(false)
const error = ref('')

const showForm = ref(false)
const submitting = ref(false)
const formError = ref('')
const form = reactive<{ category: CommunityCategory; title: string; body: string }>({
  category: 'chat',
  title: '',
  body: '',
})

const canPost = computed(() => userAuth.isLoggedIn)

async function load() {
  loading.value = true
  error.value = ''
  try {
    const res = await listCommunityPosts({
      category: filters.category || undefined,
      sort: filters.sort,
      q: filters.q.trim() || undefined,
      page: filters.page,
      page_size: filters.pageSize,
    })
    posts.value = res.items
    total.value = res.total
    totalPages.value = res.totalPages || Math.max(1, Math.ceil(res.total / filters.pageSize))
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '加载失败，请稍后重试'
  } finally {
    loading.value = false
  }
}

function onPageChange(p: number) {
  filters.page = p
  load()
}

// 筛选条件统一做 250ms 防抖：避免搜索输入时每敲一个字符都打一次接口。
let filterTimer: ReturnType<typeof setTimeout> | null = null
watch(
  () => [filters.category, filters.sort, filters.q] as const,
  () => {
    if (filterTimer) clearTimeout(filterTimer)
    filterTimer = setTimeout(() => {
      filterTimer = null
      filters.page = 1
      load()
    }, 250)
  },
)
onBeforeUnmount(() => {
  if (filterTimer) clearTimeout(filterTimer)
})

function resetFilters() {
  filters.category = ''
  filters.sort = 'latest'
  filters.q = ''
  filters.page = 1
  // 条件未变化时 watcher 不会触发，主动补一次加载。
  if (!filterTimer) load()
}

function openForm() {
  formError.value = ''
  if (!canPost.value) {
    router.push({ path: '/login', query: { redirect: '/community' } })
    return
  }
  showForm.value = true
}

async function submitPost() {
  formError.value = ''
  if (!form.title.trim()) {
    formError.value = '请填写标题'
    return
  }
  if (!form.body.trim()) {
    formError.value = '请填写正文'
    return
  }
  submitting.value = true
  try {
    const post = await createCommunityPost({
      category: form.category,
      title: form.title.trim(),
      body: form.body.trim(),
    })
    showForm.value = false
    form.title = ''
    form.body = ''
    await router.push({ name: 'community-post', params: { id: post.id } })
  } catch (err) {
    formError.value = err instanceof ApiError ? err.message : '发帖失败，请稍后重试'
  } finally {
    submitting.value = false
  }
}

function excerpt(body: string): string {
  const text = body.replace(/\s+/g, ' ').trim()
  return text.length > 160 ? `${text.slice(0, 160)}…` : text
}

onMounted(async () => {
  if (!userAuth.initialized) await userAuth.restoreSession()
  load()
})
</script>

<template>
  <PageContainer maxWidth="max-w-5xl">
    <Breadcrumbs :items="[{ label: '玩家社区' }]" />

    <div class="mb-6 flex flex-wrap items-start justify-between gap-3">
      <div>
        <h1 class="mb-2 text-2xl font-bold text-text-primary md:text-3xl">玩家社区</h1>
        <p class="text-sm text-text-secondary">
          交流玩法心得、求助问题、展示作品；请遵守社区礼仪，禁止发布违规内容。
        </p>
      </div>
      <button
        type="button"
        class="rounded-md bg-grass-500 px-4 py-2 text-sm font-medium text-ink-900 transition hover:bg-grass-400"
        @click="openForm"
      >
        发布帖子
      </button>
    </div>

    <!-- 发帖表单 -->
    <section v-if="showForm" class="card mb-6 space-y-4 p-5">
      <div class="flex items-center justify-between">
        <h2 class="text-base font-semibold text-text-primary">发布新帖子</h2>
        <button
          type="button"
          class="rounded-md border border-border-light px-3 py-2 text-sm text-text-secondary transition hover:bg-ink-700 hover:text-text-primary"
          @click="showForm = false"
        >
          取消
        </button>
      </div>
      <p v-if="formError" class="rounded border border-red-500/40 bg-red-500/10 px-3 py-2 text-sm text-red-300">
        {{ formError }}
      </p>
      <div class="grid gap-3 sm:grid-cols-2">
        <label class="block text-sm text-text-secondary">
          <span class="mb-1 block">分类</span>
          <select
            v-model="form.category"
            class="h-10 w-full rounded-md border border-border-dark bg-ink-800 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
          >
            <option v-for="c in COMMUNITY_CATEGORIES" :key="c" :value="c">
              {{ COMMUNITY_CATEGORY_LABELS[c] }}
            </option>
          </select>
        </label>
        <label class="block text-sm text-text-secondary">
          <span class="mb-1 block">标题（≤120 字）</span>
          <input
            v-model="form.title"
            class="h-10 w-full rounded-md border border-border-dark bg-ink-800 px-3 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
            maxlength="120"
            placeholder="用一句话说明主题"
          />
        </label>
      </div>
      <label class="block text-sm text-text-secondary">
        <span class="mb-1 block">正文（≤10000 字）</span>
        <textarea
          v-model="form.body"
          class="min-h-[10rem] w-full resize-y rounded-md border border-border-dark bg-ink-800 px-3 py-2 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
          maxlength="10000"
          placeholder="支持换行；请勿发布个人隐私或侵权内容。"
        />
      </label>
      <div class="flex justify-end">
        <button
          type="button"
          class="rounded-md bg-grass-500 px-4 py-2 text-sm font-medium text-ink-900 transition hover:bg-grass-400 disabled:cursor-not-allowed disabled:opacity-60"
          :disabled="submitting"
          @click="submitPost"
        >
          {{ submitting ? '发布中…' : '发布' }}
        </button>
      </div>
    </section>

    <!-- 筛选 -->
    <div class="mb-6 flex flex-wrap items-center gap-3">
      <div class="relative min-w-[14rem] flex-1">
        <input
          v-model="filters.q"
          type="search"
          class="h-10 w-full rounded-md border border-border-dark bg-ink-800 pl-9 pr-3 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
          placeholder="搜索标题或正文"
          aria-label="搜索帖子"
        />
        <svg
          class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-text-muted"
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
          stroke-width="2"
        >
          <path stroke-linecap="round" d="M21 21l-4.35-4.35M17 10.5a6.5 6.5 0 11-13 0 6.5 6.5 0 0113 0z" />
        </svg>
      </div>
      <select
        v-model="filters.category"
        class="h-10 w-auto rounded-md border border-border-dark bg-ink-800 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
        aria-label="按分类筛选"
      >
        <option value="">全部分类</option>
        <option v-for="c in COMMUNITY_CATEGORIES" :key="c" :value="c">
          {{ COMMUNITY_CATEGORY_LABELS[c] }}
        </option>
      </select>
      <select
        v-model="filters.sort"
        class="h-10 w-auto rounded-md border border-border-dark bg-ink-800 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
        aria-label="排序方式"
      >
        <option value="latest">最新发布</option>
        <option value="hot">最热</option>
      </select>
      <button
        type="button"
        class="rounded-md border border-border-light px-3 py-2 text-sm text-text-secondary transition hover:bg-ink-700 hover:text-text-primary"
        @click="resetFilters"
      >
        重置
      </button>
    </div>

    <p v-if="!loading && !error" class="mb-3 text-xs text-text-muted">共 {{ total }} 个帖子</p>

    <LoadingSkeleton v-if="loading" :count="4" />
    <ErrorState v-else-if="error" :message="error" retryable @retry="load" />

    <div v-else-if="posts.length" class="space-y-3">
      <article v-for="post in posts" :key="post.id" class="card card-hover p-5">
        <div class="mb-2 flex flex-wrap items-center gap-2 text-xs text-text-muted">
          <span class="rounded border border-border-light bg-ink-700 px-2 py-0.5 text-text-secondary">
            {{ COMMUNITY_CATEGORY_LABELS[post.category] }}
          </span>
          <span>{{ communityAuthorName(post.author) }}</span>
          <span class="font-mono">{{ post.author.user_no }}</span>
          <span>{{ formatDateTime(post.created_at) }}</span>
        </div>
        <h2 class="mb-2 text-lg font-semibold text-text-primary">
          <RouterLink
            :to="{ name: 'community-post', params: { id: post.id } }"
            class="hover:text-grass-300 hover:underline"
          >
            {{ post.title }}
          </RouterLink>
        </h2>
        <p class="mb-3 line-clamp-2 text-sm text-text-secondary">{{ excerpt(post.body) }}</p>
        <div class="flex items-center gap-4 text-xs text-text-muted">
          <span>👍 {{ post.like_count }}</span>
          <span>💬 {{ post.comment_count }}</span>
          <RouterLink
            :to="{ name: 'community-post', params: { id: post.id } }"
            class="ml-auto text-grass-400 hover:underline"
          >
            查看详情
          </RouterLink>
        </div>
      </article>
    </div>

    <EmptyState
      v-else
      title="还没有帖子"
      description="成为第一个发帖的人，分享你的 Minecraft 心得吧。"
      action-label="发布帖子"
      @action="openForm"
    />

    <Pagination
      :page="filters.page"
      :page-size="filters.pageSize"
      :total="total"
      :total-pages="totalPages"
      @change="onPageChange"
    />
  </PageContainer>
</template>
