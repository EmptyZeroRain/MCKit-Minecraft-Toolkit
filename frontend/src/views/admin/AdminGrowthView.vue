<script setup lang="ts">
// 用户成长管理（jingyan.md §10）。
//
// 单一一级菜单，内部以 Tab 组织：概览 / 用户详情 / 经验规则 / 等级 / 每日任务 / 成就 / 经验流水 / 审计。
// 所有 Tab 同属 ops.growth 权限（侧边栏过滤 + 后端 403 兜底）。
import AdminTabs from '@/components/admin/AdminTabs.vue'
import { useAdminTabs } from '@/composables/useAdminTabs'
import AdminGrowthOverviewPanel from './AdminGrowthOverviewPanel.vue'
import AdminGrowthUserDetailPanel from './AdminGrowthUserDetailPanel.vue'
import AdminGrowthRulesPanel from './AdminGrowthRulesPanel.vue'
import AdminGrowthLevelsPanel from './AdminGrowthLevelsPanel.vue'
import AdminGrowthTasksPanel from './AdminGrowthTasksPanel.vue'
import AdminGrowthAchievementsPanel from './AdminGrowthAchievementsPanel.vue'
import AdminGrowthLedgerPanel from './AdminGrowthLedgerPanel.vue'
import AdminGrowthAuditPanel from './AdminGrowthAuditPanel.vue'

const { tabs, active, select } = useAdminTabs([
  { key: 'overview', label: '成长概览', perm: 'ops.growth' },
  { key: 'users', label: '用户详情', perm: 'ops.growth' },
  { key: 'rules', label: '经验规则', perm: 'ops.growth' },
  { key: 'levels', label: '等级', perm: 'ops.growth' },
  { key: 'tasks', label: '每日任务', perm: 'ops.growth' },
  { key: 'achievements', label: '成就', perm: 'ops.growth' },
  { key: 'ledger', label: '经验流水', perm: 'ops.growth' },
  { key: 'audit', label: '审计日志', perm: 'ops.growth' },
])
</script>

<template>
  <div>
    <h1 class="mb-4 text-xl font-bold text-text-primary">用户成长</h1>
    <AdminTabs
      :tabs="tabs"
      :active="active"
      aria-label="用户成长"
      empty-description="当前账号没有「用户成长」权限（ops.growth），请联系管理员分配。"
      @select="select"
    >
      <AdminGrowthOverviewPanel v-if="active === 'overview'" />
      <AdminGrowthUserDetailPanel v-else-if="active === 'users'" />
      <AdminGrowthRulesPanel v-else-if="active === 'rules'" />
      <AdminGrowthLevelsPanel v-else-if="active === 'levels'" />
      <AdminGrowthTasksPanel v-else-if="active === 'tasks'" />
      <AdminGrowthAchievementsPanel v-else-if="active === 'achievements'" />
      <AdminGrowthLedgerPanel v-else-if="active === 'ledger'" />
      <AdminGrowthAuditPanel v-else-if="active === 'audit'" />
    </AdminTabs>
  </div>
</template>
