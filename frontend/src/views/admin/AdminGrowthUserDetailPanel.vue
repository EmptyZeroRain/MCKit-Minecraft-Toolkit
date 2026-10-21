<script setup lang="ts">
// 用户成长详情（jingyan.md §10，后台 ops.growth）。
//
// 先搜索用户（含管理员），再按用户聚合：
//   * 等级：当前等级 / 称号 / 经验进度 / 人工覆盖；
//   * 成就：已解锁数量与每项解锁时间；
//   * 经验：当前 / 累计获得 / 累计冲正与最近流水；
//   * 礼物：礼物币余额与最近流水；
//   * 详细日志：成长审计（人工调整、成就授予 / 撤销等）。
//
// 每个分类与条目使用原生 details 做可展开的树状展示；明细接口返回最近 200 条，
// 同时给出总数，避免大用户一次拉取全部历史。
import { computed, onMounted, ref } from 'vue'
import Pagination from '@/components/common/Pagination.vue'
import { ApiError } from '@/types/api'
import type { AdminTableColumn } from '@/types/admin'
import AdminTable from '@/components/admin/AdminTable.vue'
import {
  fetchAdminGrowthUserDetail,
  fetchAdminGrowthUsers,
} from '@/api/growth'
import {
  LEDGER_SOURCE_LABELS,
  RARITY_LABELS,
  type AdminGrowthUserDetail,
  type AdminGrowthUserSummary,
} from '@/types/growth'
import { formatDateTime } from '@/utils/format'

const inputClass =
  'w-full rounded-md border border-border-dark bg-ink-900 px-3 py-2 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none'

const users = ref<AdminGrowthUserSummary[]>([])
const total = ref(0)
const totalPages = ref(1)
const page = ref(1)
const pageSize = 15
const q = ref('')
const loadingUsers = ref(false)
const usersError = ref('')

const selected = ref<AdminGrowthUserSummary | null>(null)
const detail = ref<AdminGrowthUserDetail | null>(null)
const loadingDetail = ref(false)
const detailError = ref('')

const columns: AdminTableColumn[] = [
  { key: 'username', label: '用户' },
  { key: 'level', label: '等级', width: '100px' },
  { key: 'current_xp', label: '当前经验', width: '110px' },
  { key: 'achievement_count', label: '成就', width: '80px' },
  { key: 'gift_balance', label: '礼物币', width: '100px' },
  { key: 'action', label: '', align: 'right', width: '90px' },
]

const progressWidth = computed(() => {
  const p = detail.value?.level_progress_percent ?? 0
  return `${Math.max(0, Math.min(100, p))}%`
})

const giftPositive = (amount: number) => amount >= 0
const roleLabel = (role: string) => (role === 'user' ? '' : role === 'admin' ? '管理员' : '后台账号')
const coinTypeLabel = (coinType: string) => (coinType === 'checkin' ? '签到币' : '通用币')
const sourceLabel = (source: string) => LEDGER_SOURCE_LABELS[source] ?? (source || '—')
const rarityLabel = (rarity: string) => RARITY_LABELS[rarity] ?? (rarity || '—')

async function loadUsers(reset = false) {
  if (reset) page.value = 1
  loadingUsers.value = true
  usersError.value = ''
  try {
    const res = await fetchAdminGrowthUsers({
      q: q.value.trim() || undefined,
      page: page.value,
      page_size: pageSize,
    })
    users.value = res.items
    total.value = res.total
    totalPages.value = res.totalPages || Math.max(1, Math.ceil(res.total / pageSize))
  } catch (err) {
    usersError.value = err instanceof ApiError ? err.message : '用户列表加载失败'
  } finally {
    loadingUsers.value = false
  }
}

async function loadDetail(user: AdminGrowthUserSummary) {
  selected.value = user
  loadingDetail.value = true
  detailError.value = ''
  detail.value = null
  try {
    detail.value = await fetchAdminGrowthUserDetail(user.user_id)
  } catch (err) {
    detailError.value = err instanceof ApiError ? err.message : '用户详情加载失败'
  } finally {
    loadingDetail.value = false
  }
}

function onPageChange(p: number) {
  page.value = p
  void loadUsers()
}

onMounted(() => loadUsers())
</script>

