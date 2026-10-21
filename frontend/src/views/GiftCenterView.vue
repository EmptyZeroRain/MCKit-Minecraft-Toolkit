<script setup lang="ts">
// 礼物中心（GiftCoins.md）：签到领签到币、用礼物币兑换礼物、查看流水。
//
// 所有余额变化由服务端判定并写账本；前端只展示与发起请求。
import { computed, onMounted, ref } from 'vue'
import PageContainer from '@/components/layout/PageContainer.vue'
import Breadcrumbs from '@/components/layout/Breadcrumbs.vue'
import LoadingSkeleton from '@/components/common/LoadingSkeleton.vue'
import Pagination from '@/components/common/Pagination.vue'
import { ApiError } from '@/types/api'
import {
  checkinGift,
  fetchGiftCatalog,
  fetchGiftLedger,
  fetchGiftOverview,
  fetchGiftRedemptions,
  purchaseMakeupCard,
  redeemGift,
  useMakeupCard,
} from '@/api/gift'
import type {
  GiftCatalogItem,
  GiftOverview,
  GiftPublicLedgerItem,
  GiftRedemption,
  GiftWallet,
} from '@/types/gift'
import { useUserAuthStore } from '@/stores/user-auth'
import { applyPageMeta } from '@/utils/seo'

applyPageMeta({
  title: '礼物中心',
  description: '每日签到领取签到礼物币，兑换 Minecraft 风格虚拟礼物。',
  canonicalPath: '/gifts',
  robot: 'noindex,nofollow',
})

const userAuth = useUserAuthStore()

const loading = ref(true)
const error = ref('')
const notice = ref('')

const overview = ref<GiftOverview | null>(null)
const catalog = ref<GiftCatalogItem[]>([])
const checkingIn = ref(false)
const redeemingId = ref('')
const buyingMakeup = ref(false)
const usingMakeup = ref(false)

const redemptions = ref<GiftRedemption[]>([])
const redemptionTotal = ref(0)
const redemptionPage = ref(1)
const redemptionTotalPages = ref(1)

const ledger = ref<GiftPublicLedgerItem[]>([])
const ledgerTotal = ref(0)
const ledgerPage = ref(1)
const ledgerTotalPages = ref(1)
const coinFilter = ref<'' | 'general' | 'checkin'>('')

const checkinDisabled = computed(
  () => checkingIn.value || !overview.value || overview.value.checked_in_today,
)

function formatTime(iso: string): string {
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString('zh-CN')
}

function coinLabel(coinType: string): string {
  return coinType === 'checkin' ? '签到币' : '通用币'
}

function eventLabel(eventType: string): string {
  switch (eventType) {
    case 'admin_adjustment':
      return '后台调整'
    case 'daily_checkin':
      return '每日签到'
    case 'gift_redeem':
      return '兑换礼物'
    case 'makeup_card_purchase':
      return '购买补签卡'
    default:
      return eventType
  }
}

const canBuyMakeupCard = computed(
  () =>
    !!overview.value &&
    !buyingMakeup.value &&
    overview.value.makeup_card_price > 0 &&
    overview.value.checkin_balance >= overview.value.makeup_card_price,
)

const canUseMakeupCard = computed(
  () =>
    !!overview.value &&
    !usingMakeup.value &&
    overview.value.makeup_cards > 0 &&
    !!overview.value.latest_missed_date,
)

function xpBreakdownText(): string {
  if (!overview.value) return ''
  const parts = [`基础 +${overview.value.checkin_base_xp}`]
  if (overview.value.checkin_streak_xp > 0) parts.push(`连续 +${overview.value.checkin_streak_xp}`)
  if (overview.value.checkin_milestone_xp > 0) {
    parts.push(`里程碑 +${overview.value.checkin_milestone_xp}`)
  }
  if (overview.value.checkin_penalty_xp > 0) {
    parts.push(`断签 -${overview.value.checkin_penalty_xp}`)
  }
  return parts.join(' / ')
}

function newRequestKey(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }
  return `redeem-${Date.now()}-${Math.random().toString(36).slice(2)}`
}

