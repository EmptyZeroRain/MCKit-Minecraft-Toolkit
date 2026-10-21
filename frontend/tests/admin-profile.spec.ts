// 用户信息页（/admin/profile）测试：
//   1) 页面只保留账号安全三块：修改密码 / 双因子认证 / 登录历史（不再有昵称头像）；
//   2) 登录历史展示设备、IP、时间与结果，无法识别的 UA 显示「未知设备」；
//   3) 修改密码：新密码不能与当前密码相同、长度不足时拦截；
//   4) 双因子认证：绑定流程、恢复码、解绑。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory } from 'vue-router'
import AdminProfileView from '@/views/admin/AdminProfileView.vue'
import { useAuthStore } from '@/stores/auth'
import type { AdminUser } from '@/types/admin'

const authApi = vi.hoisted(() => ({
  changePassword: vi.fn(),
  listLoginHistory: vi.fn(),
  mfaStatus: vi.fn(),
  mfaSetup: vi.fn(),
  mfaEnable: vi.fn(),
  mfaDisable: vi.fn(),
}))
vi.mock('@/api/admin-auth', () => authApi)

function makeRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: { template: '<div />' } },
      { path: '/login', component: { template: '<div />' } },
      { path: '/admin/login', component: { template: '<div />' } },
      { path: '/admin/profile', component: AdminProfileView },
    ],
  })
}

async function mountView(user: Partial<AdminUser> = {}) {
  const pinia = createPinia()
  setActivePinia(pinia)
  const auth = useAuthStore()
  auth.user = {
    id: 'a1',
    username: 'admin',
    nickname: '管理员',
    role: 'admin',
    is_active: true,
    last_login_at: null,
    created_at: '2026-10-01T00:00:00Z',
    ...user,
  } as AdminUser

  const router = makeRouter()
  await router.push('/admin/profile')
  await router.isReady()

  const wrapper = mount(AdminProfileView, { global: { plugins: [pinia, router] } })
  await flushPromises()
  return { wrapper, auth }
}

const MAC_UA =
  'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36'
const WINDOWS_UA =
  'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0 Safari/537.36 Edg/125.0'

describe('用户信息页 · 页面结构', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    authApi.listLoginHistory.mockReset()
    authApi.listLoginHistory.mockResolvedValue([])
    authApi.mfaStatus.mockReset()
    authApi.mfaStatus.mockResolvedValue({
      enabled: false,
      pending: false,
      recovery_codes_left: 0,
      last_used_at: null,
      configured: true,
    })
  })

  it('只保留修改密码 / 双因子认证 / 登录历史三块', async () => {
    const { wrapper } = await mountView()
    const text = wrapper.text()
    expect(text).toContain('修改密码')
    expect(text).toContain('双因子认证（MFA）')
    expect(text).toContain('登录历史')
    // 后台管理账号不再维护昵称 / 头像，也不展示会话指纹。
    expect(text).not.toContain('用户信息')
    expect(text).not.toContain('昵称')
    expect(wrapper.find('input[type="file"]').exists()).toBe(false)
    expect(text).not.toContain('会话指纹')
  })

  it('必须改密时展示提示', async () => {
    const { wrapper } = await mountView({ must_change_password: true })
    expect(wrapper.text()).toContain('你的密码已被管理员重置')
  })
})

describe('用户信息页 · 登录历史', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    authApi.listLoginHistory.mockReset()
    authApi.mfaStatus.mockReset()
    authApi.mfaStatus.mockResolvedValue({
      enabled: false,
      pending: false,
      recovery_codes_left: 0,
      last_used_at: null,
      configured: true,
    })
  })

  it('展示设备 / IP / 时间 / 结果', async () => {
    authApi.listLoginHistory.mockResolvedValue([
      {
        created_at: '2026-10-20T08:30:00Z',
        source_ip: '203.0.113.9',
        user_agent: MAC_UA,
        result: 'success',
        action: 'admin.login',
      },
      {
        created_at: '2026-10-19T21:00:00Z',
        source_ip: '198.51.100.7',
        user_agent: '',
        result: 'failure',
        action: 'admin.login',
      },
    ])

    const { wrapper } = await mountView()
    const text = wrapper.text()
    expect(text).toContain('203.0.113.9')
    expect(text).toContain('198.51.100.7')
    expect(text).toContain('Chrome · macOS')
    // 没有 UA 的历史数据不能假装识别成功。
    expect(text).toContain('未知设备')
    expect(text).toContain('成功')
    expect(text).toContain('失败')
  })

  it('多设备按浏览器与系统区分', async () => {
    authApi.listLoginHistory.mockResolvedValue([
      {
        created_at: '2026-10-20T08:30:00Z',
        source_ip: '203.0.113.9',
        user_agent: WINDOWS_UA,
        result: 'success',
        action: 'admin.login',
      },
    ])
    const { wrapper } = await mountView()
    expect(wrapper.text()).toContain('Edge · Windows')
  })

  it('刷新按钮重新拉取登录历史', async () => {
    authApi.listLoginHistory.mockResolvedValue([])
    const { wrapper } = await mountView()
    expect(authApi.listLoginHistory).toHaveBeenCalledTimes(1)

    const refresh = wrapper.findAll('button').find((b) => b.text().includes('刷新'))
    await refresh!.trigger('click')
    await flushPromises()
    expect(authApi.listLoginHistory).toHaveBeenCalledTimes(2)
  })
})

