// MC 源码浏览器（mcsrc）页面测试：
//   1) 未就绪 + 管理员 -> 显示「准备该版本」，点击后触发 prepare 并刷新为类列表；
//   2) 未就绪 + 未登录 -> 不显示准备按钮，改为提示联系管理员；
//   3) 已就绪 -> 渲染类索引表格（含内部类标记）；
//   4) 后端返回 NEED_PREPARE -> 展示「尚未建立类索引」空态而非报错。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory } from 'vue-router'

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
import type {
  McsrcClass,
  McsrcClassStructure,
  McsrcHierarchy,
  McsrcStatus,
} from '@/types/mcsrc'

const ADMIN = {
  id: 'a1',
  username: 'admin',
  role: 'admin',
  permissions: ['ops.mcsrc'],
  is_active: true,
  last_login_at: null,
  created_at: '2026-10-01T00:00:00Z',
}

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
    mcsrcApi.listPrepared.mockResolvedValue([])
    mcsrcApi.getStatus.mockResolvedValue(pendingStatus())
    mcsrcApi.listClasses.mockResolvedValue({ items: CLASSES, total: 2, page: 1, page_size: 30 })
    mcsrcApi.getClassDetail.mockResolvedValue(structureOf())
    mcsrcApi.getClassHierarchy.mockResolvedValue(hierarchyOf())
    authApi.me.mockRejectedValue(new Error('未登录'))
  })

  it('管理员在未就绪版本上可触发「准备该版本」，并刷新出类列表', async () => {
    authApi.me.mockResolvedValue(ADMIN)
    mcsrcApi.prepare.mockResolvedValue(readyStatus())
    const wrapper = await mountView()

    // 默认选中最新正式版，并已查询状态。
    expect(mcsrcApi.getStatus).toHaveBeenCalledWith('1.21.4', 'client')
    expect(wrapper.text()).toContain('尚未准备')

    const btn = wrapper.findAll('button').find((b) => b.text().includes('准备该版本'))
    expect(btn).toBeTruthy()
    await btn!.trigger('click')
    await flushPromises()

    expect(mcsrcApi.prepare).toHaveBeenCalledWith('1.21.4', 'client')
    // 准备完成后重新拉取缓存列表与类索引。
    expect(mcsrcApi.listPrepared).toHaveBeenCalled()
    expect(mcsrcApi.listClasses).toHaveBeenCalledWith(
      expect.objectContaining({ version: '1.21.4', side: 'client', page: 1, page_size: 30 }),
    )
    expect(wrapper.text()).toContain('索引就绪')
    expect(wrapper.text()).toContain('net.minecraft.server.MinecraftServer')
  })

  it('未登录访客不显示准备按钮，改为提示联系管理员', async () => {
    const wrapper = await mountView()

    expect(wrapper.findAll('button').some((b) => b.text().includes('准备该版本'))).toBe(false)
    expect(wrapper.text()).toContain('请联系管理员')
    // 未就绪时不请求类索引，避免无意义的后端 409。
    expect(mcsrcApi.listClasses).not.toHaveBeenCalled()
  })

  it('已就绪版本渲染类索引表格，并标记内部类', async () => {
    mcsrcApi.getStatus.mockResolvedValue(readyStatus())
    mcsrcApi.listClasses.mockResolvedValue({ items: CLASSES, total: 2, page: 1, page_size: 30 })
    const wrapper = await mountView()

    const rows = wrapper.findAll('tbody tr')
    expect(rows.length).toBe(2)
    expect(rows[0].text()).toContain('net.minecraft.server.MinecraftServer')
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

  it('点击类名打开详情：渲染结构视图、继承关系，并默认折叠合成方法', async () => {
    mcsrcApi.getStatus.mockResolvedValue(readyStatus())
    const wrapper = await mountView()

    const link = wrapper.findAll('button').find((b) => b.text() === 'net.minecraft.server.MinecraftServer')
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
    // 打开详情后类列表被详情面板替换。
    expect(wrapper.findAll('button').some((b) => b.text() === 'net.minecraft.server.MinecraftServer$1')).toBe(false)
  })

  it('勾选后可显示合成方法，点击祖先类可继续下钻', async () => {
    mcsrcApi.getStatus.mockResolvedValue(readyStatus())
    const wrapper = await mountView()
    await wrapper.findAll('button').find((b) => b.text() === 'net.minecraft.server.MinecraftServer')!.trigger('click')
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
    await wrapper.findAll('button').find((b) => b.text() === 'net.minecraft.server.MinecraftServer')!.trigger('click')
    await flushPromises()

    const detail = wrapper.find('[data-test="class-detail"]')
    expect(detail.text()).toContain('void tick()')
    expect(detail.text()).not.toContain('继承关系不可用')
    expect(wrapper.find('[data-test="class-hierarchy"]').exists()).toBe(false)
  })

  it('结构视图加载失败时展示错误，并可返回类列表', async () => {
    mcsrcApi.getStatus.mockResolvedValue(readyStatus())
    mcsrcApi.getClassDetail.mockRejectedValue(new ApiError('类不存在', 'CLASS_NOT_FOUND'))
    const wrapper = await mountView()
    await wrapper.findAll('button').find((b) => b.text() === 'net.minecraft.server.MinecraftServer')!.trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-test="class-detail"]').text()).toContain('类不存在')

    const back = wrapper.findAll('button').find((b) => b.text() === '返回列表')
    expect(back).toBeTruthy()
    await back!.trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-test="class-detail"]').exists()).toBe(false)
    expect(wrapper.findAll('tbody tr').length).toBe(2)
  })
})
