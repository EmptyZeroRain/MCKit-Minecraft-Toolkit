<script setup lang="ts">
// 管理后台布局：侧边栏 + 顶栏 + 内容区
import AdminSidebar from '@/components/admin/AdminSidebar.vue'
import { useRoute } from 'vue-router'

const route = useRoute()

const pageTitle = () =>
  (route.meta.title as string | undefined) ?? '管理后台'
</script>

<template>
  <div class="flex min-h-screen bg-ink-950">
    <!-- TODO §14：键盘用户可跳过侧边栏直达内容区 -->
    <a class="skip-link" href="#admin-main-content">跳到主要内容</a>
    <AdminSidebar />
    <div class="flex min-w-0 flex-1 flex-col">
      <header class="sticky top-0 z-30 flex h-16 items-center border-b border-border-dark px-6 backdrop-blur" style="background-color: var(--mc-header-bg)">
        <h1 class="font-semibold text-text-primary">{{ pageTitle() }}</h1>
      </header>
      <main id="admin-main-content" tabindex="-1" class="flex-1 p-6">
        <RouterView />
      </main>
    </div>
  </div>
</template>