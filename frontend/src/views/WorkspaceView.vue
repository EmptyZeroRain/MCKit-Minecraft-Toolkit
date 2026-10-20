<script setup lang="ts">
// 个人工作台（需求 §12）：收藏 / 最近使用 / 常用种子 / 礼物与成长摘要。
// 账户资料与安全设置已迁移至 /settings，工作台只保留摘要与入口。
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import PageContainer from '@/components/layout/PageContainer.vue'
import Breadcrumbs from '@/components/layout/Breadcrumbs.vue'
import LoadingSkeleton from '@/components/common/LoadingSkeleton.vue'
import { ApiError } from '@/types/api'
import {
  FAVORITE_KIND_LABELS,
  type FavoriteKind,
  type UserFavoriteItem,
} from '@/types/user'
import { useUserAuthStore } from '@/stores/user-auth'
import { useUserWorkspaceStore } from '@/stores/user-workspace'
import { applyPageMeta } from '@/utils/seo'
import { fetchGiftOverview } from '@/api/gift'
import { fetchGrowthOverview } from '@/api/growth'
import type { GiftOverview } from '@/types/gift'
import type { GrowthOverview } from '@/types/growth'

applyPageMeta({
  title: '我的工作台',
  description: '管理收藏、最近使用与常用种子。',
  canonicalPath: '/workspace',
})

const router = useRouter()
const userAuth = useUserAuthStore()
const workspace = useUserWorkspaceStore()

const error = ref('')
const notice = ref('')
const busy = ref('')
const giftOverview = ref<GiftOverview | null>(null)
const growth = ref<GrowthOverview | null>(null)

const inputClass =
  'w-full min-w-0 rounded-md border border-border-dark bg-ink-900 px-3 py-2 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none'

const seedForm = reactive({ name: '', seed: '', note: '' })

const avatarBroken = ref(false)
const accountAvatar = computed(() =>
  avatarBroken.value ? '' : (userAuth.user?.avatar_url ?? ''),
)
const avatarFallback = computed(() => userAuth.displayName.charAt(0).toUpperCase() || '?')
const isBackendAccount = computed(() => {
  const role = userAuth.user?.role
  return !!role && role !== 'user'
})
const accountRoleLabel = computed(() => {
  const role = userAuth.user?.role
  if (role === 'admin') return '管理员'
  if (role === 'editor') return '编辑'
  if (isBackendAccount.value) return '后台账号'
  return '普通用户'
})

const KIND_ORDER: FavoriteKind[] = [
  'tool',
  'item',
  'potion',
  'villager',
  'mob',
  'redstone_design',
  'mod',
  'modpack',
  'resource',
  'wiki',
]

/** 收藏按类型分组，空组不展示 */
const favoriteGroups = computed(() =>
  KIND_ORDER.map((kind) => ({
    kind,
    label: FAVORITE_KIND_LABELS[kind],
    items: workspace.favorites.filter((f) => f.kind === kind),
  })).filter((g) => g.items.length > 0),
)

const recentGiftItems = computed(() => giftOverview.value?.recent.slice(0, 3) ?? [])

/** 访客阶段按活跃天数展示进度，普通等级按经验进度展示。 */
const growthProgress = computed(() => {
  if (!growth.value) return 0
  if (growth.value.level === 0) {
    const target = Math.max(1, growth.value.guest_active_days)
    return Math.min(100, Math.round((growth.value.active_days / target) * 100))
  }
  return Math.min(100, Math.max(0, growth.value.level_progress_percent))
})

const growthHint = computed(() => {
  if (!growth.value) return ''
  if (growth.value.level === 0) {
    return `活跃 ${growth.value.active_days}/${growth.value.guest_active_days} 天解锁 Lv.1`
  }
  if (growth.value.is_max_level) return '已达到当前最高等级'
  return `距下一级还需 ${growth.value.xp_to_next_level} 经验`
})

function errText(e: unknown, fallback: string): string {
  return e instanceof ApiError ? e.message : fallback
}

