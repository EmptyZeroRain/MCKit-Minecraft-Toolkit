// MC 源码浏览器 —— 「查看源码」（浏览器内 Vineflower 反编译）页面测试：
//   1) 从类列表点击「查看源码」-> 调用反编译并在源码标签内渲染源码；
//   2) 从结构标签点击「查看源码」-> 新增并激活源码标签；
//   3) 反编译失败 -> 展示错误并可关闭标签回到列表；
//   4) 切换版本 -> 关闭全部标签。
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
    props: ['source', 'language', 'tokens', 'reveal'],
    emits: ['jump'],
    // 桩组件通过按钮把「源码内跳转」抛给页面，便于在 jsdom 中断言跳转链路。
    template: `<div data-test="source-stub">
      <span data-test="stub-source">{{ source }}</span>
      <span data-test="stub-token-count">{{ (tokens || []).length }}</span>
      <span data-test="stub-reveal">{{ reveal ? reveal.type + ':' + reveal.name : '' }}</span>
      <button data-test="stub-jump-class" @click="$emit('jump', { className: 'net.minecraft.world.level.Level', reveal: null })">jump-class</button>
      <button data-test="stub-jump-method" @click="$emit('jump', { className: 'net.minecraft.server.MinecraftServer', reveal: { type: 'method', name: 'tick', descriptor: '()V' } })">jump-method</button>
    </div>`,
  },
}))

const decompiler = vi.hoisted(() => ({ decompileClass: vi.fn() }))
vi.mock('@/mcsrc/decompiler', () => decompiler)

const mcsrcApi = vi.hoisted(() => ({
  getStatus: vi.fn(),
  prepare: vi.fn(),
  listClasses: vi.fn(),
  getClassDetail: vi.fn(),
  getClassHierarchy: vi.fn(),
  getClassBytecode: vi.fn(),
  getClassReferences: vi.fn(),
  getClassMapping: vi.fn(),
}))
vi.mock('@/api/mcsrc', () => mcsrcApi)

const versionsApi = vi.hoisted(() => ({ listVersions: vi.fn() }))
vi.mock('@/api/versions', () => versionsApi)

const authApi = vi.hoisted(() => ({ me: vi.fn() }))
vi.mock('@/api/admin-auth', () => authApi)

import McsrcView from '@/views/McsrcView.vue'
import { clearDeclarationCache } from '@/mcsrc/navigation'
import { ApiError } from '@/types/api'
import type { McsrcClass, McsrcClassStructure, McsrcHierarchy, McsrcStatus, McsrcToken } from '@/types/mcsrc'

const CLASS_NAME = 'net.minecraft.server.MinecraftServer'
const LEVEL_NAME = 'net.minecraft.world.level.Level'

/** 一段足以覆盖 token 计数与跳转的源码 token */
const TOKENS: McsrcToken[] = [
  { type: 'class', start: 0, length: 6, declaration: true, className: CLASS_NAME, inJar: true },
  { type: 'method', start: 30, length: 4, declaration: false, className: LEVEL_NAME, name: 'tick', descriptor: '()V', inJar: true },
]

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
  return wrapper
    .findAll('[data-test="mcsrc-class-row"]')
    [rowIndex].findAll('button')
    .find((b) => b.text() === text)
}

