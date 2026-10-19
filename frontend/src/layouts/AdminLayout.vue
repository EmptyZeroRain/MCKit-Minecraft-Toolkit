<script setup lang="ts">
// 管理后台布局：侧边栏 + 顶栏 + 内容区
// 顶栏右上角收敛为「用户」菜单（返回前台 / 用户信息 / 修改密码 / 退出登录），
// 避免多个按钮挤占顶栏，且会话类操作集中在同一入口。
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import AdminSidebar from '@/components/admin/AdminSidebar.vue'
import { useAdminSession } from '@/composables/useAdminSession'
import { useAuthStore } from '@/stores/auth'

const route = useRoute()
const auth = useAuthStore()
const { logout, goHome } = useAdminSession()

const pageTitle = () =>
  (route.meta.title as string | undefined) ?? '管理后台'

const menuOpen = ref(false)

function toggleMenu() {
  menuOpen.value = !menuOpen.value
}

function closeMenu() {
  menuOpen.value = false
}

/** 关闭菜单后再执行动作，避免菜单残留在跳转过程中。 */
function runAndClose(action: () => void) {
  closeMenu()
  action()
}

function onClickOutside(e: MouseEvent) {
  if (!(e.target as HTMLElement).closest('.admin-user-menu')) menuOpen.value = false
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') menuOpen.value = false
}

onMounted(() => {
  document.addEventListener('click', onClickOutside)
  document.addEventListener('keydown', onKeydown)
})
onBeforeUnmount(() => {
  document.removeEventListener('click', onClickOutside)
  document.removeEventListener('keydown', onKeydown)
})
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
        <div class="admin-user-menu relative shrink-0">
          <button
            type="button"
            class="flex items-center gap-2 rounded-md border border-border-dark px-3 py-1.5 text-xs text-text-secondary transition hover:bg-ink-700 hover:text-text-primary"
            :title="auth.displayName"
            aria-haspopup="menu"
            :aria-expanded="menuOpen"
            @click="toggleMenu"
          >
            <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.8">
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z"
              />
            </svg>
            用户
            <svg class="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" />
            </svg>
          </button>

          <div
            v-if="menuOpen"
            role="menu"
            class="absolute right-0 top-full z-40 mt-2 w-40 overflow-hidden rounded-md border border-border-dark bg-ink-900 py-1 shadow-lg"
          >
            <button
              type="button"
              role="menuitem"
              class="flex w-full items-center px-3 py-2 text-left text-xs text-text-secondary transition hover:bg-ink-700 hover:text-text-primary"
              @click="runAndClose(goHome)"
            >
              返回前台
            </button>
            <RouterLink
              role="menuitem"
              to="/admin/profile#account"
              class="flex w-full items-center px-3 py-2 text-left text-xs text-text-secondary transition hover:bg-ink-700 hover:text-text-primary"
              @click="closeMenu"
            >
              用户信息
            </RouterLink>
            <RouterLink
              role="menuitem"
              to="/admin/profile#password"
              class="flex w-full items-center px-3 py-2 text-left text-xs text-text-secondary transition hover:bg-ink-700 hover:text-text-primary"
              @click="closeMenu"
            >
              修改密码
            </RouterLink>
            <button
              type="button"
              role="menuitem"
              class="flex w-full items-center px-3 py-2 text-left text-xs text-danger-mc transition hover:bg-danger-mc/10"
              @click="runAndClose(logout)"
            >
              退出登录
            </button>
          </div>
        </div>
      </header>
      <main id="admin-main-content" tabindex="-1" class="flex-1 p-6">
        <RouterView />
      </main>
    </div>
  </div>
</template>
