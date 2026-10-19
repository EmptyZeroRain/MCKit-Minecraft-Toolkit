// MC 源码浏览器 —— 跳转目标解析单元测试。
// 覆盖：类 token 直通、成员在引用点所属类命中、沿祖先链定位、字段按名匹配、
// 全部未命中时回退、索引缺失（类不在 jar 内）不抛异常、成员集合缓存复用。
import { beforeEach, describe, expect, it, vi } from 'vitest'

const mcsrcApi = vi.hoisted(() => ({ getClassDetail: vi.fn(), getClassHierarchy: vi.fn() }))
vi.mock('@/api/mcsrc', () => mcsrcApi)

import { clearDeclarationCache, resolveDeclaration } from '@/mcsrc/navigation'
import type { McsrcClassStructure, McsrcHierarchy } from '@/types/mcsrc'

const SERVER = 'net.minecraft.server.MinecraftServer'
const LEVEL = 'net.minecraft.world.level.Level'
const OBJECT = 'java.lang.Object'

interface Members {
  fields?: { name: string; descriptor: string }[]
  methods?: { name: string; descriptor: string }[]
}

function detailOf(name: string, members: Members = {}): McsrcClassStructure {
  return {
    name,
    simple: name.split('.').pop() as string,
    pkg: name.split('.').slice(0, -1).join('.'),
    kind: 'class',
    modifiers: [],
    major: 52,
    minor: 0,
    super: OBJECT,
    in_jar_super: false,
    interfaces: [],
    in_jar_interfaces: [],
    fields: (members.fields ?? []).map((f) => ({
      name: f.name,
      modifiers: [],
      type: 'int',
      descriptor: f.descriptor,
      synthetic: false,
    })),
    methods: (members.methods ?? []).map((m) => ({
      name: m.name,
      modifiers: [],
      type: 'void',
      params: [],
      descriptor: m.descriptor,
      synthetic: false,
    })),
    inner: false,
  }
}

function hierarchyOf(ancestors: string[]): McsrcHierarchy {
  return {
    name: SERVER,
    super: LEVEL,
    interfaces: [],
    ancestors,
    ancestors_in_jar: false,
    descendants: [],
    descendant_count: 0,
    total_descendants: 0,
    truncated: false,
  }
}

describe('mcsrc 跳转目标解析', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    clearDeclarationCache()
    mcsrcApi.getClassDetail.mockResolvedValue(detailOf(SERVER))
    mcsrcApi.getClassHierarchy.mockResolvedValue(hierarchyOf([LEVEL, OBJECT]))
  })

  it('类 token 直接返回自身，不查询成员索引', async () => {
    const res = await resolveDeclaration('1.21.4', 'client', { className: LEVEL, type: 'class' })
    expect(res).toEqual({ className: LEVEL, reveal: null })
    expect(mcsrcApi.getClassDetail).not.toHaveBeenCalled()
    expect(mcsrcApi.getClassHierarchy).not.toHaveBeenCalled()
  })

  it('成员声明在引用点所属类时直接命中', async () => {
    mcsrcApi.getClassDetail.mockResolvedValue(detailOf(SERVER, { methods: [{ name: 'tick', descriptor: '()V' }] }))

    const res = await resolveDeclaration('1.21.4', 'client', {
      className: SERVER,
      type: 'method',
      name: 'tick',
      descriptor: '()V',
    })

    expect(res.className).toBe(SERVER)
    expect(res.reveal).toEqual({ type: 'method', name: 'tick', descriptor: '()V' })
    // 命中即止，不必再查继承关系。
    expect(mcsrcApi.getClassHierarchy).not.toHaveBeenCalled()
  })

  it('成员声明在父类时沿祖先链向上定位', async () => {
    mcsrcApi.getClassDetail.mockImplementation(async (_version: string, name: string) =>
      name === LEVEL ? detailOf(LEVEL, { methods: [{ name: 'tick', descriptor: '()V' }] }) : detailOf(name),
    )

    const res = await resolveDeclaration('1.21.4', 'client', {
      className: SERVER,
      type: 'method',
      name: 'tick',
      descriptor: '()V',
    })

    expect(res.className).toBe(LEVEL)
    expect(res.reveal).toEqual({ type: 'method', name: 'tick', descriptor: '()V' })
    expect(mcsrcApi.getClassDetail).toHaveBeenCalledWith('1.21.4', SERVER, 'client')
    expect(mcsrcApi.getClassDetail).toHaveBeenCalledWith('1.21.4', LEVEL, 'client')
  })

  it('字段按名匹配（描述符可缺省）', async () => {
    mcsrcApi.getClassDetail.mockResolvedValue(detailOf(SERVER, { fields: [{ name: 'running', descriptor: 'Z' }] }))

    const res = await resolveDeclaration('1.21.4', 'client', {
      className: SERVER,
      type: 'field',
      name: 'running',
    })

    expect(res.className).toBe(SERVER)
    expect(res.reveal).toEqual({ type: 'field', name: 'running', descriptor: undefined })
  })

  it('全部未命中时回退到引用点所属类（仍可打开源码）', async () => {
    const res = await resolveDeclaration('1.21.4', 'client', {
      className: SERVER,
      type: 'method',
      name: 'missing',
      descriptor: '()V',
    })

    expect(res.className).toBe(SERVER)
    expect(res.reveal).toEqual({ type: 'method', name: 'missing', descriptor: '()V' })
  })

  it('索引缺失（类不在 jar 内）不抛异常，且成员集合命中缓存只查一次', async () => {
    mcsrcApi.getClassDetail.mockRejectedValue(new Error('CLASS_NOT_FOUND'))
    mcsrcApi.getClassHierarchy.mockRejectedValue(new Error('CLASS_NOT_FOUND'))
    const query = { className: SERVER, type: 'field' as const, name: 'ghost' }

    const first = await resolveDeclaration('1.21.4', 'client', query)
    const second = await resolveDeclaration('1.21.4', 'client', query)

    expect(first.className).toBe(SERVER)
    expect(second.className).toBe(SERVER)
    expect(mcsrcApi.getClassDetail).toHaveBeenCalledTimes(1)
    expect(mcsrcApi.getClassHierarchy).toHaveBeenCalledTimes(2)
  })
})
