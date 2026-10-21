// 安全审计视图测试：
//   1) 默认加载全部审计，表格带「来源 IP」列（此前搜索有 IP、列表却没有）；
//   2) 切到「安全日志」后按 security=true 请求，列改为「安全事件 + 设备」；
//   3) 两个视图的动作筛选取值域不同，切换时清空条件；
//   4) 对象类型 / 结果 / 来源 IP 筛选真实透传（此前后端忽略，属回归点）；
//   5) 「详情」展示原始日志（before/after 摘要 + 请求 ID）；
//   6) 导出使用当前筛选条件。
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

const MAC_UA =
  'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36'

function log(partial: Partial<AuditLog> = {}): AuditLog {
  return {
    id: 'l1',
    actor_admin_id: 'a1',
    actor_username: 'admin',
    action: 'admin.login',
    object_type: 'admin',
    object_id: 'a1',
    before_summary: null,
    after_summary: { user_agent: MAC_UA },
    request_id: 'req-1',
    source_ip: '203.0.113.9',
    result: 'failure',
    created_at: '2026-10-20T00:00:00Z',
    ...partial,
  }
}

function paged(items: AuditLog[]) {
  return { items, total: items.length, page: 1, pageSize: 10, totalPages: 1, suggestions: [] }
}

async function mountView() {
  setActivePinia(createPinia())
  const wrapper = mount(AdminAuditLogsView)
  await flushPromises()
  return wrapper
}

describe('审计日志与安全日志', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    document.body.innerHTML = ''
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

  it('默认加载全部审计日志，并展示来源 IP', async () => {
    const wrapper = await mountView()
    expect(api.listAuditLogs).toHaveBeenCalledWith(
      expect.objectContaining({ page: 1, page_size: 10 }),
    )
    expect(api.listAuditLogs.mock.calls[0][0].security).toBeUndefined()
    expect(wrapper.text()).toContain('admin.login')
    // 搜索框有「来源 IP」，表格必须有对应列，否则筛选条件无从核对。
    expect(wrapper.text()).toContain('来源 IP')
    expect(wrapper.text()).toContain('203.0.113.9')
  })

  it('切换到安全日志后按 security 过滤，并使用事件 / 设备列', async () => {
    const wrapper = await mountView()
    const btn = wrapper.findAll('button').find((b) => b.text() === '安全日志')
    await btn!.trigger('click')
    await flushPromises()

    expect(api.listAuditLogs).toHaveBeenLastCalledWith(
      expect.objectContaining({ security: true, page: 1 }),
    )
    const text = wrapper.text()
    expect(text).toContain('安全日志只包含账号安全动作')
    expect(text).toContain('安全事件')
    expect(text).toContain('设备')
    // 动作名翻译为中文事件名，而不是裸露的 admin.login。
    expect(text).toContain('Chrome · macOS')
    // 安全日志用固定枚举筛选动作，不再出现自由文本的「动作」输入框。
    expect(wrapper.find('input[placeholder="动作（如 resource.create）"]').exists()).toBe(false)
    expect(wrapper.find('select[aria-label="安全事件类型"]').exists()).toBe(true)
  })

  it('切换视图时清空只属于上一个视图的动作条件', async () => {
    const wrapper = await mountView()
    const actionInput = wrapper.find('input[placeholder="动作（如 resource.create）"]')
    await actionInput.setValue('resource.create')
    await actionInput.trigger('change')
    await flushPromises()

    const btn = wrapper.findAll('button').find((b) => b.text() === '安全日志')
    await btn!.trigger('click')
    await flushPromises()

    expect(api.listAuditLogs).toHaveBeenLastCalledWith(
      expect.objectContaining({ action: undefined, security: true }),
    )
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

    await wrapper.find('select[aria-label="结果"]').setValue('failure')
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

  it('「详情」展示原始日志（变更前后摘要与请求 ID）', async () => {
    ;(api.listAuditLogs as ReturnType<typeof vi.fn>).mockResolvedValue(
      paged([
        log({
          before_summary: { status: 'pending' },
          after_summary: { status: 'published', user_agent: MAC_UA },
        }),
      ]),
    )
    const wrapper = await mountView()

    const detail = wrapper.findAll('button').find((b) => b.text() === '查看')
    await detail!.trigger('click')
    await flushPromises()

    const raw = document.body.textContent ?? ''
    expect(raw).toContain('原始日志')
    expect(raw).toContain('req-1')
    expect(raw).toContain('"status": "published"')
    expect(raw).toContain('203.0.113.9')
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
