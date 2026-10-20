<script setup lang="ts">
// 礼物运营页：钱包、礼品目录、签到设置与三类排行榜统一入口。
//
// 标签状态写入 URL 查询参数（tab），旧 /admin/gift-catalog 会重定向到
// /admin/gifts?tab=catalog，方便收藏与分享。这里不依赖 Router 实例，
// 避免单测挂载容器时必须注入完整路由。
import { ref } from 'vue'
import AdminGiftWalletPanel from './gift/AdminGiftWalletPanel.vue'
import AdminGiftCatalogPanel from './gift/AdminGiftCatalogPanel.vue'
import AdminGiftCheckinPanel from './gift/AdminGiftCheckinPanel.vue'
import AdminGiftCheckinRankingPanel from './gift/AdminGiftCheckinRankingPanel.vue'
import AdminGiftRankingPanel from './gift/AdminGiftRankingPanel.vue'
import AdminGiftCoinRankingPanel from './gift/AdminGiftCoinRankingPanel.vue'

const tabs = [
  { key: 'wallet', label: '钱包' },
  { key: 'catalog', label: '礼品目录' },
  { key: 'checkin', label: '签到设置' },
  { key: 'checkin-ranking', label: '签到排行榜' },
  { key: 'gift-ranking', label: '礼物排行榜' },
  { key: 'coin-ranking', label: '礼物币排行榜' },
] as const

type TabKey = (typeof tabs)[number]['key']

const tabKeys = new Set<string>(tabs.map((tab) => tab.key))

function readTab(): TabKey {
  if (typeof window === 'undefined') return 'wallet'
  const value = new URLSearchParams(window.location.search).get('tab') ?? ''
  return tabKeys.has(value) ? (value as TabKey) : 'wallet'
}

const activeTab = ref<TabKey>(readTab())

function selectTab(tab: TabKey) {
  activeTab.value = tab
  if (typeof window === 'undefined') return
  const url = new URL(window.location.href)
  url.searchParams.set('tab', tab)
  window.history.replaceState({}, '', url.toString())
}
</script>

<template>
  <div>
    <div class="mb-4">
      <h1 class="text-xl font-bold text-text-primary">礼物</h1>
      <p class="mt-1 text-xs text-text-muted">
        礼物币钱包、礼品目录、签到配置与运营排行榜统一入口；所有余额变动写入不可删除的账本流水。
      </p>
    </div>

    <div class="mb-5 flex flex-wrap gap-2 border-b border-border-dark pb-2" role="tablist" aria-label="礼物功能">
      <button
        v-for="tab in tabs"
        :key="tab.key"
        type="button"
        role="tab"
        :aria-selected="activeTab === tab.key"
        class="rounded-md px-3 py-2 text-sm transition"
        :class="
          activeTab === tab.key
            ? 'bg-grass-600 font-medium text-white'
            : 'text-text-secondary hover:bg-ink-700 hover:text-text-primary'
        "
        @click="selectTab(tab.key)"
      >
        {{ tab.label }}
      </button>
    </div>

    <AdminGiftWalletPanel v-if="activeTab === 'wallet'" />
    <AdminGiftCatalogPanel v-else-if="activeTab === 'catalog'" />
    <AdminGiftCheckinPanel v-else-if="activeTab === 'checkin'" />
    <AdminGiftCheckinRankingPanel v-else-if="activeTab === 'checkin-ranking'" />
    <AdminGiftRankingPanel v-else-if="activeTab === 'gift-ranking'" />
    <AdminGiftCoinRankingPanel v-else-if="activeTab === 'coin-ranking'" />
  </div>
</template>