function splitText(item: GiftCatalogItem): string {
  if (!overview.value) return `消耗 ${item.price} 礼物币`
  if (!item.allow_checkin) return `消耗 ${item.price} 通用币`
  const checkin = Math.min(overview.value.checkin_balance, item.price)
  const general = item.price - checkin
  if (checkin <= 0) return `消耗 ${general} 通用币`
  if (general <= 0) return `消耗 ${checkin} 签到币`
  return `消耗 ${checkin} 签到币 + ${general} 通用币`
}

function applyWallet(wallet: GiftWallet): void {
  if (!overview.value) return
  overview.value = {
    ...overview.value,
    general_balance: wallet.general_balance,
    checkin_balance: wallet.checkin_balance,
    total_balance: wallet.total_balance,
    updated_at: wallet.updated_at,
  }
}

async function loadOverview(): Promise<void> {
  overview.value = await fetchGiftOverview()
}

async function loadCatalog(): Promise<void> {
  catalog.value = await fetchGiftCatalog()
}

async function loadRedemptions(page = redemptionPage.value): Promise<void> {
  const res = await fetchGiftRedemptions({ page, page_size: 10 })
  redemptions.value = res.items
  redemptionTotal.value = res.total
  redemptionPage.value = res.page
  redemptionTotalPages.value = res.totalPages
}

async function loadLedger(page = ledgerPage.value): Promise<void> {
  const res = await fetchGiftLedger({
    coin_type: coinFilter.value || undefined,
    page,
    page_size: 10,
  })
  ledger.value = res.items
  ledgerTotal.value = res.total
  ledgerPage.value = res.page
  ledgerTotalPages.value = res.totalPages
}

async function loadAll(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    if (!userAuth.initialized) await userAuth.restoreSession()
    await Promise.all([loadOverview(), loadCatalog(), loadRedemptions(1), loadLedger(1)])
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '礼物中心加载失败'
  } finally {
    loading.value = false
  }
}

async function onCheckin(): Promise<void> {
  error.value = ''
  notice.value = ''
  checkingIn.value = true
  try {
    const result = await checkinGift()
    applyWallet(result.wallet)
    const xpText =
      result.xp > 0
        ? `，获得 ${result.xp} 经验（连续 ${result.checkin_streak} 天）`
        : `，连续 ${result.checkin_streak} 天`
    notice.value =
      result.reward > 0
        ? `签到成功，获得 ${result.reward} 签到礼物币${xpText}。`
        : `签到成功；今日签到奖励为 0${xpText}。`
    await Promise.all([loadOverview(), loadCatalog(), loadLedger(1)])
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '签到失败'
  } finally {
    checkingIn.value = false
  }
}

async function onBuyMakeupCard(): Promise<void> {
  error.value = ''
  notice.value = ''
  if (!overview.value) return
  if (!canBuyMakeupCard.value) {
    error.value = `签到礼物币不足，购买补签卡需要 ${overview.value.makeup_card_price} 签到币。`
    return
  }
  if (
    !window.confirm(
      `确定花费 ${overview.value.makeup_card_price} 签到礼物币购买 1 张补签卡吗？`,
    )
  ) {
    return
  }
  buyingMakeup.value = true
  try {
    const result = await purchaseMakeupCard(newRequestKey())
    applyWallet(result.wallet)
    notice.value = `购买成功，当前持有 ${result.makeup_cards} 张补签卡。`
    await Promise.all([loadOverview(), loadLedger(1)])
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '购买补签卡失败'
  } finally {
    buyingMakeup.value = false
  }
}

async function onUseMakeupCard(): Promise<void> {
  error.value = ''
  notice.value = ''
  if (!overview.value) return
  if (!canUseMakeupCard.value) {
    error.value = overview.value.makeup_cards <= 0 ? '没有可用的补签卡。' : '当前没有可补签的漏签日。'
    return
  }
  const date = overview.value.latest_missed_date
  if (!window.confirm(`确定使用 1 张补签卡补 ${date} 的经验吗？补签不补签到天数。`)) {
    return
  }
  usingMakeup.value = true
  try {
    const result = await useMakeupCard(newRequestKey())
    applyWallet(result.wallet)
    notice.value = `补签 ${result.xp_date} 成功，获得 ${result.xp} 经验；剩余 ${result.makeup_cards} 张补签卡。`
    await Promise.all([loadOverview(), loadLedger(1)])
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '使用补签卡失败'
  } finally {
    usingMakeup.value = false
  }
}

