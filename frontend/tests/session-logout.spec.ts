// 统一会话退出回归测试：
// 前后台共用同一服务端会话 Cookie，但前端有两套独立 store。
// 管理员登录 → 切换前台 → 点击退出后，管理端 store 必须一并清空，
// 否则访问 /login 会被守卫判定为「已登录管理员」并送回 /admin。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

const userApi = vi.hoisted(() => ({
  logout: vi.fn().mockResolvedValue(undefined),
  me: vi.fn(),
  login: vi.fn(),
  unifiedLogin: vi.fn(),
  register: vi.fn(),
  updateProfile: vi.fn(),
  changePassword: vi.fn(),
  deleteAccount: vi.fn(),
  listFavorites: vi.fn().mockResolvedValue([]),
}))

const adminApi = vi.hoisted(() => ({
  logout: vi.fn().mockResolvedValue(undefined),
  me: vi.fn(),
  login: vi.fn(),
  changePassword: vi.fn(),
  updateProfile: vi.fn(),
}))

vi.mock('@/api/user', () => userApi)
vi.mock('@/api/admin-auth', () => adminApi)

import router from '@/router'
import { useSessionLogout } from '@/composables/useSessionLogout'
import { useAuthStore } from '@/stores/auth'
import { useUserAuthStore } from '@/stores/user-auth'
import { useUserWorkspaceStore } from '@/stores/user-workspace'

const FAVORITE = {
  id: 'f1',
  kind: 'tool',
  object_id: 'seed-tools',
  label: '种子工具',
  url: '/seed-tools',
  created_at: '2026-01-01T00:00:00Z',
}

/** 模拟管理员通过统一登录页登录后的状态：两套 store 同时有值。 */
function seedAdminSession() {
  const auth = useAuthStore()
  auth.user = { id: 'a1', username: 'admin', role: 'admin' } as never
  auth.initialized = true

  const userAuth = useUserAuthStore()
  userAuth.user = { id: 'a1', username: 'admin', role: 'admin' }
  userAuth.initialized = true

  useUserWorkspaceStore().favorites = [FAVORITE] as never
  return { auth, userAuth }
}

describe('统一会话退出', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    // jsdom 未实现 scrollTo，路由切换时会被 scrollBehavior 调用。
    vi.stubGlobal('scrollTo', vi.fn())
    userApi.logout.mockResolvedValue(undefined)
    adminApi.logout.mockResolvedValue(undefined)
  })

  it('管理员从前台退出时同时清空两套 store 与个人数据', async () => {
    const { auth, userAuth } = seedAdminSession()

    await useSessionLogout().logout()

    expect(adminApi.logout).toHaveBeenCalledTimes(1)
    expect(userApi.logout).not.toHaveBeenCalled()
    expect(auth.isLoggedIn).toBe(false)
    expect(userAuth.isLoggedIn).toBe(false)
    expect(useUserWorkspaceStore().favorites).toHaveLength(0)
  })

  it('普通用户退出只调用用户登出接口并清空登录态', async () => {
    const userAuth = useUserAuthStore()
    userAuth.user = { id: 'u1', username: 'alice', role: 'user' }
    userAuth.initialized = true

    await useSessionLogout().logout()

    expect(userApi.logout).toHaveBeenCalledTimes(1)
    expect(adminApi.logout).not.toHaveBeenCalled()
    expect(userAuth.isLoggedIn).toBe(false)
  })

  it('管理员前台退出后访问 /login 不再被送回后台', async () => {
    seedAdminSession()
    await useSessionLogout().logout()

    await router.push('/')
    await router.push('/login')
    await router.isReady()

    expect(router.currentRoute.value.path).toBe('/login')
  })
})