describe('MC 源码浏览器 - 查看源码', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    // 跳转解析的成员集合缓存是模块级的，用例间必须清理（各用例的 mock 成员集合不同）。
    clearDeclarationCache()
    versionsApi.listVersions.mockResolvedValue({
      latest: { release: '1.21.4', snapshot: '25w01a' },
      versions: [
        { id: '1.21.4', type: 'release', url: '', time: '', releaseTime: '' },
        { id: '1.20.1', type: 'release', url: '', time: '', releaseTime: '' },
      ],
    })
    mcsrcApi.getStatus.mockResolvedValue(readyStatus())
    mcsrcApi.listClasses.mockResolvedValue({ items: CLASSES, total: 2, page: 1, page_size: 30 })
    mcsrcApi.getClassDetail.mockResolvedValue(structureOf())
    mcsrcApi.getClassHierarchy.mockResolvedValue(hierarchyOf())
    mcsrcApi.getClassMapping.mockResolvedValue({ available: false, members: [], truncated: false })
    authApi.me.mockRejectedValue(new Error('未登录'))
    decompiler.decompileClass.mockResolvedValue({
      className: CLASS_NAME,
      source: 'public abstract class MinecraftServer {}',
      language: 'java',
      tokens: TOKENS,
    })
  })

  it('从类列表点击「查看源码」触发反编译并渲染源码', async () => {
    const wrapper = await mountView()
    await rowButton(wrapper, 0, '查看源码')!.trigger('click')
    await flushPromises()

    expect(decompiler.decompileClass).toHaveBeenCalledWith('1.21.4', 'client', CLASS_NAME, 'abc')
    const panel = wrapper.find('[data-test="class-source"]')
    expect(panel.exists()).toBe(true)
    expect(panel.text()).toContain(CLASS_NAME)
    expect(wrapper.find('[data-test="source-stub"]').text()).toContain('public abstract class MinecraftServer {}')
    // token 透传给查看器，且展示跳转提示。
    expect(wrapper.find('[data-test="stub-token-count"]').text()).toBe(String(TOKENS.length))
    expect(wrapper.find('[data-test="mcsrc-source-hint"]').text()).toContain('点击')
    // VS Code 式布局：源码标签在右侧，左侧类列表依然可见。
    expect(wrapper.findAll('[data-test="mcsrc-class-row"]').length).toBe(2)
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

  it('反编译失败时展示错误，并可关闭标签回到类列表', async () => {
    decompiler.decompileClass.mockRejectedValue(new ApiError('反编译引擎不可用', 'INTERNAL_ERROR'))
    const wrapper = await mountView()
    await rowButton(wrapper, 0, '查看源码')!.trigger('click')
    await flushPromises()

    const panel = wrapper.find('[data-test="class-source"]')
    expect(panel.text()).toContain('反编译引擎不可用')

    await wrapper.find('[data-test="mcsrc-tab-close"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-test="class-source"]').exists()).toBe(false)
    expect(wrapper.findAll('[data-test="mcsrc-class-row"]').length).toBe(2)
  })

  it('从结构标签点击「查看源码」时，新增并激活源码标签', async () => {
    const wrapper = await mountView()
    await rowButton(wrapper, 0, '查看结构')!.trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-test="class-detail"]').exists()).toBe(true)

    const openSource = wrapper
      .find('[data-test="class-detail"]')
      .findAll('button')
      .find((b) => b.text() === '查看源码')
    await openSource!.trigger('click')
    await flushPromises()

    expect(decompiler.decompileClass).toHaveBeenCalledWith('1.21.4', 'client', CLASS_NAME, 'abc')
    // 结构标签保留在标签栏，只是不再是激活标签。
    expect(wrapper.findAll('[data-test="mcsrc-tab-close"]').length).toBe(2)
    expect(wrapper.find('[data-test="class-source"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="class-detail"]').exists()).toBe(false)
  })

  it('切换版本会关闭全部标签', async () => {
    const wrapper = await mountView()
    await rowButton(wrapper, 0, '查看源码')!.trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-test="class-source"]').exists()).toBe(true)

    // 版本列表收在侧栏顶部的下拉浮层里：先展开再选另一个版本。
    await wrapper.find('[data-test="mcsrc-version-picker"]').trigger('click')
    const otherVersion = wrapper.findAll('button').find((b) => b.text().includes('1.20.1'))
    expect(otherVersion).toBeTruthy()
    await otherVersion!.trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-test="class-source"]').exists()).toBe(false)
    expect(wrapper.findAll('[data-test="mcsrc-tab-close"]').length).toBe(0)
  })

  it('源码内跳转：类 token 打开目标类源码', async () => {
    const wrapper = await mountView()
    await rowButton(wrapper, 0, '查看源码')!.trigger('click')
    await flushPromises()
    expect(decompiler.decompileClass).toHaveBeenCalledTimes(1)

    await wrapper.find('[data-test="stub-jump-class"]').trigger('click')
    await flushPromises()

    // 类 token 不需要查询成员声明，直接反编译目标类。
    expect(mcsrcApi.getClassDetail).not.toHaveBeenCalled()
    expect(decompiler.decompileClass).toHaveBeenLastCalledWith('1.21.4', 'client', LEVEL_NAME, 'abc')
    expect(wrapper.find('[data-test="class-source"]').text()).toContain(LEVEL_NAME)
    expect(wrapper.find('[data-test="stub-reveal"]').text()).toBe('')
  })

  it('源码内跳转：方法引用沿继承链定位到父类声明', async () => {
    // MinecraftServer 自身没有 tick()，真正声明在父类 Level。
    mcsrcApi.getClassDetail.mockImplementation(async (_version: string, name: string) => {
      const structure = structureOf(name)
      structure.methods = name === LEVEL_NAME ? structure.methods : []
      return structure
    })
    const wrapper = await mountView()
    await rowButton(wrapper, 0, '查看源码')!.trigger('click')
    await flushPromises()

    await wrapper.find('[data-test="stub-jump-method"]').trigger('click')
    await flushPromises()

    expect(mcsrcApi.getClassHierarchy).toHaveBeenCalledWith('1.21.4', CLASS_NAME, 'client')
    expect(decompiler.decompileClass).toHaveBeenLastCalledWith('1.21.4', 'client', LEVEL_NAME, 'abc')
    // 跳转落点被带给查看器（stub-reveal 渲染 type:name）。
    expect(wrapper.find('[data-test="stub-reveal"]').text()).toBe('method:tick')
  })

  it('同一类内跳转不重新反编译，仅更新定位目标', async () => {
    // 声明就在当前类里：解析结果与当前源码类一致。
    const wrapper = await mountView()
    await rowButton(wrapper, 0, '查看源码')!.trigger('click')
    await flushPromises()
    expect(decompiler.decompileClass).toHaveBeenCalledTimes(1)

    await wrapper.find('[data-test="stub-jump-method"]').trigger('click')
    await flushPromises()

    expect(decompiler.decompileClass).toHaveBeenCalledTimes(1)
    expect(wrapper.find('[data-test="stub-reveal"]').text()).toBe('method:tick')
  })
})
