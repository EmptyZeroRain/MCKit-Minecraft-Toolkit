<script setup lang="ts">
// 社区帖子详情：正文、点赞、留言列表与留言表单。
import { computed, onMounted, reactive, ref } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import PageContainer from '@/components/layout/PageContainer.vue'
import Breadcrumbs from '@/components/layout/Breadcrumbs.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import ErrorState from '@/components/common/ErrorState.vue'
import LoadingSkeleton from '@/components/common/LoadingSkeleton.vue'
import Pagination from '@/components/common/Pagination.vue'
import { ApiError } from '@/types/api'
import {
  createCommunityComment,
  deleteCommunityComment,
  deleteCommunityPost,
  fetchCommunityPost,
  listCommunityComments,
  toggleCommunityLike,
} from '@/api/community'
import {
  COMMUNITY_CATEGORY_LABELS,
  communityAuthorName,
  type CommunityComment,
  type CommunityPost,
} from '@/types/community'
import { useUserAuthStore } from '@/stores/user-auth'
import { formatDateTime } from '@/utils/format'
import { applyPageMeta } from '@/utils/seo'

const route = useRoute()
const router = useRouter()
const userAuth = useUserAuthStore()

const post = ref<CommunityPost | null>(null)
const comments = ref<CommunityComment[]>([])
const total = ref(0)
const totalPages = ref(1)
const page = ref(1)
const pageSize = 30
const loading = ref(true)
const error = ref('')
const commentError = ref('')
const submitting = ref(false)
const liking = ref(false)
const actionError = ref('')

const postId = computed(() => String(route.params.id ?? ''))
const isLoggedIn = computed(() => userAuth.isLoggedIn)

applyPageMeta({
  title: '社区帖子',
  description: '查看玩家社区帖子与留言。',
  canonicalPath: `/community/${postId.value}`,
  robot: 'noindex,follow',
})

async function loadPost() {
  post.value = await fetchCommunityPost(postId.value)
}

async function loadComments() {
  const res = await listCommunityComments({
    post_id: postId.value,
    page: page.value,
    page_size: pageSize,
  })
  comments.value = res.items
  total.value = res.total
  totalPages.value = res.totalPages || Math.max(1, Math.ceil(res.total / pageSize))
}

async function loadAll() {
  loading.value = true
  error.value = ''
  try {
    await Promise.all([loadPost(), loadComments()])
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '加载失败，请稍后重试'
  } finally {
    loading.value = false
  }
}

function onPageChange(p: number) {
  page.value = p
  loadComments().catch(() => {
    commentError.value = '留言加载失败'
  })
}

async function toggleLike() {
  if (!post.value || liking.value) return
  actionError.value = ''
  if (!isLoggedIn.value) {
    await router.push({ path: '/login', query: { redirect: route.fullPath } })
    return
  }
  liking.value = true
  try {
    const res = await toggleCommunityLike(post.value.id)
    post.value.liked = res.liked
    post.value.like_count = res.like_count
  } catch (err) {
    actionError.value = err instanceof ApiError ? err.message : '操作失败，请稍后重试'
  } finally {
    liking.value = false
  }
}

const commentDraft = reactive({ body: '' })

async function submitComment() {
  commentError.value = ''
  if (!isLoggedIn.value) {
    await router.push({ path: '/login', query: { redirect: route.fullPath } })
    return
  }
  const body = commentDraft.body.trim()
  if (!body) {
    commentError.value = '请填写留言内容'
    return
  }
  submitting.value = true
  try {
    await createCommunityComment({ post_id: postId.value, body })
    commentDraft.body = ''
    page.value = Math.max(1, totalPages.value)
    await Promise.all([loadPost(), loadComments()])
  } catch (err) {
    commentError.value = err instanceof ApiError ? err.message : '留言失败，请稍后重试'
  } finally {
    submitting.value = false
  }
}

// 作者归属统一用对外用户 ID 判断，前端不依赖也不保存内部 UUID。
const currentUserNo = computed(() => userAuth.user?.user_no ?? '')
const canDeletePost = computed(
  () => !!currentUserNo.value && post.value?.author.user_no === currentUserNo.value,
)

function canDeleteComment(comment: CommunityComment): boolean {
  if (!currentUserNo.value) return false
  return (
    comment.author.user_no === currentUserNo.value ||
    post.value?.author.user_no === currentUserNo.value
  )
}

async function removePost() {
  if (!post.value) return
  if (!window.confirm('确定删除该帖子？留言与点赞将一并删除且不可恢复。')) return
  actionError.value = ''
  try {
    await deleteCommunityPost(post.value.id)
    await router.replace({ name: 'community' })
  } catch (err) {
    actionError.value = err instanceof ApiError ? err.message : '删除失败'
  }
}

async function removeComment(comment: CommunityComment) {
  if (!window.confirm('确定删除该留言？')) return
  commentError.value = ''
  try {
    await deleteCommunityComment(comment.id)
    await Promise.all([loadPost(), loadComments()])
  } catch (err) {
    commentError.value = err instanceof ApiError ? err.message : '删除失败'
  }
}

