// 后台登录 · 双因子认证（MFA）交互测试：
//   1) 密码正确且账号启用 MFA 时，进入第二步验证码输入，且不视为已登录；
//   2) 提交验证码后调用 verifyMFA 并跳转；
//   3) 验证码错误时留在当前步骤并展示错误；
//   4) 「返回重新输入密码」会丢弃票据并清空密码。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'

const authApi = vi.hoisted(() => ({
  login: vi.fn(),
  verifyMFA: vi.fn(),
  logout: vi.fn(),
  me: vi.fn(),
  changePassword: vi.fn(),
  updateProfile: vi.fn(),
  listSessions: vi.fn(),
  revokeSessions: vi.fn(),
  mfaStatus: vi.fn(),
  mfaSetup: vi.fn(),
  mfaEnable: vi.fn(),
  mfaDisable: vi.fn(),
}))
vi.mock('@/api/admin-auth', () => authApi)

import AdminLoginView from '@/views/admin/AdminLoginView.vue'
import { useAuthStore } from '@/stores/auth'
import type { AdminUser } from '@/types/admin'

const ADMIN: AdminUser = {
  id: 'acc-1',
  username: 'admin',
  nickname: '管理员',
  avatar_url: '',
  role: 'admin',
  is_active: true,
  must_change_password: false,
  last_login_at: null,
  created_at: '2026-10-01T00:00:00Z',
  permissions: ['content.resources'],
}

async function mountView() {
  setActivePinia(createPinia())
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: { template: '<div />' } },
      { path: '/admin', component: { template: '<div />' } },
      { path: '/admin/login', component: { template: '<div />' } },
    ],
  })
  const wrapper = mount(AdminLoginView, { global: { plugins: [router] } })
  await flushPromises()
  return { wrapper, router }
}

/** 填写用户名 / 密码并提交。 */
async function submitPassword(wrapper: Awaited<ReturnType<typeof mountView>>['wrapper']) {
  const inputs = wrapper.findAll('input')
  await inputs[0].setValue('admin')
  await inputs[1].setValue('correct-password')
  await wrapper.find('form').trigger('submit')
  await flushPromises()
}

function buttonByText(wrapper: Awaited<ReturnType<typeof mountView>>['wrapper'], text: string) {
  return wrapper.findAll('button').find((b) => b.text().includes(text))
}

describe('后台登录 · 双因子认证', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('账号启用 MFA 时进入验证码步骤且不写入登录态', async () => {
    authApi.login.mockResolvedValue({ mfa_required: true, mfa_ticket: 'ticket-abc' })
    const { wrapper } = await mountView()
    const auth = useAuthStore()

    await submitPassword(wrapper)

    expect(authApi.login).toHaveBeenCalledWith({ username: 'admin', password: 'correct-password' })
    expect(wrapper.text()).toContain('已启用双因子认证')
    expect(wrapper.find('input[autocomplete="one-time-code"]').exists()).toBe(true)
    expect(auth.user).toBeNull()
  })

  it('提交验证码后完成登录并跳转', async () => {
    authApi.login.mockResolvedValue({ mfa_required: true, mfa_ticket: 'ticket-abc' })
    authApi.verifyMFA.mockResolvedValue(ADMIN)
    const { wrapper, router } = await mountView()

    await submitPassword(wrapper)
    await wrapper.find('input[autocomplete="one-time-code"]').setValue('123456')
    await buttonByText(wrapper, '验证并登录')!.trigger('click')
    await flushPromises()

    expect(authApi.verifyMFA).toHaveBeenCalledWith('ticket-abc', '123456')
    expect(useAuthStore().user?.username).toBe('admin')
    expect(router.currentRoute.value.path).toBe('/admin')
  })

  it('验证码错误时留在验证步骤并展示错误', async () => {
    authApi.login.mockResolvedValue({ mfa_required: true, mfa_ticket: 'ticket-abc' })
    authApi.verifyMFA.mockRejectedValue(new Error('验证码错误或已失效'))
    const { wrapper, router } = await mountView()

    await submitPassword(wrapper)
    await wrapper.find('input[autocomplete="one-time-code"]').setValue('000000')
    await buttonByText(wrapper, '验证并登录')!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('验证码错误或已失效')
    expect(useAuthStore().user).toBeNull()
    expect(router.currentRoute.value.path).not.toBe('/admin')
  })

  it('返回重新输入密码会丢弃票据', async () => {
    authApi.login.mockResolvedValue({ mfa_required: true, mfa_ticket: 'ticket-abc' })
    const { wrapper } = await mountView()

    await submitPassword(wrapper)
    await buttonByText(wrapper, '返回重新输入密码')!.trigger('click')
    await flushPromises()

    expect(wrapper.find('input[autocomplete="username"]').exists()).toBe(true)
    expect(wrapper.find('input[autocomplete="one-time-code"]').exists()).toBe(false)
  })

  it('未启用 MFA 时直接登录', async () => {
    authApi.login.mockResolvedValue({ user: ADMIN })
    const { wrapper, router } = await mountView()

    await submitPassword(wrapper)

    expect(useAuthStore().user?.username).toBe('admin')
    expect(router.currentRoute.value.path).toBe('/admin')
    expect(authApi.verifyMFA).not.toHaveBeenCalled()
  })
})