async function onRedeem(item: GiftCatalogItem): Promise<void> {
  error.value = ''
  notice.value = ''
  if (!item.can_afford) {
    error.value = '礼物币不足，无法兑换该礼物。'
    return
  }
  if (!window.confirm(`确定兑换「${item.name}」吗？${splitText(item)}。`)) {
    return
  }
  redeemingId.value = item.id
  try {
    const result = await redeemGift(item.id, newRequestKey())
    applyWallet(result.wallet)
    const spent: string[] = []
    if (result.redemption.checkin_spent > 0) spent.push(`${result.redemption.checkin_spent} 签到币`)
    if (result.redemption.general_spent > 0) spent.push(`${result.redemption.general_spent} 通用币`)
    notice.value = `成功兑换「${result.redemption.gift_name}」，消耗 ${spent.join(' + ')}。`
    await Promise.all([loadCatalog(), loadRedemptions(1), loadLedger(1)])
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '兑换失败'
  } finally {
    redeemingId.value = ''
  }
}

function onLedgerFilter(): void {
  void loadLedger(1)
}

function changeRedemptionPage(p: number): void {
  void loadRedemptions(p)
}

function changeLedgerPage(p: number): void {
  void loadLedger(p)
}

onMounted(loadAll)
</script>

<template>
  <PageContainer maxWidth="max-w-5xl">
    <Breadcrumbs :items="[{ label: '礼物中心' }]" />
    <div class="mb-6 flex flex-wrap items-start justify-between gap-3">
      <div>
        <h1 class="mb-1 text-2xl font-bold text-text-primary md:text-3xl">礼物中心</h1>
        <p class="text-sm text-text-secondary">
          每日签到领取签到礼物币，兑换礼物；礼物币不可提现、不可转账。
        </p>
      </div>
      <RouterLink
        to="/workspace"
        class="rounded-md border border-border-dark bg-ink-800 px-3 py-2 text-sm text-text-secondary transition hover:bg-ink-700"
      >
        返回工作台
      </RouterLink>
    </div>

    <p v-if="error" class="mb-4 rounded-card border border-danger-mc/40 bg-danger-mc/10 px-4 py-3 text-sm text-danger-mc" role="alert">{{ error }}</p>
    <p v-if="notice" class="mb-4 rounded-card border border-grass-500/40 bg-grass-500/10 px-4 py-3 text-sm text-grass-300" role="status">{{ notice }}</p>

    <LoadingSkeleton v-if="loading" :count="3" />

    <template v-else-if="overview">
      <!-- 余额 + 签到 -->
      <section class="card mb-6 p-5">
        <div class="flex flex-col gap-5 lg:flex-row lg:items-center lg:justify-between">
          <div class="grid flex-1 gap-4 sm:grid-cols-3">
            <div>
              <p class="text-xs text-text-muted">礼物币合计</p>
              <p class="mt-1 text-2xl font-bold text-grass-300">{{ overview.total_balance }}</p>
            </div>
            <div>
              <p class="text-xs text-text-muted">签到礼物币</p>
              <p class="mt-1 text-2xl font-bold text-text-primary">{{ overview.checkin_balance }}</p>
              <p class="text-[11px] text-text-muted">仅可兑换标记「签到币可兑」的礼物</p>
            </div>
            <div>
              <p class="text-xs text-text-muted">通用礼物币</p>
              <p class="mt-1 text-2xl font-bold text-text-primary">{{ overview.general_balance }}</p>
              <p class="text-[11px] text-text-muted">可兑换全部礼物</p>
            </div>
          </div>
          <div class="shrink-0 rounded-card border border-border-dark bg-ink-900/60 p-4 lg:w-80">
            <div class="flex items-baseline justify-between gap-2">
              <p class="text-sm font-medium text-text-primary">每日签到</p>
              <span class="text-[11px] text-text-muted">连续 {{ overview.checkin_streak }} 天 · 最长 {{ overview.checkin_longest_streak }} 天</span>
            </div>
            <p class="mt-1 text-xs text-text-muted">
              今日奖励 {{ overview.checkin_reward }} 签到礼物币 · {{ overview.checkin_today_xp }} 经验
            </p>
            <p class="mt-1 text-[11px] text-text-muted">{{ xpBreakdownText() }}</p>
            <p v-if="!overview.checked_in_today" class="mt-1 text-[11px] text-text-muted">
              签到后连续 {{ overview.next_checkin_streak }} 天
            </p>
            <button
              type="button"
              class="mt-3 w-full rounded-md bg-grass-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-grass-500 disabled:cursor-not-allowed disabled:opacity-60"
              :disabled="checkinDisabled"
              @click="onCheckin"
            >
              {{ overview.checked_in_today ? '今日已签到' : checkingIn ? '签到中…' : '立即签到' }}
            </button>
            <p class="mt-2 text-[11px] text-text-muted">
              签到日期：{{ overview.checkin_date }}
            </p>
          </div>
        </div>
      </section>

      <!-- 补签卡 -->
      <section class="card mb-6 p-5">
        <div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <h2 class="text-lg font-bold text-text-primary">补签卡</h2>
            <p class="mt-1 text-xs text-text-muted">
              持有 {{ overview.makeup_cards }} 张 · 单价 {{ overview.makeup_card_price }} 签到币。
              只能使用签到礼物币购买；补签只补经验，不补签到天数、不恢复连续签到、不计入排行榜。
            </p>
            <p class="mt-1 text-[11px] text-text-muted">
              <template v-if="overview.latest_missed_date">
                最近漏签日：{{ overview.latest_missed_date }}
              </template>
              <template v-else>当前没有可补签的漏签日。</template>
            </p>
          </div>
          <div class="flex shrink-0 gap-2">
            <button
              type="button"
              class="rounded-md border border-grass-500/50 bg-grass-500/10 px-4 py-2 text-sm font-medium text-grass-300 transition hover:bg-grass-500/20 disabled:cursor-not-allowed disabled:opacity-50"
              :disabled="!canBuyMakeupCard"
              @click="onBuyMakeupCard"
            >
              {{ buyingMakeup ? '购买中…' : '购买补签卡' }}
            </button>
            <button
              type="button"
              class="rounded-md bg-grass-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-grass-500 disabled:cursor-not-allowed disabled:opacity-50"
              :disabled="!canUseMakeupCard"
              @click="onUseMakeupCard"
            >
              {{ usingMakeup ? '补签中…' : '使用补签卡' }}
            </button>
          </div>
        </div>
      </section>

      <!-- 礼物目录 -->
      <section class="mb-6">
        <div class="mb-3 flex items-baseline justify-between gap-3">
          <h2 class="text-lg font-bold text-text-primary">兑换礼物</h2>
          <p class="text-xs text-text-muted">允许签到币的礼物优先扣签到币，不足部分再扣通用币。</p>
        </div>
        <div v-if="catalog.length" class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          <article
            v-for="item in catalog"
            :key="item.id"
            class="card flex flex-col p-4"
          >
            <div class="flex items-start gap-3">
              <span class="text-3xl" aria-hidden="true">{{ item.icon }}</span>
              <div class="min-w-0 flex-1">
                <div class="flex flex-wrap items-center gap-2">
                  <h3 class="font-semibold text-text-primary">{{ item.name }}</h3>
                  <span
                    v-if="item.allow_checkin"
                    class="rounded-full border border-grass-500/40 bg-grass-500/10 px-2 py-0.5 text-[11px] text-grass-300"
                  >
                    签到币可兑
                  </span>
                </div>
                <p class="mt-1 line-clamp-2 text-xs text-text-muted">{{ item.description || '—' }}</p>
              </div>
            </div>
            <div class="mt-3 flex items-baseline gap-1">
              <span class="text-xl font-bold text-grass-300">{{ item.price }}</span>
              <span class="text-xs text-text-muted">礼物币 / 个</span>
            </div>
            <p class="mt-1 text-[11px] text-text-muted">{{ splitText(item) }}</p>
            <button
              type="button"
              class="mt-3 rounded-md bg-grass-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-grass-500 disabled:cursor-not-allowed disabled:opacity-50"
              :disabled="!item.can_afford || redeemingId === item.id"
              @click="onRedeem(item)"
            >
              {{ redeemingId === item.id ? '兑换中…' : item.can_afford ? '兑换' : '余额不足' }}
            </button>
          </article>
        </div>
        <p v-else class="card p-8 text-center text-sm text-text-muted">暂无可兑换的礼物。</p>
      </section>

      <!-- 我的礼物 -->
      <section class="card mb-6 p-5">
        <div class="mb-3 flex items-baseline justify-between gap-3">
          <h2 class="text-lg font-bold text-text-primary">我的礼物</h2>
          <span class="text-xs text-text-muted">共 {{ redemptionTotal }} 件</span>
        </div>
        <div v-if="redemptions.length" class="divide-y divide-border-dark">
          <div v-for="item in redemptions" :key="item.id" class="flex items-center gap-3 py-3">
            <span class="text-2xl" aria-hidden="true">{{ item.gift_icon }}</span>
            <div class="min-w-0 flex-1">
              <p class="font-medium text-text-primary">{{ item.gift_name }}</p>
              <p class="text-[11px] text-text-muted">
                {{ formatTime(item.created_at) }} ·
                <template v-if="item.checkin_spent > 0">{{ item.checkin_spent }} 签到币</template>
                <template v-if="item.checkin_spent > 0 && item.general_spent > 0"> + </template>
                <template v-if="item.general_spent > 0">{{ item.general_spent }} 通用币</template>
              </p>
            </div>
            <span class="font-mono text-xs text-text-muted">{{ item.price }} 币</span>
          </div>
        </div>
        <p v-else class="py-6 text-center text-sm text-text-muted">还没有兑换过礼物，去挑一个吧。</p>
        <Pagination
          class="mt-3"
          :page="redemptionPage"
          :page-size="10"
          :total="redemptionTotal"
          :total-pages="redemptionTotalPages"
          @change="changeRedemptionPage"
        />
      </section>

      <!-- 流水 -->
      <section class="card p-5">
        <div class="mb-3 flex flex-wrap items-end justify-between gap-3">
          <div>
            <h2 class="text-lg font-bold text-text-primary">礼物币流水</h2>
            <p class="text-xs text-text-muted">每笔变动都可在服务端账本追溯。</p>
          </div>
          <label class="text-xs text-text-muted">
            币种
            <select
              v-model="coinFilter"
              class="ml-1 rounded-md border border-border-dark bg-ink-900 px-2 py-1.5 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
              @change="onLedgerFilter"
            >
              <option value="">全部</option>
              <option value="checkin">签到币</option>
              <option value="general">通用币</option>
            </select>
          </label>
        </div>
        <div v-if="ledger.length" class="divide-y divide-border-dark">
          <div v-for="item in ledger" :key="item.id" class="flex flex-wrap items-center gap-x-3 gap-y-1 py-2.5 text-sm">
            <span class="min-w-0 flex-1 truncate text-text-secondary">{{ item.reason || eventLabel(item.event_type) }}</span>
            <span class="rounded-full border border-border-dark px-2 py-0.5 text-[11px] text-text-muted">
              {{ coinLabel(item.coin_type) }}
            </span>
            <span :class="item.amount > 0 ? 'font-medium text-grass-300' : 'font-medium text-danger-mc'">
              {{ item.amount > 0 ? '+' : '' }}{{ item.amount }}
            </span>
            <span class="w-24 text-right text-xs text-text-muted">余额 {{ item.balance_after }}</span>
            <span class="w-40 text-right text-[11px] text-text-muted">{{ formatTime(item.created_at) }}</span>
          </div>
        </div>
        <p v-else class="py-6 text-center text-sm text-text-muted">暂无礼物币流水。</p>
        <Pagination
          class="mt-3"
          :page="ledgerPage"
          :page-size="10"
          :total="ledgerTotal"
          :total-pages="ledgerTotalPages"
          @change="changeLedgerPage"
        />
      </section>
    </template>
  </PageContainer>
</template>
