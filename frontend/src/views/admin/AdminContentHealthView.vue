<script setup lang="ts">
// 内容健康合并页：数据质量 + 数据导入任务 + 关键字屏蔽 + 链接健康。
//
// 四者都围绕「内容是否可用、可信」：数据质量覆盖字段与规则现状，
// 数据导入任务覆盖数据是怎么进来的（来源与失败明细），
// 关键字屏蔽覆盖用户生成内容的提交拦截，链接健康覆盖资源来源链接。
// 前两者共用 ops.dataquality 权限，链接健康使用独立的 ops.linkhealth。
import AdminTabs from '@/components/admin/AdminTabs.vue'
import { useAdminTabs } from '@/composables/useAdminTabs'
import AdminDataQualityView from './AdminDataQualityView.vue'
import AdminDataImportJobsView from './AdminDataImportJobsView.vue'
import AdminKeywordBlocklistView from './AdminKeywordBlocklistView.vue'
import AdminLinkHealthView from './AdminLinkHealthView.vue'

const { tabs, active, select } = useAdminTabs([
  { key: 'quality', label: '数据质量', perm: 'ops.dataquality' },
  { key: 'jobs', label: '数据导入任务', perm: 'ops.dataquality' },
  { key: 'keywords', label: '关键字屏蔽', perm: 'ops.dataquality' },
  { key: 'links', label: '链接健康', perm: 'ops.linkhealth' },
])
</script>

<template>
  <AdminTabs
    :tabs="tabs"
    :active="active"
    aria-label="内容健康"
    empty-description="当前账号没有「数据质量」或「链接健康」权限，请联系管理员分配。"
    @select="select"
  >
    <AdminDataQualityView v-if="active === 'quality'" />
    <AdminDataImportJobsView v-else-if="active === 'jobs'" />
    <AdminKeywordBlocklistView v-else-if="active === 'keywords'" />
    <AdminLinkHealthView v-else-if="active === 'links'" />
  </AdminTabs>
</template>
