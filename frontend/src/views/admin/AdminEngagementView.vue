<script setup lang="ts">
// 用户互动合并页：反馈管理 + 社区管理 + 用户评论。
//
// content.feedback 与 content.community 权限点独立判断；社区管理与用户评论
// 共用 content.community，但后端接口分别按帖子维度 / 全站维度鉴权。
import AdminTabs from '@/components/admin/AdminTabs.vue'
import { useAdminTabs } from '@/composables/useAdminTabs'
import AdminCommunityView from './AdminCommunityView.vue'
import AdminCommentsView from './AdminCommentsView.vue'
import AdminFeedbackView from './AdminFeedbackView.vue'

const { tabs, active, select } = useAdminTabs([
  { key: 'feedback', label: '反馈管理', perm: 'content.feedback' },
  { key: 'community', label: '社区管理', perm: 'content.community' },
  { key: 'comments', label: '用户评论', perm: 'content.community' },
])
</script>

<template>
  <AdminTabs
    :tabs="tabs"
    :active="active"
    aria-label="用户互动"
    empty-description="当前账号没有「反馈管理」或「社区管理」权限，请联系管理员分配。"
    @select="select"
  >
    <AdminFeedbackView v-if="active === 'feedback'" />
    <AdminCommunityView v-else-if="active === 'community'" />
    <AdminCommentsView v-else-if="active === 'comments'" />
  </AdminTabs>
</template>
