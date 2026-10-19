// MC 源码缓存运维页测试：
//   1) 展示缓存占用与「重建 / 清理」操作；
//   2) 重建只入队（调用 rebuild API），不在请求内等待；
//   3) 占用超过阈值时提示容量告警。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'

const api = vi.hoisted(() => ({
  listMcsrcCache: vi.fn(),
  previewMcsrcPurge: vi.fn(),
  purgeAllMcsrcCache: vi.fn(),
  purgeMcsrcCache: vi.fn(),
  rebuildMcsrcCache: vi.fn(),
}))
vi.mock('@/api/admin-mcsrc', () => api)

import AdminMcsrcView from '@/views/admin/AdminMcsrcView.vue'

const ENTRY = {
  version: '1.20.4',
  side: 'client' as const,
  cached: true,
  size: 25_000_000,
  sha1: 'abcdef1234567890',
  indexed: true,
  class_count: 12000,
  index_size: 3_000_000,
  meta_size: 512,
  total_size: 28_000_512,
  fetched_at: '2026-10-01T00:00:00Z',
  index_built_at: '2026-10-01T00:01:00Z',
}

async function mountView(overrides: Record<string, unknown> = {}) {
  setActivePinia(createPinia())
  ;(api.listMcsrcCache as ReturnType<typeof vi.fn>).mockResolvedValue({
    items: [ENTRY],
    usage: {
      entries: 1,
      jar_bytes: 25_000_000,
      index_bytes: 3_000_000,
      meta_bytes: 512,
      total_bytes: 28_000_512,
    },
    alert_threshold_bytes: 64 << 20,
    ...overrides,
  })
  ;(api.rebuildMcsrcCache as ReturnType<typeof vi.fn>).mockResolvedValue({
    job_id: '11111111-2222-3333-4444-555555555555',
    type: 'mcsrc.rebuild',
    version: '1.20.4',
    side: 'client',
  })
  const wrapper = mount(AdminMcsrcView, { global: { stubs: { RouterLink: true } } })
  await flushPromises()
  return wrapper
}

describe('MC 源码缓存运维', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('渲染缓存条目与操作按钮', async () => {
    const wrapper = await mountView()
    const text = wrapper.text()
    expect(text).toContain('1.20.4')
    expect(text).toContain('已建索引')
    expect(wrapper.findAll('button').some((b) => b.text() === '重建')).toBe(true)
    expect(wrapper.findAll('button').some((b) => b.text() === '清理')).toBe(true)
  })

  it('重建按版本 + 端入队后台任务', async () => {
    const wrapper = await mountView()
    const btn = wrapper.findAll('button').find((b) => b.text() === '重建')
    await btn!.trigger('click')
    await flushPromises()
    expect(api.rebuildMcsrcCache).toHaveBeenCalledWith('1.20.4', 'client')
  })

  it('占用超过阈值时提示容量告警', async () => {
    const wrapper = await mountView({ alert_threshold_bytes: 1 << 20 })
    expect(wrapper.text()).toContain('已达到告警阈值')
  })

  it('未超过阈值时不提示容量告警', async () => {
    const wrapper = await mountView({ alert_threshold_bytes: 1024 << 20 })
    expect(wrapper.text()).not.toContain('已达到告警阈值')
  })
})
