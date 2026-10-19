<script setup lang="ts">
// 成长中心（jingyan.md §11.1 / §11.2）。
//
// 原则：经验 / 等级 / 任务 / 成就状态全部由服务端判定，前端只做展示与拉取。
// 不提供「领取经验」按钮——首期每日任务达到目标即由服务端自动发奖。
import { computed, onMounted, ref } from 'vue'
import PageContainer from '@/components/layout/PageContainer.vue'
import Breadcrumbs from '@/components/layout/Breadcrumbs.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import LoadingSkeleton from '@/components/common/LoadingSkeleton.vue'
import { ApiError } from '@/types/api'
import {
  fetchGrowthAchievements,
  fetchGrowthDailyTasks,
  fetchGrowthLedger,
  fetchGrowthOverview,
} from '@/api/growth'
import {
  DAILY_TASK_STATUS_LABELS,
  LEDGER_SOURCE_LABELS,
  RARITY_LABELS,
  type GrowthAchievement,
  type GrowthDailyTask,
  type GrowthDailyTasks,
  type GrowthLedgerItem,
  type GrowthOverview,
} from '@/types/growth'
import { useUserAuthStore } from '@/stores/user-auth'
import { applyPageMeta } from '@/utils/seo'

applyPageMeta({
  title: '成长中心',
  description: '查看你的等级、经验、每日任务与成就进度。',
  canonicalPath: '/growth',
  robot: 'noindex,nofollow',
})

const userAuth = useUserAuthStore()

const loading = ref(true)
const error = ref('')
const overview = ref<GrowthOverview | null>(null)
const daily = ref<GrowthDailyTasks | null>(null)
const ledger = ref<GrowthLedgerItem[]>([])
const achievements = ref<GrowthAchievement[]>([])

const avatarBroken = ref(false)
const avatarUrl = computed(() => userAuth.user?.avatar_url ?? '')
const avatarFallback = computed(() => userAuth.displayName.charAt(0).toUpperCase() || '?')

/** 今日任务：进行中排前，已完成排后 */
const dailyTasks = computed<GrowthDailyTask[]>(() => daily.value?.tasks ?? [])

/** 最近解锁成就（已解锁优先，按解锁时间倒序，取前 6 条） */
const recentAchievements = computed(() =>
  achievements.value
    .filter((a) => a.unlocked)
    .sort((a, b) => (b.unlocked_at ?? '').localeCompare(a.unlocked_at ?? ''))
    .slice(0, 6),
)

/** 成就完成比例（0-100） */
const achievementPercent = computed(() => {
  const total = overview.value?.achievement_total ?? 0
  if (total <= 0) return 0
  return Math.round(((overview.value?.unlocked_achievements ?? 0) * 100) / total)
})

function sourceLabel(item: GrowthLedgerItem): string {
  return LEDGER_SOURCE_LABELS[item.source_type] ?? item.source_type
}

function taskStatusLabel(status: string): string {
  return DAILY_TASK_STATUS_LABELS[status] ?? status
}

function rarityLabel(rarity: string): string {
  return RARITY_LABELS[rarity] ?? rarity
}

function formatTime(iso: string | null | undefined): string {
  if (!iso) return '—'
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '—' : d.toLocaleString('zh-CN')
}

function errText(e: unknown, fallback: string): string {
  return e instanceof ApiError ? e.message : fallback
}

