// 搜索关键词高亮测试（需求 5.1：标题/摘要关键词高亮）
import { describe, expect, it } from 'vitest'
import { highlightSegments } from '@/utils/highlight'

describe('关键词高亮 · 切分', () => {
  it('空文本返回空数组', () => {
    expect(highlightSegments('', 'stone')).toEqual([])
  })

  it('空关键词返回单段且不命中', () => {
    expect(highlightSegments('石头', '   ')).toEqual([{ text: '石头', match: false }])
  })

  it('命中关键词并保留前后文', () => {
    expect(highlightSegments('钻石镐 stone pickaxe', 'stone')).toEqual([
      { text: '钻石镐 ', match: false },
      { text: 'stone', match: true },
      { text: ' pickaxe', match: false },
    ])
  })

  it('大小写不敏感', () => {
    const segs = highlightSegments('Stone', 'stone')
    expect(segs).toEqual([{ text: 'Stone', match: true }])
  })

  it('命中多处出现', () => {
    const segs = highlightSegments('aXbXc', 'x')
    expect(segs.filter((s) => s.match)).toHaveLength(2)
    expect(segs.map((s) => s.text).join('')).toBe('aXbXc')
  })

  it('关键词在首尾时边界正确', () => {
    expect(highlightSegments('stone', 'stone')).toEqual([{ text: 'stone', match: true }])
    expect(highlightSegments('astone', 'a')).toEqual([
      { text: 'a', match: true },
      { text: 'stone', match: false },
    ])
  })

  it('无命中时返回整段', () => {
    expect(highlightSegments('钻石', 'iron')).toEqual([{ text: '钻石', match: false }])
  })

  it('正则元字符按字面量处理，不做正则解释', () => {
    // 若未转义，"a.c" 会误匹配 "abc"
    expect(highlightSegments('abc', 'a.c')).toEqual([{ text: 'abc', match: false }])
    expect(highlightSegments('a.c', 'a.c')).toEqual([{ text: 'a.c', match: true }])
  })

  it('切分后各段拼接等于原文', () => {
    const text = 'Minecraft 合成配方 craft'
    const segs = highlightSegments(text, 'c')
    expect(segs.map((s) => s.text).join('')).toBe(text)
  })
})
