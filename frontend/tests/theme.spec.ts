// 主题切换测试（需求：明暗主题切换 + 本地持久化）
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useThemeStore } from '@/stores/theme'

function createMemoryStorage() {
  const store = new Map<string, string>()
  return {
    getItem: (k: string) => store.get(k) ?? null,
    setItem: (k: string, v: string) => {
      store.set(k, v)
    },
    removeItem: (k: string) => {
      store.delete(k)
    },
  }
}

function stubMedia(matches: boolean) {
  const listeners: Array<(e: { matches: boolean }) => void> = []
  Object.defineProperty(window, 'matchMedia', {
    configurable: true,
    writable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches,
      media: query,
      addEventListener: (_type: string, cb: (e: { matches: boolean }) => void) => {
        listeners.push(cb)
      },
      addListener: vi.fn(),
      removeListener: vi.fn(),
      removeEventListener: vi.fn(),
    })),
  })
  return listeners
}

beforeEach(() => {
  const mem = createMemoryStorage()
  vi.stubGlobal('localStorage', mem)
  document.documentElement.removeAttribute('data-theme')
  setActivePinia(createPinia())
})

describe('主题切换 store', () => {
  it('无本地偏好时跟随系统设置', () => {
    stubMedia(true) // 系统亮色
    const theme = useThemeStore()
    theme.init()
    expect(theme.mode).toBe('light')
    expect(document.documentElement.getAttribute('data-theme')).toBe('light')
  })

  it('系统暗色时默认 dark', () => {
    stubMedia(false)
    const theme = useThemeStore()
    theme.init()
    expect(theme.mode).toBe('dark')
  })

  it('toggle 在明暗之间切换并写入 data-theme', () => {
    stubMedia(false)
    const theme = useThemeStore()
    theme.init()
    expect(theme.mode).toBe('dark')

    theme.toggle()
    expect(theme.mode).toBe('light')
    expect(document.documentElement.getAttribute('data-theme')).toBe('light')
    // 持久化已写入 localStorage
    const raw = window.localStorage.getItem('mckit:theme')!
    expect(JSON.parse(raw)).toEqual({ version: 1, data: 'light' })

    theme.toggle()
    expect(theme.mode).toBe('dark')
    expect(document.documentElement.getAttribute('data-theme')).toBe('dark')
  })

  it('本地持久化优先级高于系统偏好', () => {
    stubMedia(true) // 系统亮色
    // 预先写入暗色偏好
    window.localStorage.setItem('mckit:theme', JSON.stringify({ version: 1, data: 'dark' }))
    const theme = useThemeStore()
    theme.init()
    expect(theme.mode).toBe('dark')
  })

  it('损坏的本地数据回退系统偏好，不崩溃', () => {
    stubMedia(false)
    window.localStorage.setItem('mckit:theme', '{broken json')
    const theme = useThemeStore()
    theme.init()
    expect(theme.mode).toBe('dark')
    expect(document.documentElement.getAttribute('data-theme')).toBe('dark')
  })
})