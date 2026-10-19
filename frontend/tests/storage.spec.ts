// 本地存储安全性测试（需求 13.3：收藏写入/读取异常不会崩溃）
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { loadStored, saveStored, removeStored } from '@/utils/storage'
import { parseFavorites } from '@/stores/favorites'

// 为测试提供可控的 localStorage 模拟
const STORAGE_KEY = 'mckit:favorites'

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

beforeEach(() => {
  const mem = createMemoryStorage()
  vi.stubGlobal('localStorage', mem)
})

describe('安全存储', () => {
  it('schema 版本匹配时可读取', () => {
    saveStored('favorites', { tools: ['a'], resources: [] }, 1)
    expect(loadStored('favorites', 1)).toEqual({ tools: ['a'], resources: [] })
  })

  it('schema 版本不匹配返回 null（安全重置路径）', () => {
    saveStored('favorites', { tools: ['a'] }, 1)
    expect(loadStored('favorites', 2)).toBeNull()
  })

  it('损坏 JSON 不抛出，返回 null', () => {
    window.localStorage.setItem(STORAGE_KEY, '{not valid json')
    expect(() => loadStored('favorites', 1)).not.toThrow()
    expect(loadStored('favorites', 1)).toBeNull()
  })

  it('非对象数据安全处理', () => {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify({ version: 1, data: 'oops' }))
    const data = loadStored('favorites', 1)
    const parsed = parseFavorites(data)
    expect(parsed.tools).toEqual([])
    expect(parsed.resources).toEqual([])
  })

  it('parseFavorites 过滤非法元素', () => {
    const parsed = parseFavorites({ tools: ['valid', 42, null], resources: 'nope' })
    expect(parsed.tools).toEqual(['valid'])
    expect(parsed.resources).toEqual([])
    expect(parsed).not.toHaveProperty('extra')
  })

  it('removeStored 可清除', () => {
    saveStored('favorites', { tools: ['a'] }, 1)
    removeStored('favorites')
    expect(window.localStorage.getItem(STORAGE_KEY)).toBeNull()
  })
})