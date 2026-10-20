// 管理后台会话操作（返回前台 / 退出登录）。
// 侧边栏与顶栏共用同一实现，避免两处逻辑分叉。
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useUiStore } from '@/stores/ui'

export function useAdminSession() {
  const router = useRouter()
  const auth = useAuthStore()
  const ui = useUiStore()

  /** 退出管理会话并回到统一登录页。 */
  async function logout() {
    try {
      await auth.logout()
      ui.success('已退出登录')
      router.push('/login')
    } catch (err) {
      ui.error('退出失败，请重试')
      console.error(err)
    }
  }

  /** 切回前台站点（保留当前登录态，仅做路由跳转）。 */
  function goHome() {
    router.push('/')
  }

  return { logout, goHome }
}
