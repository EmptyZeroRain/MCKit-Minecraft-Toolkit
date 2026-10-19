// MC 源码浏览器 —— 「查看源码」（浏览器内 Vineflower 反编译）页面测试：
//   1) 从类列表点击「查看源码」-> 调用反编译并渲染源码；
//   2) 从类详情面板点击「查看源码」-> 源码面板替换详情面板；
//   3) 反编译失败 -> 展示错误并可返回列表；
//   4) 切换版本 -> 关闭源码面板。
//
// Monaco 查看器被 mock 成轻量占位组件：jsdom 无法运行 monaco，也不应在单测中加载它。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory } from 'vue-router'

vi.mock('@/components/mcsrc/SourceViewer.vue', () => ({
  // __esModule 让 Vue 的异步组件 interop 正确取到 default（否则会把 mock 命名空间当组件）。
  __esModule: true,
  default: {
    name: 'SourceViewer',
    props: ['source', 'language'],
    template: '<div data-test="source-stub">{{ source }}</div>',
  },
}))

const decompiler = vi.hoisted(() => ({ decompileClass: vi.fn() }))
vi.mock('@/mcsrc/decompiler', () => decompiler)

const mcsrcApi = vi.hoisted(() => ({
  getStatus: vi.fn(),
  listPrepared: vi.fn(),
  prepare: vi.fn(),
  listClasses: vi.fn(),
  getClassDetail: vi.fn(),
  getClassHierarchy: vi.fn(),
}))
vi.mock('@/api/mcsrc', () => mcsrcApi)

const versionsApi = vi.hoisted(() => ({ listVersions: vi.fn() }))
vi.mock('@/api/versions', () => versionsApi)

const authApi = vi.hoisted(() => ({ me: vi.fn() }))
vi.mock('@/api/admin-auth', () => authApi)

import McsrcView from '@/views/McsrcView.vue'
import { ApiError } from '@/types/api'
import type { McsrcClass, McsrcClassStructure, McsrcHierarchy, McsrcStatus } from '@/types/mcsrc'

const CLASS_NAME = 'net.minecraft.server.MinecraftServer'

function readyStatus(): McsrcStatus {
  return {
    version: '1.21.4',
    side: 'client',
    cached: true,
    size: 1_048_576,
    sha1: 'abc',
    fetched_at: '2026-10-01T00:00:00Z',
    indexed: true,
    class_count: 2,
    truncated: false,
    index_built_at: '2026-10-01T00:01:00Z',
  }
}

const CLASSES: McsrcClass[] = [
  { name: CLASS_NAME, simple: 'MinecraftServer', pkg: 'net.minecraft.server', inner: false, size: 4096 },
  {
    name: 'net.minecraft.server.MinecraftServer$1',
    simple: 'MinecraftServer$1',
    pkg: 'net.minecraft.server',
    inner: true,
    size: 512,
  },
]

function structureOf(name = CLASS_NAME): McsrcClassStructure {
  return {
    name,
    simple: name.split('.').pop() as string,
    pkg: 'net.minecraft.server',
    kind: 'class',
    modifiers: ['public', 'abstract'],
    major: 52,
    minor: 0,
    super: 'net.minecraft.world.level.Level',
    in_jar_super: true,
    interfaces: [],
    in_jar_interfaces: [],
    fields: [],
    methods: [{ name: 'tick', modifiers: ['public'], type: 'void', params: [], descriptor: '()V', synthetic: false }],
    inner: false,
  }
}

function hierarchyOf(name = CLASS_NAME): McsrcHierarchy {
  return {
    name,
    super: 'net.minecraft.world.level.Level',
    interfaces: [],
    ancestors: ['net.minecraft.world.level.Level', 'java.lang.Object'],
    ancestors_in_jar: false,
    descendants: [],
    descendant_count: 0,
    total_descendants: 0,
    truncated: false,
  }
}

async function mountView() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: { template: '<div />' } },
      { path: '/tools', component: { template: '<div />' } },
      { path: '/mcsrc', component: { template: '<div />' } },
    ],
  })
  await router.push('/mcsrc')
  await router.isReady()
  const wrapper = mount(McsrcView, { global: { plugins: [pinia, router] } })
  await flushPromises()
  return wrapper
}

/** 类列表中某一行内的按钮 */
function rowButton(wrapper: ReturnType<typeof mount>, rowIndex: number, text: string) {
  return wrapper.findAll('tbody tr')[rowIndex].findAll('button').find((b) => b.text() === text)
}

