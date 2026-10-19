<script setup lang="ts">
// 顶部导航：LOGO、主导航、全局搜索、移动端菜单按钮
import { ref, onMounted } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import GlobalSearch from '@/components/search/GlobalSearch.vue'
import ThemeToggle from '@/components/ThemeToggle.vue'
import MobileNav from '@/components/layout/MobileNav.vue'
import { useUserAuthStore } from '@/stores/user-auth'

interface NavItem {
  label: string
  to: string
}

// 顶层导航只放「页面级」入口；具体工具统一收进工具中心，
// 避免导航项随工具增加而膨胀、在 max-w-7xl 容器下换行溢出。
const navItems: NavItem[] = [
  { label: '首页', to: '/' },
  { label: '工具中心', to: '/tools' },
  { label: '开发文档', to: '/dev' },
  { label: '资源中心', to: '/resources' },
  { label: '关于本站', to: '/about' },
]

const route = useRoute()
const router = useRouter()
const scrolled = ref(false)
const searchOpen = ref(false)
const drawerOpen = ref(false)
const userAuth = useUserAuthStore()

function onScroll() {
  scrolled.value = window.scrollY > 8
}

onMounted(() => {
  window.addEventListener('scroll', onScroll, { passive: true })
  onScroll()
  // 恢复用户会话（幂等）：让导航与收藏按钮在任意页面都知道登录态。
  void userAuth.restoreSession()
})

async function onLogout() {
  await userAuth.logout()
  if (route.path.startsWith('/workspace')) router.push('/')
}

function isActive(to: string): boolean {
  if (to === '/') return route.path === '/'
  return route.path.startsWith(to)
}
</script>

<template>
  <header
    class="sticky top-0 z-40 border-b border-border-dark backdrop-blur transition-shadow"
    :class="{ 'shadow-lg': scrolled }"
    style="background-color: var(--mc-header-bg)"
  >
    <div class="mx-auto flex h-16 max-w-7xl items-center gap-4 px-4 sm:px-6">
      <!-- 移动端菜单按钮 -->
      <button
        class="inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-md text-text-secondary hover:bg-ink-700 hover:text-text-primary lg:hidden"
        aria-label="打开菜单"
        :aria-expanded="drawerOpen"
        @click="drawerOpen = true"
      >
        <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M3.75 6.75h16.5M3.75 12h16.5m-16.5 5.25h16.5" />
        </svg>
      </button>

      <RouterLink to="/" class="flex shrink-0 items-center gap-2" aria-label="MC 工具站首页">
        <img src="/favicon.svg" alt="" class="h-9 w-9" />
        <span class="hidden text-lg font-bold tracking-wide sm:block">
          MC <span class="text-grass-400">工具站</span>
        </span>
      </RouterLink>

      <!-- 桌面导航 -->
      <nav class="hidden flex-1 items-center gap-1 lg:flex" aria-label="主导航">
        <RouterLink
          v-for="item in navItems"
          :key="item.to"
          :to="item.to"
          class="rounded-md px-3 py-2 text-sm text-text-secondary transition hover:bg-ink-700 hover:text-text-primary"
          :class="{ '!bg-ink-700 !text-grass-300': isActive(item.to) }"
        >
          {{ item.label }}
        </RouterLink>
      </nav>

      <div class="ml-auto flex items-center gap-2">
        <ThemeToggle />
        <!-- 桌面搜索 -->
        <div class="hidden w-64 md:block">
          <GlobalSearch />
        </div>
        <button
          class="inline-flex h-10 w-10 items-center justify-center rounded-md text-text-secondary hover:bg-ink-700 hover:text-text-primary md:hidden"
          aria-label="打开搜索"
          @click="searchOpen = !searchOpen"
        >
          <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" d="M21 21l-4.35-4.35M17 10.5a6.5 6.5 0 11-13 0 6.5 6.5 0 0113 0z" />
          </svg>
        </button>
        <RouterLink
          v-if="userAuth.isLoggedIn"
          to="/workspace"
          class="hidden h-10 items-center gap-1.5 rounded-md border border-border-dark px-3 text-sm text-text-secondary transition hover:bg-ink-700 hover:text-text-primary sm:inline-flex"
          title="我的工作台"
        >
          <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M15.75 6a3.75 3.75 0 11-7.5 0 3.75 3.75 0 017.5 0zM4.5 19.5a7.5 7.5 0 0115 0v.75H4.5v-.75z" />
          </svg>
          <span class="max-w-[6rem] truncate">{{ userAuth.displayName }}</span>
        </RouterLink>
        <RouterLink
          v-else
          to="/login"
          class="hidden h-10 items-center gap-1.5 rounded-md border border-border-dark px-3 text-sm text-text-secondary transition hover:bg-ink-700 hover:text-text-primary sm:inline-flex"
          title="登录 / 注册"
        >
          登录
        </RouterLink>
        <button
          v-if="userAuth.isLoggedIn"
          type="button"
          class="hidden h-10 w-10 items-center justify-center rounded-md text-text-secondary transition hover:bg-ink-700 hover:text-text-primary sm:inline-flex"
          aria-label="退出登录"
          title="退出登录"
          @click="onLogout"
        >
          <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M15.75 9V5.25A2.25 2.25 0 0013.5 3h-6A2.25 2.25 0 005.25 5.25v13.5A2.25 2.25 0 007.5 21h6a2.25 2.25 0 002.25-2.25V15M19.5 12H9.75m9.75 0l-3-3m3 3l-3 3" />
          </svg>
        </button>
        <RouterLink
          to="/admin"
          class="hidden h-10 items-center gap-1.5 rounded-md border border-border-dark px-3 text-sm text-text-secondary transition hover:bg-ink-700 hover:text-text-primary sm:inline-flex"
          title="管理后台"
        >
          <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
          </svg>
          管理
        </RouterLink>
      </div>
    </div>

    <!-- 移动端搜索展开层 -->
    <div v-if="searchOpen" class="border-t border-border-dark bg-ink-900 px-4 py-3 md:hidden">
      <GlobalSearch autofocus @done="searchOpen = false" />
    </div>
  </header>

  <!-- 移动端导航抽屉 -->
  <MobileNav :open="drawerOpen" @close="drawerOpen = false" />
</template>