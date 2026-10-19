// MC 版本清单工具测试（TODO §6-C）
import { describe, expect, it, vi, beforeEach } from 'vitest'

vi.mock('@/api/versions', () => ({ listVersions: vi.fn() }))

import { listVersions } from '@/api/versions'
import {
  ALL_VERSIONS,
  ALL_VERSIONS_LABEL,
  FALLBACK_RELEASE_VERSIONS,
  buildVersionOptions,
  loadReleaseVersions,
  majorVersions,
  resetVersionCache,
} from '@/utils/mc-versions'

const mockedVersions = vi.mocked(listVersions)

function versionList(ids: string[]) {
  return {
    latest: { release: ids[0] ?? '', snapshot: '' },
    versions: ids.map((id) => ({ id, type: 'release', url: '', time: '', releaseTime: '' })),
  } as never
}

beforeEach(() => {
  vi.clearAllMocks()
  resetVersionCache()
})

describe('loadReleaseVersions', () => {
  it('从官方清单提取正式版 id（保持最新在前的顺序）', async () => {
    mockedVersions.mockResolvedValue(versionList(['1.21.4', '1.21.1', '1.20.1']))
    const got = await loadReleaseVersions()
    expect(got).toEqual(['1.21.4', '1.21.1', '1.20.1'])
    expect(mockedVersions).toHaveBeenCalledWith({ type: 'release' })
  })

  it('内存缓存：同一会话只请求一次，并发调用合并', async () => {
    mockedVersions.mockResolvedValue(versionList(['1.21']))
    const [a, b] = await Promise.all([loadReleaseVersions(), loadReleaseVersions()])
    expect(a).toEqual(b)
    await loadReleaseVersions()
    expect(mockedVersions).toHaveBeenCalledTimes(1)
  })

  it('拉取失败时回退兜底清单，不抛异常', async () => {
    mockedVersions.mockRejectedValue(new Error('network down'))
    const got = await loadReleaseVersions()
    expect(got).toEqual(FALLBACK_RELEASE_VERSIONS)
  })

  it('上游返回空清单时同样回退兜底', async () => {
    mockedVersions.mockResolvedValue(versionList([]))
    const got = await loadReleaseVersions()
    expect(got.length).toBeGreaterThan(0)
    expect(got).toEqual(FALLBACK_RELEASE_VERSIONS)
  })
})

describe('majorVersions', () => {
  it('归并主版本并去重，保持传入顺序', () => {
    expect(majorVersions(['1.21.4', '1.21.1', '1.21', '1.20.2', '1.20'])).toEqual(['1.21', '1.20'])
  })

  it('忽略无法识别的条目', () => {
    expect(majorVersions(['25w03a', '1.20.1'])).toEqual(['1.20'])
  })

  it('空输入返回空数组', () => {
    expect(majorVersions([])).toEqual([])
  })
})

describe('buildVersionOptions', () => {
  it('默认不加入通配项', () => {
    expect(buildVersionOptions(['1.21', '1.20'])).toEqual([
      { label: '1.21', value: '1.21' },
      { label: '1.20', value: '1.20' },
    ])
  })

  it('includeAll 时「全部 MC 版本」在最前，值为通配 all', () => {
    const got = buildVersionOptions(['1.21'], { includeAll: true })
    expect(got[0]).toEqual({ label: ALL_VERSIONS_LABEL, value: ALL_VERSIONS })
    expect(got[1]).toEqual({ label: '1.21', value: '1.21' })
  })
})
