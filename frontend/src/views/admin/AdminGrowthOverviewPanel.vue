<script setup lang="ts">
// 成长概览（jingyan.md §10.1）。
//
// 统计口径说明（与后端 repo.Stats 一致，避免把事件数量等同于奖励笔数）：
//   - 今日 / 近 7 日发放经验：仅统计 delta_xp > 0 的正向流水合计；
//   - 今日奖励笔数：今日正向流水条数；
//   - 冲正经验：source_type=reversal 的负向流水绝对值合计；
//   - 人工调整笔数：source_type=manual 的流水条数；
//   - 成就解锁率：累计解锁数 ÷（活跃成长用户数 × 启用成就数）。
import { computed, onMounted, ref } from 'vue'
import { ApiError } from '@/types/api'
import { fetchAdminGrowthOverview } from '@/api/growth'
import type { AdminGrowthOverview } from '@/types/growth'

const loading = ref(true)
const error = ref('')
const data = ref<AdminGrowthOverview | null>(null)

const levelDistribution = computed(() => data.value?.level_distribution ?? [])

const maxLevelCount = computed(() =>
  levelDistribution.value.reduce((max, b) => Math.max(max, b.count), 0),
)

/** 成就解锁率（0-100，口径见文件头注释） */
const unlockRate = computed(() => {
  const d = data.value
  if (!d) return 0
  const denom = d.active_users * d.achievement_count
  if (denom <= 0) return 0
  return Math.round((d.unlocked_total * 100) / denom)
})

async function load() {
  loading.value = true
  error.value = ''
  try {
    data.value = await fetchAdminGrowthOverview()
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '概览加载失败'
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <div>
    <p
      v-if="error"
      class="mb-4 rounded-card border border-danger-mc/40 bg-danger-mc/10 px-4 py-3 text-sm text-danger-mc"
      role="alert"
    >
      {{ error }}
      <button type="button" class="ml-2 underline" @click="load">重试</button>
    </p>

    <div v-if="loading" class="text-sm text-text-muted">加载中…</div>

    <template v-else-if="data">
      <p class="mb-4 text-xs text-text-muted">
        业务日期 {{ data.date }}（{{ data.timezone }}）；经验按业务时区切分，数值为服务端聚合结果。
      </p>

      <dl class="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <div class="card p-4">
          <dt class="text-xs text-text-muted">活跃成长用户数</dt>
          <dd class="mt-1 text-2xl font-bold text-text-primary">{{ data.active_users }}</dd>
        </div>
        <div class="card p-4">
          <dt class="text-xs text-text-muted">今日发放经验</dt>
          <dd class="mt-1 text-2xl font-bold text-grass-300">{{ data.xp_issued_today }}</dd>
        </div>
        <div class="card p-4">
          <dt class="text-xs text-text-muted">近 7 日发放经验</dt>
          <dd class="mt-1 text-2xl font-bold text-grass-300">{{ data.xp_issued_7days }}</dd>
        </div>
        <div class="card p-4">
          <dt class="text-xs text-text-muted">今日奖励笔数</dt>
          <dd class="mt-1 text-2xl font-bold text-text-primary">{{ data.ledger_today }}</dd>
        </div>
        <div class="card p-4">
          <dt class="text-xs text-text-muted">今日任务参与人数</dt>
          <dd class="mt-1 text-2xl font-bold text-text-primary">{{ data.task_participants_today }}</dd>
        </div>
        <div class="card p-4">
          <dt class="text-xs text-text-muted">今日任务完成数</dt>
          <dd class="mt-1 text-2xl font-bold text-text-primary">{{ data.tasks_completed_today }}</dd>
        </div>
        <div class="card p-4">
          <dt class="text-xs text-text-muted">累计成就解锁</dt>
          <dd class="mt-1 text-2xl font-bold text-text-primary">
            {{ data.unlocked_total }}
            <span class="text-sm font-normal text-text-muted">/ {{ data.achievement_count }} 种</span>
          </dd>
          <dd class="mt-0.5 text-xs text-text-muted">解锁率 {{ unlockRate }}%</dd>
        </div>
        <div class="card p-4">
          <dt class="text-xs text-text-muted">冲正 / 人工调整</dt>
          <dd class="mt-1 text-2xl font-bold text-text-primary">
            {{ data.reversed_total }}
            <span class="text-sm font-normal text-text-muted">/ {{ data.manual_adjust_count }} 笔</span>
          </dd>
        </div>
      </dl>

      <!-- 等级分布 -->
      <section class="card mt-6 p-5">
        <div class="mb-3 flex items-center justify-between">
          <h2 class="text-sm font-medium text-text-secondary">等级分布</h2>
          <span class="text-xs text-text-muted">规则 {{ data.total_rules }} 条 · 任务 {{ data.total_tasks }} 个</span>
        </div>
        <ul v-if="levelDistribution.length" class="space-y-2">
          <li v-for="bucket in levelDistribution" :key="bucket.level" class="flex items-center gap-3">
            <span class="w-14 shrink-0 text-xs text-text-muted">Lv.{{ bucket.level }}</span>
            <div class="h-2.5 flex-1 overflow-hidden rounded-full bg-ink-800">
              <div
                class="h-full rounded-full bg-grass-500"
                :style="{ width: `${maxLevelCount > 0 ? Math.round((bucket.count * 100) / maxLevelCount) : 0}%` }"
              />
            </div>
            <span class="w-12 shrink-0 text-right text-xs text-text-secondary">{{ bucket.count }}</span>
          </li>
        </ul>
        <p v-else class="text-sm text-text-muted">暂无等级分布数据。</p>
      </section>

      <p class="mt-4 text-xs text-text-muted">
        口径：发放经验仅计正向流水；冲正以负向流水表达，原始流水不删除；等级仅作展示，不影响任何权限。
      </p>
    </template>
  </div>
</template>
