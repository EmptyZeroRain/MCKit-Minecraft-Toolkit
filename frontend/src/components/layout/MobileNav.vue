<script setup lang="ts">
// 移动端导航抽屉（需求 4.3）
import { RouterLink, useRoute } from 'vue-router'
import ThemeToggle from '@/components/ThemeToggle.vue'

defineProps<{ open: boolean }>()
const emit = defineEmits<{ close: [] }>()
const route = useRoute()

// 与桌面端一致的顶层导航；具体工具在工具中心内提供
const navItems = [
  { label: '首页', to: '/' },
  { label: '工具中心', to: '/tools' },
  { label: '资源中心', to: '/resources' },
  { label: '关于本站', to: '/about' },
  { label: '问题反馈', to: '/feedback' },
  { label: '管理后台', to: '/admin' },
]

function isActive(to: string): boolean {
  if (to === '/') return route.path === '/'
  return route.path.startsWith(to)
}
</script>

<template>
  <Teleport to="body">
    <Transition name="drawer">
      <div v-if="open" class="fixed inset-0 z-50 lg:hidden">
        <!-- 遮罩 -->
        <div class="absolute inset-0 bg-black/50" aria-hidden="true" @click="emit('close')" />
        <!-- 抽屉 -->
        <div
          class="absolute inset-y-0 right-0 flex w-72 max-w-[85vw] flex-col border-l border-border-dark bg-ink-800 shadow-2xl"
          role="dialog"
          aria-modal="true"
          aria-label="移动端导航"
        >
          <div class="flex h-16 items-center justify-between border-b border-border-dark px-5">
            <div class="flex items-center gap-2">
              <img src="/favicon.svg" alt="" class="h-8 w-8" />
              <span class="font-bold text-text-primary">MC <span class="text-grass-400">工具站</span></span>
            </div>
            <button
              type="button"
              class="inline-flex h-9 w-9 items-center justify-center rounded-md text-text-secondary hover:bg-ink-700 hover:text-text-primary"
              aria-label="关闭菜单"
              @click="emit('close')"
            >
              ✕
            </button>
          </div>

          <nav class="flex-1 overflow-y-auto px-4 py-4" aria-label="移动端导航">
            <ul class="space-y-1">
              <li v-for="item in navItems" :key="item.to">
                <RouterLink
                  :to="item.to"
                  class="block rounded-md px-3 py-2.5 text-sm text-text-secondary transition hover:bg-ink-700 hover:text-text-primary"
                  :class="{ '!bg-ink-700 !text-grass-300': isActive(item.to) }"
                  @click="emit('close')"
                >
                  {{ item.label }}
                </RouterLink>
              </li>
            </ul>
          </nav>

          <div class="flex items-center justify-between border-t border-border-dark px-5 py-4">
            <span class="text-xs text-text-muted">外观</span>
            <ThemeToggle />
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.drawer-enter-active,
.drawer-leave-active {
  transition: opacity 0.2s ease;
}
.drawer-enter-active .absolute.inset-y-0,
.drawer-leave-active .absolute.inset-y-0 {
  transition: transform 0.25s ease;
}
.drawer-enter-from,
.drawer-leave-to {
  opacity: 0;
}
.drawer-enter-from .absolute.inset-y-0 {
  transform: translateX(100%);
}
.drawer-leave-to .absolute.inset-y-0 {
  transform: translateX(100%);
}
</style>