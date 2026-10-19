// ===== 明暗主题状态（本地持久化 + 跟随系统偏好）=====
// 存储 key 带 schema 版本，读取异常时安全回退系统偏好。
import { defineStore } from 'pinia'
import { loadStored, saveStored } from '@/utils/storage'

const STORAGE_KEY = 'theme'
const SCHEMA_VERSION = 1
const THEME_ATTR = 'data-theme'

export type ThemeMode = 'light' | 'dark'

function detectSystem(): ThemeMode {
  if (typeof window !== 'undefined' && window.matchMedia) {
    return window.matchMedia('(prefers-color-scheme: light)').matches
      ? 'light'
      : 'dark'
  }
  return 'dark'
}

export const useThemeStore = defineStore('theme', {
  state: () => ({
    mode: 'dark' as ThemeMode,
    initialized: false,
  }),
  getters: {
    isDark: (state) => state.mode === 'dark',
    isLight: (state) => state.mode === 'light',
  },
  actions: {
    /** 应用启动时恢复；无偏好或损坏时跟随系统 */
    init() {
      if (this.initialized) return
      this.initialized = true
      const saved = loadStored<ThemeMode>(STORAGE_KEY, SCHEMA_VERSION)
      this.mode = saved === 'light' || saved === 'dark' ? saved : detectSystem()
      this.apply()
      // 监听系统主题变化（仅当用户未手动设置时生效）
      window.matchMedia('(prefers-color-scheme: light)').addEventListener(
        'change',
        () => {
          const manual = loadStored<ThemeMode>(STORAGE_KEY, SCHEMA_VERSION)
          if (!manual) {
            this.mode = detectSystem()
            this.apply()
          }
        },
      )
    },
    toggle() {
      this.mode = this.mode === 'dark' ? 'light' : 'dark'
      saveStored(STORAGE_KEY, this.mode, SCHEMA_VERSION)
      this.apply()
    },
    setMode(mode: ThemeMode) {
      this.mode = mode
      saveStored(STORAGE_KEY, mode, SCHEMA_VERSION)
      this.apply()
    },
    /** 将主题写入 <html> 的 data-theme 属性 */
    apply() {
      if (typeof document !== 'undefined') {
        document.documentElement.setAttribute(THEME_ATTR, this.mode)
        // 同步顶部状态栏颜色（Android Chrome / Safari）
        const meta = document.querySelector('meta[name="theme-color"]')
        if (meta) {
          meta.setAttribute(
            'content',
            this.mode === 'light' ? '#f7f9fc' : '#0b1220',
          )
        }
      }
    },
  },
})