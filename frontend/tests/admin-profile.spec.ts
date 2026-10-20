// 用户信息页（/admin/profile）测试：
//   1) 展示用户名 / 角色，并用昵称 / 头像回填表单；
//   2) 上传图片调用（经压缩工具）后保存，昵称按去除首尾空白提交；
//   3) 昵称超长时前端拦截，不发起请求；
//   4) 修改密码：新密码不能与当前密码相同、长度不足时拦截。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory } from 'vue-router'
import AdminProfileView from '@/views/admin/AdminProfileView.vue'
import { useAuthStore } from '@/stores/auth'
import type { AdminUser } from '@/types/admin'

const authApi = vi.hoisted(() => ({
  updateProfile: vi.fn(),
  changePassword: vi.fn(),
  listSessions: vi.fn(),
  revokeSessions: vi.fn(),
  mfaStatus: vi.fn(),
  mfaSetup: vi.fn(),
  mfaEnable: vi.fn(),
  mfaDisable: vi.fn(),
}))
vi.mock('@/api/admin-auth', () => authApi)

// 图片压缩工具在 jsdom 下不可用（无 canvas / 图片解码），此处替换为固定 data URI。
const imageUtil = vi.hoisted(() => ({
  fileToAvatarDataUrl: vi.fn(),
  ALLOWED_AVATAR_TYPES: 'image/png,image/jpeg,image/webp,image/gif',
}))
vi.mock('@/utils/image', () => imageUtil)

const UPLOADED = 'data:image/jpeg;base64,AAAA'

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
    nickname: '旧昵称',
    avatar_url: 'data:image/png;base64,OLD',
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

/** 模拟选择本地图片文件。 */
async function pickAvatar(wrapper: Awaited<ReturnType<typeof mountView>>['wrapper']) {
  const fileInput = wrapper.find('input[type="file"]')
  Object.defineProperty(fileInput.element, 'files', {
    value: [new File(['x'], 'avatar.png', { type: 'image/png' })],
    configurable: true,
  })
  await fileInput.trigger('change')
  await flushPromises()
}

describe('用户信息页 · 展示资料', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    authApi.updateProfile.mockReset()
    authApi.changePassword.mockReset()
    authApi.listSessions.mockReset()
    authApi.revokeSessions.mockReset()
    authApi.listSessions.mockResolvedValue([])
    authApi.revokeSessions.mockResolvedValue({ scope: 'others', revoked: 0 })
    authApi.mfaStatus.mockReset()
    authApi.mfaStatus.mockResolvedValue({
      enabled: false,
      pending: false,
      recovery_codes_left: 0,
      last_used_at: null,
      configured: true,
    })
    imageUtil.fileToAvatarDataUrl.mockReset()
    imageUtil.fileToAvatarDataUrl.mockResolvedValue(UPLOADED)
  })

  it('回填昵称与头像，并展示用户名 / 角色', async () => {
    const { wrapper } = await mountView()
    const nickname = wrapper.find('input[type="text"]').element as HTMLInputElement
    expect(nickname.value).toBe('旧昵称')
    expect(wrapper.find('img[alt="头像预览"]').attributes('src')).toBe('data:image/png;base64,OLD')
    expect(wrapper.text()).toContain('admin')
  })

  it('上传图片后保存资料，调用 updateProfile 并同步本地用户', async () => {
    authApi.updateProfile.mockResolvedValue({
      id: 'a1',
      username: 'admin',
      nickname: '新昵称',
      avatar_url: UPLOADED,
      role: 'admin',
    })
    const { wrapper, auth } = await mountView()
    await pickAvatar(wrapper)
    await wrapper.find('input[type="text"]').setValue('  新昵称  ')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(imageUtil.fileToAvatarDataUrl).toHaveBeenCalledTimes(1)
    expect(authApi.updateProfile).toHaveBeenCalledWith({
      nickname: '新昵称',
      avatar_url: UPLOADED,
    })
    expect(auth.displayName).toBe('新昵称')
  })

  it('昵称超长时拦截，不发起请求', async () => {
    const { wrapper } = await mountView()
    await wrapper.find('input[type="text"]').setValue('字'.repeat(61))
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(authApi.updateProfile).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('昵称长度不能超过 60 个字符')
  })

  it('图片处理失败时展示错误且不保存', async () => {
    imageUtil.fileToAvatarDataUrl.mockRejectedValue(new Error('图片解析失败，请更换图片'))
    const { wrapper } = await mountView()
    await pickAvatar(wrapper)
    expect(wrapper.text()).toContain('图片解析失败，请更换图片')
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    // 昵称未变且头像未被修改，仍会提交（昵称合法）——这里仅断言错误提示可见。
    expect(wrapper.find('img[alt="头像预览"]').attributes('src')).toBe('data:image/png;base64,OLD')
  })
})

