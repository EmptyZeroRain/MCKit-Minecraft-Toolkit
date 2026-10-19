<script setup lang="ts">
// 收藏按钮：
//   - 已登录：收藏保存在服务端（按账号隔离、跨设备同步）；
//   - 未登录：仅「工具 / 资源」沿用浏览器本地收藏，保持免登录可用；
//     物品 / 百科在未登录时提示先登录再收藏。
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useFavoritesStore } from '@/stores/favorites'
import { useUserAuthStore } from '@/stores/user-auth'
import { useUserWorkspaceStore } from '@/stores/user-workspace'
import type { FavoriteKind } from '@/types/user'

const props = defineProps<{
  /** 收藏类型：工具 / 物品 / 资源 / 百科 */
  kind: FavoriteKind
  /** 对象唯一标识（slug / item id） */
  slug: string
  /** 展示名称（写入个人工作台，缺省用 slug） */
  label?: string
  /** 详情页路径（写入个人工作台，缺省为空） */
  url?: string
}>()

const router = useRouter()
const route = useRoute()
const favorites = useFavoritesStore()
const userAuth = useUserAuthStore()
const workspace = useUserWorkspaceStore()

const busy = ref(false)
const hint = ref('')

const isFav = computed(() => {
  if (userAuth.isLoggedIn) return workspace.isFavorite(props.kind, props.slug)
  if (props.kind === 'tool') return favorites.isToolFavorite(props.slug)
  if (props.kind === 'resource') return favorites.isResourceFavorite(props.slug)
  return false
})

/** 未登录且该类型不支持本地收藏时，按钮改为「登录后可收藏」 */
const needLogin = computed(() => !userAuth.isLoggedIn && props.kind !== 'tool' && props.kind !== 'resource')

async function toggle() {
  if (busy.value) return
  hint.value = ''

  if (userAuth.isLoggedIn) {
    busy.value = true
    try {
      await workspace.toggle(props.kind, props.slug, props.label ?? props.slug, props.url ?? '')
    } catch {
      hint.value = '操作失败，请稍后重试'
    } finally {
      busy.value = false
    }
    return
  }

  if (needLogin.value) {
    router.push({ path: '/login', query: { redirect: route.fullPath } })
    return
  }

  if (props.kind === 'tool') favorites.toggleTool(props.slug)
  else if (props.kind === 'resource') favorites.toggleResource(props.slug)
}
</script>

<template>
  <span class="inline-flex items-center gap-2">
    <button
      type="button"
      class="inline-flex items-center gap-1.5 rounded-md border px-2.5 py-1.5 text-xs transition disabled:opacity-60"
      :class="
        isFav
          ? 'border-amber-mc/60 bg-amber-mc/10 text-amber-mc'
          : 'border-border-light bg-ink-700 text-text-secondary hover:bg-ink-600 hover:text-text-primary'
      "
      :aria-pressed="isFav"
      :disabled="busy"
      :title="needLogin ? '登录后可收藏' : isFav ? '取消收藏' : '收藏'"
      @click="toggle"
    >
      <svg class="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
        <path
          stroke-linecap="round"
          stroke-linejoin="round"
          d="M11.48 3.499a.562.562 0 011.04 0l2.125 5.111a.563.563 0 00.475.345l5.518.442c.499.04.701.663.321.988l-4.204 3.602a.563.563 0 00-.182.557l1.285 5.385a.562.562 0 01-.84.61l-4.725-2.885a.563.563 0 00-.586 0L6.982 20.54a.562.562 0 01-.84-.61l1.285-5.386a.562.562 0 00-.182-.557l-4.204-3.602a.562.562 0 01.321-.988l5.518-.442a.563.563 0 00.475-.345L11.48 3.5z"
        />
      </svg>
      {{ isFav ? '已收藏' : needLogin ? '登录后可收藏' : '收藏' }}
    </button>
    <span v-if="hint" class="text-xs text-danger-mc">{{ hint }}</span>
  </span>
</template>
