// 后台操作手册页：确认关键分组与站内跳转存在（操作帮助不能被删成空白页）。
import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'

import AdminHelpView from '@/views/admin/AdminHelpView.vue'

function mountView() {
  return mount(AdminHelpView, {
    global: {
      stubs: {
        RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' },
      },
    },
  })
}

describe('后台操作手册', () => {
  it('渲染全部分组与常见问题', () => {
    const text = mountView().text()
    for (const title of ['权限与账号', '内容审核', '运营与运维', '导出与数据', '无障碍与键盘操作']) {
      expect(text).toContain(title)
    }
    expect(text).toContain('如何一次处理多条投稿？')
    expect(text).toContain('任务失败了怎么重试？')
  })

  it('每条指引都指向真实后台路径', () => {
    const links = mountView().findAll('a[href^="/admin/"]')
    expect(links.length).toBeGreaterThan(0)
    const hrefs = links.map((a) => a.attributes('href'))
    expect(hrefs).toContain('/admin/ops?tab=alerts')
    expect(hrefs).toContain('/admin/ops?tab=jobs')
    expect(hrefs).toContain('/admin/accounts?tab=roles')
    expect(hrefs).toContain('/admin/review?tab=submissions')
    expect(hrefs).toContain('/admin/sessions')
  })
})
