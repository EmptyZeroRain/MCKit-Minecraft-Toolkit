// 页脚测试：常用工具只展示前 4 个内置工具，其余走「工具中心」入口。
import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import AppFooter from '@/components/layout/AppFooter.vue'
import { BUILTIN_TOOLS } from '@/utils/builtin-tools'

/** 页脚会链接到站点信息页与内置工具落地页，路由表需覆盖，否则 vue-router 会告警。 */
function makeRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      '/',
      '/about',
      '/feedback',
      '/privacy',
      '/copyright',
      '/tools',
      ...BUILTIN_TOOLS.map((tool) => tool.route),
    ].map((path) => ({ path, component: { template: '<div />' } })),
  })
}

async function mountFooter() {
  const router = makeRouter()
  await router.push('/')
  await router.isReady()
  return mount(AppFooter, { global: { plugins: [router] } })
}

describe('页脚', () => {
  it('常用工具只展示前 4 个内置工具', async () => {
    const wrapper = await mountFooter()
    const items = wrapper.findAll('[data-test="footer-tools"] li')

    expect(items).toHaveLength(4)
    expect(items.map((li) => li.text())).toEqual(BUILTIN_TOOLS.slice(0, 4).map((tool) => tool.name))
    // 第 5 个及之后的工具不出现在页脚。
    expect(wrapper.text()).not.toContain(BUILTIN_TOOLS[4].name)
  })

  it('提供「查看全部工具」入口指向工具中心', async () => {
    const wrapper = await mountFooter()
    const link = wrapper.findAll('a').find((a) => a.text().includes('查看全部工具'))

    expect(link).toBeTruthy()
    expect(link!.attributes('href')).toBe('/tools')
  })

  it('站点信息与非官方声明保持完整', async () => {
    const wrapper = await mountFooter()

    expect(wrapper.find('nav[aria-label="站点信息"]').findAll('li')).toHaveLength(4)
    expect(wrapper.text()).toContain('非官方社区项目')
    expect(wrapper.text()).toContain('MC 工具站')
  })
})
