<script setup lang="ts">
// 页脚：站点信息、相关页面链接、非官方声明
import { RouterLink } from 'vue-router'
import { BUILTIN_TOOLS } from '@/utils/builtin-tools'

const year = new Date().getFullYear()
</script>

<template>
  <footer class="border-t border-border-dark bg-ink-900">
    <div class="mx-auto grid max-w-7xl gap-8 px-4 py-10 sm:px-6 md:grid-cols-3">
      <div>
        <div class="mb-3 flex items-center gap-2">
          <img src="/favicon.svg" alt="" class="h-8 w-8" />
          <span class="text-lg font-bold">MC <span class="text-grass-400">工具站</span></span>
        </div>
        <p class="text-sm leading-relaxed text-text-secondary">
          Minecraft Java 版玩家工具与资源导航。本网站是<strong class="text-text-primary">非官方社区项目</strong>，
          与 Mojang Studios、Microsoft 无任何关联。
        </p>
      </div>

      <nav aria-label="站点信息">
        <h2 class="mb-3 text-sm font-semibold text-text-primary">站点信息</h2>
        <ul class="space-y-2 text-sm text-text-secondary">
          <li><RouterLink to="/about" class="hover:text-grass-300">关于本站</RouterLink></li>
          <li><RouterLink to="/feedback" class="hover:text-grass-300">问题反馈</RouterLink></li>
          <li><RouterLink to="/privacy" class="hover:text-grass-300">隐私政策</RouterLink></li>
          <li><RouterLink to="/copyright" class="hover:text-grass-300">版权与侵权</RouterLink></li>
        </ul>
      </nav>

      <div>
        <h2 class="mb-3 text-sm font-semibold text-text-primary">常用工具</h2>
        <ul class="space-y-2 text-sm text-text-secondary">
          <li v-for="tool in BUILTIN_TOOLS" :key="tool.slug">
            <RouterLink :to="tool.route" class="hover:text-grass-300">{{ tool.name }}</RouterLink>
          </li>
        </ul>
      </div>
    </div>

    <div class="border-t border-border-dark py-4 text-center text-xs text-text-muted">
      © {{ year }} MC 工具站 · 本站仅提供索引与导航，资源版权归原作者所有
    </div>
  </footer>
</template>