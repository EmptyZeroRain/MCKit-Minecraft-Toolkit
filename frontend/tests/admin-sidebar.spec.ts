// 管理侧边栏角色收敛测试（需求 6.2：editor 不显示 admin 专属入口）
// 说明：gj.md §2.2 做了菜单合并（分类与标签 / 投稿审核 / 运营分析 / 账号管理），
// 因此这里断言的是合并后的入口名，并保留「权限点独立、editor 不可见」的收敛约束。
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
      { path: '/admin/insights', component: { template: '<div />' } },
      { path: '/admin/content-tasks', component: { template: '<div />' } },
      { path: '/admin/review', component: { template: '<div />' } },
      { path: '/admin/resources', component: { template: '<div />' } },
      { path: '/admin/tools', component: { template: '<div />' } },
      { path: '/admin/wiki', component: { template: '<div />' } },
      { path: '/admin/taxonomy', component: { template: '<div />' } },
      { path: '/admin/feedback', component: { template: '<div />' } },
      { path: '/admin/profile', component: { template: '<div />' } },
      { path: '/admin/accounts', component: { template: '<div />' } },
      { path: '/admin/growth', component: { template: '<div />' } },
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

  it('admin 可见「账号管理」与「用户信息」', async () => {
    const wrapper = await mountWithRole('admin')
    const text = wrapper.text()
    expect(text).toContain('账号管理')
    expect(text).toContain('用户信息')
  })

  it('admin 可见「数据质量」', async () => {
    const wrapper = await mountWithRole('admin')
    expect(wrapper.text()).toContain('数据质量')
  })

  it('admin 可见合并后的「运营分析」（含搜索词统计）', async () => {
    const wrapper = await mountWithRole('admin')
    expect(wrapper.text()).toContain('运营分析')
  })

  it('admin 可见「内容建设任务」，editor 不可见（ops.contenttasks 独立权限点）', async () => {
    const adminWrapper = await mountWithRole('admin')
    expect(adminWrapper.text()).toContain('内容建设任务')

    const editorWrapper = await mountWithRole('editor')
    expect(editorWrapper.text()).not.toContain('内容建设任务')
  })

  it('admin 可见「用户成长」，editor 不可见（ops.growth 独立权限点）', async () => {
    const adminWrapper = await mountWithRole('admin')
    expect(adminWrapper.text()).toContain('用户成长')

    const editorWrapper = await mountWithRole('editor')
    expect(editorWrapper.text()).not.toContain('用户成长')
  })

  it('admin 可见合并后的「投稿审核」', async () => {
    const wrapper = await mountWithRole('admin')
    expect(wrapper.text()).toContain('投稿审核')
  })

  it('editor 不显示 admin 专属入口', async () => {
    const wrapper = await mountWithRole('editor')
    const text = wrapper.text()
    expect(text).not.toContain('操作日志')
    expect(text).not.toContain('数据质量')
    expect(text).not.toContain('运营分析')
    expect(text).not.toContain('投稿审核')
    expect(text).not.toContain('账号管理')
    // 非 admin 专属入口仍应可见
    expect(text).toContain('反馈管理')
    expect(text).toContain('资源管理')
  })

  it('editor 可见内容管理入口「分类与标签」', async () => {
    const wrapper = await mountWithRole('editor')
    expect(wrapper.text()).toContain('分类与标签')
  })

  it('editor 不显示「账号管理」，但可见「用户信息」（可自助改密）', async () => {
    const wrapper = await mountWithRole('editor')
    const text = wrapper.text()
    expect(text).not.toContain('账号管理')
    expect(text).toContain('用户信息')
  })

  it('侧边栏提供「返回前台」与「退出登录」', async () => {
    const wrapper = await mountWithRole('admin')
    const text = wrapper.text()
    expect(text).toContain('返回前台')
    expect(text).toContain('退出登录')
  })
})
