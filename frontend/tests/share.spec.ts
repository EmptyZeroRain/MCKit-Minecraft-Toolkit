// 分享链接与查询参数工具测试（TODO §14）
// 重点覆盖「分享链接不含秘密信息」这一安全约束，以及默认值省略与非法值回退。
import { describe, expect, it } from 'vitest'
import {
  buildShareUrl,
  changedOnly,
  isSecretParam,
  queryBool,
  queryNumber,
  queryString,
  sanitizeShareQuery,
  sanitizeShareUrl,
} from '@/utils/share'

const ORIGIN = 'https://mc.example.com'

describe('分享参数安全防线', () => {
  it('识别常见机密参数名', () => {
    for (const key of [
      'token',
      'csrf_token',
      'X-CSRF-Token',
      'session',
      'session_id',
      'cookie',
      'password',
      'passwd',
      'authorization',
      'apiKey',
      'api_key',
      'signature',
      'ticket',
      'secret',
    ]) {
      expect(isSecretParam(key), key).toBe(true)
    }
  })

  it('正常业务参数不误判为机密', () => {
    for (const key of ['q', 'kind', 'seed', 'version', 'radius', 'type', 'x', 'z', 'from', 'to']) {
      expect(isSecretParam(key), key).toBe(false)
    }
  })

  it('sanitizeShareQuery 丢弃空值与机密参数', () => {
    expect(
      sanitizeShareQuery({
        q: 'stone',
        kind: '',
        radius: 0,
        token: 'abc',
        csrf: 'x',
        nothing: null,
        missing: undefined,
      }),
    ).toEqual({ q: 'stone', radius: '0' })
  })

  it('changedOnly 只保留与默认值不同的字段', () => {
    expect(
      changedOnly(
        { q: 'stone', kind: '', version: '1.21', radius: 8 },
        { q: '', kind: '', version: '1.21', radius: 8 },
      ),
    ).toEqual({ q: 'stone' })
  })
})

describe('buildShareUrl：稳定、可复现、不含秘密信息', () => {
  it('输出绝对 URL 并保留有效查询参数', () => {
    expect(buildShareUrl('/block-ids', { q: '石头', kind: 'item' }, ORIGIN)).toBe(
      `${ORIGIN}/block-ids?q=%E7%9F%B3%E5%A4%B4&kind=item`,
    )
  })

  it('丢弃 path 上自带的查询串与 hash（只由调用方显式提供状态）', () => {
    const url = buildShareUrl('/seed-map?token=leaked#section', { seed: '1' }, ORIGIN)
    expect(url).toBe(`${ORIGIN}/seed-map?seed=1`)
    expect(url).not.toContain('leaked')
    expect(url).not.toContain('#')
  })

  it('剥离疑似机密的查询参数', () => {
    const url = buildShareUrl('/x', { a: '1', csrf_token: 'bad', session: 'bad' }, ORIGIN)
    expect(url).toBe(`${ORIGIN}/x?a=1`)
  })

  it('默认状态产出「无参数」的干净链接', () => {
    expect(buildShareUrl('/block-ids', changedOnly({ q: '', kind: '' }, { q: '', kind: '' }), ORIGIN)).toBe(
      `${ORIGIN}/block-ids`,
    )
  })
})

describe('sanitizeShareUrl：清理地址栏链接', () => {
  it('剥离机密查询参数，保留业务参数与顺序', () => {
    expect(sanitizeShareUrl(`${ORIGIN}/seed-map?seed=42&csrf_token=bad&radius=8`)).toBe(
      `${ORIGIN}/seed-map?seed=42&radius=8`,
    )
  })

  it('剥离 hash 中的机密片段（如 OAuth 隐式授权 token），保留普通锚点', () => {
    expect(sanitizeShareUrl(`${ORIGIN}/x#access_token=secret&state=ok`)).toBe(`${ORIGIN}/x#state=ok`)
    expect(sanitizeShareUrl(`${ORIGIN}/x#chunk-3`)).toBe(`${ORIGIN}/x#chunk-3`)
  })

  it('非 URL 输入原样返回（不影响主流程）', () => {
    expect(sanitizeShareUrl('not a url')).toBe('not a url')
  })
})

describe('路由 query 读取：类型安全与回退', () => {
  it('queryString 处理缺失、数组与空串', () => {
    expect(queryString('a')).toBe('a')
    expect(queryString(undefined, 'd')).toBe('d')
    expect(queryString('', 'd')).toBe('d')
    expect(queryString(['a', 'b'], 'd')).toBe('d')
  })

  it('queryNumber 处理非法值并支持数组取首项', () => {
    expect(queryNumber('12', 8)).toBe(12)
    expect(queryNumber(['12', '13'], 8)).toBe(12)
    expect(queryNumber('abc', 8)).toBe(8)
    expect(queryNumber('', 8)).toBe(8)
    expect(queryNumber(undefined, 8)).toBe(8)
    expect(queryNumber('Infinity', 8)).toBe(8)
  })

  it('queryBool 只认 1/true', () => {
    expect(queryBool('1')).toBe(true)
    expect(queryBool('true')).toBe(true)
    expect(queryBool('TRUE')).toBe(true)
    expect(queryBool('0')).toBe(false)
    expect(queryBool('yes')).toBe(false)
    expect(queryBool(undefined, true)).toBe(true)
  })
})
