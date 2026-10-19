// 管理侧边栏角色收敛测试（需求 6.2：editor 不显示 admin 专属入口）
import { describe, expect, it, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory } from 'vue-router'
import AdminSidebar from '@/components/admin/AdminSidebar.vue'
import { useAuthStore } from '@/stores/auth'
import type { AdminUser } from '@/types/admin'

function makeRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: { template: '<div />' } },
      { path: '/admin', component: { template: '<div />' } },
      { path: '/admin/audit-logs', component: { template: '<div />' } },
      { path: '/admin/data-quality', component: { template: '<div />' } },
      { path: '/admin/search-terms', component: { template: '<div />' } },
      { path: '/admin/analytics', component: { template: '<div />' } },
      { path: '/admin/resources', component: { template: '<div />' } },
      { path: '/admin/tools', component: { template: '<div />' } },
      { path: '/admin/wiki', component: { template: '<div />' } },
      { path: '/admin/categories', component: { template: '<div />' } },
      { path: '/admin/tags', component: { template: '<div />' } },
      { path: '/admin/feedback', component: { template: '<div />' } },
      { path: '/admin/profile', component: { template: '<div />' } },
      { path: '/admin/admins', component: { template: '<div />' } },
    ],
  })
}

async function mountWithRole(role: 'admin' | 'editor') {
  const pinia = createPinia()
  setActivePinia(pinia)
  const auth = useAuthStore()
  auth.user = { username: role, role } as AdminUser

  const router = makeRouter()
  await router.push('/admin')
  await router.isReady()

  return mount(AdminSidebar, { global: { plugins: [pinia, router] } })
}

describe('管理侧边栏 · 角色入口', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('admin 可见「操作日志」', async () => {
    const wrapper = await mountWithRole('admin')
    const text = wrapper.text()
    expect(text).toContain('操作日志')
    expect(text).toContain('反馈管理')
  })

  it('admin 可见「管理员账号」与「账号设置」', async () => {
    const wrapper = await mountWithRole('admin')
    const text = wrapper.text()
    expect(text).toContain('管理员账号')
    expect(text).toContain('账号设置')
  })

  it('admin 可见「数据质量」', async () => {
    const wrapper = await mountWithRole('admin')
    expect(wrapper.text()).toContain('数据质量')
  })

  it('admin 可见「搜索词统计」', async () => {
    const wrapper = await mountWithRole('admin')
    expect(wrapper.text()).toContain('搜索词统计')
  })

  it('admin 可见「运营看板」（TODO §13）', async () => {
    const wrapper = await mountWithRole('admin')
    expect(wrapper.text()).toContain('运营看板')
  })

  it('editor 不显示 admin 专属的「操作日志」', async () => {
    const wrapper = await mountWithRole('editor')
    const text = wrapper.text()
    expect(text).not.toContain('操作日志')
    expect(text).not.toContain('数据质量')
    expect(text).not.toContain('搜索词统计')
    expect(text).not.toContain('运营看板')
    // 非 admin 专属入口仍应可见
    expect(text).toContain('反馈管理')
    expect(text).toContain('资源管理')
  })

  it('editor 不显示「管理员账号」，但可见「账号设置」（可自助改密）', async () => {
    const wrapper = await mountWithRole('editor')
    const text = wrapper.text()
    expect(text).not.toContain('管理员账号')
    expect(text).toContain('账号设置')
  })
})
