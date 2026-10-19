// MC 源码浏览器 —— 字节码 / 查找引用 / 映射重命名 / 备注 页面测试：
//   1) 类列表「查看字节码」-> 调用反汇编接口并在字节码标签内渲染（language=bytecode）；
//   2) 结构视图「查找引用」（类 / 成员）-> 渲染引用分组，点击引用点跳到调用方成员；
//   3) 结构视图展示类映射名与成员映射名；
//   4) 类备注与成员备注的编辑（localStorage）与源码视图内的只读展示。
//
// Monaco 查看器被 mock 成轻量占位组件：jsdom 无法运行 monaco，也不应在单测中加载它。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory } from 'vue-router'

vi.mock('@/components/mcsrc/SourceViewer.vue', () => ({
  __esModule: true,
  default: {
    name: 'SourceViewer',
    props: ['source', 'language', 'tokens', 'reveal'],
    emits: ['jump'],
    template: `<div data-test="source-stub">
      <span data-test="stub-source">{{ source }}</span>
      <span data-test="stub-language">{{ language }}</span>
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

import McsrcView from '@/views/McsrcView.vue'
import { ApiError } from '@/types/api'
import type {
  McsrcClass,
  McsrcClassBytecode,
  McsrcClassMapping,
  McsrcClassStructure,
  McsrcHierarchy,
  McsrcReferences,
  McsrcStatus,
} from '@/types/mcsrc'

const CLASS_NAME = 'net.minecraft.server.MinecraftServer'
const CALLER_NAME = 'net.minecraft.server.Main'
const NOTES_KEY = 'mckit-mcsrc-notes'

function readyStatus(): McsrcStatus {
  return {
    version: '1.21.4',
    side: 'client',
    cached: true,
    size: 1_048_576,
    sha1: 'abc',
    fetched_at: '2026-10-01T00:00:00Z',
    indexed: true,
    class_count: 1,
    truncated: false,
    index_built_at: '2026-10-01T00:01:00Z',
  }
}

const CLASSES: McsrcClass[] = [
  { name: CLASS_NAME, simple: 'MinecraftServer', pkg: 'net.minecraft.server', inner: false, size: 4096 },
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
    fields: [{ name: 'running', modifiers: ['private'], type: 'boolean', descriptor: 'Z', synthetic: false }],
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

function bytecodeOf(name = CLASS_NAME): McsrcClassBytecode {
  return {
    name,
    simple: name.split('.').pop() as string,
    pkg: 'net.minecraft.server',
    kind: 'class',
    major: 52,
    minor: 0,
    text: '// class version 52.0\npublic abstract class MinecraftServer {}',
    method_count: 1,
    instruction_count: 3,
    truncated: false,
  }
}

/** 类映射：类名与成员名都与混淆名不同，便于断言 */
function mappingOf(): McsrcClassMapping {
  return {
    available: true,
    set_key: 'mojang-1.21.4',
    mc_version: '1.21.4',
    mapping_type: 'mojang',
    matched_by: 'obf',
    class: {
      obf_name: 'net/minecraft/server/MinecraftServer',
      mid_name: '',
      named_name: 'net.minecraft.server.ServerMain',
      package: 'net.minecraft.server',
    },
    members: [
      { kind: 'method', obf_name: 'tick', mid_name: '', named_name: 'doTick', obf_desc: '()V', named_desc: '()V', side: 'client' },
      { kind: 'field', obf_name: 'running', mid_name: '', named_name: 'isRunning', obf_desc: 'Z', named_desc: 'Z', side: 'client' },
    ],
    truncated: false,
  }
}

function referencesOf(): McsrcReferences {
  return {
    key: `c:${CLASS_NAME}`,
    kind: 'class',
    owner: CLASS_NAME,
    sites: [
      {
        from_class: CALLER_NAME,
        from_kind: 'method',
        from_name: 'main',
        from_descriptor: '([Ljava/lang/String;)V',
      },
    ],
    total: 1,
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

/** 打开结构标签 */
async function openStructure(wrapper: ReturnType<typeof mount>) {
  await rowButton(wrapper, 0, '查看结构')!.trigger('click')
  await flushPromises()
}

describe('MC 源码浏览器 - 字节码 / 引用 / 映射 / 备注', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    localStorage.clear()
    versionsApi.listVersions.mockResolvedValue({
      latest: { release: '1.21.4', snapshot: '25w01a' },
      versions: [{ id: '1.21.4', type: 'release', url: '', time: '', releaseTime: '' }],
    })
    mcsrcApi.getStatus.mockResolvedValue(readyStatus())
    mcsrcApi.listClasses.mockResolvedValue({ items: CLASSES, total: 1, page: 1, page_size: 30 })
    mcsrcApi.getClassDetail.mockResolvedValue(structureOf())
    mcsrcApi.getClassHierarchy.mockResolvedValue(hierarchyOf())
    mcsrcApi.getClassMapping.mockResolvedValue(mappingOf())
    mcsrcApi.getClassBytecode.mockResolvedValue(bytecodeOf())
    mcsrcApi.getClassReferences.mockResolvedValue(referencesOf())
    decompiler.decompileClass.mockResolvedValue({
      className: CLASS_NAME,
      source: 'public abstract class MinecraftServer {}',
      language: 'java',
      tokens: [],
    })
  })

  it('类列表「查看字节码」渲染反汇编文本（bytecode 语言）', async () => {
    const wrapper = await mountView()
    await rowButton(wrapper, 0, '查看字节码')!.trigger('click')
    await flushPromises()

    expect(mcsrcApi.getClassBytecode).toHaveBeenCalledWith('1.21.4', CLASS_NAME, 'client')
    const panel = wrapper.find('[data-test="class-bytecode"]')
    expect(panel.exists()).toBe(true)
    expect(panel.text()).toContain('1 方法 · 3 指令')
    expect(wrapper.find('[data-test="stub-language"]').text()).toBe('bytecode')
    expect(wrapper.find('[data-test="stub-source"]').text()).toContain('class version 52')
  })

  it('字节码加载失败时展示错误并可重试', async () => {
    mcsrcApi.getClassBytecode
      .mockRejectedValueOnce(new ApiError('字节码不可用', 'CLASS_NOT_FOUND'))
      .mockResolvedValue(bytecodeOf())
    const wrapper = await mountView()
    await rowButton(wrapper, 0, '查看字节码')!.trigger('click')
    await flushPromises()

    const panel = wrapper.find('[data-test="class-bytecode"]')
    expect(panel.text()).toContain('字节码不可用')

    await panel.findAll('button').find((b) => b.text() === '重新加载')!.trigger('click')
    await flushPromises()
    expect(mcsrcApi.getClassBytecode).toHaveBeenCalledTimes(2)
    expect(wrapper.find('[data-test="stub-source"]').text()).toContain('class version 52')
  })

  it('结构视图「查找引用」渲染分组，点击引用点跳到调用方成员', async () => {
    const wrapper = await mountView()
    await openStructure(wrapper)

    await wrapper.find('[data-test="mcsrc-find-references"]').trigger('click')
    await flushPromises()

    expect(mcsrcApi.getClassReferences).toHaveBeenCalledWith(
      expect.objectContaining({ version: '1.21.4', side: 'client', kind: 'class', class: CLASS_NAME }),
    )
    const refs = wrapper.find('[data-test="mcsrc-references"]')
    expect(refs.text()).toContain('共 1 处引用')
    expect(refs.text()).toContain(CALLER_NAME)
    expect(refs.text()).toContain('main([Ljava/lang/String;)V')

    // 点击引用点：打开调用方源码并定位到该成员。
    await refs.find('[data-test="mcsrc-reference-site"]').trigger('click')
    await flushPromises()
    expect(decompiler.decompileClass).toHaveBeenLastCalledWith('1.21.4', 'client', CALLER_NAME, 'abc')
    expect(wrapper.find('[data-test="class-source"]').exists()).toBe(true)
  })

  it('成员「引用」按成员查询', async () => {
    const wrapper = await mountView()
    await openStructure(wrapper)

    const refButtons = wrapper.find('[data-test="class-detail"]').findAll('button').filter((b) => b.text() === '引用')
    expect(refButtons.length).toBeGreaterThan(0)
    await refButtons[0].trigger('click')
    await flushPromises()

    expect(mcsrcApi.getClassReferences).toHaveBeenCalledWith(
      expect.objectContaining({ kind: 'field', name: 'running', descriptor: 'Z' }),
    )
  })

  it('结构视图展示类映射名与成员映射名', async () => {
    const wrapper = await mountView()
    await openStructure(wrapper)

    const detail = wrapper.find('[data-test="class-detail"]')
    const classMapping = detail.find('[data-test="mcsrc-mapping-class"]')
    expect(classMapping.text()).toContain('ServerMain')
    expect(classMapping.text()).toContain('mojang · mojang-1.21.4')
    // 成员映射名（按混淆名匹配）。
    expect(detail.text()).toContain('isRunning')
    expect(detail.text()).toContain('doTick')
  })

  it('未导入映射时不展示映射列内容', async () => {
    mcsrcApi.getClassMapping.mockResolvedValue({ available: false, members: [], truncated: false })
    const wrapper = await mountView()
    await openStructure(wrapper)

    const detail = wrapper.find('[data-test="class-detail"]')
    expect(detail.find('[data-test="mcsrc-mapping-class"]').exists()).toBe(false)
    expect(detail.text()).toContain('running')
  })

  it('结构视图可编辑类备注，源码视图可查看类备注', async () => {
    const wrapper = await mountView()
    await openStructure(wrapper)

    await wrapper.find('[data-test="mcsrc-class-note-toggle"]').trigger('click')
    await flushPromises()
    const editor = wrapper.find('[data-test="mcsrc-notes-editor"]')
    expect(editor.exists()).toBe(true)
    await editor.find('textarea').setValue('这是类备注')
    await editor.find('[data-test="mcsrc-notes-save"]').trigger('click')
    await flushPromises()
    // 只写入浏览器本地存储。
    expect(localStorage.getItem(NOTES_KEY)).toContain('这是类备注')

    // 打开源码标签 -> 「备注」切换展示类备注。
    await wrapper
      .find('[data-test="class-detail"]')
      .findAll('button')
      .find((b) => b.text() === '查看源码')!
      .trigger('click')
    await flushPromises()
    await wrapper.find('[data-test="mcsrc-source-note-toggle"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-test="mcsrc-source-note"]').text()).toContain('这是类备注')
  })

  it('结构视图可编辑成员备注', async () => {
    const wrapper = await mountView()
    await openStructure(wrapper)

    const detail = wrapper.find('[data-test="class-detail"]')
    const noteButtons = detail.findAll('button').filter((b) => b.text() === '备注')
    expect(noteButtons.length).toBeGreaterThan(0)
    await noteButtons[0].trigger('click')
    await flushPromises()

    const editor = detail.find('[data-test="mcsrc-notes-editor"]')
    expect(editor.exists()).toBe(true)
    await editor.find('textarea').setValue('字段备注内容')
    await editor.find('[data-test="mcsrc-notes-save"]').trigger('click')
    await flushPromises()
    expect(localStorage.getItem(NOTES_KEY)).toContain('字段备注内容')
  })
})
