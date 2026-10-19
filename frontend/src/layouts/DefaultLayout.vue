<script setup lang="ts">
// 主站布局：头部 + 内容 + 页脚
import AppHeader from '@/components/layout/AppHeader.vue'
import AppFooter from '@/components/layout/AppFooter.vue'
</script>

<template>
  <div class="mc-grid-bg flex min-h-screen flex-col bg-ink-950">
    <!-- TODO §14：键盘用户可跳过顶部导航直达正文 -->
    <a class="skip-link" href="#main-content">跳到主要内容</a>
    <AppHeader />
    <main id="main-content" tabindex="-1" class="flex-1">
      <RouterView />
    </main>
    <AppFooter />
  </div>
</template>
