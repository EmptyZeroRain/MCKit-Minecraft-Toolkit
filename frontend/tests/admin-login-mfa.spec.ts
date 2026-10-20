// 统一登录 · 双因子认证（MFA）交互测试：
//   1) 后台账号密码正确且启用 MFA 时，进入第二步验证码输入，且不视为已登录；
//   2) 提交验证码后调用 verifyMFA 并跳转后台；
//   3) 验证码错误时留在当前步骤并展示错误；
//   4) 「返回重新输入密码」会丢弃票据并清空密码；
//   5) 未启用 MFA 的后台账号直接登录；普通用户登录后进入工作台。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { ApiError } from '@/types/api'

const userApi = vi.hoisted(() => ({
  unifiedLogin: vi.fn(),
  fetchCaptcha: vi.fn(),
  register: vi.fn(),
  login: vi.fn(),
  logout: vi.fn(),
  me: vi.fn(),
  listFavorites: vi.fn(),
}))

const authApi = vi.hoisted(() => ({
  verifyMFA: vi.fn(),
  login: vi.fn(),
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

vi.mock('@/api/user', () => userApi)
vi.mock('@/api/admin-auth', () => authApi)

import UserLoginView from '@/views/UserLoginView.vue'
import { useAuthStore } from '@/stores/auth'
import { useUserAuthStore } from '@/stores/user-auth'
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
      { path: '/workspace', component: { template: '<div />' } },
      { path: '/privacy', component: { template: '<div />' } },
    ],
  })
  const wrapper = mount(UserLoginView, { global: { plugins: [router] } })
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

describe('统一登录 · 双因子认证', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    userApi.unifiedLogin.mockReset()
    authApi.verifyMFA.mockReset()
    userApi.listFavorites.mockResolvedValue([])
    userApi.me.mockResolvedValue({
      id: ADMIN.id,
      username: ADMIN.username,
      nickname: ADMIN.nickname,
      role: 'admin',
    })
  })

  it('后台账号启用 MFA 时进入验证码步骤且不写入登录态', async () => {
    userApi.unifiedLogin.mockResolvedValue({ mfa_required: true, mfa_ticket: 'ticket-abc' })
    const { wrapper } = await mountView()
    const auth = useAuthStore()

    await submitPassword(wrapper)

    expect(userApi.unifiedLogin).toHaveBeenCalledWith({
      identifier: 'admin',
      password: 'correct-password',
    })
    expect(wrapper.text()).toContain('已启用双因子认证')
    expect(wrapper.find('input[autocomplete="one-time-code"]').exists()).toBe(true)
    expect(auth.user).toBeNull()
    expect(useUserAuthStore().user).toBeNull()
  })

  it('提交验证码后完成登录并跳转后台', async () => {
    userApi.unifiedLogin.mockResolvedValue({ mfa_required: true, mfa_ticket: 'ticket-abc' })
    authApi.verifyMFA.mockResolvedValue(ADMIN)
    const { wrapper, router } = await mountView()

    await submitPassword(wrapper)
    await wrapper.find('input[autocomplete="one-time-code"]').setValue('123456')
    await buttonByText(wrapper, '验证并登录')!.trigger('click')
    await flushPromises()

    expect(authApi.verifyMFA).toHaveBeenCalledWith('ticket-abc', '123456')
    expect(useAuthStore().user?.username).toBe('admin')
    expect(useUserAuthStore().user?.username).toBe('admin')
    expect(router.currentRoute.value.path).toBe('/admin')
  })

  it('验证码错误时留在验证步骤并展示错误', async () => {
    userApi.unifiedLogin.mockResolvedValue({ mfa_required: true, mfa_ticket: 'ticket-abc' })
    authApi.verifyMFA.mockRejectedValue(
      new ApiError('验证码错误或已失效', 'UNAUTHORIZED', { status: 401 }),
    )
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
    userApi.unifiedLogin.mockResolvedValue({ mfa_required: true, mfa_ticket: 'ticket-abc' })
    const { wrapper } = await mountView()

    await submitPassword(wrapper)
    await buttonByText(wrapper, '返回重新输入密码')!.trigger('click')
    await flushPromises()

    expect(wrapper.find('input[autocomplete="username"]').exists()).toBe(true)
    expect(wrapper.find('input[autocomplete="one-time-code"]').exists()).toBe(false)
  })

  it('后台账号未启用 MFA 时直接登录并跳转后台', async () => {
    userApi.unifiedLogin.mockResolvedValue({ ...ADMIN })
    const { wrapper, router } = await mountView()

    await submitPassword(wrapper)

    expect(useAuthStore().user?.username).toBe('admin')
    expect(router.currentRoute.value.path).toBe('/admin')
    expect(authApi.verifyMFA).not.toHaveBeenCalled()
  })

  it('普通用户登录后进入个人工作台', async () => {
    userApi.unifiedLogin.mockResolvedValue({
      id: 'u1',
      username: 'alice',
      nickname: '爱丽丝',
      email: 'alice@example.com',
      role: 'user',
    })
    const { wrapper, router } = await mountView()

    await submitPassword(wrapper)

    expect(useAuthStore().user).toBeNull()
    expect(useUserAuthStore().user?.username).toBe('alice')
    expect(router.currentRoute.value.path).toBe('/workspace')
  })
})
