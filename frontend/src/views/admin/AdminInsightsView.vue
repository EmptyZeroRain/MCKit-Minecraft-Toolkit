<script setup lang="ts">
// 运营分析合并页（gj.md §2.2「运营分析」）。
//
// ops.analytics 与 ops.searchstats 权限点独立判断：无权限的 Tab 不展示，
// 直接请求对应 API 后端仍返回 403。所有指标的计算口径见各子页说明。
import AdminTabs from '@/components/admin/AdminTabs.vue'
import { useAdminTabs } from '@/composables/useAdminTabs'
import AdminAnalyticsView from './AdminAnalyticsView.vue'
import AdminSearchTermsView from './AdminSearchTermsView.vue'

const { tabs, active, select } = useAdminTabs([
  { key: 'analytics', label: '运营看板', perm: 'ops.analytics' },
  { key: 'search-terms', label: '搜索词统计', perm: 'ops.searchstats' },
])
</script>

<template>
  <AdminTabs
    :tabs="tabs"
    :active="active"
    aria-label="运营分析"
    empty-description="当前账号没有「运营看板」或「搜索词统计」权限，请联系管理员分配。"
    @select="select"
  >
    <AdminAnalyticsView v-if="active === 'analytics'" />
    <AdminSearchTermsView v-else-if="active === 'search-terms'" />
  </AdminTabs>
</template>
