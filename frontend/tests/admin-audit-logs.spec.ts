// 审计与安全事件视图测试：
//   1) 默认加载全部审计；
//   2) 切到「安全事件」后按 security=true 请求；
//   3) 对象类型 / 结果 / 来源 IP 筛选会真实透传（此前后端忽略，属回归点）；
//   4) 导出使用当前筛选条件。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'

const api = vi.hoisted(() => ({
  listAuditLogs: vi.fn(),
  exportAuditLogs: vi.fn(),
}))
vi.mock('@/api/admin-system', () => api)

import AdminAuditLogsView from '@/views/admin/AdminAuditLogsView.vue'
import type { AuditLog } from '@/types/audit'

function log(partial: Partial<AuditLog> = {}): AuditLog {
  return {
    id: 'l1',
    actor_admin_id: 'a1',
    actor_username: 'admin',
    action: 'admin.login',
    object_type: 'admin',
    object_id: 'a1',
    before_summary: null,
    after_summary: null,
    request_id: 'req-1',
    source_ip: '203.0.113.9',
    result: 'failure',
    created_at: '2026-10-20T00:00:00Z',
    ...partial,
  }
}

function paged(items: AuditLog[]) {
  return { items, total: items.length, page: 1, pageSize: 20, totalPages: 1, suggestions: [] }
}

async function mountView() {
  setActivePinia(createPinia())
  const wrapper = mount(AdminAuditLogsView)
  await flushPromises()
  return wrapper
}

describe('审计日志与安全事件', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    ;(api.listAuditLogs as ReturnType<typeof vi.fn>).mockResolvedValue(paged([log()]))
    ;(api.exportAuditLogs as ReturnType<typeof vi.fn>).mockResolvedValue({
      blob: new Blob(['a,b']),
      truncated: false,
    })
    // jsdom 没有实现这些 API。
    URL.createObjectURL = vi.fn(() => 'blob:mock')
    URL.revokeObjectURL = vi.fn()
    // 避免 jsdom 对 <a download> 的点击尝试真实跳转（打印 navigation not implemented）。
    HTMLAnchorElement.prototype.click = vi.fn()
  })

  it('默认加载全部审计日志', async () => {
    const wrapper = await mountView()
    expect(api.listAuditLogs).toHaveBeenCalledWith(
      expect.objectContaining({ page: 1, page_size: 20 }),
    )
    expect(wrapper.text()).toContain('admin.login')
    expect(api.listAuditLogs.mock.calls[0][0].security).toBeUndefined()
  })

  it('切换到安全事件后按 security 过滤', async () => {
    const wrapper = await mountView()
    const btn = wrapper.findAll('button').find((b) => b.text() === '安全事件')
    await btn!.trigger('click')
    await flushPromises()
    expect(api.listAuditLogs).toHaveBeenLastCalledWith(
      expect.objectContaining({ security: true, page: 1 }),
    )
    expect(wrapper.text()).toContain('仅显示登录 / 改密 / MFA / 会话撤销')
  })

  it('对象类型 / 结果 / 来源 IP 筛选真实透传', async () => {
    const wrapper = await mountView()
    const inputs = wrapper.findAll('input')
    await inputs[1].setValue('admin')
    await inputs[1].trigger('change')
    await flushPromises()
    expect(api.listAuditLogs).toHaveBeenLastCalledWith(
      expect.objectContaining({ object_type: 'admin' }),
    )

    await wrapper.find('select').setValue('failure')
    await flushPromises()
    expect(api.listAuditLogs).toHaveBeenLastCalledWith(
      expect.objectContaining({ result: 'failure' }),
    )

    await inputs[2].setValue('203.0.113.9')
    await inputs[2].trigger('change')
    await flushPromises()
    expect(api.listAuditLogs).toHaveBeenLastCalledWith(
      expect.objectContaining({ source_ip: '203.0.113.9' }),
    )
  })

  it('导出携带当前筛选条件', async () => {
    const wrapper = await mountView()
    const btn = wrapper.findAll('button').find((b) => b.text() === '导出 CSV')
    await btn!.trigger('click')
    await flushPromises()
    const call = api.exportAuditLogs.mock.calls[0][0] as Record<string, unknown>
    expect(call.action).toBeUndefined()
    expect(call.page).toBeUndefined()
  })
})
