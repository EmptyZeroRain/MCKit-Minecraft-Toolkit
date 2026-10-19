// ===== 管理端认证状态（需求 6.1）=====
// 注意：前端仅改善体验，真正的权限校验在后端。
// 不在 localStorage 存放令牌——管理会话使用服务端 Cookie。
import { defineStore } from 'pinia'
import type { AdminUser } from '@/types/admin'
import * as adminAuthApi from '@/api/admin-auth'

export const useAuthStore = defineStore('auth', {
  state: () => ({
    user: null as AdminUser | null,
    loaded: false,
    initialized: false,
  }),
  getters: {
    isLoggedIn: (state) => state.user !== null,
    isAdmin: (state) => state.user?.role === 'admin',
    displayName: (state) => state.user?.username ?? '',
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
    /** 登录（凭证由后端校验） */
    async login(username: string, password: string): Promise<AdminUser> {
      const user = await adminAuthApi.login({ username, password })
      this.user = user
      return user
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