function isInternal(url: string): boolean {
  return url.startsWith('/')
}

function formatTime(iso: string): string {
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '—' : d.toLocaleString('zh-CN')
}

function giftCoinLabel(coinType: string): string {
  return coinType === 'checkin' ? '签到币' : '通用币'
}

async function loadPageData() {
  error.value = ''
  const [workspaceResult, giftResult, growthResult] = await Promise.allSettled([
    workspace.loadAll(),
    fetchGiftOverview(),
    fetchGrowthOverview(),
  ])

  if (workspaceResult.status === 'rejected') {
    error.value = errText(workspaceResult.reason, '个人数据加载失败')
  }
  giftOverview.value = giftResult.status === 'fulfilled' ? giftResult.value : null
  growth.value = growthResult.status === 'fulfilled' ? growthResult.value : null
}

onMounted(async () => {
  if (!userAuth.initialized) await userAuth.restoreSession()
  await loadPageData()
})

async function handleLogout() {
  await userAuth.logout()
  await router.push('/')
}

async function removeFavorite(item: UserFavoriteItem) {
  error.value = ''
  try {
    await workspace.removeFavorite(item.kind, item.object_id)
  } catch (e) {
    error.value = errText(e, '取消收藏失败')
  }
}

async function clearRecents() {
  if (!window.confirm('确定清空全部最近使用记录吗？此操作不可撤销。')) return
  error.value = ''
  try {
    await workspace.clearRecents()
    notice.value = '最近使用已清空。'
  } catch (e) {
    error.value = errText(e, '清空失败')
  }
}

async function saveSeed() {
  error.value = ''
  notice.value = ''
  if (!seedForm.name.trim() || !seedForm.seed.trim()) {
    error.value = '名称与种子均为必填'
    return
  }
  busy.value = 'seed'
  try {
    await workspace.saveSeed({
      name: seedForm.name.trim(),
      seed: seedForm.seed.trim(),
      note: seedForm.note.trim(),
    })
    seedForm.name = ''
    seedForm.seed = ''
    seedForm.note = ''
    notice.value = '常用种子已保存。'
  } catch (e) {
    error.value = errText(e, '保存失败')
  } finally {
    busy.value = ''
  }
}

async function deleteSeed(id: string) {
  error.value = ''
  try {
    await workspace.deleteSeed(id)
  } catch (e) {
    error.value = errText(e, '删除失败')
  }
}
</script>

