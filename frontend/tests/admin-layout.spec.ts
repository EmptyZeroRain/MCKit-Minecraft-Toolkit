// 管理后台顶栏可用性测试：
//   1) 顶栏始终提供「返回前台」与「退出登录」（长侧边栏中不易发现，故固定到顶栏）；
//   2) 「返回前台」跳到前台首页，保留登录态（仅路由跳转）；
//   3) 「退出登录」调用登出接口并回到后台登录页。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory } from 'vue-router'
import AdminLayout from '@/layouts/AdminLayout.vue'
import { useAuthStore } from '@/stores/auth'
import type { AdminUser } from '@/types/admin'

const authApi = vi.hoisted(() => ({
  login: vi.fn(),
  logout: vi.fn().mockResolvedValue(undefined),
  me: vi.fn(),
  changePassword: vi.fn(),
}))
vi.mock('@/api/admin-auth', () => authApi)

const Root = { template: '<RouterView />' }

function makeRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: { template: '<div>前台首页</div>' } },
      { path: '/admin/login', component: { template: '<div>登录</div>' } },
      {
        path: '/admin',
        component: AdminLayout,
        children: [{ path: '', component: { template: '<div>仪表盘内容</div>' } }],
      },
    ],
  })
}

async function mountLayout() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const auth = useAuthStore()
  auth.user = {
    id: 'a1',
    username: 'admin',
    role: 'admin',
    is_active: true,
    last_login_at: null,
    created_at: '2026-10-01T00:00:00Z',
  } as AdminUser

  const router = makeRouter()
  await router.push('/admin')
  await router.isReady()

  const wrapper = mount(Root, { global: { plugins: [pinia, router] } })
  await flushPromises()
  return { wrapper, router }
}

describe('管理后台顶栏 · 切换与退出', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    authApi.logout.mockClear()
  })

  it('顶栏提供「返回前台」与「退出登录」', async () => {
    const { wrapper } = await mountLayout()
    const texts = wrapper.findAll('header button').map((b) => b.text())
    expect(texts).toContain('返回前台')
    expect(texts).toContain('退出登录')
  })

  it('点击「返回前台」跳转到前台首页', async () => {
    const { wrapper, router } = await mountLayout()
    const btn = wrapper.findAll('header button').find((b) => b.text() === '返回前台')!
    await btn.trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/')
    expect(authApi.logout).not.toHaveBeenCalled()
  })

  it('点击「退出登录」调用登出并回到后台登录页', async () => {
    const { wrapper, router } = await mountLayout()
    const btn = wrapper.findAll('header button').find((b) => b.text() === '退出登录')!
    await btn.trigger('click')
    await flushPromises()
    expect(authApi.logout).toHaveBeenCalledTimes(1)
    expect(router.currentRoute.value.path).toBe('/admin/login')
  })
})
