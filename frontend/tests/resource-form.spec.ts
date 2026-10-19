// 资源后台表单转换与校验测试（TODO §6）
import { describe, expect, it } from 'vitest'
import {
  dependencyToForm,
  formToDependency,
  formToReleasePayload,
  joinListText,
  releaseToForm,
  splitList,
  toDateInputValue,
  validateDependencyForms,
  validateReleaseForms,
  validateVersionText,
  type DependencyForm,
  type ReleaseForm,
} from '@/utils/resource-form'
import type { ResourceRelease } from '@/types/resource'

function makeRelease(overrides: Partial<ResourceRelease> = {}): ResourceRelease {
  return {
    id: 'rel1',
    resource_id: 'r1',
    version_label: '15.2.0.51',
    game_versions: ['1.20.1'],
    loader_types: ['forge'],
    source_url: 'https://example.com/jei/15',
    released_at: '2023-11-05T00:00:00Z',
    notes: '稳定版',
    is_latest: true,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function makeReleaseForm(overrides: Partial<ReleaseForm> = {}): ReleaseForm {
  return {
    version_label: '1.0',
    game_versions_text: '1.20.1',
    loader_types_text: 'forge',
    source_url: 'https://example.com/1',
    released_at: '2024-01-01',
    notes: '',
    is_latest: false,
    ...overrides,
  }
}

function makeDependencyForm(overrides: Partial<DependencyForm> = {}): DependencyForm {
  return {
    name: 'Fabric API',
    type: 'dependency',
    resource_url: '',
    required: false,
    note: '',
    ...overrides,
  }
}

describe('splitList / joinListText', () => {
  it('支持中英文逗号并去空白', () => {
    expect(splitList('1.20.1, 1.21，1.21.1')).toEqual(['1.20.1', '1.21', '1.21.1'])
    expect(splitList('')).toEqual([])
    expect(splitList(null)).toEqual([])
  })

  it('数组转文本', () => {
    expect(joinListText(['a', 'b'])).toBe('a, b')
    expect(joinListText(null)).toBe('')
  })
})

describe('toDateInputValue', () => {
  it('从 RFC3339 截取日期部分', () => {
    expect(toDateInputValue('2023-11-05T00:00:00Z')).toBe('2023-11-05')
    expect(toDateInputValue('2023-11-05')).toBe('2023-11-05')
  })

  it('空值与非法值返回空串', () => {
    expect(toDateInputValue(null)).toBe('')
    expect(toDateInputValue('2023/11/05')).toBe('')
  })
})

describe('依赖项转换', () => {
  it('缺省 type 归一为 dependency', () => {
    const form = dependencyToForm({ name: 'Forge' })
    expect(form.type).toBe('dependency')
    expect(form.required).toBe(false)
  })

  it('冲突项不携带必需标记，避免语义矛盾', () => {
    const out = formToDependency(makeDependencyForm({ type: 'conflict', required: true }))
    expect(out.type).toBe('conflict')
    expect(out.required).toBe(false)
  })

  it('可选字段为空时不写入', () => {
    const out = formToDependency(makeDependencyForm({ resource_url: '  ', note: ' ' }))
    expect(out.resource_url).toBeUndefined()
    expect(out.note).toBeUndefined()
  })

  it('往返转换保持关键字段', () => {
    const out = formToDependency(
      dependencyToForm({ name: ' Forge ', type: 'prerequisite', resource_url: 'https://e.com', required: true, note: '先装' }),
    )
    expect(out).toEqual({
      name: 'Forge',
      type: 'prerequisite',
      required: true,
      resource_url: 'https://e.com',
      note: '先装',
    })
  })
})

describe('发布版本转换', () => {
  it('接口 → 表单：日期截断、列表转文本', () => {
    const form = releaseToForm(makeRelease())
    expect(form.version_label).toBe('15.2.0.51')
    expect(form.game_versions_text).toBe('1.20.1')
    expect(form.loader_types_text).toBe('forge')
    expect(form.released_at).toBe('2023-11-05')
    expect(form.is_latest).toBe(true)
  })

  it('表单 → 载荷：空日期转为 null', () => {
    const payload = formToReleasePayload(makeReleaseForm({ released_at: '' }))
    expect(payload.released_at).toBeNull()
    expect(payload.game_versions).toEqual(['1.20.1'])
    expect(payload.loader_types).toEqual(['forge'])
  })

  it('表单 → 载荷：去除首尾空白', () => {
    const payload = formToReleasePayload(makeReleaseForm({ version_label: ' 1.0 ', source_url: ' https://e.com/1 ' }))
    expect(payload.version_label).toBe('1.0')
    expect(payload.source_url).toBe('https://e.com/1')
  })
})

describe('validateReleaseForms', () => {
  it('合法表单通过', () => {
    expect(validateReleaseForms([])).toBeNull()
    expect(validateReleaseForms([makeReleaseForm()])).toBeNull()
  })

  it('版本标签必填且唯一', () => {
    expect(validateReleaseForms([makeReleaseForm({ version_label: ' ' })])).toContain('版本标签必填')
    expect(
      validateReleaseForms([makeReleaseForm({ version_label: 'a' }), makeReleaseForm({ version_label: 'a' })]),
    ).toContain('重复')
  })

  it('发布页链接必填且必须为 http(s)', () => {
    expect(validateReleaseForms([makeReleaseForm({ source_url: '' })])).toContain('必填')
    expect(validateReleaseForms([makeReleaseForm({ source_url: 'javascript:alert(1)' })])).toContain('http(s)')
  })

  it('加载器必须在枚举内', () => {
    expect(validateReleaseForms([makeReleaseForm({ loader_types_text: 'bukkit' })])).toContain('不支持的加载器')
    expect(validateReleaseForms([makeReleaseForm({ loader_types_text: 'forge, neoforge' })])).toBeNull()
  })

  it('至多一个最新标记', () => {
    expect(
      validateReleaseForms([
        makeReleaseForm({ version_label: 'a', is_latest: true }),
        makeReleaseForm({ version_label: 'b', is_latest: true }),
      ]),
    ).toContain('只能有一个')
  })
})

describe('validateDependencyForms', () => {  it('名称必填', () => {
    expect(validateDependencyForms([makeDependencyForm({ name: ' ' })])).toContain('名称必填')
  })

  it('链接可选但必须为 http(s)', () => {
    expect(validateDependencyForms([makeDependencyForm({ resource_url: '' })])).toBeNull()
    expect(validateDependencyForms([makeDependencyForm({ resource_url: 'ftp://a/b' })])).toContain('http(s)')
  })
})

describe('validateVersionText（TODO §6-C）', () => {
  it('空值或普通声明通过（格式交由后端裁决）', () => {
    expect(validateVersionText('')).toBeNull()
    expect(validateVersionText('1.20.1, 1.21')).toBeNull()
    expect(validateVersionText('1.16-1.17, 1.18+')).toBeNull()
  })

  it('通配 all 单独出现时通过', () => {
    expect(validateVersionText('all')).toBeNull()
    expect(validateVersionText('*')).toBeNull()
  })

  it('通配与具体版本并列时拦截', () => {
    expect(validateVersionText('all, 1.20.1')).toContain('不能同时填写')
    expect(validateVersionText('1.20.1, *')).toContain('不能同时填写')
  })

  it('发布版本表单会校验版本声明', () => {
    expect(validateReleaseForms([makeReleaseForm({ game_versions_text: 'all, 1.20.1' })])).toContain(
      '不能同时填写',
    )
  })
})
