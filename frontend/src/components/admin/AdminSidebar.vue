<script setup lang="ts">
// 管理后台侧边栏
import { computed } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useAdminSession } from '@/composables/useAdminSession'

const route = useRoute()
const auth = useAuthStore()
const { logout, goHome } = useAdminSession()

interface NavItem {
  label: string
  to: string
  icon: string
  /** 需要的权限点：不具备则隐藏（后端仍做 403 兜底） */
  perm?: string
  /** 任一权限点满足即可见（用于合并入口，如分类与标签） */
  permsAny?: string[]
}
interface NavGroup {
  title: string
  items: NavItem[]
}

// 管理导航原始清单（按权限点过滤，见 navGroups）。
// 信息架构遵循「按工作流分组」：审核与运营不再按数据表拆成十余个入口，
// 而是收敛为 5 个合并页；旧路由在 router 中保留重定向，权限点仍独立。
const navGroupsRaw: NavGroup[] = [
  {
    title: '概览',
    items: [{ label: '仪表盘', to: '/admin', icon: 'M3 12l9-9 9 9M5 10v10h14V10' }],
  },
  {
    title: '内容管理',
    items: [
      { label: '资源管理', to: '/admin/resources', perm: 'content.resources', icon: 'M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14M4 6h16m-16 12h16' },
      { label: '工具管理', to: '/admin/tools', perm: 'content.tools', icon: 'M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z' },
      { label: '百科管理', to: '/admin/wiki', perm: 'content.wiki', icon: 'M12 6.042A8.967 8.967 0 006 3.75c-1.052 0-2.062.18-3 .512v14.25A8.987 8.987 0 016 18c2.305 0 4.408.867 6 2.292m0-14.25a8.966 8.966 0 016-2.292c1.052 0 2.062.18 3 .512v14.25A8.987 8.987 0 0018 18a8.967 8.967 0 00-6 2.292m0-14.25v14.25' },
      { label: '分类与标签', to: '/admin/taxonomy', permsAny: ['content.categories', 'content.tags'], icon: 'M7 7h.01M7 3h5c.512 0 1.024.195 1.414.586l7 7a2 2 0 010 2.828l-7 7a2 2 0 01-2.828 0l-7-7A1.994 1.994 0 013 12V7a4 4 0 014-4z' },
    ],
  },
  {
    title: '审核与运营',
    items: [
      { label: '投稿审核', to: '/admin/review', permsAny: ['review.redstone', 'review.mods', 'content.resources'], icon: 'M13 10V3L4 14h7v7l9-11h-7z' },
      { label: '运营分析', to: '/admin/insights', permsAny: ['ops.analytics', 'ops.searchstats', 'ops.contenttasks'], icon: 'M3 3v18h18M7 15l3-4 3 3 5-7' },
      { label: '用户互动', to: '/admin/engagement', permsAny: ['content.feedback', 'content.community'], icon: 'M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z' },
      { label: '内容健康', to: '/admin/content-health', permsAny: ['ops.dataquality', 'ops.linkhealth'], icon: 'M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2m-3 7h3m-3 4h3m-6-4h.01M9 16h.01' },
      { label: '用户成长', to: '/admin/growth', perm: 'ops.growth', icon: 'M3 17l6-6 4 4 8-8M21 7v5h-5' },
      { label: '礼物', to: '/admin/gifts', perm: 'ops.gifts', icon: 'M20 12v10H4V12M2 7h20v5H2zM12 22V7M12 7H7.5a2.5 2.5 0 010-5C11 2 12 7 12 7zm0 0h4.5a2.5 2.5 0 000-5C13 2 12 7 12 7z' },
    ],
  },
  {
    title: '系统管理',
    items: [
      { label: '账号与权限', to: '/admin/accounts', permsAny: ['accounts.admins', 'accounts.users', 'roles.manage'], icon: 'M12 4.354a4 4 0 110 5.292M15 21H3v-1a6 6 0 0112 0v1zm0 0h6v-1a6 6 0 00-9-5.197M13 7a4 4 0 11-8 0 4 4 0 018 0z' },
      { label: '会话管理', to: '/admin/sessions', perm: 'ops.sessions', icon: 'M15.75 5.25a3 3 0 013 3m3 0a6 6 0 01-7.029 5.912c-.563-.097-1.159.026-1.563.43L10.5 17.25H8.25v2.25H6v2.25H2.25v-2.818c0-.597.237-1.17.659-1.591l6.499-6.499c.404-.404.527-1 .43-1.563A6 6 0 1121.75 8.25z' },
      { label: '安全审计', to: '/admin/audit-logs', perm: 'ops.audit', icon: 'M9 17v-2m3 2v-4m3 4v-6m2 10H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z' },
      { label: '系统运维', to: '/admin/ops', permsAny: ['ops.alerts', 'ops.jobs', 'ops.mcsrc'], icon: 'M20.25 6.375c0 2.278-3.694 4.125-8.25 4.125S3.75 8.653 3.75 6.375m16.5 0c0-2.278-3.694-4.125-8.25-4.125S3.75 4.097 3.75 6.375m16.5 0v11.25c0 2.278-3.694 4.125-8.25 4.125s-8.25-1.847-8.25-4.125V6.375m16.5 5.625c0 2.278-3.694 4.125-8.25 4.125s-8.25-1.847-8.25-4.125' },
      { label: '操作手册', to: '/admin/help', icon: 'M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.747 0 3.332.477 4.5 1.253v13C19.832 18.477 18.247 18 16.5 18c-1.746 0-3.332.477-4.5 1.253' },
      { label: '用户信息', to: '/admin/profile', icon: 'M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z' },
    ],
  },
]

