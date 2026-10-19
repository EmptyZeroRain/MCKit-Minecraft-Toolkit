<script setup lang="ts">
// 投稿审核合并页（gj.md §2.2「投稿审核」）。
//
// 统一审核工作台的交互框架，但不统一业务模型：
//   - review.redstone 与 review.mods 权限点独立判断；
//   - 红石设计与 Mod / 整合包各自使用独立的状态机、字段校验与后端接口。
import AdminTabs from '@/components/admin/AdminTabs.vue'
import { useAdminTabs } from '@/composables/useAdminTabs'
import AdminRedstoneView from './AdminRedstoneView.vue'
import AdminModsView from './AdminModsView.vue'
import AdminSubmissionsView from './AdminSubmissionsView.vue'

const { tabs, active, select } = useAdminTabs([
  { key: 'redstone', label: '红石设计', perm: 'review.redstone' },
  { key: 'mods', label: 'Mod 与整合包', perm: 'review.mods' },
  // 资源投稿审核复用 content.resources（审核通过会创建 resources 记录）。
  { key: 'submissions', label: '资源投稿', perm: 'content.resources' },
])
</script>

<template>
  <AdminTabs
    :tabs="tabs"
    :active="active"
    aria-label="投稿审核"
    empty-description="当前账号没有「红石设计审核」「Mod / 整合包审核」或「资源投稿审核」权限，请联系管理员分配。"
    @select="select"
  >
    <AdminRedstoneView v-if="active === 'redstone'" />
    <AdminModsView v-else-if="active === 'mods'" />
    <AdminSubmissionsView v-else-if="active === 'submissions'" />
  </AdminTabs>
</template>
