// MC 源码浏览器（mcsrc）页面测试：
//   1) 选中版本（含首次进入的默认版本）即**自动准备**，无需手动点击；
//      未就绪时展示准备进度条（下载阶段给百分比、建索引阶段为不确定进度）；
//   2) 自动准备被拒（额度用尽 / 关闭）-> 展示提示与「重试准备」；
//   3) 已就绪 -> 渲染类列表（含内部类标记）；
//   4) 后端返回 NEED_PREPARE -> 展示「尚未建立类索引」空态而非报错。
// 布局为 VS Code 式：左侧栏类列表 + 右侧标签页（源码 / 结构），两者并存。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory } from 'vue-router'

const mcsrcApi = vi.hoisted(() => ({
  getStatus: vi.fn(),
  prepare: vi.fn(),
  listClasses: vi.fn(),
  getClassDetail: vi.fn(),
  getClassHierarchy: vi.fn(),
}))
vi.mock('@/api/mcsrc', () => mcsrcApi)

// 打开源码标签会走浏览器内反编译（Worker + WASM）：本文件只覆盖状态流转与结构视图，
// 用桩函数避免在 jsdom 里创建 Worker。
const decompiler = vi.hoisted(() => ({ decompileClass: vi.fn() }))
vi.mock('@/mcsrc/decompiler', () => decompiler)

const versionsApi = vi.hoisted(() => ({ listVersions: vi.fn() }))
vi.mock('@/api/versions', () => versionsApi)

import McsrcView from '@/views/McsrcView.vue'
import { ApiError } from '@/types/api'
import type {
  McsrcClass,
  McsrcClassStructure,
  McsrcHierarchy,
  McsrcStatus,
} from '@/types/mcsrc'

/** 未准备的版本状态 */
function pendingStatus(): McsrcStatus {
  return {
    version: '1.21.4',
    side: 'client',
    cached: false,
    size: 0,
    sha1: '',
    indexed: false,
    class_count: 0,
    truncated: false,
  }
}

/** 已建索引的版本状态 */
function readyStatus(classCount = 2): McsrcStatus {
  return {
    version: '1.21.4',
    side: 'client',
    cached: true,
    size: 1_048_576,
    sha1: 'abc',
    fetched_at: '2026-10-01T00:00:00Z',
    indexed: true,
    class_count: classCount,
    truncated: false,
    index_built_at: '2026-10-01T00:01:00Z',
  }
}

const CLASSES: McsrcClass[] = [
  {
    name: 'net.minecraft.server.MinecraftServer',
    simple: 'MinecraftServer',
    pkg: 'net.minecraft.server',
    inner: false,
    size: 4096,
  },
  {
    name: 'net.minecraft.server.MinecraftServer$1',
    simple: 'MinecraftServer$1',
    pkg: 'net.minecraft.server',
    inner: true,
    size: 512,
  },
]

/** 类结构视图（含一个编译器生成的合成方法） */
function structureOf(name = 'net.minecraft.server.MinecraftServer'): McsrcClassStructure {
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
    interfaces: ['java.lang.Runnable'],
    in_jar_interfaces: [],
    fields: [
      { name: 'running', modifiers: ['private', 'static'], type: 'boolean', descriptor: 'Z', synthetic: false },
    ],
    methods: [
      { name: 'tick', modifiers: ['public'], type: 'void', params: [], descriptor: '()V', synthetic: false },
      { name: 'access$000', modifiers: ['static', 'synthetic'], type: 'int', params: [], descriptor: '()I', synthetic: true },
    ],
    inner: false,
  }
}