describe('MC 源码浏览器 - 查看源码', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    versionsApi.listVersions.mockResolvedValue({
      latest: { release: '1.21.4', snapshot: '25w01a' },
      versions: [
        { id: '1.21.4', type: 'release', url: '', time: '', releaseTime: '' },
        { id: '1.20.1', type: 'release', url: '', time: '', releaseTime: '' },
      ],
    })
    mcsrcApi.listPrepared.mockResolvedValue([])
    mcsrcApi.getStatus.mockResolvedValue(readyStatus())
    mcsrcApi.listClasses.mockResolvedValue({ items: CLASSES, total: 2, page: 1, page_size: 30 })
    mcsrcApi.getClassDetail.mockResolvedValue(structureOf())
    mcsrcApi.getClassHierarchy.mockResolvedValue(hierarchyOf())
    authApi.me.mockRejectedValue(new Error('未登录'))
    decompiler.decompileClass.mockResolvedValue({
      className: CLASS_NAME,
      source: 'public abstract class MinecraftServer {}',
      language: 'java',
    })
  })

  it('从类列表点击「查看源码」触发反编译并渲染源码', async () => {
    const wrapper = await mountView()
    await rowButton(wrapper, 0, '查看源码')!.trigger('click')
    await flushPromises()

    expect(decompiler.decompileClass).toHaveBeenCalledWith('1.21.4', 'client', CLASS_NAME)
    const panel = wrapper.find('[data-test="class-source"]')
    expect(panel.exists()).toBe(true)
    expect(panel.text()).toContain(CLASS_NAME)
    expect(wrapper.find('[data-test="source-stub"]').text()).toContain('public abstract class MinecraftServer {}')
    // 源码面板替换了类列表。
    expect(wrapper.findAll('tbody tr').length).toBe(0)
  })

  it('反编译进行中展示加载提示', async () => {
    let resolve!: (v: { className: string; source: string; language: 'java' }) => void
    decompiler.decompileClass.mockReturnValue(new Promise((r) => (resolve = r)))
    const wrapper = await mountView()
    await rowButton(wrapper, 0, '查看源码')!.trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-test="class-source"]').text()).toContain('反编译中')

    resolve({ className: CLASS_NAME, source: 'class X {}', language: 'java' })
    await flushPromises()
    expect(wrapper.find('[data-test="class-source"]').text()).not.toContain('反编译中')
  })

  it('反编译失败时展示错误，并可返回类列表', async () => {
    decompiler.decompileClass.mockRejectedValue(new ApiError('反编译引擎不可用', 'INTERNAL_ERROR'))
    const wrapper = await mountView()
    await rowButton(wrapper, 0, '查看源码')!.trigger('click')
    await flushPromises()

    const panel = wrapper.find('[data-test="class-source"]')
    expect(panel.text()).toContain('反编译引擎不可用')

    const back = panel.findAll('button').find((b) => b.text() === '返回列表')
    await back!.trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-test="class-source"]').exists()).toBe(false)
    expect(wrapper.findAll('tbody tr').length).toBe(2)
  })

  it('从类详情面板点击「查看源码」时，源码面板替换详情面板', async () => {
    const wrapper = await mountView()
    await wrapper.findAll('button').find((b) => b.text() === CLASS_NAME)!.trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-test="class-detail"]').exists()).toBe(true)

    const openSource = wrapper
      .find('[data-test="class-detail"]')
      .findAll('button')
      .find((b) => b.text() === '查看源码')
    await openSource!.trigger('click')
    await flushPromises()

    expect(decompiler.decompileClass).toHaveBeenCalledWith('1.21.4', 'client', CLASS_NAME)
    expect(wrapper.find('[data-test="class-detail"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="class-source"]').exists()).toBe(true)
  })

  it('切换版本会关闭源码面板', async () => {
    const wrapper = await mountView()
    await rowButton(wrapper, 0, '查看源码')!.trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-test="class-source"]').exists()).toBe(true)

    // 点击左侧版本列表中的另一个版本（按钮文本含类型徽标，故用包含匹配）。
    const otherVersion = wrapper.findAll('button').find((b) => b.text().includes('1.20.1'))
    await otherVersion!.trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-test="class-source"]').exists()).toBe(false)
  })
})
