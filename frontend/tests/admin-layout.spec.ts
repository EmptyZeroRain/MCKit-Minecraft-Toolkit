// 管理后台顶栏可用性测试：
//   1) 顶栏右上角收敛为「用户」菜单入口（原「返回前台 / 退出登录」收进菜单）；
//   2) 菜单展开后提供「返回前台 / 用户信息 / 修改密码 / 退出登录」；
//   3) 「返回前台」跳到前台首页，保留登录态（仅路由跳转）；
//   4) 「退出登录」调用登出接口并回到后台登录页；
//   5) 「用户信息 / 修改密码」跳转用户信息页对应锚点。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory } from 'vue-router'
import AdminLayout from '@/layouts/AdminLayout.vue'
import { useAuthStore } from '@/stores/auth'
import { useUserAuthStore } from '@/stores/user-auth'
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
      { path: '/login', component: { template: '<div>登录</div>' } },
      { path: '/admin/login', component: { template: '<div>登录</div>' } },
      { path: '/admin/profile', component: { template: '<div>用户信息</div>' } },
      {
        path: '/admin',
        component: AdminLayout,
        children: [{ path: '', component: { template: '<div>仪表盘内容</div>' } }],
      },
      // 侧边栏会渲染全部后台入口，补一个兜底路由避免测试日志被 no match 警告淹没。
      { path: '/admin/:pathMatch(.*)*', component: { template: '<div />' } },
    ],
  })
}

async function mountLayout() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const auth = useAuthStore()
  auth.user = {
    id: 'a1',
    user_no: 'UE389F2938F',
    username: 'admin',
    nickname: '运营管理员',
    role: 'admin',
    is_active: true,
    last_login_at: null,
    created_at: '2026-10-01T00:00:00Z',
  } as AdminUser

  // 统一账号下管理员也可能拥有前台登录态（例如先切换过前台）。
  const userAuth = useUserAuthStore()
  userAuth.user = { id: 'a1', username: 'admin', role: 'admin' }
  userAuth.initialized = true

  const router = makeRouter()
  await router.push('/admin')
  await router.isReady()

  const wrapper = mount(Root, { global: { plugins: [pinia, router] } })
  await flushPromises()
  return { wrapper, router }
}

/** 展开顶栏用户菜单并返回菜单项文本列表。 */
async function openMenu(wrapper: Awaited<ReturnType<typeof mountLayout>>['wrapper']) {
  const trigger = wrapper.find('.admin-user-menu button[aria-haspopup="menu"]')
  await trigger.trigger('click')
  await flushPromises()
  return wrapper.findAll('[role="menuitem"]').map((i) => i.text())
}

describe('管理后台顶栏 · 用户菜单', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    authApi.logout.mockClear()
  })

  it('顶栏展示当前账号并提供用户菜单入口', async () => {
    const { wrapper } = await mountLayout()
    const trigger = wrapper.find('.admin-user-menu button[aria-haspopup="menu"]')
    expect(trigger.exists()).toBe(true)
    expect(trigger.text()).toContain('运营管理员')
    // 未展开时不应直接暴露菜单项
    expect(wrapper.findAll('[role="menuitem"]')).toHaveLength(0)
  })

  it('展开菜单后集中展示账号、角色与用户 ID', async () => {
    const { wrapper } = await mountLayout()
    await openMenu(wrapper)
    const menu = wrapper.find('.admin-user-menu')
    expect(menu.text()).toContain('运营管理员')
    expect(menu.text()).toContain('管理员')
    expect(menu.text()).toContain('UE389F2938F')
  })

  it('点击用户菜单展开后包含四个入口', async () => {
    const { wrapper } = await mountLayout()
    const items = await openMenu(wrapper)
    expect(items).toContain('返回前台')
    expect(items).toContain('用户信息')
    expect(items).toContain('修改密码')
    expect(items).toContain('退出登录')
  })

  it('点击「返回前台」跳转到前台首页', async () => {
    const { wrapper, router } = await mountLayout()
    await openMenu(wrapper)
    const btn = wrapper.findAll('[role="menuitem"]').find((b) => b.text() === '返回前台')!
    await btn.trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/')
    expect(authApi.logout).not.toHaveBeenCalled()
  })

  it('点击「用户信息」跳转到用户信息页（默认 Tab）', async () => {
    const { wrapper, router } = await mountLayout()
    await openMenu(wrapper)
    const link = wrapper.findAll('[role="menuitem"]').find((b) => b.text() === '用户信息')!
    await link.trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/admin/profile')
    expect(router.currentRoute.value.hash).toBe('')
  })

  it('点击「修改密码」跳转到用户信息页的修改密码 Tab', async () => {
    const { wrapper, router } = await mountLayout()
    await openMenu(wrapper)
    const link = wrapper.findAll('[role="menuitem"]').find((b) => b.text() === '修改密码')!
    await link.trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/admin/profile')
    expect(router.currentRoute.value.query.tab).toBe('password')
  })

  it('点击「退出登录」调用登出并回到统一登录页', async () => {
    const { wrapper, router } = await mountLayout()
    await openMenu(wrapper)
    const btn = wrapper.findAll('[role="menuitem"]').find((b) => b.text() === '退出登录')!
    await btn.trigger('click')
    await flushPromises()
    expect(authApi.logout).toHaveBeenCalledTimes(1)
    expect(router.currentRoute.value.path).toBe('/login')
    // 管理员从后台退出后，前台登录态也必须清空，
    // 否则再访问 /login 会被守卫判定为「已登录用户」而送回工作台。
    expect(useAuthStore().isLoggedIn).toBe(false)
    expect(useUserAuthStore().isLoggedIn).toBe(false)
  })
})