/** 继承关系（祖先链延伸至 jar 外） */
function hierarchyOf(name = 'net.minecraft.server.MinecraftServer'): McsrcHierarchy {
  return {
    name,
    super: 'net.minecraft.world.level.Level',
    interfaces: ['java.lang.Runnable'],
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

describe('MC 源码浏览器', () => {
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
    mcsrcApi.getStatus.mockResolvedValue(pendingStatus())
    mcsrcApi.listClasses.mockResolvedValue({ items: CLASSES, total: 2, page: 1, page_size: 30 })
    mcsrcApi.getClassDetail.mockResolvedValue(structureOf())
    mcsrcApi.getClassHierarchy.mockResolvedValue(hierarchyOf())
    decompiler.decompileClass.mockResolvedValue({
      className: '',
      source: '',
      language: 'java',
      tokens: [],
    })
  })

  it('选中版本即自动准备（无需点击），完成后渲染类列表', async () => {
    mcsrcApi.prepare.mockResolvedValue(readyStatus())
    const wrapper = await mountView()

    // 默认选中最新正式版，并已查询状态。
    expect(mcsrcApi.getStatus).toHaveBeenCalledWith('1.21.4', 'client')
    // 未就绪 → 自动触发 prepare，全程无需点击任何按钮。
    expect(mcsrcApi.prepare).toHaveBeenCalledWith('1.21.4', 'client')
    // 准备完成后拉取类索引。
    expect(mcsrcApi.listClasses).toHaveBeenCalledWith(
      expect.objectContaining({ version: '1.21.4', side: 'client', page: 1, page_size: 30 }),
    )
    expect(wrapper.text()).toContain('索引就绪')
    expect(wrapper.text()).toContain('MinecraftServer')
  })

  it('下载阶段展示带百分比的进度条', async () => {
    let finishPrepare: (st: McsrcStatus) => void = () => {}
    mcsrcApi.prepare.mockImplementation(
      () =>
        new Promise<McsrcStatus>((resolve) => {
          finishPrepare = resolve
        }),
    )
    // 状态接口已带上进行中的下载进度（服务端边下边报）。
    mcsrcApi.getStatus.mockResolvedValue({
      ...pendingStatus(),
      preparing: true,
      phase: 'downloading',
      downloaded: 5_242_880,
      total: 10_485_760,
    })
    const wrapper = await mountView()

    const bar = wrapper.find('[data-test="mcsrc-prepare-progress"]')
    expect(bar.exists()).toBe(true)
    expect(bar.text()).toContain('正在下载官方 jar')
    expect(bar.text()).toContain('50%')
    expect(bar.find('[role="progressbar"]').attributes('aria-valuenow')).toBe('50')

    // 收尾：让 prepare 结束，避免残留轮询定时器。
    finishPrepare(readyStatus())
    await flushPromises()
  })

  it('建索引阶段展示不确定进度条', async () => {
    mcsrcApi.prepare.mockImplementation(() => new Promise<McsrcStatus>(() => {}))
    mcsrcApi.getStatus.mockResolvedValue({
      ...pendingStatus(),
      cached: true,
      preparing: true,
      phase: 'indexing',
    })
    const wrapper = await mountView()

    const bar = wrapper.find('[data-test="mcsrc-prepare-progress"]')
    expect(bar.exists()).toBe(true)
    expect(bar.text()).toContain('正在建立类索引')
    // 不确定进度：不给出百分比。
    expect(bar.find('[role="progressbar"]').attributes('aria-valuenow')).toBeUndefined()
  })

  it('自动准备被拒（额度用尽）时展示提示与「重试准备」', async () => {
    mcsrcApi.prepare.mockRejectedValue(
      new ApiError('自动准备额度已用尽（最多自动缓存 12 个版本），请联系管理员准备该版本', 'AUTO_PREPARE_LIMIT'),
    )
    const wrapper = await mountView()

    expect(wrapper.find('[data-test="mcsrc-prepare-error"]').text()).toContain('自动准备额度已用尽')
    expect(wrapper.findAll('button').some((b) => b.text().includes('重试准备'))).toBe(true)
    // 未就绪时不请求类索引，避免无意义的后端 409。
    expect(mcsrcApi.listClasses).not.toHaveBeenCalled()
  })

  it('点击「重试准备」可重新触发自动准备', async () => {
    mcsrcApi.prepare
      .mockRejectedValueOnce(new ApiError('自动准备额度已用尽', 'AUTO_PREPARE_LIMIT'))
      .mockResolvedValue(readyStatus())
    const wrapper = await mountView()
    expect(wrapper.text()).toContain('自动准备额度已用尽')

    const retry = wrapper.findAll('button').find((b) => b.text().includes('重试准备'))
    expect(retry).toBeTruthy()
    await retry!.trigger('click')
    await flushPromises()

    expect(mcsrcApi.prepare).toHaveBeenCalledTimes(2)
    expect(mcsrcApi.listClasses).toHaveBeenCalled()
    expect(wrapper.text()).toContain('索引就绪')
  })

  it('已就绪版本渲染类列表，并标记内部类', async () => {
    mcsrcApi.getStatus.mockResolvedValue(readyStatus())
    mcsrcApi.listClasses.mockResolvedValue({ items: CLASSES, total: 2, page: 1, page_size: 30 })
    const wrapper = await mountView()

    const rows = wrapper.findAll('[data-test="mcsrc-class-row"]')
    expect(rows.length).toBe(2)
    expect(rows[0].text()).toContain('MinecraftServer')
    expect(rows[0].text()).toContain('net.minecraft.server')
    expect(rows[0].text()).not.toContain('内部类')
    expect(rows[1].text()).toContain('MinecraftServer$1')
    expect(rows[1].text()).toContain('内部类')
  })

  it('后端返回 NEED_PREPARE 时展示空态而非错误', async () => {
    mcsrcApi.getStatus.mockResolvedValue(readyStatus())
    mcsrcApi.listClasses.mockRejectedValue(new ApiError('需先准备', 'NEED_PREPARE'))
    const wrapper = await mountView()

    expect(wrapper.text()).toContain('尚未建立类索引')
    expect(wrapper.text()).not.toContain('检索失败')
  })

  it('版本状态查询失败时展示错误文案', async () => {
    mcsrcApi.getStatus.mockRejectedValue(new ApiError('服务不可用', 'MCSRC_DISABLED'))
    const wrapper = await mountView()

    expect(wrapper.text()).toContain('服务不可用')
  })

  it('点击「查看结构」打开结构标签：渲染结构视图、继承关系，并默认折叠合成方法', async () => {
    mcsrcApi.getStatus.mockResolvedValue(readyStatus())
    const wrapper = await mountView()

    const link = rowButton(wrapper, 0, '查看结构')
    expect(link).toBeTruthy()
    await link!.trigger('click')
    await flushPromises()

    expect(mcsrcApi.getClassDetail).toHaveBeenCalledWith('1.21.4', 'net.minecraft.server.MinecraftServer', 'client')
    expect(mcsrcApi.getClassHierarchy).toHaveBeenCalledWith('1.21.4', 'net.minecraft.server.MinecraftServer', 'client')

    const detail = wrapper.find('[data-test="class-detail"]')
    expect(detail.exists()).toBe(true)
    const text = detail.text()
    // 种类 / class 版本 / 修饰符 / 父类 / 接口。
    expect(text).toContain('类')
    expect(text).toContain('Java 8')
    expect(text).toContain('public abstract class MinecraftServer')
    expect(text).toContain('net.minecraft.world.level.Level')
    expect(text).toContain('java.lang.Runnable')
    // 字段与普通方法。
    expect(text).toContain('running')
    expect(text).toContain('void tick()')
    // 合成方法默认折叠。
    expect(text).not.toContain('access$000')
    expect(text).toContain('显示编译器生成成员')
    // 继承关系：祖先链含 jar 外终点。
    const hier = wrapper.find('[data-test="class-hierarchy"]')
    expect(hier.text()).toContain('java.lang.Object')
    expect(hier.text()).toContain('祖先链延伸至 jar 外')
    expect(hier.text()).toContain('无直接子类')
    // VS Code 式布局：打开结构标签后，左侧类列表仍在（标签栏在右侧主区）。
    expect(wrapper.findAll('[data-test="mcsrc-class-row"]').length).toBe(2)
    expect(wrapper.find('[data-test="mcsrc-tab-close"]').exists()).toBe(true)
  })

  it('勾选后可显示合成方法，点击祖先类可继续下钻', async () => {
    mcsrcApi.getStatus.mockResolvedValue(readyStatus())
    const wrapper = await mountView()
    await rowButton(wrapper, 0, '查看结构')!.trigger('click')
    await flushPromises()

    // 勾选「显示编译器生成成员」。
    const checkbox = wrapper.find('[data-test="class-detail"] input[type="checkbox"]')
    await checkbox.setValue(true)
    expect(wrapper.find('[data-test="class-detail"]').text()).toContain('access$000')

    // 点击可下钻的祖先（jar 内的 Level）。
    mcsrcApi.getClassDetail.mockClear()
    const ancestor = wrapper.findAll('button').find((b) => b.text() === 'net.minecraft.world.level.Level')
    expect(ancestor).toBeTruthy()
    await ancestor!.trigger('click')
    await flushPromises()
    expect(mcsrcApi.getClassDetail).toHaveBeenCalledWith('1.21.4', 'net.minecraft.world.level.Level', 'client')
  })

  it('继承关系接口失败不影响结构视图展示', async () => {
    mcsrcApi.getStatus.mockResolvedValue(readyStatus())
    mcsrcApi.getClassHierarchy.mockRejectedValue(new ApiError('继承关系不可用', 'CLASS_NOT_FOUND'))
    const wrapper = await mountView()
    await rowButton(wrapper, 0, '查看结构')!.trigger('click')
    await flushPromises()

    const detail = wrapper.find('[data-test="class-detail"]')
    expect(detail.text()).toContain('void tick()')
    expect(detail.text()).not.toContain('继承关系不可用')
    expect(wrapper.find('[data-test="class-hierarchy"]').exists()).toBe(false)
  })

  it('结构视图加载失败时展示错误，并可关闭标签回到列表', async () => {
    mcsrcApi.getStatus.mockResolvedValue(readyStatus())
    mcsrcApi.getClassDetail.mockRejectedValue(new ApiError('类不存在', 'CLASS_NOT_FOUND'))
    const wrapper = await mountView()
    await rowButton(wrapper, 0, '查看结构')!.trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-test="class-detail"]').text()).toContain('类不存在')

    const close = wrapper.find('[data-test="mcsrc-tab-close"]')
    expect(close.exists()).toBe(true)
    await close.trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-test="class-detail"]').exists()).toBe(false)
    expect(wrapper.findAll('[data-test="mcsrc-class-row"]').length).toBe(2)
  })
})
