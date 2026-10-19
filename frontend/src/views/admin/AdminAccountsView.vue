<script setup lang="ts">
// 账号管理合并页（gj.md §2.2「账号管理」）。
//
// accounts.admins 与 accounts.users 权限点独立判断；两类账号的查询与操作
// 在后端严格隔离（普通用户固定 role='user'），Tab 只是信息架构，不是权限边界。
import AdminTabs from '@/components/admin/AdminTabs.vue'
import { useAdminTabs } from '@/composables/useAdminTabs'
import AdminAdminsView from './AdminAdminsView.vue'
import AdminUsersView from './AdminUsersView.vue'

const { tabs, active, select } = useAdminTabs([
  { key: 'admins', label: '管理员', perm: 'accounts.admins' },
  { key: 'users', label: '普通用户', perm: 'accounts.users' },
])
</script>

<template>
  <AdminTabs
    :tabs="tabs"
    :active="active"
    aria-label="账号管理"
    empty-description="当前账号没有「管理员账号」或「普通用户管理」权限，请联系管理员分配。"
    @select="select"
  >
    <AdminAdminsView v-if="active === 'admins'" />
    <AdminUsersView v-else-if="active === 'users'" />
  </AdminTabs>
</template>
