// 注册页交互测试（需求 §12：昵称 / 邮箱 / 图形验证码）
// 说明：mock '@/api/user' 隔离网络，只验证表单校验、验证码加载与刷新、提交契约。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { ApiError } from '@/types/api'

vi.mock('@/api/user', () => ({
  fetchCaptcha: vi.fn(),
  register: vi.fn(),
  login: vi.fn(),
  logout: vi.fn(),
  me: vi.fn(),
  changePassword: vi.fn(),
  deleteAccount: vi.fn(),
  listFavorites: vi.fn(),
  toggleFavorite: vi.fn(),
  removeFavorite: vi.fn(),
  listRecents: vi.fn(),
  recordRecent: vi.fn(),
  clearRecents: vi.fn(),
  listSeeds: vi.fn(),
  saveSeed: vi.fn(),
  deleteSeed: vi.fn(),
}))

import { fetchCaptcha, register } from '@/api/user'
import UserLoginView from '@/views/UserLoginView.vue'

const captcha = {
  captcha_id: 'c1',
  image: 'data:image/png;base64,AAAA',
  expires_in: 300,
}

async function mountView() {
  setActivePinia(createPinia())
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: { template: '<div />' } },
      { path: '/workspace', component: { template: '<div />' } },
      { path: '/privacy', component: { template: '<div />' } },
    ],
  })
  const wrapper = mount(UserLoginView, { global: { plugins: [router] } })
  await flushPromises()
  return wrapper
}

/** 切换到注册模式并等待验证码加载 */
async function toRegister(wrapper: Awaited<ReturnType<typeof mountView>>) {
  const tabs = wrapper.findAll('button[role="tab"]')
  await tabs[1].trigger('click')
  await flushPromises()
}

describe('注册页', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    ;(fetchCaptcha as ReturnType<typeof vi.fn>).mockResolvedValue(captcha)
    ;(register as ReturnType<typeof vi.fn>).mockResolvedValue({
      id: 'u1',
      username: 'alice',
      nickname: '爱丽丝',
      email: 'alice@example.com',
    })
  })

  it('切到注册即获取验证码并渲染图片', async () => {
    const wrapper = await mountView()
    await toRegister(wrapper)

    expect(fetchCaptcha).toHaveBeenCalledTimes(1)
    const img = wrapper.find('img[alt="图形验证码"]')
    expect(img.exists()).toBe(true)
    expect(img.attributes('src')).toBe(captcha.image)
    expect(wrapper.find('#user-nickname').exists()).toBe(true)
    expect(wrapper.find('#user-email').exists()).toBe(true)
  })

  it('昵称为空时给出字段错误且不调用接口', async () => {
    const wrapper = await mountView()
    await toRegister(wrapper)

    await wrapper.find('#user-username').setValue('alice')
    await wrapper.find('#user-email').setValue('alice@example.com')
    await wrapper.find('#user-password').setValue('password123')
    await wrapper.find('#user-password2').setValue('password123')
    await wrapper.find('#user-captcha').setValue('AB12')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('昵称需为 1-60 个字符')
    expect(register).not.toHaveBeenCalled()
  })

  it('邮箱非法时给出字段错误', async () => {
    const wrapper = await mountView()
    await toRegister(wrapper)

    await wrapper.find('#user-username').setValue('alice')
    await wrapper.find('#user-nickname').setValue('爱丽丝')
    await wrapper.find('#user-email').setValue('not-an-email')
    await wrapper.find('#user-password').setValue('password123')
    await wrapper.find('#user-password2').setValue('password123')
    await wrapper.find('#user-captcha').setValue('AB12')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('请输入有效的邮箱地址')
    expect(register).not.toHaveBeenCalled()
  })

  it('验证码为空时给出字段错误', async () => {
    const wrapper = await mountView()
    await toRegister(wrapper)

    await wrapper.find('#user-username').setValue('alice')
    await wrapper.find('#user-nickname').setValue('爱丽丝')
    await wrapper.find('#user-email').setValue('alice@example.com')
    await wrapper.find('#user-password').setValue('password123')
    await wrapper.find('#user-password2').setValue('password123')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('请输入图形验证码')
    expect(register).not.toHaveBeenCalled()
  })

  it('校验通过后提交完整 payload 并切回登录', async () => {
    const wrapper = await mountView()
    await toRegister(wrapper)

    await wrapper.find('#user-username').setValue('alice')
    await wrapper.find('#user-nickname').setValue('爱丽丝')
    await wrapper.find('#user-email').setValue('alice@example.com')
    await wrapper.find('#user-password').setValue('password123')
    await wrapper.find('#user-password2').setValue('password123')
    await wrapper.find('#user-captcha').setValue('AB12')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(register).toHaveBeenCalledWith({
      username: 'alice',
      nickname: '爱丽丝',
      email: 'alice@example.com',
      password: 'password123',
      captcha_id: 'c1',
      captcha_answer: 'AB12',
    })
    expect(wrapper.text()).toContain('注册成功')
  })

  it('注册失败后重新获取验证码（一次性）', async () => {
    ;(register as ReturnType<typeof vi.fn>).mockRejectedValue(
      new ApiError('验证码错误或已过期，请重新获取', 'CAPTCHA_INVALID', { status: 422 }),
    )
    const wrapper = await mountView()
    await toRegister(wrapper)
    expect(fetchCaptcha).toHaveBeenCalledTimes(1)

    await wrapper.find('#user-username').setValue('alice')
    await wrapper.find('#user-nickname').setValue('爱丽丝')
    await wrapper.find('#user-email').setValue('alice@example.com')
    await wrapper.find('#user-password').setValue('password123')
    await wrapper.find('#user-password2').setValue('password123')
    await wrapper.find('#user-captcha').setValue('WRONG')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('验证码错误或已过期')
    expect(fetchCaptcha).toHaveBeenCalledTimes(2)
  })
})
