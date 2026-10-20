// 管理侧边栏角色收敛测试（需求 6.2：editor 不显示 admin 专属入口）
// 说明：gj.md §2.2 与后续运营整合做了菜单合并（分类与标签 / 投稿审核 / 运营分析 /
// 用户互动 / 内容健康 / 账号与权限 / 系统运维），这里断言合并后的入口名，
// 并保留「权限点独立、editor 不可见」的收敛约束。
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
      { path: '/admin/content-health', component: { template: '<div />' } },
      { path: '/admin/insights', component: { template: '<div />' } },
      { path: '/admin/engagement', component: { template: '<div />' } },
      { path: '/admin/ops', component: { template: '<div />' } },
      { path: '/admin/review', component: { template: '<div />' } },
      { path: '/admin/resources', component: { template: '<div />' } },
      { path: '/admin/tools', component: { template: '<div />' } },
      { path: '/admin/wiki', component: { template: '<div />' } },
      { path: '/admin/taxonomy', component: { template: '<div />' } },
      { path: '/admin/profile', component: { template: '<div />' } },
      { path: '/admin/accounts', component: { template: '<div />' } },
      { path: '/admin/sessions', component: { template: '<div />' } },
      { path: '/admin/help', component: { template: '<div />' } },
      { path: '/admin/growth', component: { template: '<div />' } },
      { path: '/admin/gifts', component: { template: '<div />' } },
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

  it('admin 可见「安全审计」', async () => {
    const wrapper = await mountWithRole('admin')
    const text = wrapper.text()
    expect(text).toContain('安全审计')
    expect(text).toContain('用户互动')
  })

  it('admin 可见「账号与权限」与「用户信息」', async () => {
    const wrapper = await mountWithRole('admin')
    const text = wrapper.text()
    expect(text).toContain('账号与权限')
    expect(text).toContain('用户信息')
  })

  it('admin 可见「会话管理」，editor 不可见（ops.sessions 独立权限点）', async () => {
    const adminWrapper = await mountWithRole('admin')
    expect(adminWrapper.text()).toContain('会话管理')

    const editorWrapper = await mountWithRole('editor')
    expect(editorWrapper.text()).not.toContain('会话管理')
  })

  it('admin 可见「内容健康」', async () => {
    const wrapper = await mountWithRole('admin')
    expect(wrapper.text()).toContain('内容健康')
  })

  it('admin 可见合并后的「运营分析」（含搜索词与内容任务）', async () => {
    const wrapper = await mountWithRole('admin')
    const text = wrapper.text()
    expect(text).toContain('运营分析')
    // 内容建设任务已收敛到运营分析 Tab，不再单独占菜单
    expect(text).not.toContain('内容建设任务')
  })

  it('admin 可见「系统运维」，editor 不可见', async () => {
    const adminWrapper = await mountWithRole('admin')
    expect(adminWrapper.text()).toContain('系统运维')

    const editorWrapper = await mountWithRole('editor')
    expect(editorWrapper.text()).not.toContain('系统运维')
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
    expect(text).not.toContain('安全审计')
    expect(text).not.toContain('内容健康')
    expect(text).not.toContain('运营分析')
    expect(text).not.toContain('账号与权限')
    // 编辑具备 content.resources，可审核资源投稿，因此保留「投稿审核」入口
    expect(text).toContain('投稿审核')
    // 非 admin 专属入口仍应可见
    expect(text).toContain('用户互动')
    expect(text).toContain('资源管理')
  })

  it('editor 可见内容管理入口「分类与标签」', async () => {
    const wrapper = await mountWithRole('editor')
    expect(wrapper.text()).toContain('分类与标签')
  })

  it('editor 不显示「账号与权限」，但可见「用户信息」（可自助改密）', async () => {
    const wrapper = await mountWithRole('editor')
    const text = wrapper.text()
    expect(text).not.toContain('账号与权限')
    expect(text).toContain('用户信息')
  })

  it('侧边栏提供「返回前台」与「退出登录」', async () => {
    const wrapper = await mountWithRole('admin')
    const text = wrapper.text()
    expect(text).toContain('返回前台')
    expect(text).toContain('退出登录')
  })
})
