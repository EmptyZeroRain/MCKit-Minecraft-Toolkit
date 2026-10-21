// 统一会话退出：管理员与普通用户共用同一服务端会话 Cookie。
//
// 前端却维护了两套独立登录态（stores/auth.ts 与 stores/user-auth.ts）。
// 若只清理其中一套，另一套仍会被路由守卫视为「已登录」：
//   - 后台账号切到前台后退出：管理端 store 残留 → 点击「登录 / 注册」被送回 /admin；
//   - 后台退出后回到前台：用户端 store 残留 → 点击「登录」被送回 /workspace。
// 因此所有退出入口都必须通过本 composable：只调用一次登出接口（撤销共享会话），
// 然后同时清空两套前端状态与各自的 CSRF token。
import { useAuthStore } from '@/stores/auth'
import { useUserAuthStore } from '@/stores/user-auth'
import { useUserWorkspaceStore } from '@/stores/user-workspace'
import { setCsrfToken } from '@/api/http'
import { setUserCsrfToken } from '@/api/user-http'

/** 是否为后台账号（admin / editor 等非 user 角色）。 */
function isBackendRole(role: string | undefined): boolean {
  return !!role && role !== 'user'
}

export function useSessionLogout() {
  const auth = useAuthStore()
  const userAuth = useUserAuthStore()
  const workspace = useUserWorkspaceStore()

  /** 仅清理本地登录态（会话已在服务端失效时使用，例如改密 / 撤销全部会话）。 */
  function clearLocalSession(): void {
    auth.reset()
    userAuth.reset()
    workspace.reset()
    setCsrfToken('')
    setUserCsrfToken('')
  }

  /**
   * 退出当前账号（普通用户或后台账号）。
   * 始终清理本地状态：即使登出接口失败（如会话已过期），也不应让前端停留在「假登录」。
   */
  async function logout(): Promise<void> {
    const isBackend = auth.isLoggedIn || isBackendRole(userAuth.user?.role)
    try {
      if (isBackend) {
        await auth.logout()
      } else {
        await userAuth.logout()
      }
    } catch (err) {
      console.error('退出登录失败，已清理本地会话', err)
    } finally {
      clearLocalSession()
    }
  }

  return { logout, clearLocalSession }
}
