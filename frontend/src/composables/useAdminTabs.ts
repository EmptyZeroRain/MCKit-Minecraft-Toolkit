// 后台「合并页」通用 Tab 逻辑（gj.md §2.2 菜单合并规则）。
//
// 统一处理三件事，避免每个合并页各写一遍：
//   1. 按权限点过滤可见 Tab（前端显隐仅为体验优化，后端仍 403 兜底）；
//   2. 把当前 Tab 同步到 URL query，支持刷新 / 分享 / 回退；
//   3. 非法或无权限的 Tab 参数回退到首个可用 Tab，避免出现空白页。
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

/** 一个 Tab 的声明：key 为标识，label 为展示名，perm / permsAny 为所需权限点。 */
export interface AdminTab {
  key: string
  label: string
  /** 单个权限点；与 permsAny 二选一。 */
  perm?: string
  /** 任一权限点满足即可；用于「管理员 / 普通用户」这类共享同一 Tab 的入口。 */
  permsAny?: string[]
}

/**
 * useAdminTabs 生成后台合并页的 Tab 状态。
 * @param allTabs 全部 Tab 声明（顺序即展示顺序）
 * @param queryKey URL query 键名，默认 `tab`
 */
export function useAdminTabs(allTabs: AdminTab[], queryKey = 'tab') {
  const route = useRoute()
  const router = useRouter()
  const auth = useAuthStore()

  // 仅保留当前账号有权限的 Tab。
  const tabs = computed(() =>
    allTabs.filter((t) => {
      if (t.perm) return auth.hasPermission(t.perm)
      if (t.permsAny?.length) return t.permsAny.some((p) => auth.hasPermission(p))
      return true
    }),
  )

  function isValid(value: unknown): value is string {
    return typeof value === 'string' && tabs.value.some((t) => t.key === value)
  }

  // 初始 Tab：URL 参数合法且有权限则采用，否则取首个可用 Tab。
  const active = ref<string>(
    isValid(route.query[queryKey])
      ? (route.query[queryKey] as string)
      : (tabs.value[0]?.key ?? ''),
  )

  // 监听 URL 参数：非法或无权限时回退，保证不出现「空白 Tab」。
  watch(
    () => route.query[queryKey],
    (value) => {
      if (isValid(value)) {
        active.value = value
      } else if (tabs.value.length && !tabs.value.some((t) => t.key === active.value)) {
        active.value = tabs.value[0].key
      }
    },
    { immediate: true },
  )

  function select(key: string) {
    active.value = key
    router.replace({ query: { ...route.query, [queryKey]: key } })
  }

  return { tabs, active, select }
}