// 按权限点过滤入口（需求 §6.2）；前端隐藏不是权限控制，后端仍 403 兜底。
// 单项权限用 perm，合并入口用 permsAny（任一满足即显示）。
const navGroups = computed<NavGroup[]>(() =>
  navGroupsRaw.map((group) => ({
    ...group,
    items: group.items.filter((item) => {
      if (item.permsAny?.length) return item.permsAny.some((p) => auth.hasPermission(p))
      return !item.perm || auth.hasPermission(item.perm)
    }),
  })),
)

function isActive(to: string): boolean {
  if (to === '/admin') return route.path === '/admin'
  return route.path.startsWith(to)
}
</script>

<template>
  <aside class="flex h-full w-60 shrink-0 flex-col border-r border-border-dark bg-ink-900">
    <div class="flex h-16 items-center gap-2 border-b border-border-dark px-5">
      <img src="/favicon.svg" alt="" class="h-8 w-8" />
      <div>
        <div class="text-sm font-bold text-text-primary">管理后台</div>
        <div class="text-xs text-text-muted">{{ auth.displayName }}（{{ auth.user?.role }}）</div>
      </div>
    </div>

    <nav class="flex-1 overflow-y-auto px-3 py-4" aria-label="管理导航">
      <div v-for="group in navGroups" :key="group.title" class="mb-5">
        <div class="mb-2 px-2 text-xs font-semibold uppercase tracking-wider text-text-muted">
          {{ group.title }}
        </div>
        <ul class="space-y-1">
          <li v-for="item in group.items" :key="item.to">
            <RouterLink
              :to="item.to"
              class="flex items-center gap-3 rounded-md px-2.5 py-2 text-sm text-text-secondary transition hover:bg-ink-700 hover:text-text-primary"
              :class="{ '!bg-ink-700 !text-grass-300': isActive(item.to) }"
            >
              <svg class="h-4 w-4 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.8">
                <path stroke-linecap="round" stroke-linejoin="round" :d="item.icon" />
              </svg>
              {{ item.label }}
            </RouterLink>
          </li>
        </ul>
      </div>
    </nav>

    <div class="space-y-1 border-t border-border-dark p-3">
      <button
        type="button"
        class="flex w-full items-center gap-3 rounded-md px-2.5 py-2 text-sm text-text-secondary transition hover:bg-ink-700 hover:text-text-primary"
        @click="goHome"
      >
        <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.8">
          <path stroke-linecap="round" stroke-linejoin="round" d="M2.25 12l8.954-8.955c.44-.439 1.152-.439 1.591 0L21.75 12M4.5 9.75v10.125c0 .621.504 1.125 1.125 1.125H9.75v-4.875c0-.621.504-1.125 1.125-1.125h2.25c.621 0 1.125.504 1.125 1.125V21h4.125c.621 0 1.125-.504 1.125-1.125V9.75" />
        </svg>
        返回前台
      </button>
      <button
        type="button"
        class="flex w-full items-center gap-3 rounded-md px-2.5 py-2 text-sm text-danger-mc transition hover:bg-danger-mc/10"
        @click="logout"
      >
        <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.8">
          <path stroke-linecap="round" stroke-linejoin="round" d="M15.75 9V5.25A2.25 2.25 0 0013.5 3h-6a2.25 2.25 0 00-2.25 2.25v13.5A2.25 2.25 0 007.5 21h6a2.25 2.25 0 002.25-2.25V15M12 9l-3 3m0 0l3 3m-3-3h12.75" />
        </svg>
        退出登录
      </button>
    </div>
  </aside>
</template>