describe('用户信息页 · 修改密码', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    authApi.changePassword.mockReset()
    authApi.listLoginHistory.mockResolvedValue([])
    authApi.mfaStatus.mockResolvedValue({
      enabled: false,
      pending: false,
      recovery_codes_left: 0,
      last_used_at: null,
      configured: true,
    })
  })

  async function fillPassword(
    wrapper: Awaited<ReturnType<typeof mountView>>['wrapper'],
    oldPwd: string,
    newPwd: string,
    confirm: string,
  ) {
    const pwdInputs = wrapper.findAll('input[type="password"]')
    await pwdInputs[0].setValue(oldPwd)
    await pwdInputs[1].setValue(newPwd)
    await pwdInputs[2].setValue(confirm)
  }

  it('新密码与当前密码相同时拦截', async () => {
    const { wrapper } = await mountView()
    await fillPassword(wrapper, 'samepassword12', 'samepassword12', 'samepassword12')
    const forms = wrapper.findAll('form')
    await forms[forms.length - 1].trigger('submit')
    await flushPromises()
    expect(authApi.changePassword).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('新密码不能与当前密码相同')
  })

  it('新密码长度不足时拦截', async () => {
    const { wrapper } = await mountView()
    await fillPassword(wrapper, 'oldpassword123', 'short', 'short')
    const forms = wrapper.findAll('form')
    await forms[forms.length - 1].trigger('submit')
    await flushPromises()
    expect(authApi.changePassword).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('新密码长度至少 12 位')
  })

  it('两次新密码不一致时拦截', async () => {
    const { wrapper } = await mountView()
    await fillPassword(wrapper, 'oldpassword123', 'newpassword123', 'newpassword999')
    const forms = wrapper.findAll('form')
    await forms[forms.length - 1].trigger('submit')
    await flushPromises()
    expect(authApi.changePassword).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('两次输入的新密码不一致')
  })
})

// 双因子认证：绑定 / 恢复码展示 / 解绑。
describe('用户信息页 · 双因子认证', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    authApi.changePassword.mockReset()
    authApi.listLoginHistory.mockReset()
    authApi.listLoginHistory.mockResolvedValue([])
    authApi.mfaStatus.mockReset()
    authApi.mfaSetup.mockReset()
    authApi.mfaEnable.mockReset()
    authApi.mfaDisable.mockReset()
  })

  it('服务端未配置密钥时禁用绑定并给出提示', async () => {
    authApi.mfaStatus.mockResolvedValue({
      enabled: false,
      pending: false,
      recovery_codes_left: 0,
      last_used_at: null,
      configured: false,
    })
    const { wrapper } = await mountView()
    expect(wrapper.text()).toContain('服务端未配置 MFA 加密密钥')
    expect(wrapper.findAll('button').some((b) => b.text().includes('开始绑定'))).toBe(false)
  })

  it('绑定流程：生成密钥 → 输入验证码 → 展示一次性恢复码', async () => {
    authApi.mfaStatus.mockResolvedValue({
      enabled: false,
      pending: false,
      recovery_codes_left: 0,
      last_used_at: null,
      configured: true,
    })
    authApi.mfaSetup.mockResolvedValue({
      secret: 'JBSWY3DPEHPK3PXP',
      uri: 'otpauth://totp/MC:admin?secret=JBSWY3DPEHPK3PXP',
    })
    authApi.mfaEnable.mockResolvedValue(['AAAAA-BBBBB', 'CCCCC-DDDDD'])

    const { wrapper } = await mountView()
    const start = wrapper.findAll('button').find((b) => b.text().includes('开始绑定'))
    await start!.trigger('click')
    await flushPromises()

    expect(authApi.mfaSetup).toHaveBeenCalled()
    expect(wrapper.text()).toContain('JBSWY3DPEHPK3PXP')

    await wrapper.find('input[autocomplete="one-time-code"]').setValue('123456')
    const confirm = wrapper.findAll('button').find((b) => b.text().includes('确认启用'))
    await confirm!.trigger('click')
    await flushPromises()

    expect(authApi.mfaEnable).toHaveBeenCalledWith('123456')
    expect(wrapper.text()).toContain('AAAAA-BBBBB')
    expect(wrapper.text()).toContain('仅显示这一次')
  })

  it('已启用时展示剩余恢复码，并可解绑', async () => {
    authApi.mfaStatus
      .mockResolvedValueOnce({
        enabled: true,
        pending: false,
        recovery_codes_left: 3,
        last_used_at: '2026-10-01T00:00:00Z',
        configured: true,
      })
      .mockResolvedValue({
        enabled: false,
        pending: false,
        recovery_codes_left: 0,
        last_used_at: null,
        configured: true,
      })
    authApi.mfaDisable.mockResolvedValue(undefined)

    const { wrapper } = await mountView()
    expect(wrapper.text()).toContain('已启用')
    expect(wrapper.text()).toContain('剩余恢复码')

    const toggle = wrapper.findAll('button').find((b) => b.text().includes('解绑双因子认证'))
    await toggle!.trigger('click')
    await flushPromises()

    const passwordInputs = wrapper.findAll('input[type="password"]')
    await passwordInputs[passwordInputs.length - 1].setValue('current-password')
    const codeInputs = wrapper.findAll('input[autocomplete="one-time-code"]')
    await codeInputs[codeInputs.length - 1].setValue('654321')

    const confirm = wrapper.findAll('button').find((b) => b.text().includes('确认解绑'))
    await confirm!.trigger('click')
    await flushPromises()

    expect(authApi.mfaDisable).toHaveBeenCalledWith('current-password', '654321')
  })
})
