<script setup lang="ts">
// 运营分析合并页（gj.md §2.2「运营分析」）。
//
// ops.analytics、ops.searchstats 与 ops.contenttasks 权限点独立判断：
// 无权限的 Tab 不展示，直接请求对应 API 后端仍返回 403。
// 搜索词 → 内容建设任务构成「发现缺口 → 指派跟踪」的闭环，因此放在同一入口。
import AdminTabs from '@/components/admin/AdminTabs.vue'
import { useAdminTabs } from '@/composables/useAdminTabs'
import AdminAnalyticsView from './AdminAnalyticsView.vue'
import AdminContentTasksView from './AdminContentTasksView.vue'
import AdminSearchTermsView from './AdminSearchTermsView.vue'

const { tabs, active, select } = useAdminTabs([
  { key: 'analytics', label: '运营看板', perm: 'ops.analytics' },
  { key: 'search-terms', label: '搜索词统计', perm: 'ops.searchstats' },
  { key: 'content-tasks', label: '内容建设任务', perm: 'ops.contenttasks' },
])
</script>

<template>
  <AdminTabs
    :tabs="tabs"
    :active="active"
    aria-label="运营分析"
    empty-description="当前账号没有「运营看板」「搜索词统计」或「内容建设任务」权限，请联系管理员分配。"
    @select="select"
  >
    <AdminAnalyticsView v-if="active === 'analytics'" />
    <AdminSearchTermsView v-else-if="active === 'search-terms'" />
    <AdminContentTasksView v-else-if="active === 'content-tasks'" />
  </AdminTabs>
</template>
