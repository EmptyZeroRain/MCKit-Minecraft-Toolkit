// 资源元数据展示辅助测试（TODO §6 资源兼容性与来源信息）
import { describe, expect, it } from 'vitest'
import {
  compatibilitySourceLabel,
  compatibilitySummary,
  dependencyGroups,
  linkStatusSummary,
  normalizeDependencyType,
  safeExternalUrl,
  sortReleases,
} from '@/utils/resource'
import type { Resource, ResourceRelease } from '@/types/resource'

function makeResource(overrides: Partial<Resource> = {}): Resource {
  return {
    id: 'r1',
    slug: 'jei',
    name: 'JEI',
    summary: '物品配方查看',
    description: '',
    resource_type: 'mod',
    cover_image_url: null,
    author_name: 'mezz',
    source_platform: 'CurseForge',
    source_url: 'https://example.com/jei',
    license: 'MIT',
    game_edition: 'java',
    supported_versions: [],
    loader_types: [],
    dependencies: [],
    install_guide: '',
    tags: [],
    compatibility_note: '',
    status: 'published',
    link_status: 'unchecked',
    last_checked_at: null,
    published_at: null,
    created_by: null,
    updated_by: null,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function makeRelease(overrides: Partial<ResourceRelease> = {}): ResourceRelease {
  return {
    id: 'rel1',
    resource_id: 'r1',
    version_label: '1.0',
    game_versions: ['1.20.1'],
    loader_types: ['forge'],
    source_url: 'https://example.com/jei/1',
    released_at: '2024-01-01T00:00:00Z',
    notes: '',
    is_latest: false,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('normalizeDependencyType', () => {
  it('缺省与非法值一律视为 dependency', () => {
    expect(normalizeDependencyType(undefined)).toBe('dependency')
    expect(normalizeDependencyType('')).toBe('dependency')
    expect(normalizeDependencyType('suggests')).toBe('dependency')
  })

  it('保留前置与冲突语义', () => {
    expect(normalizeDependencyType('prerequisite')).toBe('prerequisite')
    expect(normalizeDependencyType('conflict')).toBe('conflict')
  })
})

describe('dependencyGroups', () => {
  it('空输入返回空数组', () => {
    expect(dependencyGroups(undefined)).toEqual([])
    expect(dependencyGroups([])).toEqual([])
  })

  it('按前置 → 依赖 → 冲突排序，且不返回空分组', () => {
    const groups = dependencyGroups([
      { name: 'OptiFine', type: 'conflict' },
      { name: 'Forge', type: 'prerequisite' },
      { name: 'Cloth Config' },
    ])
    expect(groups.map((g) => g.type)).toEqual(['prerequisite', 'dependency', 'conflict'])
    expect(groups.map((g) => g.label)).toEqual(['前置 Mod', '依赖', '已知冲突'])
    expect(groups[1].items[0].name).toBe('Cloth Config')
  })

  it('仅含冲突时不出现其他分组', () => {
    const groups = dependencyGroups([{ name: 'X', type: 'conflict' }])
    expect(groups).toHaveLength(1)
    expect(groups[0].type).toBe('conflict')
  })
})

describe('compatibilitySummary', () => {
  it('接口返回已验证结论时原样透传并翻译依据', () => {
    const got = compatibilitySummary(makeResource(), {
      verified: true,
      game_versions: ['1.20.1'],
      loader_types: ['forge'],
      sources: ['resource', 'releases'],
      note: '备注',
    })
    expect(got.verified).toBe(true)
    expect(got.gameVersions).toEqual(['1.20.1'])
    expect(got.sourceLabel).toBe('资源级字段 + 发布版本')
    expect(got.note).toBe('备注')
  })

  it('接口返回未验证时保留后端提示文案', () => {
    const got = compatibilitySummary(makeResource(), {
      verified: false,
      game_versions: [],
      loader_types: [],
      sources: [],
      note: '',
      unverified_hint: '自定义提示',
    })
    expect(got.verified).toBe(false)
    expect(got.hint).toBe('自定义提示')
  })

  it('缺少 compatibility 字段时按「未验证」处理，绝不断言兼容', () => {
    const got = compatibilitySummary(makeResource({ supported_versions: ['1.20.1'], loader_types: ['forge'] }))
    expect(got.verified).toBe(false)
    expect(got.hint).toContain('未验证')
    // 兜底展示资源级字段，但不据此给出已验证结论
    expect(got.gameVersions).toEqual(['1.20.1'])
  })

  it('缺少 compatibility 时透传人工备注', () => {
    const got = compatibilitySummary(makeResource({ compatibility_note: '仅客户端' }))
    expect(got.note).toBe('仅客户端')
  })

  it('声明「所有版本」通配时置 allVersions 并从具体版本中剥离 all', () => {
    const got = compatibilitySummary(makeResource(), {
      verified: true,
      game_versions: ['all', '1.20.1'],
      loader_types: ['forge'],
      sources: ['resource'],
      all_versions: true,
      note: '',
    })
    expect(got.allVersions).toBe(true)
    expect(got.gameVersions).toEqual(['1.20.1'])
    expect(got.verified).toBe(true)
  })

  it('接口未给 all_versions 时也能从声明中识别通配', () => {
    const got = compatibilitySummary(makeResource(), {
      verified: true,
      game_versions: ['all'],
      loader_types: [],
      sources: ['resource'],
      note: '',
    })
    expect(got.allVersions).toBe(true)
    expect(got.gameVersions).toEqual([])
  })

  it('缺少 compatibility 时同样识别资源级通配', () => {
    const got = compatibilitySummary(makeResource({ supported_versions: ['all'] }))
    expect(got.allVersions).toBe(true)
    expect(got.gameVersions).toEqual([])
  })
})

describe('compatibilitySourceLabel', () => {
  it('空来源返回空串', () => {
    expect(compatibilitySourceLabel([])).toBe('')
    expect(compatibilitySourceLabel(undefined)).toBe('')
  })

  it('未知来源保留原文，不静默丢弃', () => {
    expect(compatibilitySourceLabel(['resource', 'custom'])).toBe('资源级字段 + custom')
  })
})

describe('linkStatusSummary', () => {
  it('按状态给出客观文案与语气', () => {
    expect(linkStatusSummary('ok')).toEqual({ label: '正常', tone: 'ok' })
    expect(linkStatusSummary('redirected').tone).toBe('warn')
    expect(linkStatusSummary('broken')).toEqual({ label: '失效', tone: 'bad' })
    expect(linkStatusSummary('error').tone).toBe('warn')
  })

  it('未知或空状态视为未检查', () => {
    expect(linkStatusSummary(null)).toEqual({ label: '未检查', tone: 'muted' })
    expect(linkStatusSummary(undefined).tone).toBe('muted')
  })
})

describe('sortReleases', () => {
  it('发布时间降序，未知时间排后，同时间按版本标签升序', () => {
    const got = sortReleases([
      makeRelease({ id: 'a', version_label: 'old', released_at: '2022-01-01T00:00:00Z' }),
      makeRelease({ id: 'b', version_label: 'unknown', released_at: null }),
      makeRelease({ id: 'c', version_label: 'new', released_at: '2024-01-01T00:00:00Z' }),
    ])
    expect(got.map((r) => r.version_label)).toEqual(['new', 'old', 'unknown'])
  })

  it('空输入返回空数组', () => {
    expect(sortReleases(null)).toEqual([])
  })
})

describe('safeExternalUrl', () => {
  it('仅放行 http(s) 链接', () => {
    expect(safeExternalUrl('https://example.com/x')).toBe('https://example.com/x')
    expect(safeExternalUrl(' http://example.com/x ')).toBe('http://example.com/x')
  })

  it('拒绝危险协议与空值', () => {
    expect(safeExternalUrl('javascript:alert(1)')).toBeNull()
    expect(safeExternalUrl('ftp://example.com')).toBeNull()
    expect(safeExternalUrl(null)).toBeNull()
    expect(safeExternalUrl('')).toBeNull()
  })
})
