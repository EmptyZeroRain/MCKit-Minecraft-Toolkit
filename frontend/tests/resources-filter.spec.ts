// 资源中心筛选测试（需求 5.7 / 13.3：版本、来源平台筛选与 URL 状态同步）
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { createRouter, createMemoryHistory } from 'vue-router'

vi.mock('@/api/public', () => ({ listResources: vi.fn() }))
// 版本筛选项动态加载（TODO §6-C）：测试中直接给定清单，避免依赖上游
vi.mock('@/api/versions', () => ({ listVersions: vi.fn() }))

import { listResources } from '@/api/public'
import { listVersions } from '@/api/versions'
import ResourcesView from '@/views/ResourcesView.vue'

const mockedList = vi.mocked(listResources)
const mockedVersions = vi.mocked(listVersions)

function makeRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: { template: '<div />' } },
      { path: '/resources', component: { template: '<div />' } },
      { path: '/resources/:slug', component: { template: '<div />' } },
    ],
  })
}

async function mountAt(query: Record<string, string> = {}) {
  const router = makeRouter()
  await router.push({ path: '/resources', query })
  await router.isReady()
  const wrapper = mount(ResourcesView, { global: { plugins: [router, createPinia()] } })
  await flushPromises()
  return { wrapper, router }
}

beforeEach(() => {
  vi.clearAllMocks()
  mockedList.mockResolvedValue({ items: [], total: 0, totalPages: 1 } as never)
  mockedVersions.mockResolvedValue({
    latest: { release: '1.21.4', snapshot: '25w03a' },
    versions: [
      { id: '1.21.4', type: 'release', url: '', time: '', releaseTime: '' },
      { id: '1.21', type: 'release', url: '', time: '', releaseTime: '' },
      { id: '1.20.1', type: 'release', url: '', time: '', releaseTime: '' },
    ],
  } as never)
})

describe('资源中心 · 筛选', () => {
  it('从 URL query 还原版本与来源平台筛选并请求', async () => {
    await mountAt({ game_version: '1.20', source_platform: 'CurseForge', loader: 'fabric' })
    expect(mockedList).toHaveBeenCalledTimes(1)
    expect(mockedList.mock.calls[0][0]).toMatchObject({
      game_version: '1.20',
      source_platform: 'CurseForge',
      loader: 'fabric',
      page: 1,
    })
  })

  it('修改来源平台会重置到第 1 页并同步到 URL', async () => {
    const { wrapper, router } = await mountAt({ page: '3' })
    mockedList.mockClear()

    await wrapper.find('input[aria-label="来源平台"]').setValue('Modrinth')
    await flushPromises()

    expect(mockedList).toHaveBeenCalledTimes(1)
    expect(mockedList.mock.calls[0][0]).toMatchObject({ source_platform: 'Modrinth', page: 1 })
    expect(router.currentRoute.value.query.source_platform).toBe('Modrinth')
    expect(router.currentRoute.value.query.page).toBeUndefined()
  })

  it('清除全部筛选会清空版本与来源平台', async () => {    const { wrapper } = await mountAt({ game_version: '1.20', source_platform: 'CurseForge' })
    mockedList.mockClear()

    const clearBtn = wrapper
      .findAll('button')
      .find((b) => b.text().includes('清除全部筛选'))
    expect(clearBtn).toBeTruthy()
    await clearBtn!.trigger('click')
    await flushPromises()

    expect(mockedList.mock.calls[0][0]).toMatchObject({
      game_version: undefined,
      source_platform: undefined,
      page: 1,
    })
  })

  it('版本筛选项覆盖官方清单主版本，并提供「全部 MC 版本」', async () => {
    const { wrapper } = await mountAt()
    const text = wrapper.text()
    expect(text).toContain('全部 MC 版本')
    // 1.21.4 与 1.21 归并为同一主版本 1.21
    expect(text).toContain('1.21')
    expect(text).toContain('1.20')
  })

  it('选择「全部 MC 版本」按通配声明筛选', async () => {
    const { wrapper } = await mountAt()
    mockedList.mockClear()

    const allBtn = wrapper.findAll('button').find((b) => b.text() === '全部 MC 版本')
    expect(allBtn).toBeTruthy()
    await allBtn!.trigger('click')
    await flushPromises()

    expect(mockedList.mock.calls[0][0]).toMatchObject({ game_version: 'all', page: 1 })
  })
})