<template>
  <PageContainer maxWidth="max-w-6xl">
    <Breadcrumbs :items="[{ label: '我的工作台' }]" />

    <!-- 标题区与下方主体共用同一宽度基线 -->
    <header class="mb-6">
      <h1 class="text-2xl font-bold tracking-tight text-text-primary md:text-3xl">我的工作台</h1>
      <p class="mt-1.5 text-sm text-text-secondary">管理收藏、最近使用与常用种子。</p>
    </header>

    <p
      v-if="error"
      class="mb-5 flex flex-wrap items-center justify-between gap-3 rounded-card border border-danger-mc/40 bg-danger-mc/10 px-4 py-3 text-sm text-danger-mc"
      role="alert"
    >
      <span>{{ error }}</span>
      <button
        type="button"
        class="rounded-md border border-danger-mc/40 px-2.5 py-1 text-xs font-medium hover:bg-danger-mc/10"
        @click="loadPageData"
      >
        重新加载
      </button>
    </p>
    <p
      v-if="notice"
      class="mb-5 rounded-card border border-grass-500/40 bg-grass-500/10 px-4 py-3 text-sm text-grass-400"
      role="status"
    >
      {{ notice }}
    </p>

    <div class="space-y-8">
      <!-- 1. 用户信息摘要 -->
      <section aria-labelledby="workspace-user-title">
        <h2 id="workspace-user-title" class="sr-only">用户信息</h2>
        <div class="card relative p-5 sm:p-6">
          <!-- 草方块色带：低对比度点缀，不干扰文字 -->
          <div
            class="absolute inset-x-0 top-0 h-1 rounded-t-xl opacity-70"
            aria-hidden="true"
            style="
              background-image: repeating-linear-gradient(
                90deg,
                var(--mc-grass-500) 0 18px,
                #7d9b58 18px 36px
              );
            "
          />

          <div class="flex flex-col gap-5 lg:flex-row lg:items-center">
            <div class="flex min-w-0 items-center gap-4">
              <div
                class="flex h-16 w-16 shrink-0 items-center justify-center overflow-hidden rounded-xl border border-grass-500/30 bg-ink-700 ring-4 ring-grass-500/10"
              >
                <img
                  v-if="accountAvatar"
                  :src="accountAvatar"
                  :alt="userAuth.displayName"
                  class="h-full w-full object-cover"
                  @error="avatarBroken = true"
                />
                <span v-else class="text-xl font-bold text-grass-400" aria-hidden="true">
                  {{ avatarFallback }}
                </span>
              </div>

              <div class="min-w-0">
                <div class="flex flex-wrap items-center gap-2">
                  <p class="break-words text-lg font-semibold text-text-primary">
                    {{ userAuth.displayName }}
                  </p>
                  <span
                    class="rounded-md border px-1.5 py-0.5 text-[11px] font-medium"
                    :class="
                      isBackendAccount
                        ? 'border-amber-mc/40 bg-amber-mc/10 text-amber-mc'
                        : 'border-border-light bg-ink-700 text-text-secondary'
                    "
                  >
                    {{ accountRoleLabel }}
                  </span>
                </div>
                <p
                  v-if="userAuth.user?.user_no"
                  class="mt-1 flex flex-wrap items-center gap-1.5 text-xs text-text-muted"
                >
                  <span>用户 ID</span>
                  <span class="break-all font-mono text-text-secondary">
                    {{ userAuth.user.user_no }}
                  </span>
                </p>
              </div>
            </div>

            <div class="flex flex-1 flex-col gap-4 sm:flex-row sm:items-center lg:justify-end">
              <!-- 等级 / 经验：只有服务端返回真实成长数据时才展示 -->
              <RouterLink
                v-if="growth"
                to="/growth"
                class="min-w-0 rounded-lg border border-border-dark bg-ink-900/60 px-4 py-3 transition hover:border-grass-500/40 sm:w-64"
              >
                <div class="flex items-center justify-between gap-3">
                  <span class="text-sm font-medium text-text-primary">
                    Lv.{{ growth.level }} · {{ growth.level_title }}
                  </span>
                  <span class="text-xs text-grass-400">成长中心 →</span>
                </div>
                <div class="mt-2 h-1.5 overflow-hidden rounded-full bg-ink-700">
                  <div
                    class="h-full rounded-full bg-grass-500 transition-all"
                    :style="{ width: `${growthProgress}%` }"
                  />
                </div>
                <p class="mt-1.5 text-[11px] text-text-muted">{{ growthHint }}</p>
              </RouterLink>

              <div class="flex flex-wrap items-center gap-2">
                <RouterLink
                  to="/settings"
                  class="inline-flex h-9 items-center gap-1.5 rounded-md border border-border-light bg-ink-700 px-3 text-sm text-text-secondary transition hover:bg-ink-600 hover:text-text-primary"
                >
                  <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2" aria-hidden="true">
                    <path stroke-linecap="round" stroke-linejoin="round" d="M10.5 6h9m-9 12h9M4.5 6h.008v.008H4.5V6zm0 12h.008v.008H4.5V18zM4.5 12h.008v.008H4.5V12zm6 0h9" />
                  </svg>
                  账户设置
                </RouterLink>

                <details class="relative">
                  <summary
                    class="inline-flex h-9 cursor-pointer list-none items-center gap-1.5 rounded-md border border-border-dark px-3 text-sm text-text-secondary transition hover:bg-ink-700 hover:text-text-primary [&::-webkit-details-marker]:hidden"
                  >
                    更多
                    <svg class="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2" aria-hidden="true">
                      <path stroke-linecap="round" stroke-linejoin="round" d="m19.5 8.25-7.5 7.5-7.5-7.5" />
                    </svg>
                  </summary>
                  <div
                    class="absolute right-0 z-20 mt-2 w-40 overflow-hidden rounded-lg border border-border-dark bg-ink-800 py-1 shadow-xl"
                  >
                    <RouterLink
                      to="/growth"
                      class="block px-3 py-2 text-sm text-text-secondary hover:bg-ink-700 hover:text-text-primary"
                    >
                      成长中心
                    </RouterLink>
                    <RouterLink
                      to="/submit-resource"
                      class="block px-3 py-2 text-sm text-text-secondary hover:bg-ink-700 hover:text-text-primary"
                    >
                      资源投稿
                    </RouterLink>
                    <RouterLink
                      v-if="isBackendAccount"
                      to="/admin"
                      class="block px-3 py-2 text-sm text-text-secondary hover:bg-ink-700 hover:text-text-primary"
                    >
                      管理后台
                    </RouterLink>
                    <button
                      type="button"
                      class="block w-full border-t border-border-dark px-3 py-2 text-left text-sm text-text-secondary hover:bg-ink-700 hover:text-text-primary"
                      @click="handleLogout"
                    >
                      退出登录
                    </button>
                  </div>
                </details>
              </div>
            </div>
          </div>
        </div>
      </section>

      <!-- 2. 礼物币与签到摘要 -->
      <section v-if="giftOverview" aria-labelledby="workspace-gift-title">
        <div class="mb-3 flex items-end justify-between gap-3">
          <div>
            <h2 id="workspace-gift-title" class="text-lg font-semibold text-text-primary">
              礼物币与签到
            </h2>
            <p class="mt-0.5 text-xs text-text-muted">签到、兑换与礼物记录都在礼物中心。</p>
          </div>
          <RouterLink
            to="/gifts"
            class="shrink-0 text-xs text-grass-400 underline-offset-2 hover:underline"
          >
            礼物中心 →
          </RouterLink>
        </div>

        <div class="card overflow-hidden">
          <div class="grid lg:grid-cols-[1.15fr_1fr]">
            <div class="p-5 sm:p-6">
              <p class="text-xs font-medium uppercase tracking-wide text-text-muted">礼物币余额</p>
              <div class="mt-2 flex flex-wrap items-end gap-x-3 gap-y-1">
                <span class="text-4xl font-bold leading-none text-grass-400">
                  {{ giftOverview.total_balance }}
                </span>
                <span class="pb-0.5 text-sm text-text-secondary">枚</span>
              </div>
              <div class="mt-4 flex flex-wrap gap-2 text-xs">
                <span class="rounded-md border border-border-dark bg-ink-900 px-2.5 py-1 text-text-secondary">
                  通用礼物币
                  <span class="ml-1 font-semibold text-text-primary">{{ giftOverview.general_balance }}</span>
                </span>
                <span class="rounded-md border border-grass-500/25 bg-grass-500/10 px-2.5 py-1 text-grass-400">
                  签到礼物币
                  <span class="ml-1 font-semibold">{{ giftOverview.checkin_balance }}</span>
                </span>
              </div>

              <div v-if="recentGiftItems.length" class="mt-5 border-t border-border-dark pt-3">
                <div class="mb-1 flex items-center justify-between">
                  <span class="text-xs font-medium text-text-secondary">最近变动</span>
                  <RouterLink to="/gifts" class="text-[11px] text-grass-400 hover:underline">
                    全部记录
                  </RouterLink>
                </div>
                <ul class="space-y-1.5">
                  <li
                    v-for="item in recentGiftItems"
                    :key="item.id"
                    class="flex items-center justify-between gap-3 text-xs"
                  >
                    <span class="min-w-0 break-words text-text-secondary">
                      {{ item.reason }}
                      <span class="text-[11px] text-text-muted">· {{ giftCoinLabel(item.coin_type) }}</span>
                    </span>
                    <span
                      class="shrink-0 font-medium"
                      :class="item.amount > 0 ? 'text-grass-400' : 'text-danger-mc'"
                    >
                      {{ item.amount > 0 ? '+' : '' }}{{ item.amount }}
                    </span>
                  </li>
                </ul>
              </div>
            </div>

            <div class="border-t border-border-dark bg-ink-900/50 p-5 sm:p-6 lg:border-l lg:border-t-0">
              <div class="flex items-center justify-between gap-3">
                <p class="text-sm font-medium text-text-primary">今日签到</p>
                <span
                  class="inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5 text-[11px] font-medium"
                  :class="
                    giftOverview.checked_in_today
                      ? 'border-grass-500/40 bg-grass-500/10 text-grass-400'
                      : 'border-amber-mc/40 bg-amber-mc/10 text-amber-mc'
                  "
                >
                  <span
                    class="h-1.5 w-1.5 rounded-full"
                    :class="giftOverview.checked_in_today ? 'bg-grass-400' : 'bg-amber-mc'"
                  />
                  {{ giftOverview.checked_in_today ? '已签到' : '未签到' }}
                </span>
              </div>

              <p class="mt-4 text-sm text-text-secondary">
                今日奖励
                <span class="ml-1 font-semibold text-text-primary">{{ giftOverview.checkin_reward }}</span>
                签到礼物币
              </p>
              <p class="mt-1 text-xs text-text-muted">
                签到日期：{{ giftOverview.checkin_date }}
              </p>

              <RouterLink
                to="/gifts"
                class="mt-5 inline-flex h-9 items-center rounded-md bg-grass-600 px-4 text-sm font-medium text-white transition hover:bg-grass-500"
              >
                {{ giftOverview.checked_in_today ? '查看礼物记录' : '去礼物中心签到' }}
              </RouterLink>
            </div>
          </div>
        </div>
      </section>

      <!-- 3. 常用功能入口 -->
      <section aria-labelledby="workspace-shortcuts-title">
        <h2 id="workspace-shortcuts-title" class="mb-3 text-lg font-semibold text-text-primary">
          常用功能
        </h2>
        <div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <a
            href="#workspace-favorites"
            class="card card-hover group flex items-start gap-3 p-4"
          >
            <span class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg border border-grass-500/25 bg-grass-500/10 text-grass-400">
              <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2" aria-hidden="true">
                <path stroke-linecap="round" stroke-linejoin="round" d="M21 8.25c0-2.485-2.099-4.5-4.688-4.5-1.935 0-3.597 1.126-4.312 2.733-.715-1.607-2.377-2.733-4.313-2.733C5.1 3.75 3 5.765 3 8.25c0 7.22 9 12 9 12s9-4.78 9-12z" />
              </svg>
            </span>
            <span class="min-w-0 flex-1">
              <span class="flex items-center justify-between gap-2">
                <span class="text-sm font-semibold text-text-primary">我的收藏</span>
                <span class="text-[11px] text-text-muted">{{ workspace.favoriteCount }}</span>
              </span>
              <span class="mt-0.5 block text-xs text-text-muted">快捷回到常看的工具与资料</span>
            </span>
          </a>

          <a
            href="#workspace-recents"
            class="card card-hover group flex items-start gap-3 p-4"
          >
            <span class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg border border-border-light bg-ink-700 text-text-secondary">
              <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2" aria-hidden="true">
                <path stroke-linecap="round" stroke-linejoin="round" d="M12 6v6h4.5m4.5 0a9 9 0 1 1-18 0 9 9 0 0 1 18 0z" />
              </svg>
            </span>
            <span class="min-w-0 flex-1">
              <span class="flex items-center justify-between gap-2">
                <span class="text-sm font-semibold text-text-primary">最近使用</span>
                <span class="text-[11px] text-text-muted">{{ workspace.recents.length }}</span>
              </span>
              <span class="mt-0.5 block text-xs text-text-muted">继续上次浏览与查询</span>
            </span>
          </a>

          <a
            href="#workspace-seeds"
            class="card card-hover group flex items-start gap-3 p-4"
          >
            <span class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg border border-amber-mc/25 bg-amber-mc/10 text-amber-mc">
              <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2" aria-hidden="true">
                <path stroke-linecap="round" stroke-linejoin="round" d="M12 21v-8.25M15.75 6.75c0 2.071-1.679 3.75-3.75 3.75S8.25 8.821 8.25 6.75 9.929 3 12 3s3.75 1.679 3.75 3.75zM6.75 14.25h10.5" />
              </svg>
            </span>
            <span class="min-w-0 flex-1">
              <span class="flex items-center justify-between gap-2">
                <span class="text-sm font-semibold text-text-primary">常用种子</span>
                <span class="text-[11px] text-text-muted">{{ workspace.seeds.length }}</span>
              </span>
              <span class="mt-0.5 block text-xs text-text-muted">保存世界种子与备注</span>
            </span>
          </a>

          <RouterLink to="/gifts" class="card card-hover group flex items-start gap-3 p-4">
            <span class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg border border-grass-500/25 bg-grass-500/10 text-grass-400">
              <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2" aria-hidden="true">
                <path stroke-linecap="round" stroke-linejoin="round" d="M21 11.25v8.25a1.5 1.5 0 0 1-1.5 1.5H4.5a1.5 1.5 0 0 1-1.5-1.5v-8.25M12 4.875A2.625 2.625 0 1 0 9.375 7.5H12m0-2.625V7.5m0-2.625A2.625 2.625 0 1 1 14.625 7.5H12m0 0v15" />
              </svg>
            </span>
            <span class="min-w-0 flex-1">
              <span class="flex items-center justify-between gap-2">
                <span class="text-sm font-semibold text-text-primary">礼物与记录</span>
                <span class="text-[11px] text-text-muted">{{ giftOverview?.total_balance ?? '—' }}</span>
              </span>
              <span class="mt-0.5 block text-xs text-text-muted">签到、兑换与礼物币流水</span>
            </span>
          </RouterLink>
        </div>
      </section>

      <LoadingSkeleton v-if="workspace.loading && !workspace.loaded" :count="3" />

      <template v-else-if="workspace.loaded || !error">
        <!-- 4. 我的收藏 -->
        <section id="workspace-favorites" class="scroll-mt-24" aria-labelledby="workspace-favorites-title">
          <div class="mb-3 flex flex-wrap items-end justify-between gap-2">
            <h2 id="workspace-favorites-title" class="text-lg font-semibold text-text-primary">
              我的收藏
              <span class="ml-1 text-sm font-normal text-text-muted">{{ workspace.favoriteCount }}</span>
            </h2>
            <RouterLink to="/tools" class="text-xs text-grass-400 hover:underline">
              去工具中心逛逛 →
            </RouterLink>
          </div>

          <div v-if="favoriteGroups.length" class="grid gap-3 md:grid-cols-2">
            <div v-for="group in favoriteGroups" :key="group.kind" class="card p-4">
              <h3 class="mb-2 text-xs font-semibold uppercase tracking-wide text-text-muted">
                {{ group.label }}
              </h3>
              <ul class="divide-y divide-border-dark">
                <li
                  v-for="item in group.items"
                  :key="item.id"
                  class="flex items-center justify-between gap-3 py-2 first:pt-0 last:pb-0"
                >
                  <div class="min-w-0">
                    <RouterLink
                      v-if="isInternal(item.url)"
                      :to="item.url"
                      class="break-words text-sm text-text-primary hover:text-grass-400"
                    >
                      {{ item.label || item.object_id }}
                    </RouterLink>
                    <a
                      v-else-if="item.url"
                      :href="item.url"
                      class="break-words text-sm text-text-primary hover:text-grass-400"
                      target="_blank"
                      rel="noopener noreferrer"
                    >
                      {{ item.label || item.object_id }}
                    </a>
                    <span v-else class="break-words text-sm text-text-primary">
                      {{ item.label || item.object_id }}
                    </span>
                    <p class="break-all text-[11px] text-text-muted">{{ item.object_id }}</p>
                  </div>
                  <button
                    type="button"
                    class="shrink-0 rounded-md border border-border-light bg-ink-700 px-2 py-1 text-[11px] text-text-secondary transition hover:bg-ink-600 hover:text-text-primary"
                    @click="removeFavorite(item)"
                  >
                    取消
                  </button>
                </li>
              </ul>
            </div>
          </div>

          <div
            v-else
            class="card flex flex-col gap-3 p-4 sm:flex-row sm:items-center sm:justify-between"
          >
            <div class="flex items-center gap-3">
              <span class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border border-border-dark bg-ink-700 text-text-muted">
                <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2" aria-hidden="true">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M21 8.25c0-2.485-2.099-4.5-4.688-4.5-1.935 0-3.597 1.126-4.312 2.733-.715-1.607-2.377-2.733-4.313-2.733C5.1 3.75 3 5.765 3 8.25c0 7.22 9 12 9 12s9-4.78 9-12z" />
                </svg>
              </span>
              <div>
                <p class="text-sm font-medium text-text-primary">还没有收藏</p>
                <p class="mt-0.5 text-xs text-text-muted">
                  在工具、物品、资源或百科页面点击「收藏」即可在此查看。
                </p>
              </div>
            </div>
            <RouterLink
              to="/tools"
              class="inline-flex h-8 shrink-0 items-center rounded-md border border-border-light px-3 text-xs text-text-secondary transition hover:bg-ink-700 hover:text-text-primary"
            >
              浏览工具
            </RouterLink>
          </div>
        </section>

        <!-- 5. 最近使用 -->
        <section id="workspace-recents" class="scroll-mt-24" aria-labelledby="workspace-recents-title">
          <div class="mb-3 flex flex-wrap items-end justify-between gap-2">
            <h2 id="workspace-recents-title" class="text-lg font-semibold text-text-primary">
              最近使用
              <span class="ml-1 text-sm font-normal text-text-muted">{{ workspace.recents.length }}</span>
            </h2>
            <button
              v-if="workspace.recents.length"
              type="button"
              class="rounded-md border border-border-light bg-ink-700 px-2.5 py-1 text-xs text-text-secondary transition hover:bg-ink-600 hover:text-text-primary"
              @click="clearRecents"
            >
              清空记录
            </button>
          </div>

          <div v-if="workspace.recents.length" class="grid gap-2 sm:grid-cols-2">
            <div
              v-for="item in workspace.recents"
              :key="item.id"
              class="card flex items-center gap-3 px-3.5 py-2.5"
            >
              <span class="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-ink-700 text-text-muted">
                <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2" aria-hidden="true">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M12 6v6h4.5m4.5 0a9 9 0 1 1-18 0 9 9 0 0 1 18 0z" />
                </svg>
              </span>
              <div class="min-w-0 flex-1">
                <RouterLink
                  v-if="isInternal(item.url)"
                  :to="item.url"
                  class="block break-words text-sm text-text-primary hover:text-grass-400"
                >
                  {{ item.label || item.object_id }}
                </RouterLink>
                <span v-else class="block break-words text-sm text-text-primary">
                  {{ item.label || item.object_id }}
                </span>
                <p class="text-[11px] text-text-muted">{{ formatTime(item.visited_at) }}</p>
              </div>
            </div>
          </div>
          <p v-else class="text-sm text-text-muted">
            暂无最近使用记录，浏览工具或资源后会自动记录（最多保留最近 50 条）。
          </p>
        </section>

        <!-- 6. 常用种子 / 查询记录 -->
        <section id="workspace-seeds" class="scroll-mt-24" aria-labelledby="workspace-seeds-title">
          <div class="mb-3 flex flex-wrap items-end justify-between gap-2">
            <h2 id="workspace-seeds-title" class="text-lg font-semibold text-text-primary">
              常用种子 / 查询记录
              <span class="ml-1 text-sm font-normal text-text-muted">{{ workspace.seeds.length }}</span>
            </h2>
            <RouterLink to="/seed-tools" class="text-xs text-grass-400 hover:underline">
              打开种子工具 →
            </RouterLink>
          </div>

          <form class="card mb-3 p-4" @submit.prevent="saveSeed">
            <div class="grid gap-3 md:grid-cols-12">
              <label class="md:col-span-3">
                <span class="mb-1 block text-xs font-medium text-text-secondary">名称（必填）</span>
                <input
                  v-model="seedForm.name"
                  :class="inputClass"
                  type="text"
                  placeholder="例如：生存主世界"
                />
              </label>
              <label class="md:col-span-3">
                <span class="mb-1 block text-xs font-medium text-text-secondary">种子（必填）</span>
                <input
                  v-model="seedForm.seed"
                  :class="inputClass"
                  type="text"
                  placeholder="例如：-4172144997902289642"
                />
              </label>
              <label class="md:col-span-4">
                <span class="mb-1 block text-xs font-medium text-text-secondary">备注（可选）</span>
                <input
                  v-model="seedForm.note"
                  :class="inputClass"
                  type="text"
                  placeholder="例如：村庄出生点"
                />
              </label>
              <div class="flex items-end md:col-span-2">
                <button
                  type="submit"
                  class="h-[38px] w-full rounded-md bg-grass-600 px-4 text-sm font-medium text-white transition hover:bg-grass-500 disabled:opacity-60"
                  :disabled="busy === 'seed'"
                >
                  保存种子
                </button>
              </div>
            </div>
          </form>

          <div v-if="workspace.seeds.length" class="card divide-y divide-border-dark">
            <div
              v-for="seed in workspace.seeds"
              :key="seed.id"
              class="flex items-center justify-between gap-3 px-4 py-2.5"
            >
              <div class="min-w-0">
                <p class="break-words text-sm text-text-primary">{{ seed.name }}</p>
                <p class="break-all font-mono text-xs text-text-muted">
                  {{ seed.seed }}<span v-if="seed.note"> · {{ seed.note }}</span>
                </p>
              </div>
              <button
                type="button"
                class="shrink-0 rounded-md border border-border-light bg-ink-700 px-2.5 py-1 text-xs text-text-secondary transition hover:bg-ink-600 hover:text-text-primary"
                @click="deleteSeed(seed.id)"
              >
                删除
              </button>
            </div>
          </div>
          <p v-else class="text-sm text-text-muted">暂无保存的种子，可在上方录入。</p>
        </section>
      </template>

      <!-- 7. 账户设置入口 -->
      <section aria-labelledby="workspace-settings-title">
        <h2 id="workspace-settings-title" class="sr-only">账户设置</h2>
        <RouterLink
          to="/settings"
          class="card card-hover group flex flex-col gap-3 p-5 sm:flex-row sm:items-center sm:justify-between"
        >
          <div class="flex items-center gap-3">
            <span class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg border border-border-light bg-ink-700 text-text-secondary">
              <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2" aria-hidden="true">
                <path stroke-linecap="round" stroke-linejoin="round" d="M15.75 6a3.75 3.75 0 1 1-7.5 0 3.75 3.75 0 0 1 7.5 0zM4.5 19.5a7.5 7.5 0 0 1 15 0v.75H4.5v-.75z" />
              </svg>
            </span>
            <div>
              <p class="text-sm font-semibold text-text-primary">账户设置</p>
              <p class="mt-0.5 text-xs text-text-secondary">
                管理个人资料与账号安全操作。
              </p>
            </div>
          </div>
          <span class="inline-flex h-9 shrink-0 items-center rounded-md border border-border-light px-3 text-sm text-text-secondary transition group-hover:text-text-primary">
            进入账户设置 →
          </span>
        </RouterLink>
      </section>
    </div>
  </PageContainer>
</template>
