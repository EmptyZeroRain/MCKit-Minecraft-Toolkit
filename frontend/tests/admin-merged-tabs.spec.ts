// 合并页 Tab 行为：按权限点过滤、默认首个可用 Tab、切换后同步 URL query。
// 子页统一 stub，避免真实 API 请求，只验证信息架构本身。
import { describe, expect, it } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import type { Component } from 'vue'
import { useAuthStore } from '@/stores/auth'
import type { AdminUser } from '@/types/admin'
import AdminAccountsView from '@/views/admin/AdminAccountsView.vue'
import AdminContentHealthView from '@/views/admin/AdminContentHealthView.vue'
import AdminEngagementView from '@/views/admin/AdminEngagementView.vue'
import AdminInsightsView from '@/views/admin/AdminInsightsView.vue'
import AdminOpsCenterView from '@/views/admin/AdminOpsCenterView.vue'

const stubs = {
  AdminAccountListView: { template: '<div data-test="accounts" />' },
  AdminRolesView: { template: '<div data-test="roles" />' },
  AdminAnalyticsView: { template: '<div data-test="analytics" />' },
  AdminSearchTermsView: { template: '<div data-test="search-terms" />' },
  AdminContentTasksView: { template: '<div data-test="content-tasks" />' },
  AdminFeedbackView: { template: '<div data-test="feedback" />' },
  AdminCommunityView: { template: '<div data-test="community" />' },
  AdminCommentsView: { template: '<div data-test="comments" />' },
  AdminDataQualityView: { template: '<div data-test="quality" />' },
  AdminKeywordBlocklistView: { template: '<div data-test="keywords" />' },
  AdminLinkHealthView: { template: '<div data-test="links" />' },
  AdminAlertsView: { template: '<div data-test="alerts" />' },
  AdminJobsView: { template: '<div data-test="jobs" />' },
  AdminMcsrcView: { template: '<div data-test="mcsrc" />' },
}

const paths = ['/admin/insights', '/admin/engagement', '/admin/content-health', '/admin/ops', '/admin/accounts']

async function mountPage(component: Component, path: string, permissions: string[]) {
  const pinia = createPinia()
  setActivePinia(pinia)
  const auth = useAuthStore()
  auth.user = {
    id: 'a1',
    username: 'admin',
    role: 'admin',
    permissions,
  } as AdminUser

  const router = createRouter({
    history: createMemoryHistory(),
    routes: paths.map((p) => ({ path: p, component: { template: '<div />' } })),
  })
  await router.push(path)
  await router.isReady()

  const wrapper = mount(component, {
    global: { plugins: [pinia, router], stubs },
  })
  await flushPromises()
  return { wrapper, router }
}

function tabLabels(wrapper: ReturnType<typeof mount>) {
  return wrapper.findAll('[role="tab"]').map((t) => t.text())
}

describe('后台合并页 Tab', () => {
  it('运营分析：三项权限全部可见，默认运营看板', async () => {
    const { wrapper } = await mountPage(AdminInsightsView, '/admin/insights', [
      'ops.analytics',
      'ops.searchstats',
      'ops.contenttasks',
    ])
    expect(tabLabels(wrapper)).toEqual(['运营看板', '搜索词统计', '内容建设任务'])
    expect(wrapper.find('[data-test="analytics"]').exists()).toBe(true)
  })

  it('运营分析：只有内容任务权限时不显示其它 Tab', async () => {
    const { wrapper } = await mountPage(AdminInsightsView, '/admin/insights', ['ops.contenttasks'])
    expect(tabLabels(wrapper)).toEqual(['内容建设任务'])
    expect(wrapper.find('[data-test="content-tasks"]').exists()).toBe(true)
  })

  it('用户互动：反馈与社区权限独立过滤', async () => {
    const feedback = await mountPage(AdminEngagementView, '/admin/engagement', ['content.feedback'])
    expect(tabLabels(feedback.wrapper)).toEqual(['反馈管理'])

    const community = await mountPage(AdminEngagementView, '/admin/engagement', ['content.community'])
    expect(tabLabels(community.wrapper)).toEqual(['社区管理', '用户评论'])
    expect(community.wrapper.find('[data-test="community"]').exists()).toBe(true)
  })

  it('用户互动：用户评论 Tab 独立打开', async () => {
    const { wrapper } = await mountPage(AdminEngagementView, '/admin/engagement?tab=comments', [
      'content.community',
    ])
    expect(tabLabels(wrapper)).toEqual(['社区管理', '用户评论'])
    expect(wrapper.find('[data-test="comments"]').exists()).toBe(true)
  })

  it('内容健康：数据质量权限同时显示关键字屏蔽', async () => {
    const { wrapper } = await mountPage(AdminContentHealthView, '/admin/content-health', [
      'ops.dataquality',
    ])
    expect(tabLabels(wrapper)).toEqual(['数据质量', '关键字屏蔽'])
    expect(wrapper.find('[data-test="quality"]').exists()).toBe(true)
  })

  it('内容健康：链接健康单独授权时默认链接 Tab', async () => {
    const { wrapper } = await mountPage(AdminContentHealthView, '/admin/content-health', ['ops.linkhealth'])
    expect(tabLabels(wrapper)).toEqual(['链接健康'])
    expect(wrapper.find('[data-test="links"]').exists()).toBe(true)
  })

  it('系统运维：告警 / 后台任务 / 缓存权限独立组合', async () => {
    const { wrapper } = await mountPage(AdminOpsCenterView, '/admin/ops', ['ops.jobs', 'ops.mcsrc'])
    expect(tabLabels(wrapper)).toEqual(['后台任务', 'MC 源码缓存'])
    expect(wrapper.find('[data-test="jobs"]').exists()).toBe(true)
  })

  it('账号与权限：普通用户权限只显示账号 Tab', async () => {
    const { wrapper } = await mountPage(AdminAccountsView, '/admin/accounts', ['accounts.users'])
    expect(tabLabels(wrapper)).toEqual(['账号管理'])
    expect(wrapper.find('[data-test="accounts"]').exists()).toBe(true)
  })

  it('账号与权限：角色权限只显示角色 Tab', async () => {
    const { wrapper } = await mountPage(AdminAccountsView, '/admin/accounts?tab=roles', ['roles.manage'])
    expect(tabLabels(wrapper)).toEqual(['角色与权限'])
    expect(wrapper.find('[data-test="roles"]').exists()).toBe(true)
  })

  it('账号与权限：切换角色 Tab 后同步 URL query', async () => {
    const { wrapper, router } = await mountPage(AdminAccountsView, '/admin/accounts', [
      'accounts.admins',
      'roles.manage',
    ])
    expect(tabLabels(wrapper)).toEqual(['账号管理', '角色与权限'])

    await wrapper.findAll('[role="tab"]')[1].trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-test="roles"]').exists()).toBe(true)
    expect(router.currentRoute.value.query.tab).toBe('roles')
  })
})
