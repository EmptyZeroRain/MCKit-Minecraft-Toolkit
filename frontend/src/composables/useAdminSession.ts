// 管理后台会话操作（返回前台 / 退出登录）。
// 侧边栏与顶栏共用同一实现，避免两处逻辑分叉。
import { useRouter } from 'vue-router'
import { useUiStore } from '@/stores/ui'
import { useSessionLogout } from '@/composables/useSessionLogout'

export function useAdminSession() {
  const router = useRouter()
  const ui = useUiStore()
  const { logout: logoutSession } = useSessionLogout()

  /** 退出共享会话（前后台登录态一并清理），回到统一登录页。 */
  async function logout() {
    await logoutSession()
    ui.success('已退出登录')
    router.push('/login')
  }

  /** 切回前台站点（保留当前登录态，仅做路由跳转）。 */
  function goHome() {
    router.push('/')
  }

  return { logout, goHome }
}
