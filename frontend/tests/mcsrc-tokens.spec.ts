// MC 源码浏览器 —— 源码 token 工具（纯函数）单元测试。
// 覆盖：偏移命中判定（左闭右开 / 越界）、行列 ↔ 偏移换算、成员跳转片段编解码、声明定位。
import { describe, expect, it } from 'vitest'
import {
  findDeclarationToken,
  findTokenAtOffset,
  offsetAtPosition,
  parseMemberFragment,
  tokenMemberFragment,
} from '@/mcsrc/tokens'
import type { McsrcToken } from '@/types/mcsrc'

function token(partial: Partial<McsrcToken> & { start: number; length: number }): McsrcToken {
  return { type: 'class', declaration: false, className: 'a.B', inJar: true, ...partial }
}

const TOKENS: McsrcToken[] = [
  token({ start: 0, length: 5, type: 'class', className: 'a.B' }),
  token({ start: 10, length: 4, type: 'field', className: 'a.B', name: 'count', descriptor: 'I' }),
  token({ start: 20, length: 6, type: 'method', className: 'a.B', name: 'tick', descriptor: '()V' }),
]

describe('mcsrc token 工具', () => {
  it('按偏移命中 token（区间左闭右开）', () => {
    expect(findTokenAtOffset(TOKENS, 0)?.type).toBe('class')
    expect(findTokenAtOffset(TOKENS, 4)?.type).toBe('class')
    // 5 是 class token 的右边界，已不属于它。
    expect(findTokenAtOffset(TOKENS, 5)).toBeNull()
    expect(findTokenAtOffset(TOKENS, 13)?.type).toBe('field')
    expect(findTokenAtOffset(TOKENS, 25)?.type).toBe('method')
    expect(findTokenAtOffset(TOKENS, 26)).toBeNull()
  })

  it('空 token 列表 / 非法偏移返回 null', () => {
    expect(findTokenAtOffset([], 0)).toBeNull()
    expect(findTokenAtOffset(undefined, 0)).toBeNull()
    expect(findTokenAtOffset(TOKENS, -1)).toBeNull()
  })

  it('行列 → 字符偏移（行号 1 起，列号 1 起）', () => {
    const lines = ['ab', 'cdef']
    expect(offsetAtPosition(lines, 1, 1)).toBe(0)
    expect(offsetAtPosition(lines, 1, 3)).toBe(2)
    // 第二行首字符：第一行 2 字符 + 换行符 1 = 3。
    expect(offsetAtPosition(lines, 2, 1)).toBe(3)
    expect(offsetAtPosition(lines, 2, 5)).toBe(7)
    // 超出范围的行不越界（钳制到最后一行之后）。
    expect(offsetAtPosition(lines, 99, 1)).toBe(8)
  })

  it('成员 token → 跳转片段', () => {
    expect(tokenMemberFragment(TOKENS[0])).toBe('')
    expect(tokenMemberFragment(TOKENS[1])).toBe('field:count')
    expect(tokenMemberFragment(TOKENS[2])).toBe('method:tick:()V')
    // 缺名字时不可跳转。
    expect(tokenMemberFragment(token({ start: 0, length: 1, type: 'method' }))).toBe('')
  })

  it('跳转片段 → 定位目标', () => {
    expect(parseMemberFragment('field:count')).toEqual({ type: 'field', name: 'count' })
    expect(parseMemberFragment('method:tick:()V')).toEqual({
      type: 'method',
      name: 'tick',
      descriptor: '()V',
    })
    // 描述符可缺省（旧链接 / 手工构造）。
    expect(parseMemberFragment('method:tick')).toEqual({ type: 'method', name: 'tick' })
    expect(parseMemberFragment('')).toBeNull()
    expect(parseMemberFragment(undefined)).toBeNull()
    expect(parseMemberFragment('bogus')).toBeNull()
    expect(parseMemberFragment('class:Foo')).toBeNull()
  })

  it('定位成员声明：方法按描述符区分重载，字段按名匹配', () => {
    const declarations: McsrcToken[] = [
      token({ start: 5, length: 4, type: 'method', declaration: true, name: 'tick', descriptor: '(I)V' }),
      token({ start: 15, length: 4, type: 'method', declaration: true, name: 'tick', descriptor: '()V' }),
      token({ start: 25, length: 5, type: 'field', declaration: true, name: 'count', descriptor: 'I' }),
      token({ start: 40, length: 4, type: 'method', name: 'tick', descriptor: '()V' }),
    ]
    expect(findDeclarationToken(declarations, { type: 'method', name: 'tick', descriptor: '()V' })?.start).toBe(15)
    expect(findDeclarationToken(declarations, { type: 'method', name: 'tick', descriptor: '(I)V' })?.start).toBe(5)
    // 描述符未知时退化为按名匹配。
    expect(findDeclarationToken(declarations, { type: 'method', name: 'tick' })?.start).toBe(5)
    expect(findDeclarationToken(declarations, { type: 'field', name: 'count' })?.start).toBe(25)
    // 只认声明处：仅存引用时找不到。
    expect(findDeclarationToken(declarations, { type: 'method', name: 'missing' })).toBeNull()
    expect(findDeclarationToken(undefined, { type: 'field', name: 'count' })).toBeNull()
  })
})
