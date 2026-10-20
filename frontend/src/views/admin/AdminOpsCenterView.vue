<script setup lang="ts">
// 系统运维合并页：运营告警 + 后台任务 + MC 源码缓存。
//
// 三者共享「发现异常 → 执行/重试 → 清理派生数据」的运维链路，但权限点
// ops.alerts / ops.jobs / ops.mcsrc 独立判断，后端仍逐接口鉴权。
import AdminTabs from '@/components/admin/AdminTabs.vue'
import { useAdminTabs } from '@/composables/useAdminTabs'
import AdminAlertsView from './AdminAlertsView.vue'
import AdminJobsView from './AdminJobsView.vue'
import AdminMcsrcView from './AdminMcsrcView.vue'

const { tabs, active, select } = useAdminTabs([
  { key: 'alerts', label: '运营告警', perm: 'ops.alerts' },
  { key: 'jobs', label: '后台任务', perm: 'ops.jobs' },
  { key: 'mcsrc', label: 'MC 源码缓存', perm: 'ops.mcsrc' },
])
</script>

<template>
  <AdminTabs
    :tabs="tabs"
    :active="active"
    aria-label="系统运维"
    empty-description="当前账号没有「运营告警」「后台任务」或「MC 源码缓存」权限，请联系管理员分配。"
    @select="select"
  >
    <AdminAlertsView v-if="active === 'alerts'" />
    <AdminJobsView v-else-if="active === 'jobs'" />
    <AdminMcsrcView v-else-if="active === 'mcsrc'" />
  </AdminTabs>
</template>
