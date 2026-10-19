// ===== 最近使用工具（需求 5.9）=====
// 浏览器本地存储，带 schema 版本；解析失败安全重置。
import { defineStore } from 'pinia'
import { loadStored, saveStored, removeStored } from '@/utils/storage'
import { builtinToolRoute } from '@/utils/builtin-tools'
import { useUserAuthStore } from '@/stores/user-auth'
import { useUserWorkspaceStore } from '@/stores/user-workspace'

const STORAGE_KEY = 'recent-tools'
const SCHEMA_VERSION = 1
const MAX_RECENT = 12

interface RecentEntry {
  slug: string
  name: string
  category: string
  lastUsedAt: number // epoch ms
}

export const useRecentToolsStore = defineStore('recentTools', {
  state: () => ({
    entries: [] as RecentEntry[],
  }),
  getters: {
    recentList: (state) =>
      [...state.entries]
        .sort((a, b) => b.lastUsedAt - a.lastUsedAt)
        .slice(0, MAX_RECENT),
  },
  actions: {
    restore() {
      const data = loadStored<RecentEntry[]>(STORAGE_KEY, SCHEMA_VERSION)
      if (data === null) {
        this.entries = []
        this.persist()
        return
      }
      const valid = (Array.isArray(data) ? data : []).filter(
        (e): e is RecentEntry =>
          !!e && typeof e.slug === 'string' && typeof e.name === 'string',
      )
      this.entries = valid
      if (valid.length !== data.length) this.persist()
    },
    persist() {
      saveStored(STORAGE_KEY, this.entries, SCHEMA_VERSION)
    },
    /** 记录一次使用；按 slug 去重并移到最前 */
    record(slug: string, name: string, category: string) {
      this.entries = [
        { slug, name, category, lastUsedAt: Date.now() },
        ...this.entries.filter((e) => e.slug !== slug),
      ].slice(0, MAX_RECENT)
      this.persist()
      this.syncToServer(slug, name)
    },
    /** 已登录时同步到服务端「最近使用」（跨设备可见）；失败静默。 */
    syncToServer(slug: string, name: string) {
      const auth = useUserAuthStore()
      if (!auth.isLoggedIn) return
      const url = builtinToolRoute(slug) ?? `/tools/${slug}`
      void useUserWorkspaceStore().recordRecent('tool', slug, name, url)
    },
    clearAll() {
      this.entries = []
      removeStored(STORAGE_KEY)
    },
  },
})