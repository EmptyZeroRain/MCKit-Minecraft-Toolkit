// ===== UI 全局状态：轻量 Toast（需求 4.2 复制成功提示等）=====
import { defineStore } from 'pinia'

export type ToastType = 'success' | 'error' | 'info' | 'warning'

export interface ToastItem {
  id: number
  type: ToastType
  message: string
}

let seed = 0

export const useUiStore = defineStore('ui', {
  state: () => ({
    toasts: [] as ToastItem[],
  }),
  actions: {
    push(type: ToastType, message: string) {
      seed += 1
      const id = seed
      this.toasts.push({ id, type, message })
      // 3.5 秒后自动移除
      setTimeout(() => this.remove(id), 3500)
    },
    success(message: string) {
      this.push('success', message)
    },
    error(message: string) {
      this.push('error', message)
    },
    info(message: string) {
      this.push('info', message)
    },
    warning(message: string) {
      this.push('warning', message)
    },
    remove(id: number) {
      this.toasts = this.toasts.filter((t) => t.id !== id)
    },
  },
})