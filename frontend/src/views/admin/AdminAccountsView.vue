<script setup lang="ts">
// 账号与权限合并页：账号列表 + 角色与权限。
//
// 账号 Tab 由管理员账号（accounts.admins）与普通用户（accounts.users）共享，
// 两者只要具备其一即可进入；角色 Tab 单独要求 roles.manage。
// 账号列表内部仍按权限点收敛可查看范围，角色编辑接口由后端逐次鉴权。
import AdminTabs from '@/components/admin/AdminTabs.vue'
import { useAdminTabs } from '@/composables/useAdminTabs'
import AdminAccountListView from './AdminAccountListView.vue'
import AdminRolesView from './AdminRolesView.vue'

const { tabs, active, select } = useAdminTabs([
  { key: 'accounts', label: '账号管理', permsAny: ['accounts.admins', 'accounts.users'] },
  { key: 'roles', label: '角色与权限', perm: 'roles.manage' },
])
</script>

<template>
  <AdminTabs
    :tabs="tabs"
    :active="active"
    aria-label="账号与权限"
    empty-description="当前账号没有「管理员账号」「普通用户管理」或「角色管理」权限，请联系管理员分配。"
    @select="select"
  >
    <AdminAccountListView v-if="active === 'accounts'" />
    <AdminRolesView v-else-if="active === 'roles'" />
  </AdminTabs>
</template>
