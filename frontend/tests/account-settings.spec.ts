// 账户设置页：工作台迁移出的资料 / 密码 / 删除账号表单必须可独立访问。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'

import AccountSettingsView from '@/views/AccountSettingsView.vue'
import { useUserAuthStore } from '@/stores/user-auth'

async function mountView() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const userAuth = useUserAuthStore()
  userAuth.user = {
    id: 'u1',
    user_no: 'UE389F2938F',
    username: 'alice',
    nickname: '爱丽丝',
    role: 'user',
  }
  userAuth.initialized = true

  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: { template: '<div />' } },
      { path: '/workspace', component: { template: '<div />' } },
      { path: '/login', component: { template: '<div />' } },
    ],
  })

  const wrapper = mount(AccountSettingsView, {
    global: {
      plugins: [pinia, router],
      stubs: { RouterLink: { template: '<a><slot /></a>' } },
    },
  })
  await flushPromises()
  return wrapper
}

describe('账户设置页', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('展示个人资料、修改密码与删除账号三个区域', async () => {
    const wrapper = await mountView()
    const text = wrapper.text()

    expect(text).toContain('账户设置')
    expect(text).toContain('个人资料')
    expect(text).toContain('修改密码')
    expect(text).toContain('删除账号')
  })

  it('资料表单使用当前昵称回填', async () => {
    const wrapper = await mountView()
    const nickname = wrapper.findAll('input').find((input) => input.attributes('maxlength'))
    expect((nickname!.element as HTMLInputElement).value).toBe('爱丽丝')
  })
})
