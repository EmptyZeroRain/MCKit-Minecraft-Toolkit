<script setup lang="ts">
// 分类与标签合并页（gj.md §2.2 菜单合并规则）。
//
// content.categories 与 content.tags 权限点独立判断：只有其中一项权限时
// 只显示对应 Tab；直接访问无权限 Tab（含 URL 参数）会回退到可用 Tab。
import AdminTabs from '@/components/admin/AdminTabs.vue'
import { useAdminTabs } from '@/composables/useAdminTabs'
import AdminCategoriesView from './AdminCategoriesView.vue'
import AdminTagsView from './AdminTagsView.vue'

const { tabs, active, select } = useAdminTabs([
  { key: 'categories', label: '分类', perm: 'content.categories' },
  { key: 'tags', label: '标签', perm: 'content.tags' },
])
</script>

<template>
  <AdminTabs
    :tabs="tabs"
    :active="active"
    aria-label="分类与标签"
    empty-description="当前账号没有「分类管理」或「标签管理」权限，请联系管理员分配。"
    @select="select"
  >
    <AdminCategoriesView v-if="active === 'categories'" />
    <AdminTagsView v-else-if="active === 'tags'" />
  </AdminTabs>
</template>
