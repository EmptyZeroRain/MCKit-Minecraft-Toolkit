<script setup lang="ts">
// 404 页面（需求 3.1）
import { RouterLink } from 'vue-router'
import PageContainer from '@/components/layout/PageContainer.vue'
import { applyPageMeta } from '@/utils/seo'

applyPageMeta({
  title: '页面不存在',
  robot: 'noindex,nofollow',
})
</script>

<template>
  <PageContainer maxWidth="max-w-3xl">
    <div class="flex flex-col items-center justify-center py-20 text-center">
      <div class="mb-6 text-6xl font-bold text-grass-400 md:text-8xl">404</div>
      <h1 class="mb-3 text-2xl font-bold text-text-primary">这个方块不存在</h1>
      <p class="mb-8 text-text-secondary">
        你访问的页面不存在或已被移除。试试返回首页，或使用搜索找回需要的工具与内容。
      </p>
      <div class="flex flex-wrap justify-center gap-3">
        <RouterLink
          to="/"
          class="rounded-md bg-grass-600 px-5 py-2.5 text-sm font-medium text-white transition hover:bg-grass-500"
        >
          返回首页
        </RouterLink>
        <RouterLink
          to="/tools"
          class="rounded-md border border-border-light bg-ink-700 px-5 py-2.5 text-sm text-text-primary transition hover:bg-ink-600"
        >
          浏览工具
        </RouterLink>
        <RouterLink
          to="/resources"
          class="rounded-md border border-border-light bg-ink-700 px-5 py-2.5 text-sm text-text-primary transition hover:bg-ink-600"
        >
          资源中心
        </RouterLink>
      </div>
    </div>
  </PageContainer>
</template>