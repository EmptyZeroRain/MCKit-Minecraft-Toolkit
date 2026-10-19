// MC 源码浏览器 —— 反编译客户端（主线程）单元测试。
// 覆盖：jar 按 (version, side) 只下载一次、Worker 复用、请求透传、失败不缓存、类名格式转换。
// 说明：Worker 与 comlink 均被 mock —— jsdom 无 Worker / WASM 运行时。
import { beforeEach, describe, expect, it, vi } from 'vitest'

const wrapSpy = vi.hoisted(() => vi.fn())
vi.mock('comlink', () => ({ wrap: wrapSpy }))

const mcsrcApi = vi.hoisted(() => ({ fetchJarBlob: vi.fn() }))
vi.mock('@/api/mcsrc', () => mcsrcApi)

import { clearJarCache, decompileClass, getJar, jarKey } from '@/mcsrc/decompiler'
import { toClassFilePath, toDottedClassName, toInternalClassName } from '@/mcsrc/names'

const decompileClassMock = vi.fn()

class FakeWorker {
  constructor(
    public url: URL,
    public options?: { type?: string; name?: string },
  ) {}
}

describe('mcsrc 反编译客户端', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    clearJarCache()
    vi.stubGlobal('Worker', FakeWorker)
    decompileClassMock.mockImplementation(async (req: { className: string }) => ({
      className: req.className,
      source: `// ${req.className}`,
      language: 'java',
    }))
    wrapSpy.mockReturnValue({ decompileClass: decompileClassMock })
    mcsrcApi.fetchJarBlob.mockResolvedValue(new Blob(['jar-bytes']))
  })

  it('同一版本只下载一次 jar，并把类名透传给 Worker', async () => {
    const res = await decompileClass('1.21.4', 'client', 'net.minecraft.server.MinecraftServer')
    expect(res.source).toBe('// net.minecraft.server.MinecraftServer')

    await decompileClass('1.21.4', 'client', 'net.minecraft.world.level.Level')

    // jar 只取一次。
    expect(mcsrcApi.fetchJarBlob).toHaveBeenCalledTimes(1)
    expect(mcsrcApi.fetchJarBlob).toHaveBeenCalledWith('1.21.4', 'client')
    // Worker 只创建一次（wrap 复用）。
    expect(wrapSpy).toHaveBeenCalledTimes(1)
    // 请求体：类名 + jar 缓存键 + jar 字节。
    expect(decompileClassMock).toHaveBeenLastCalledWith(
      expect.objectContaining({
        className: 'net.minecraft.world.level.Level',
        jarName: '1.21.4-client',
      }),
    )
    expect(decompileClassMock.mock.calls[1][0].jar).toBeInstanceOf(Blob)
  })

  it('不同侧别分别缓存 jar', async () => {
    await getJar('1.21.4', 'client')
    await getJar('1.21.4', 'client')
    await getJar('1.21.4', 'server')
    expect(mcsrcApi.fetchJarBlob).toHaveBeenCalledTimes(2)
    expect(mcsrcApi.fetchJarBlob).toHaveBeenNthCalledWith(1, '1.21.4', 'client')
    expect(mcsrcApi.fetchJarBlob).toHaveBeenNthCalledWith(2, '1.21.4', 'server')
  })

  it('jar 下载失败不缓存，允许重试', async () => {
    mcsrcApi.fetchJarBlob.mockRejectedValueOnce(new Error('网络异常'))
    await expect(getJar('1.21.4', 'client')).rejects.toThrow('网络异常')

    mcsrcApi.fetchJarBlob.mockResolvedValueOnce(new Blob(['ok']))
    await expect(getJar('1.21.4', 'client')).resolves.toBeInstanceOf(Blob)
    expect(mcsrcApi.fetchJarBlob).toHaveBeenCalledTimes(2)
  })

  it('并发请求同一版本只触发一次下载', async () => {
    await Promise.all([getJar('1.21.4', 'client'), getJar('1.21.4', 'client')])
    expect(mcsrcApi.fetchJarBlob).toHaveBeenCalledTimes(1)
  })

  it('jar 缓存键与类名格式转换', () => {
    expect(jarKey('1.21.4', 'client')).toBe('1.21.4-client')
    expect(toInternalClassName('net.minecraft.server.MinecraftServer')).toBe(
      'net/minecraft/server/MinecraftServer',
    )
    expect(toDottedClassName('net/minecraft/server/MinecraftServer')).toBe(
      'net.minecraft.server.MinecraftServer',
    )
    expect(toClassFilePath('net.minecraft.world.level.Level$ExplosionInteraction')).toBe(
      'net/minecraft/world/level/Level$ExplosionInteraction.class',
    )
  })
})
