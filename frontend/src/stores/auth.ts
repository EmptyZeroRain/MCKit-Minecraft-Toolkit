// ===== 管理端认证状态（需求 6.1）=====
// 注意：前端仅改善体验，真正的权限校验在后端。
// 不在 localStorage 存放令牌——管理会话使用服务端 Cookie。
import { defineStore } from 'pinia'
import type { AdminUser } from '@/types/admin'
import * as adminAuthApi from '@/api/admin-auth'
import type { LoginOutcome } from '@/api/admin-auth'

export const useAuthStore = defineStore('auth', {
  state: () => ({
    user: null as AdminUser | null,
    loaded: false,
    initialized: false,
  }),
  getters: {
    isLoggedIn: (state) => state.user !== null,
    isAdmin: (state) => state.user?.role === 'admin',
    /** 展示名：优先昵称，未设置时回退到登录用户名 */
    displayName: (state) => state.user?.nickname || state.user?.username || '',
    /** 头像地址（未设置时为空串，由组件决定占位展示） */
    avatarUrl: (state) => state.user?.avatar_url ?? '',
    /**
     * 是否具备某权限点（决定后台入口显隐）。
     * 后端登录 / 会话接口会返回 permissions；缺失时按内置角色默认权限兜底
     * （admin 全权、editor 仅内容管理），与后端内置角色保持一致。
     * 注意：前端显隐只是体验优化，真正的权限校验在后端。
     */
    hasPermission: (state) => (perm: string) => {
      const user = state.user
      if (!user) return false
      if (user.permissions) return user.permissions.includes(perm)
      if (user.role === 'admin') return true
      if (user.role === 'editor') return perm.startsWith('content.')
      return false
    },
  },
  actions: {
    /** 应用启动时恢复会话（调用 /auth/me） */
    async restoreSession(): Promise<void> {
      if (this.initialized) return
      this.initialized = true
      try {
        const user = await adminAuthApi.me()
        this.user = user
      } catch {
        this.user = null
      } finally {
        this.loaded = true
      }
    },
    /** 登录（凭证由后端校验）；启用 MFA 的账号返回票据，需再调 verifyMFA */
    async login(username: string, password: string): Promise<LoginOutcome> {
      const outcome = await adminAuthApi.login({ username, password })
      if (!outcome.mfa_required) this.user = outcome.user
      return outcome
    },
    /** 第二因子校验，成功后写入登录态 */
    async verifyMFA(ticket: string, code: string): Promise<AdminUser> {
      const user = await adminAuthApi.verifyMFA(ticket, code)
      this.user = user
      return user
    },
    /** 更新当前账号展示资料（昵称 / 头像），成功后同步本地用户信息 */
    async updateProfile(nickname: string, avatarUrl: string): Promise<void> {
      const user = await adminAuthApi.updateProfile({ nickname, avatar_url: avatarUrl })
      // 后端只回传展示字段，合并进现有 user，避免覆盖权限等既有信息。
      this.user = { ...(this.user as AdminUser), ...user }
    },
    async logout(): Promise<void> {
      try {
        await adminAuthApi.logout()
      } finally {
        this.user = null
      }
    },
    /** 静默刷新会话（401 重试路径） */
    async refreshSession(): Promise<void> {
      if (!this.user) throw new Error('not logged in')
      const user = await adminAuthApi.me()
      this.user = user
    },
    reset() {
      this.user = null
    },
  },
})