onMounted(async () => {
  if (!userAuth.initialized) await userAuth.restoreSession()
  try {
    const [ov, dt, lg, ach] = await Promise.all([
      fetchGrowthOverview(),
      fetchGrowthDailyTasks(),
      fetchGrowthLedger({ page: 1, page_size: 10 }),
      fetchGrowthAchievements(),
    ])
    overview.value = ov
    daily.value = dt
    ledger.value = lg.items
    achievements.value = ach
  } catch (e) {
    error.value = errText(e, '成长数据加载失败')
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <PageContainer maxWidth="max-w-4xl">
    <Breadcrumbs :items="[{ label: '成长中心' }]" />
    <div class="mb-6">
      <h1 class="mb-1 text-2xl font-bold text-text-primary md:text-3xl">成长中心</h1>
      <p class="text-sm text-text-secondary">
        经验、等级、任务与成就均由服务端判定，此处仅作展示。
      </p>
    </div>

    <p
      v-if="error"
      class="mb-4 rounded-card border border-danger-mc/40 bg-danger-mc/10 px-4 py-3 text-sm text-danger-mc"
      role="alert"
    >
      {{ error }}
    </p>

    <LoadingSkeleton v-if="loading" :count="3" />

    <template v-else-if="overview">
      <!-- 等级与经验 -->
      <section class="card mb-6 p-5">
        <div class="flex flex-col gap-4 sm:flex-row sm:items-center">
          <img
            v-if="avatarUrl && !avatarBroken"
            :src="avatarUrl"
            :alt="userAuth.displayName"
            class="h-16 w-16 shrink-0 rounded-full border border-border-dark object-cover"
            @error="avatarBroken = true"
          />
          <div
            v-else
            class="flex h-16 w-16 shrink-0 items-center justify-center rounded-full border border-border-dark bg-ink-800 text-xl font-semibold text-text-muted"
            aria-hidden="true"
          >
            {{ avatarFallback }}
          </div>
          <div class="min-w-0 flex-1">
            <div class="flex flex-wrap items-baseline gap-2">
              <span class="text-lg font-bold text-text-primary">{{ userAuth.displayName }}</span>
              <span class="rounded-full border border-grass-500/40 bg-grass-500/10 px-2 py-0.5 text-xs text-grass-300">
                Lv.{{ overview.level }} · {{ overview.level_title }}
              </span>
            </div>
            <p class="mt-1 text-sm text-text-secondary">
              当前有效经验 <span class="font-semibold text-text-primary">{{ overview.current_xp }}</span>
              · 今日获得 <span class="font-semibold text-grass-300">{{ overview.today_xp }}</span>
            </p>
          </div>
        </div>

        <!-- 升级进度条 -->
        <div class="mt-5">
          <div class="mb-1.5 flex items-center justify-between text-xs text-text-muted">
            <span v-if="overview.is_max_level">已达最高等级</span>
            <span v-else>距下一级还需 {{ overview.xp_to_next_level }} 经验</span>
            <span>{{ overview.level_progress_percent }}%</span>
          </div>
          <div class="h-2.5 w-full overflow-hidden rounded-full bg-ink-800">
            <div
              class="h-full rounded-full bg-grass-500 transition-all"
              :style="{ width: `${overview.level_progress_percent}%` }"
            />
          </div>
        </div>

        <dl class="mt-5 grid grid-cols-2 gap-3 text-sm sm:grid-cols-4">
          <div class="rounded-md bg-ink-800/60 p-3">
            <dt class="text-xs text-text-muted">累计获得</dt>
            <dd class="mt-0.5 font-semibold text-text-primary">{{ overview.lifetime_earned_xp }}</dd>
          </div>
          <div class="rounded-md bg-ink-800/60 p-3">
            <dt class="text-xs text-text-muted">累计冲正</dt>
            <dd class="mt-0.5 font-semibold text-text-primary">{{ overview.lifetime_reversed_xp }}</dd>
          </div>
          <div class="rounded-md bg-ink-800/60 p-3">
            <dt class="text-xs text-text-muted">今日任务</dt>
            <dd class="mt-0.5 font-semibold text-text-primary">
              {{ overview.today_tasks_completed }} / {{ overview.today_tasks_total }}
            </dd>
          </div>
          <div class="rounded-md bg-ink-800/60 p-3">
            <dt class="text-xs text-text-muted">成就解锁</dt>
            <dd class="mt-0.5 font-semibold text-text-primary">
              {{ overview.unlocked_achievements }} / {{ overview.achievement_total }}
            </dd>
          </div>
        </dl>
      </section>

      <!-- 今日任务 -->
      <section class="mb-6">
        <div class="mb-3 flex items-center justify-between">
          <h2 class="text-lg font-bold text-text-primary">今日任务</h2>
          <span v-if="daily" class="text-xs text-text-muted">
            {{ daily.date }}（{{ daily.timezone }}）
          </span>
        </div>
        <div v-if="dailyTasks.length" class="space-y-3">
          <div v-for="task in dailyTasks" :key="task.code" class="card p-4">
            <div class="flex items-start justify-between gap-3">
              <div class="min-w-0">
                <p class="text-sm font-medium text-text-primary">{{ task.name }}</p>
                <p v-if="task.description" class="mt-0.5 text-xs text-text-muted">{{ task.description }}</p>
              </div>
              <span
                class="shrink-0 rounded-full px-2 py-0.5 text-xs"
                :class="
                  taskStatusLabel(task.status) === '已完成'
                    ? 'bg-grass-500/10 text-grass-300'
                    : 'bg-ink-700 text-text-secondary'
                "
              >
                {{ taskStatusLabel(task.status) }}
              </span>
            </div>
            <div class="mt-3">
              <div class="mb-1 flex items-center justify-between text-xs text-text-muted">
                <span>进度 {{ Math.min(task.progress, task.target) }} / {{ task.target }}</span>
                <span>奖励 +{{ task.reward_xp }} 经验</span>
              </div>
              <div class="h-2 w-full overflow-hidden rounded-full bg-ink-800">
                <div
                  class="h-full rounded-full bg-grass-500 transition-all"
                  :style="{ width: `${task.target > 0 ? Math.min(100, Math.round((task.progress * 100) / task.target)) : 0}%` }"
                />
              </div>
              <p v-if="task.completed_at" class="mt-1.5 text-xs text-text-muted">
                完成于 {{ formatTime(task.completed_at) }}
              </p>
            </div>
          </div>
        </div>
        <EmptyState v-else title="今日暂无任务" description="站点暂未配置可用的每日任务。" />
      </section>

      <!-- 最近经验流水 -->
      <section class="mb-6">
        <h2 class="mb-3 text-lg font-bold text-text-primary">最近经验流水</h2>
        <div v-if="ledger.length" class="card divide-y divide-border-dark">
          <div v-for="item in ledger" :key="item.id" class="flex items-center justify-between gap-3 px-4 py-2.5">
            <div class="min-w-0">
              <p class="truncate text-sm text-text-primary">{{ item.reason || sourceLabel(item) }}</p>
              <p class="text-xs text-text-muted">
                {{ sourceLabel(item) }}{{ item.rule_code ? ` · ${item.rule_code}` : '' }} ·
                {{ formatTime(item.created_at) }}
              </p>
            </div>
            <span
              class="shrink-0 text-sm font-semibold"
              :class="item.delta_xp >= 0 ? 'text-grass-300' : 'text-danger-mc'"
            >
              {{ item.delta_xp >= 0 ? '+' : '' }}{{ item.delta_xp }}
            </span>
          </div>
        </div>
        <EmptyState v-else title="暂无经验记录" description="完成业务行为后，经验流水会显示在这里。" />
      </section>

      <!-- 最近成就 -->
      <section>
        <div class="mb-3 flex items-center justify-between">
          <h2 class="text-lg font-bold text-text-primary">最近解锁成就</h2>
          <span class="text-xs text-text-muted">
            完成比例 {{ achievementPercent }}%（{{ overview.unlocked_achievements }}/{{ overview.achievement_total }}）
          </span>
        </div>
        <div v-if="recentAchievements.length" class="grid gap-3 sm:grid-cols-2">
          <div v-for="ach in recentAchievements" :key="ach.code" class="card flex items-start gap-3 p-4">
            <img
              v-if="ach.icon_url"
              :src="ach.icon_url"
              :alt="ach.name"
              class="h-10 w-10 shrink-0 rounded-md object-cover"
            />
            <div
              v-else
              class="flex h-10 w-10 shrink-0 items-center justify-center rounded-md bg-ink-800 text-lg"
              aria-hidden="true"
            >
              🏆
            </div>
            <div class="min-w-0">
              <p class="text-sm font-medium text-text-primary">{{ ach.name }}</p>
              <p class="mt-0.5 text-xs text-text-muted">
                {{ rarityLabel(ach.rarity) }} · {{ formatTime(ach.unlocked_at) }}
              </p>
            </div>
          </div>
        </div>
        <EmptyState v-else title="尚未解锁成就" description="继续探索，达成条件后成就会在这里揭晓。" />
      </section>
    </template>
  </PageContainer>
</template>
