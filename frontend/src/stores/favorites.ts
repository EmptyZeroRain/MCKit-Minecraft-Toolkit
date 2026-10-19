// ===== 收藏状态（需求 5.9）=====
// 首期保存在浏览器本地：带 schema 版本，解析失败安全重置，
// 不本地存储任何令牌/密码等敏感信息。
import { defineStore } from 'pinia'
import { loadStored, saveStored, removeStored } from '@/utils/storage'

const STORAGE_KEY = 'favorites'
const SCHEMA_VERSION = 1

interface FavoritesData {
  /** 收藏的工具 slug 列表 */
  tools: string[]
  /** 收藏的资源 slug 列表 */
  resources: string[]
}

const EMPTY: FavoritesData = { tools: [], resources: [] }

export function parseFavorites(raw: unknown): FavoritesData {
  if (!raw || typeof raw !== 'object') return { ...EMPTY }
  const obj = raw as Record<string, unknown>
  const toArray = (v: unknown): string[] =>
    Array.isArray(v) ? (v.filter((x) => typeof x === 'string') as string[]) : []
  return { tools: toArray(obj.tools), resources: toArray(obj.resources) }
}

export const useFavoritesStore = defineStore('favorites', {
  state: () => ({
    tools: [] as string[],
    resources: [] as string[],
  }),
  getters: {
    isToolFavorite: (state) => (slug: string) => state.tools.includes(slug),
    isResourceFavorite: (state) => (slug: string) => state.resources.includes(slug),
    count: (state) => state.tools.length + state.resources.length,
  },
  actions: {
    /** 从 localStorage 恢复；损坏数据重置为默认并写回 */
    restore() {
      const data = loadStored<FavoritesData>(STORAGE_KEY, SCHEMA_VERSION)
      if (data === null) {
        this.reset()
      } else {
        const parsed = parseFavorites(data)
        this.tools = parsed.tools
        this.resources = parsed.resources
      }
    },
    persist() {
      saveStored(STORAGE_KEY, { tools: this.tools, resources: this.resources }, SCHEMA_VERSION)
    },
    toggleTool(slug: string) {
      const idx = this.tools.indexOf(slug)
      if (idx >= 0) this.tools.splice(idx, 1)
      else this.tools.push(slug)
      this.persist()
    },
    toggleResource(slug: string) {
      const idx = this.resources.indexOf(slug)
      if (idx >= 0) this.resources.splice(idx, 1)
      else this.resources.push(slug)
      this.persist()
    },
    /** 清除本地所有收藏数据 */
    clearAll() {
      this.reset()
      removeStored(STORAGE_KEY)
    },
    reset() {
      this.tools = []
      this.resources = []
    },
  },
})