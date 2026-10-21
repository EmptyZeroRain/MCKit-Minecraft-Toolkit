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

/** 构造登录历史分页响应（默认每页 10 条）。 */
function historyPage(items: unknown[] = [], total = items.length, page = 1) {
  return { items, total, page, page_size: 10 }
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
  return { wrapper, auth, router }
}

const MAC_UA =
  'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36'
const WINDOWS_UA =
  'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0 Safari/537.36 Edg/125.0'

/** 切换到指定 Tab（按钮文案即标签）。 */
async function openTab(
  wrapper: Awaited<ReturnType<typeof mountView>>['wrapper'],
  label: string,
) {
  const tab = wrapper.findAll('[role="tab"]').find((t) => t.text() === label)
  expect(tab, `未找到 Tab：${label}`).toBeTruthy()
  await tab!.trigger('click')
  await flushPromises()
}

describe('用户信息页 · 页面结构', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    authApi.listLoginHistory.mockReset()
    authApi.listLoginHistory.mockResolvedValue(historyPage())
    authApi.mfaStatus.mockReset()
    authApi.mfaStatus.mockResolvedValue({
      enabled: false,
      pending: false,
      recovery_codes_left: 0,
      last_used_at: null,
      configured: true,
    })
  })

  it('以 Tab 组织：修改密码 / 双因子认证 / 登录历史', async () => {
    const { wrapper } = await mountView()
    const tabs = wrapper.findAll('[role="tab"]').map((t) => t.text())
    expect(tabs).toEqual(['修改密码', '双因子认证（MFA）', '登录历史'])

    // 默认停在修改密码，其它两块内容不渲染。
    expect(wrapper.find('#password').exists()).toBe(true)
    expect(wrapper.find('#mfa').exists()).toBe(false)
    expect(wrapper.find('#history').exists()).toBe(false)

    // 后台管理账号不再维护昵称 / 头像，也不展示会话指纹。
    expect(wrapper.text()).not.toContain('昵称')
    expect(wrapper.find('input[type="file"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('会话指纹')
  })

  it('切换 Tab 同步到 URL query，且只渲染当前块', async () => {
    const { wrapper, router } = await mountView()
    await openTab(wrapper, '登录历史')

    expect(wrapper.find('#history').exists()).toBe(true)
    expect(wrapper.find('#password').exists()).toBe(false)
    expect(router.currentRoute.value.query.tab).toBe('history')

    await openTab(wrapper, '双因子认证（MFA）')
    expect(wrapper.find('#mfa').exists()).toBe(true)
    expect(router.currentRoute.value.query.tab).toBe('mfa')
  })

  it('进入 MFA / 登录历史 Tab 才发起各自请求', async () => {
    const { wrapper } = await mountView()
    // 默认在修改密码：两个只读接口都不该被调用。
    expect(authApi.mfaStatus).not.toHaveBeenCalled()
    expect(authApi.listLoginHistory).not.toHaveBeenCalled()

    await openTab(wrapper, '双因子认证（MFA）')
    expect(authApi.mfaStatus).toHaveBeenCalledTimes(1)

    await openTab(wrapper, '登录历史')
    expect(authApi.listLoginHistory).toHaveBeenCalledTimes(1)
    // 来回切换不重复请求。
    await openTab(wrapper, '双因子认证（MFA）')
    expect(authApi.mfaStatus).toHaveBeenCalledTimes(1)
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
    authApi.listLoginHistory.mockResolvedValue(
      historyPage(
        [
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
        ],
        2,
      ),
    )

    const { wrapper } = await mountView()
    await openTab(wrapper, '登录历史')
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
    authApi.listLoginHistory.mockResolvedValue(
      historyPage(
        [
          {
            created_at: '2026-10-20T08:30:00Z',
            source_ip: '203.0.113.9',
            user_agent: WINDOWS_UA,
            result: 'success',
            action: 'admin.login',
          },
        ],
        1,
      ),
    )
    const { wrapper } = await mountView()
    await openTab(wrapper, '登录历史')
    expect(wrapper.text()).toContain('Edge · Windows')
  })

  it('刷新按钮重新拉取登录历史', async () => {
    authApi.listLoginHistory.mockResolvedValue(historyPage())
    const { wrapper } = await mountView()
    await openTab(wrapper, '登录历史')
    expect(authApi.listLoginHistory).toHaveBeenCalledTimes(1)
    expect(authApi.listLoginHistory).toHaveBeenCalledWith(1, 10)

    const refresh = wrapper.findAll('button').find((b) => b.text().includes('刷新'))
    await refresh!.trigger('click')
    await flushPromises()
    expect(authApi.listLoginHistory).toHaveBeenCalledTimes(2)
  })

  it('默认每页 10 条，可翻页加载更多', async () => {
    const makeItem = (ip: string, day: number) => ({
      created_at: `2026-10-${String(day).padStart(2, '0')}T00:00:00Z`,
      source_ip: ip,
      user_agent: MAC_UA,
      result: 'success',
      action: 'admin.login',
    })
    authApi.listLoginHistory.mockImplementation(async (page: number) =>
      page === 2
        ? historyPage([makeItem('198.51.100.11', 2), makeItem('198.51.100.12', 1)], 12, 2)
        : historyPage(
            Array.from({ length: 10 }, (_, i) => makeItem(`203.0.113.${i + 1}`, 20 - i)),
            12,
            1,
          ),
    )

    const { wrapper } = await mountView()
    await openTab(wrapper, '登录历史')

    // 服务端分页：首屏只渲染接口返回的 10 条，并给出总条数。
    expect(authApi.listLoginHistory).toHaveBeenCalledWith(1, 10)
    expect(wrapper.findAll('tbody tr')).toHaveLength(10)
    expect(wrapper.text()).toContain('共 12 条')

    await wrapper.find('nav[aria-label="分页"] button[aria-label="下一页"]').trigger('click')
    await flushPromises()

    expect(authApi.listLoginHistory).toHaveBeenLastCalledWith(2, 10)
    expect(wrapper.findAll('tbody tr')).toHaveLength(2)
    expect(wrapper.text()).toContain('198.51.100.11')
    expect(wrapper.text()).toContain('第 2/2 页')
  })
})

describe('用户信息页 · 修改密码', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    authApi.changePassword.mockReset()
    authApi.listLoginHistory.mockResolvedValue(historyPage())
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
    authApi.listLoginHistory.mockResolvedValue(historyPage())
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
    await openTab(wrapper, '双因子认证（MFA）')
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
    await openTab(wrapper, '双因子认证（MFA）')
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
    await openTab(wrapper, '双因子认证（MFA）')
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
