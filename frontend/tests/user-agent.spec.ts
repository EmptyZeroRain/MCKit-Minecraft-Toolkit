// User-Agent 友好化：登录历史与安全日志要把原始 UA 归纳成「浏览器 · 系统」。
import { describe, expect, it } from 'vitest'
import { describeUserAgent, parseUserAgent } from '@/utils/user-agent'

const UA = {
  chromeMac:
    'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36',
  edgeWindows:
    'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0 Safari/537.36 Edg/125.0',
  safariMac:
    'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Safari/605.1.15',
  firefoxLinux: 'Mozilla/5.0 (X11; Linux x86_64; rv:126.0) Gecko/20100101 Firefox/126.0',
  safariIOS:
    'Mozilla/5.0 (iPhone; CPU iPhone OS 17_4 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Mobile/15E148 Safari/604.1',
  chromeAndroid:
    'Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Mobile Safari/537.36',
  opera: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/125.0 Safari/537.36 OPR/111.0',
  curl: 'curl/8.4.0',
}

describe('parseUserAgent', () => {
  it('识别常见浏览器与系统', () => {
    expect(parseUserAgent(UA.chromeMac)).toEqual({ browser: 'Chrome', os: 'macOS' })
    expect(parseUserAgent(UA.firefoxLinux)).toEqual({ browser: 'Firefox', os: 'Linux' })
    expect(parseUserAgent(UA.chromeAndroid)).toEqual({ browser: 'Chrome', os: 'Android' })
    expect(parseUserAgent(UA.safariIOS)).toEqual({ browser: 'Safari', os: 'iOS' })
  })

  it('优先识别伪装成 Chrome / Safari 的客户端', () => {
    // Edge 的 UA 同时含 Chrome 与 Safari，必须先判 Edge。
    expect(parseUserAgent(UA.edgeWindows)).toEqual({ browser: 'Edge', os: 'Windows' })
    expect(parseUserAgent(UA.opera)).toEqual({ browser: 'Opera', os: 'Windows' })
    // Safari 的 UA 含 Chrome 不出现的 "Version/x Safari/x" 组合。
    expect(parseUserAgent(UA.safariMac)).toEqual({ browser: 'Safari', os: 'macOS' })
  })

  it('非浏览器客户端回退到 UA 首段标识', () => {
    expect(parseUserAgent(UA.curl)).toEqual({ browser: 'curl/8.4.0', os: '未知系统' })
  })

  it('空值返回 null，由调用方决定文案', () => {
    expect(parseUserAgent('')).toBeNull()
    expect(parseUserAgent(null)).toBeNull()
    expect(parseUserAgent(undefined)).toBeNull()
    expect(parseUserAgent('   ')).toBeNull()
  })
})

describe('describeUserAgent', () => {
  it('输出「浏览器 · 系统」', () => {
    expect(describeUserAgent(UA.chromeMac)).toBe('Chrome · macOS')
    expect(describeUserAgent(UA.edgeWindows)).toBe('Edge · Windows')
  })

  it('没有 UA 的历史数据标记为未知设备', () => {
    expect(describeUserAgent('')).toBe('未知设备')
    expect(describeUserAgent(null)).toBe('未知设备')
  })
})
