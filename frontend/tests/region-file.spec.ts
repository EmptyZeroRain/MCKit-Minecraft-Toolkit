// 区块文件校验测试（需求 13.3：表单校验）
import { describe, expect, it } from 'vitest'
import {
  MAX_LEVEL_UPLOAD_BYTES,
  MAX_REGION_UPLOAD_BYTES,
  classifySaveFile,
  formatMiB,
  parseRegionFileName,
  validateLevelFile,
  validateRegionFile,
} from '@/utils/region-file'

describe('区块文件名解析', () => {
  it.each([
    ['r.0.0.mca', 0, 0],
    ['r.-1.2.mca', -1, 2],
    ['R.3.4.MCA', 3, 4],
    ['r.123.-456.mca', 123, -456],
  ])('%s -> (%i, %i)', (name, x, z) => {
    expect(parseRegionFileName(name)).toEqual({ x, z })
  })

  it.each([
    'region.mca',
    'r.0.0.mca.bak',
    'r.a.b.mca',
    'r.0.mca',
    '0.0.mca',
    '',
  ])('非法文件名 %s 返回 null', (name) => {
    expect(parseRegionFileName(name)).toBeNull()
  })
})

describe('区块文件校验', () => {
  it('合法文件名与大小通过', () => {
    expect(validateRegionFile('r.0.0.mca', 1024)).toBeNull()
    expect(validateRegionFile('r.-2.5.mca', MAX_REGION_UPLOAD_BYTES)).toBeNull()
  })

  it('文件名非法时给出可读提示', () => {
    const msg = validateRegionFile('myregion.mca', 1024)
    expect(msg).toContain('r.0.0.mca')
  })

  it('超出上限时提示大小', () => {
    const msg = validateRegionFile('r.0.0.mca', MAX_REGION_UPLOAD_BYTES + 1)
    expect(msg).toContain('64 MiB')
  })

  it('formatMiB 保留两位小数', () => {
    expect(formatMiB(1024 * 1024)).toBe('1.00 MiB')
    expect(formatMiB(0)).toBe('0.00 MiB')
  })
})

describe('存档文件类型判定', () => {
  it.each([
    ['level.dat', 'level'],
    ['Level.DAT', 'level'],
    ['r.0.0.mca', 'region'],
    ['r.-3.7.mca', 'region'],
  ] as const)('%s -> %s', (name, kind) => {
    expect(classifySaveFile(name)).toBe(kind)
  })

  it.each(['level.dat.bak', 'myworld.dat', 'region.mca', ''])(
    '无法识别 %s 返回 null',
    (name) => {
      expect(classifySaveFile(name)).toBeNull()
    },
  )
})

describe('level.dat 校验', () => {
  it('合法文件名与大小通过', () => {
    expect(validateLevelFile('level.dat', 4096)).toBeNull()
    expect(validateLevelFile('LEVEL.DAT', MAX_LEVEL_UPLOAD_BYTES)).toBeNull()
  })

  it('文件名非法时提示 level.dat', () => {
    expect(validateLevelFile('world.dat', 1024)).toContain('level.dat')
  })

  it('超出上限时提示大小', () => {
    expect(validateLevelFile('level.dat', MAX_LEVEL_UPLOAD_BYTES + 1)).toContain('64 MiB')
  })
})