onMounted(async () => {
  if (!userAuth.initialized) await userAuth.restoreSession()
  await loadAll()
})
</script>

<template>
  <PageContainer maxWidth="max-w-3xl">
    <Breadcrumbs :items="[{ label: '玩家社区', to: '/community' }, { label: '帖子详情' }]" />

    <LoadingSkeleton v-if="loading" :count="3" />
    <ErrorState v-else-if="error" :message="error" retryable @retry="loadAll" />

    <template v-else-if="post">
      <article class="card p-5 sm:p-6">
        <div class="mb-3 flex flex-wrap items-center gap-2 text-xs text-text-muted">
          <span class="rounded border border-border-light bg-ink-700 px-2 py-0.5 text-text-secondary">
            {{ COMMUNITY_CATEGORY_LABELS[post.category] }}
          </span>
          <span>{{ communityAuthorName(post.author) }}</span>
          <span class="font-mono">{{ post.author.user_no }}</span>
          <span>{{ formatDateTime(post.created_at) }}</span>
          <span v-if="post.status === 'hidden'" class="text-amber-300">已隐藏</span>
        </div>

        <h1 class="mb-4 text-2xl font-bold text-text-primary">{{ post.title }}</h1>
        <div class="whitespace-pre-wrap text-sm leading-7 text-text-secondary">{{ post.body }}</div>

        <p
          v-if="actionError"
          class="mt-4 rounded border border-red-500/40 bg-red-500/10 px-3 py-2 text-sm text-red-300"
        >
          {{ actionError }}
        </p>

        <div class="mt-6 flex flex-wrap items-center gap-3 border-t border-border-dark pt-4">
          <button
            type="button"
            class="inline-flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-sm transition disabled:cursor-not-allowed disabled:opacity-60"
            :class="
              post.liked
                ? 'border-grass-500 bg-grass-500/15 text-grass-300'
                : 'border-border-light text-text-secondary hover:bg-ink-700'
            "
            :aria-pressed="post.liked"
            :disabled="liking"
            @click="toggleLike"
          >
            👍 {{ post.liked ? '已点赞' : '点赞' }}（{{ post.like_count }}）
          </button>
          <span class="text-sm text-text-muted">💬 {{ post.comment_count }} 条留言</span>
          <button
            v-if="canDeletePost"
            type="button"
            class="ml-auto rounded-md border border-red-500/40 px-3 py-1.5 text-sm text-red-300 transition hover:bg-red-500/10"
            @click="removePost"
          >
            删除帖子
          </button>
        </div>
      </article>

      <section class="mt-6">
        <h2 class="mb-3 text-lg font-semibold text-text-primary">留言（{{ total }}）</h2>

        <p
          v-if="commentError"
          class="mb-3 rounded border border-red-500/40 bg-red-500/10 px-3 py-2 text-sm text-red-300"
        >
          {{ commentError }}
        </p>

        <div v-if="comments.length" class="space-y-3">
          <article v-for="comment in comments" :key="comment.id" class="card p-4">
            <div class="mb-2 flex flex-wrap items-center gap-2 text-xs text-text-muted">
              <span class="font-medium text-text-secondary">{{ communityAuthorName(comment.author) }}</span>
              <span class="font-mono">{{ comment.author.user_no }}</span>
              <span>{{ formatDateTime(comment.created_at) }}</span>
              <button
                v-if="canDeleteComment(comment)"
                type="button"
                class="ml-auto text-red-300 hover:underline"
                @click="removeComment(comment)"
              >
                删除
              </button>
            </div>
            <p class="whitespace-pre-wrap text-sm text-text-secondary">{{ comment.body }}</p>
          </article>
        </div>
        <EmptyState
          v-else
          title="还没有留言"
          description="友善交流，从第一条留言开始。"
        />

        <Pagination
          :page="page"
          :page-size="pageSize"
          :total="total"
          :total-pages="totalPages"
          @change="onPageChange"
        />

        <!-- 留言表单 -->
        <div v-if="isLoggedIn" class="card mt-6 space-y-3 p-4">
          <label class="block text-sm text-text-secondary">
            <span class="mb-1 block">发表留言（≤2000 字）</span>
            <textarea
              v-model="commentDraft.body"
              class="min-h-[6rem] w-full rounded-md border border-border-dark bg-ink-900 px-3 py-2 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
              maxlength="2000"
              placeholder="友善发言，禁止人身攻击与广告。"
            />
          </label>
          <div class="flex justify-end">
            <button
              type="button"
              class="rounded-md bg-grass-500 px-4 py-2 text-sm font-medium text-ink-900 transition hover:bg-grass-400 disabled:cursor-not-allowed disabled:opacity-60"
              :disabled="submitting"
              @click="submitComment"
            >
              {{ submitting ? '提交中…' : '发表留言' }}
            </button>
          </div>
        </div>
        <p v-else class="card mt-6 p-4 text-sm text-text-secondary">
          <RouterLink
            :to="{ path: '/login', query: { redirect: route.fullPath } }"
            class="text-grass-400 hover:underline"
          >
            登录
          </RouterLink>
          后即可点赞与留言。
        </p>
      </section>
    </template>
  </PageContainer>
</template>
