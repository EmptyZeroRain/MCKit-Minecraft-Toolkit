<script setup lang="ts">
// 管理后台布局：侧边栏 + 顶栏 + 内容区
// 顶栏固定提供「返回前台」与「退出登录」，避免在长侧边栏中不易发现（与侧边栏共用同一实现）。
import AdminSidebar from '@/components/admin/AdminSidebar.vue'
import { useRoute } from 'vue-router'
import { useAdminSession } from '@/composables/useAdminSession'

const route = useRoute()
const { logout, goHome } = useAdminSession()

const pageTitle = () =>
  (route.meta.title as string | undefined) ?? '管理后台'
</script>

<template>
  <div class="flex min-h-screen bg-ink-950">
    <!-- TODO §14：键盘用户可跳过侧边栏直达内容区 -->
    <a class="skip-link" href="#admin-main-content">跳到主要内容</a>
    <AdminSidebar />
    <div class="flex min-w-0 flex-1 flex-col">
      <header
        class="sticky top-0 z-30 flex h-16 items-center justify-between gap-4 border-b border-border-dark px-6 backdrop-blur"
        style="background-color: var(--mc-header-bg)"
      >
        <h1 class="truncate font-semibold text-text-primary">{{ pageTitle() }}</h1>
        <div class="flex shrink-0 items-center gap-2">
          <button
            type="button"
            class="rounded-md border border-border-dark px-3 py-1.5 text-xs text-text-secondary transition hover:bg-ink-700 hover:text-text-primary"
            @click="goHome"
          >
            返回前台
          </button>
          <button
            type="button"
            class="rounded-md border border-danger-mc/40 px-3 py-1.5 text-xs text-danger-mc transition hover:bg-danger-mc/10"
            @click="logout"
          >
            退出登录
          </button>
        </div>
      </header>
      <main id="admin-main-content" tabindex="-1" class="flex-1 p-6">
        <RouterView />
      </main>
    </div>
  </div>
</template>
