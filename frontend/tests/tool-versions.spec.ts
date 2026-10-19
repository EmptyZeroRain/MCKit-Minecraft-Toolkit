// 版本特性与「版本不支持 / 不确定」提示（TODO §2）测试：
//   1) 已登记特性的生效版本边界精确判定（1.20.4 vs 1.20.5 等）；
//   2) 未填 / 快照 / 预览版统一归为「不确定」，不把理论结果说成确定结果；
//   3) 不支持的提示文案明确包含「理论值」边界说明；
//   4) 批量判断只返回非 supported 项。
import { describe, expect, it } from 'vitest'
import {
  UNKNOWN_VERSION_MESSAGE,
  VERSION_FEATURES,
  checkFeatureSupport,
  checkFeaturesSupport,
  getVersionFeature,
  isDecidableVersion,
  semverOf,
} from '@/utils/tool-versions'

describe('§2 版本特性注册表', () => {
  it('每个特性 key 唯一且字段完整', () => {
    const keys = VERSION_FEATURES.map((f) => f.key)
    expect(new Set(keys).size).toBe(keys.length)
    for (const f of VERSION_FEATURES) {
      expect(f.label).not.toBe('')
      expect(f.note).not.toBe('')
      expect(semverOf(f.since)).not.toBeNull()
    }
  })

  it('按 key 可查询到特性', () => {
    expect(getVersionFeature('data_components')?.since).toBe('1.20.5')
    expect(getVersionFeature('nope')).toBeUndefined()
  })
})

describe('§2 特性支持判定', () => {
  it('物品数据组件：1.20.5 起支持，1.20.4 不支持', () => {
    expect(checkFeatureSupport('data_components', '1.20.5').level).toBe('supported')
    expect(checkFeatureSupport('data_components', '1.21').level).toBe('supported')
    expect(checkFeatureSupport('data_components', '1.20.4').level).toBe('unsupported')
  })

  it('/effect give：1.19 起支持，1.18 不支持', () => {
    expect(checkFeatureSupport('effect_give', '1.19').level).toBe('supported')
    expect(checkFeatureSupport('effect_give', '1.18.2').level).toBe('unsupported')
  })

  it('试炼密室：1.21 起支持，1.20 不支持', () => {
    expect(checkFeatureSupport('trial_chambers', '1.21').level).toBe('supported')
    expect(checkFeatureSupport('trial_chambers', '1.20').level).toBe('unsupported')
  })

  it('苍白橡木：1.21.4 起支持，1.21.3 不支持', () => {
    expect(checkFeatureSupport('pale_oak', '1.21.4').level).toBe('supported')
    expect(checkFeatureSupport('pale_oak', '1.21.3').level).toBe('unsupported')
  })

  it('不支持的提示文案明确标注「理论值 / 不代表实际行为」，不冒充确定结论', () => {
    const s = checkFeatureSupport('data_components', '1.20.4')
    expect(s.message).toContain('1.20.5')
    expect(s.message).toContain('1.20.4')
    expect(s.message).toContain('理论值')
    expect(s.message).toContain('不代表该版本的实际行为')
  })
})

describe('§2 版本不确定处理', () => {
  it('未填 / 快照 / 预览版 / 非法输入统一为 unknown', () => {
    for (const v of ['', '   ', '24w14a', '1.21-pre1', '1.21-rc1', 'abc']) {
      const s = checkFeatureSupport('data_components', v)
      expect(s.level).toBe('unknown')
      expect(s.message).toBe(UNKNOWN_VERSION_MESSAGE)
    }
  })

  it('isDecidableVersion：正式版可裁决，其余不可', () => {
    expect(isDecidableVersion('1.21')).toBe(true)
    expect(isDecidableVersion('1.20.5')).toBe(true)
    expect(isDecidableVersion('')).toBe(false)
    expect(isDecidableVersion('24w14a')).toBe(false)
    expect(isDecidableVersion('1.21-pre1')).toBe(false)
  })
})

describe('§2 批量判断', () => {
  it('只返回非 supported 项', () => {
    const out = checkFeaturesSupport(['data_components', 'effect_give'], '1.18')
    expect(out.map((s) => s.feature.key)).toEqual(['data_components', 'effect_give'])
    expect(out.every((s) => s.level === 'unsupported')).toBe(true)

    const none = checkFeaturesSupport(['data_components', 'effect_give'], '1.21')
    expect(none).toEqual([])
  })
})
