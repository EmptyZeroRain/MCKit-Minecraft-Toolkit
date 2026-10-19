<script setup lang="ts">
// 管理仪表盘：内容概览
import { onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { listResources } from '@/api/admin-resources'
import { listTools } from '@/api/admin-tools'
import { listWiki } from '@/api/admin-wiki'
import { listFeedback } from '@/api/admin-system'
import { listUsers } from '@/api/admin-users'

const auth = useAuthStore()

const stats = ref({
  resources: 0,
  tools: 0,
  wiki: 0,
  feedbackNew: 0,
  users: 0,
})

const loading = ref(true)

onMounted(async () => {
  try {
    const [r, t, w, f, u] = await Promise.all([
      listResources({ page: 1, page_size: 1 }),
      listTools({ page: 1, page_size: 1 }),
      listWiki({ page: 1, page_size: 1 }),
      listFeedback({ page: 1, page_size: 1, status: 'new' }),
      listUsers({ page: 1, page_size: 1 }),
    ])
    stats.value = {
      resources: r.total,
      tools: t.total,
      wiki: w.total,
      feedbackNew: f.total,
      users: u.total,
    }
  } finally {
    loading.value = false
  }
})

const cards = [
  { label: '资源总数', value: 'resources', to: '/admin/resources', icon: 'M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14M4 6h16m-16 12h16' },
  { label: '工具总数', value: 'tools', to: '/admin/tools', icon: 'M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z' },
  { label: '百科总数', value: 'wiki', to: '/admin/wiki', icon: 'M12 6.042A8.967 8.967 0 006 3.75c-1.052 0-2.062.18-3 .512v14.25A8.987 8.987 0 016 18c2.305 0 4.408.867 6 2.292m0-14.25a8.966 8.966 0 016-2.292c1.052 0 2.062.18 3 .512v14.25A8.987 8.987 0 0018 18a8.967 8.967 0 00-6 2.292m0-14.25v14.25' },
  { label: '新反馈', value: 'feedbackNew', to: '/admin/feedback', icon: 'M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z' },
  { label: '用户总数', value: 'users', to: '/admin/users', icon: 'M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0zm6 3a2 2 0 11-4 0 2 2 0 014 0zM7 10a2 2 0 11-4 0 2 2 0 014 0z' },
]
</script>

<template>
  <div>
    <div class="mb-6 rounded-card border border-grass-500/30 bg-grass-500/10 p-4 text-sm text-grass-300">
      欢迎回来，{{ auth.displayName }}。当前权限：{{ auth.user?.role === 'admin' ? '管理员' : '编辑' }}
      （最终权限校验由服务端执行）
    </div>

    <div v-if="loading" class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-5">
      <div v-for="i in 5" :key="i" class="card h-28 animate-pulse" />
    </div>

    <div v-else class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-5">
      <RouterLink
        v-for="card in cards"
        :key="card.label"
        :to="card.to"
        class="card card-hover p-5"
      >
        <div class="mb-3 flex items-center justify-between">
          <span class="text-sm text-text-secondary">{{ card.label }}</span>
          <svg class="h-5 w-5 text-grass-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.8">
            <path stroke-linecap="round" stroke-linejoin="round" :d="card.icon" />
          </svg>
        </div>
        <div class="text-3xl font-bold text-text-primary">
          {{ stats[card.value as keyof typeof stats] }}
        </div>
      </RouterLink>
    </div>

    <div class="mt-8 grid gap-4 lg:grid-cols-3">
      <RouterLink to="/admin/resources/new" class="card card-hover flex items-center gap-4 p-5">
        <span class="flex h-10 w-10 items-center justify-center rounded-lg bg-grass-500/15 text-grass-400">
          <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M12 4v16m8-8H4" />
          </svg>
        </span>
        <div>
          <div class="font-medium text-text-primary">新增资源</div>
          <div class="text-xs text-text-muted">录入 Mod / 整合包等资源索引</div>
        </div>
      </RouterLink>
      <RouterLink to="/admin/tools/new" class="card card-hover flex items-center gap-4 p-5">
        <span class="flex h-10 w-10 items-center justify-center rounded-lg bg-grass-500/15 text-grass-400">
          <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M12 4v16m8-8H4" />
          </svg>
        </span>
        <div>
          <div class="font-medium text-text-primary">新增工具</div>
          <div class="text-xs text-text-muted">录入工具中心条目</div>
        </div>
      </RouterLink>
      <RouterLink to="/admin/wiki/new" class="card card-hover flex items-center gap-4 p-5">
        <span class="flex h-10 w-10 items-center justify-center rounded-lg bg-grass-500/15 text-grass-400">
          <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M12 4v16m8-8H4" />
          </svg>
        </span>
        <div>
          <div class="font-medium text-text-primary">新增百科</div>
          <div class="text-xs text-text-muted">编写百科条目</div>
        </div>
      </RouterLink>
    </div>
  </div>
</template>