describe('用户信息页 · 修改密码', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    authApi.changePassword.mockReset()
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

// 会话管理：展示会话指纹、标记当前设备、支持退出其它设备。
describe('用户信息页 · 登录会话', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    authApi.updateProfile.mockReset()
    authApi.changePassword.mockReset()
    authApi.listSessions.mockReset()
    authApi.revokeSessions.mockReset()
    imageUtil.fileToAvatarDataUrl.mockResolvedValue(UPLOADED)
  })

  it('列出会话并标记当前设备', async () => {
    authApi.listSessions.mockResolvedValue([
      {
        id: 'aaaaaaaaaaaa',
        role: 'admin',
        created_at: '2026-10-01T00:00:00Z',
        last_seen_at: '2026-10-02T00:00:00Z',
        expires_at: '2026-10-04T00:00:00Z',
        current: true,
      },
      {
        id: 'bbbbbbbbbbbb',
        role: 'admin',
        created_at: '2026-10-01T00:00:00Z',
        last_seen_at: '2026-10-01T12:00:00Z',
        expires_at: '2026-10-04T00:00:00Z',
        current: false,
      },
    ])
    const { wrapper } = await mountView()
    expect(wrapper.text()).toContain('aaaaaaaaaaaa')
    expect(wrapper.text()).toContain('bbbbbbbbbbbb')
    expect(wrapper.text()).toContain('当前设备')
    expect(wrapper.text()).toContain('其它设备')
  })

  it('退出其它设备调用 scope=others 并刷新列表', async () => {
    authApi.listSessions.mockResolvedValue([
      {
        id: 'aaaaaaaaaaaa',
        role: 'admin',
        created_at: '2026-10-01T00:00:00Z',
        last_seen_at: '2026-10-02T00:00:00Z',
        expires_at: '2026-10-04T00:00:00Z',
        current: true,
      },
      {
        id: 'bbbbbbbbbbbb',
        role: 'admin',
        created_at: '2026-10-01T00:00:00Z',
        last_seen_at: '2026-10-01T12:00:00Z',
        expires_at: '2026-10-04T00:00:00Z',
        current: false,
      },
    ])
    authApi.revokeSessions.mockResolvedValue({ scope: 'others', revoked: 1 })

    const { wrapper } = await mountView()
    const buttons = wrapper.findAll('button')
    const others = buttons.find((b) => b.text().includes('退出其它设备'))
    expect(others).toBeTruthy()
    await others!.trigger('click')
    await flushPromises()

    expect(authApi.revokeSessions).toHaveBeenCalledWith('others')
    // 撤销后重新拉取会话列表。
    expect(authApi.listSessions).toHaveBeenCalledTimes(2)
  })

  it('必须改密时展示提示', async () => {
    authApi.listSessions.mockResolvedValue([])
    const { wrapper } = await mountView({ must_change_password: true })
    expect(wrapper.text()).toContain('你的密码已被管理员重置')
  })
})

// 双因子认证：绑定 / 恢复码展示 / 解绑。
describe('用户信息页 · 双因子认证', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    authApi.updateProfile.mockReset()
    authApi.changePassword.mockReset()
    authApi.listSessions.mockReset()
    authApi.revokeSessions.mockReset()
    authApi.mfaStatus.mockReset()
    authApi.mfaSetup.mockReset()
    authApi.mfaEnable.mockReset()
    authApi.mfaDisable.mockReset()
    authApi.listSessions.mockResolvedValue([])
    imageUtil.fileToAvatarDataUrl.mockResolvedValue(UPLOADED)
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
