// ===== 普通用户登录状态（需求 §12）=====
// 与管理员 store（stores/auth.ts）相互独立：
//   - 令牌不落地：使用服务端 HttpOnly 统一会话 Cookie（SESSION_COOKIE_NAME）；
//   - 前端仅在内存保存用户信息与（会话绑定的）CSRF token；
//   - CSRF token 由 api/user-http.ts 统一附加到写请求。
import { defineStore } from 'pinia'
import type { RegisterPayload, UserAccount } from '@/types/user'
import * as userApi from '@/api/user'
import { useUserWorkspaceStore } from '@/stores/user-workspace'

export const useUserAuthStore = defineStore('user-auth', {
  state: () => ({
    user: null as UserAccount | null,
    loaded: false,
    initialized: false,
  }),
  getters: {
    isLoggedIn: (state) => state.user !== null,
    /** 展示名优先用昵称，未设置时回退到用户名 */
    displayName: (state) => state.user?.nickname || state.user?.username || '',
  },
  actions: {
    /** 应用启动 / 访问受限页时恢复会话 */
    async restoreSession(): Promise<void> {
      if (this.initialized) return
      this.initialized = true
      try {
        this.user = await userApi.me()
        await useUserWorkspaceStore().loadFavorites()
      } catch {
        this.user = null
      } finally {
        this.loaded = true
      }
    },
    async login(identifier: string, password: string): Promise<UserAccount> {
      const account = await userApi.login({ identifier, password })
      this.user = account
      this.loaded = true
      this.initialized = true
      await useUserWorkspaceStore().loadFavorites()
      return account
    },
    /** 注册（不自动登录）；需携带图形验证码 */
    async register(payload: RegisterPayload): Promise<UserAccount> {
      return userApi.register(payload)
    },
    /** 更新展示资料（昵称 / 头像），成功后同步本地用户信息 */
    async updateProfile(nickname: string, avatarUrl: string): Promise<void> {
      const account = await userApi.updateProfile({ nickname, avatar_url: avatarUrl })
      this.user = { ...(this.user as UserAccount), ...account }
    },
    async logout(): Promise<void> {
      try {
        await userApi.logout()
      } finally {
        this.reset()
        useUserWorkspaceStore().reset()
      }
    },
    /** 修改密码：成功后后端使全部会话失效，前端同步清理登录态与个人数据 */
    async changePassword(oldPassword: string, newPassword: string): Promise<void> {
      await userApi.changePassword(oldPassword, newPassword)
      this.reset()
      useUserWorkspaceStore().reset()
    },
    /** 删除账号：成功后个人数据（收藏 / 最近使用 / 常用种子）一并清除 */
    async deleteAccount(password: string): Promise<void> {
      await userApi.deleteAccount(password)
      this.reset()
      useUserWorkspaceStore().reset()
    },
    /** 清空前端登录态（会话失效 / 401 时调用） */
    reset() {
      this.user = null
    },
  },
})