<template>
  <div class="space-y-5">
    <section class="card p-4">
      <form class="flex flex-wrap gap-3" @submit.prevent="loadUsers(true)">
        <input
          v-model="q"
          :class="inputClass"
          class="sm:max-w-md"
          type="search"
          placeholder="搜索用户名 / 昵称 / 用户编号"
        />
        <button
          type="submit"
          class="rounded-md border border-border-light bg-ink-700 px-4 py-2 text-sm text-text-secondary transition hover:bg-ink-600"
        >
          搜索用户
        </button>
        <span class="self-center text-xs text-text-muted">共 {{ total }} 个账号（含管理员）</span>
      </form>
    </section>

    <p
      v-if="usersError"
      class="rounded-card border border-danger-mc/40 bg-danger-mc/10 px-4 py-3 text-sm text-danger-mc"
      role="alert"
    >
      {{ usersError }}
      <button type="button" class="ml-2 underline" @click="loadUsers()">重试</button>
    </p>

    <AdminTable
      :columns="columns"
      :rows="users"
      :loading="loadingUsers"
      row-key="user_id"
      empty-text="没有匹配的用户"
    >
      <template #cell-username="{ row }">
        <div class="min-w-0">
          <p class="truncate font-medium text-text-primary">
            {{ row.nickname || row.username }}
            <span v-if="roleLabel(row.role)" class="ml-1 rounded bg-amber-500/15 px-1.5 py-0.5 text-[10px] text-amber-300">
              {{ roleLabel(row.role) }}
            </span>
            <span v-if="!row.is_active" class="ml-1 text-xs text-danger-mc">已停用</span>
          </p>
          <p class="truncate font-mono text-xs text-text-muted">
            {{ row.username }} · {{ row.user_no }}
          </p>
        </div>
      </template>
      <template #cell-level="{ row }">
        <span class="text-text-primary">Lv.{{ row.level }}</span>
        <span class="ml-1 text-xs text-text-muted">{{ row.level_title }}</span>
        <span v-if="row.is_overridden" class="ml-1 rounded bg-amber-500/15 px-1 py-0.5 text-[10px] text-amber-300">
          人工
        </span>
        <span v-else-if="row.level === 0" class="ml-1 text-[10px] text-text-muted">
          活跃 {{ row.active_days }}/{{ row.guest_active_days }} 天
        </span>
      </template>
      <template #cell-current_xp="{ row }">
        <span class="text-grass-300">{{ row.current_xp }}</span>
      </template>
      <template #cell-achievement_count="{ row }">
        <span>{{ row.achievement_count }}</span>
      </template>
      <template #cell-gift_balance="{ row }">
        <span class="text-amber-300">{{ row.gift_balance }}</span>
      </template>
      <template #cell-action="{ row }">
        <button
          type="button"
          class="rounded border border-border-light px-2 py-1 text-xs text-text-secondary transition hover:bg-ink-700"
          @click="loadDetail(row)"
        >
          查看详情
        </button>
      </template>
    </AdminTable>

    <Pagination
      :page="page"
      :page-size="pageSize"
      :total="total"
      :total-pages="totalPages"
      @change="onPageChange"
    />

    <p v-if="loadingDetail" class="text-sm text-text-muted">用户详情加载中…</p>
    <p
      v-else-if="detailError"
      class="rounded-card border border-danger-mc/40 bg-danger-mc/10 px-4 py-3 text-sm text-danger-mc"
      role="alert"
    >
      {{ detailError }}
      <button v-if="selected" type="button" class="ml-2 underline" @click="loadDetail(selected)">
        重试
      </button>
    </p>

    <section v-else-if="detail" class="card p-5">
      <header class="mb-4 flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 class="text-lg font-bold text-text-primary">
            {{ detail.user.nickname || detail.user.username }}
          </h2>
          <p class="mt-0.5 font-mono text-xs text-text-muted">
            {{ detail.user.username }} · {{ detail.user.user_no }} ·
            {{ roleLabel(detail.user.role) || '普通用户' }} ·
            {{ detail.user.is_active ? '正常' : '已停用' }}
          </p>
        </div>
        <p class="text-xs text-text-muted">
          成长投影更新：{{ formatDateTime(detail.user.updated_at || detail.user.created_at) }}
        </p>
      </header>

      <dl class="mb-5 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <div class="rounded-card border border-border-dark bg-ink-900/60 p-3">
          <dt class="text-xs text-text-muted">等级</dt>
          <dd class="mt-1 text-xl font-bold text-text-primary">
            Lv.{{ detail.user.level }}
            <span class="text-sm font-normal text-text-secondary">{{ detail.user.level_title }}</span>
          </dd>
          <dd class="text-xs text-text-muted">
            基础 Lv.{{ detail.base_level }}
            <span v-if="detail.level_override !== null">
              · 人工覆盖 Lv.{{ detail.level_override }}（经验不自动改级）
            </span>
            <span v-else-if="detail.user.level === 0">
              · 活跃 {{ detail.user.active_days }}/{{ detail.user.guest_active_days }} 天
            </span>
          </dd>
        </div>
        <div class="rounded-card border border-border-dark bg-ink-900/60 p-3">
          <dt class="text-xs text-text-muted">成就</dt>
          <dd class="mt-1 text-xl font-bold text-text-primary">
            {{ detail.user.achievement_count }}
            <span class="text-sm font-normal text-text-muted">/ {{ detail.achievement_total }}</span>
          </dd>
          <dd class="text-xs text-text-muted">已解锁 / 启用成就总数</dd>
        </div>
        <div class="rounded-card border border-border-dark bg-ink-900/60 p-3">
          <dt class="text-xs text-text-muted">当前经验</dt>
          <dd class="mt-1 text-xl font-bold text-grass-300">{{ detail.user.current_xp }}</dd>
          <dd class="text-xs text-text-muted">
            累计获得 {{ detail.user.lifetime_earned_xp }} · 冲正 {{ detail.user.lifetime_reversed_xp }}
          </dd>
        </div>
        <div class="rounded-card border border-border-dark bg-ink-900/60 p-3">
          <dt class="text-xs text-text-muted">礼物币</dt>
          <dd class="mt-1 text-xl font-bold text-amber-300">{{ detail.user.gift_balance }}</dd>
          <dd class="text-xs text-text-muted">
            通用 {{ detail.user.gift_general_balance }} · 签到 {{ detail.user.gift_checkin_balance }}
          </dd>
          <dd class="text-xs text-text-muted">共 {{ detail.gift_total }} 笔收支流水</dd>
        </div>
      </dl>

      <div class="space-y-3">
        <details open class="tree-group rounded-card border border-border-dark bg-ink-900/40">
          <summary class="cursor-pointer px-4 py-3 text-sm font-medium text-text-primary">
            🌟 等级与经验进度
          </summary>
          <div class="ml-5 border-l border-border-dark py-2 pl-4 pr-4">
            <div class="mb-1 flex items-center justify-between text-xs text-text-muted">
              <span>Lv.{{ detail.user.level }} · {{ detail.user.level_title }}</span>
              <span v-if="detail.level_override !== null">已人工覆盖，不自动升级</span>
              <span v-else-if="detail.user.level === 0">
                活跃 {{ detail.user.active_days }}/{{ detail.user.guest_active_days }} 天
              </span>
              <span v-else-if="detail.next_level_xp !== null">
                下一级需 {{ detail.xp_to_next_level }} 经验
              </span>
              <span v-else>已达最高等级</span>
            </div>
            <div class="h-2 overflow-hidden rounded-full bg-ink-800">
              <div class="h-full rounded-full bg-grass-500" :style="{ width: progressWidth }" />
            </div>
            <p class="mt-2 text-xs text-text-muted">
              等级进度 {{ detail.level_progress_percent }}%。
              等级仅用于展示，不影响任何权限。
              <template v-if="detail.level_override !== null">
                当前等级由后台人工覆盖，经验继续累积但不会自动改变等级。
              </template>
              <template v-else-if="detail.user.level === 0">
                当前为访客等级，已攒 {{ detail.user.current_xp }} 经验；累计满
                {{ detail.user.guest_active_days }} 个活跃日后进入 Lv.1，经验自动生效。
              </template>
              <template v-else>等级进度会随经验变化自动更新。</template>
            </p>
          </div>
        </details>

        <details open class="tree-group rounded-card border border-border-dark bg-ink-900/40">
          <summary class="cursor-pointer px-4 py-3 text-sm font-medium text-text-primary">
            🏆 成就（{{ detail.achievements.length }} / {{ detail.achievement_total }}）
          </summary>
          <div class="ml-5 border-l border-border-dark py-2 pl-4 pr-4">
            <p v-if="!detail.achievements.length" class="text-xs text-text-muted">暂无已解锁成就。</p>
            <details
              v-for="item in detail.achievements"
              :key="item.id"
              class="mb-2 rounded border border-border-dark bg-ink-900/60"
            >
              <summary class="cursor-pointer px-3 py-2 text-sm text-text-primary">
                {{ item.name }}
                <span class="ml-2 text-xs text-text-muted">{{ formatDateTime(item.unlocked_at) }}</span>
              </summary>
              <div class="border-t border-border-dark px-3 py-2 text-xs text-text-secondary">
                <p>{{ item.description || '—' }}</p>
                <p class="mt-1 text-text-muted">
                  {{ item.category || '未分类' }} · {{ rarityLabel(item.rarity) }} ·
                  奖励 {{ item.reward_xp }} 经验 · {{ item.grant_source === 'manual' ? '人工授予' : '自动解锁' }}
                </p>
              </div>
            </details>
          </div>
        </details>

        <details open class="tree-group rounded-card border border-border-dark bg-ink-900/40">
          <summary class="cursor-pointer px-4 py-3 text-sm font-medium text-text-primary">
            📈 经验数据（最近 {{ detail.experience.length }} / 共 {{ detail.experience_total }} 笔）
          </summary>
          <div class="ml-5 border-l border-border-dark py-2 pl-4 pr-4">
            <p v-if="!detail.experience.length" class="text-xs text-text-muted">暂无经验流水。</p>
            <details
              v-for="item in detail.experience"
              :key="item.id"
              class="mb-2 rounded border border-border-dark bg-ink-900/60"
            >
              <summary class="cursor-pointer px-3 py-2 text-sm">
                <span :class="item.delta_xp >= 0 ? 'text-grass-300' : 'text-danger-mc'">
                  {{ item.delta_xp >= 0 ? '+' : '' }}{{ item.delta_xp }} XP
                </span>
                <span class="ml-2 text-text-secondary">{{ item.reason || sourceLabel(item.source_type) }}</span>
                <span class="ml-2 text-xs text-text-muted">{{ formatDateTime(item.created_at) }}</span>
              </summary>
              <div class="border-t border-border-dark px-3 py-2 text-xs text-text-secondary">
                <p>来源：{{ sourceLabel(item.source_type) }}</p>
                <p class="mt-1 text-text-muted">
                  变动后余额 {{ item.balance_after }} XP · 来源 ID {{ item.source_id || '—' }} ·
                  流水 {{ item.id }}
                </p>
              </div>
            </details>
          </div>
        </details>

        <details open class="tree-group rounded-card border border-border-dark bg-ink-900/40">
          <summary class="cursor-pointer px-4 py-3 text-sm font-medium text-text-primary">
            🎁 礼物数据（余额 {{ detail.user.gift_balance }} · 最近 {{ detail.gifts.length }} / 共 {{ detail.gift_total }} 笔）
          </summary>
          <div class="ml-5 border-l border-border-dark py-2 pl-4 pr-4">
            <p v-if="!detail.gifts.length" class="text-xs text-text-muted">暂无礼物币流水。</p>
            <details
              v-for="item in detail.gifts"
              :key="item.id"
              class="mb-2 rounded border border-border-dark bg-ink-900/60"
            >
              <summary class="cursor-pointer px-3 py-2 text-sm">
                <span :class="giftPositive(item.amount) ? 'text-amber-300' : 'text-danger-mc'">
                  {{ item.amount >= 0 ? '+' : '' }}{{ item.amount }} 礼物币
                </span>
                <span class="ml-2 text-text-secondary">{{ item.reason || item.event_type }}</span>
                <span class="ml-2 text-xs text-text-muted">{{ formatDateTime(item.created_at) }}</span>
              </summary>
              <div class="border-t border-border-dark px-3 py-2 text-xs text-text-secondary">
                <p>币种：{{ coinTypeLabel(item.coin_type) }} · 事件：{{ item.event_type }} · 来源：{{ item.source_type }}</p>
                <p class="mt-1 text-text-muted">
                  变动后余额 {{ item.balance_after }} · 来源 ID {{ item.source_id || '—' }} ·
                  流水 {{ item.id }}
                </p>
              </div>
            </details>
          </div>
        </details>

        <details open class="tree-group rounded-card border border-border-dark bg-ink-900/40">
          <summary class="cursor-pointer px-4 py-3 text-sm font-medium text-text-primary">
            📋 详细日志（最近 {{ detail.audits.length }} / 共 {{ detail.audit_total }} 条）
          </summary>
          <div class="ml-5 border-l border-border-dark py-2 pl-4 pr-4">
            <p v-if="!detail.audits.length" class="text-xs text-text-muted">暂无成长审计日志。</p>
            <details
              v-for="item in detail.audits"
              :key="item.id"
              class="mb-2 rounded border border-border-dark bg-ink-900/60"
            >
              <summary class="cursor-pointer px-3 py-2 text-sm">
                <span class="font-mono text-xs text-text-primary">{{ item.action }}</span>
                <span class="ml-2 text-text-secondary">{{ item.reason || '—' }}</span>
                <span class="ml-2 text-xs text-text-muted">{{ formatDateTime(item.created_at) }}</span>
              </summary>
              <div class="border-t border-border-dark px-3 py-2 text-xs text-text-secondary">
                <p>对象：{{ item.entity_type }}{{ item.entity_id ? ` · ${item.entity_id}` : '' }}</p>
                <p class="mt-1 text-text-muted">操作者 {{ item.operator_user_id || '系统' }} · 日志 {{ item.id }}</p>
              </div>
            </details>
          </div>
        </details>
      </div>
    </section>
  </div>
</template>

<style scoped>
.tree-group summary {
  list-style-position: inside;
}
</style>
