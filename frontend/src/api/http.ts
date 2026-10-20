// ===== Axios 实例：统一超时、错误处理与鉴权 =====
import axios, {
  AxiosError,
  type AxiosRequestConfig,
  type InternalAxiosRequestConfig,
} from 'axios'
import router from '@/router'
import type { ApiErrorBody, ApiResponse } from '@/types/api'
import { ApiError } from '@/types/api'
import { useAuthStore } from '@/stores/auth'

// 开发/生产 API 基础地址
// 默认同源 /api/v1（开发环境经 vite proxy 转发到后端）
const BASE_URL = import.meta.env.VITE_API_BASE_URL || '/api/v1'

const http = axios.create({
  baseURL: BASE_URL,
  timeout: 15000,
  headers: { 'Content-Type': 'application/json' },
  withCredentials: true, // 管理端会话 Cookie
})

// 请求注入：写操作附加 X-Request-ID（若存在）与 CSRF 请求头
let csrfToken = ''
export function setCsrfToken(token: string) {
  csrfToken = token
}

http.interceptors.request.use((config: InternalAxiosRequestConfig) => {
  if (csrfToken && config.method !== 'get') {
    config.headers.set('X-CSRF-Token', csrfToken)
  }
  return config
})

// 统一的错误翻译，避免把服务端原始错误直接展示给访客。
// 具体错误码必须在后端控制，前端只做展示化处理。
const CODE_MESSAGES: Record<string, string> = {
  VALIDATION_ERROR: '提交的内容不符合要求，请检查后重试',
  UNAUTHORIZED: '登录状态已失效，请重新登录',
  FORBIDDEN: '没有权限执行该操作',
  NOT_FOUND: '请求的内容不存在或已下架',
  CONFLICT: '内容已存在，请检查唯一标识（如 slug）',
  RATE_LIMITED: '操作过于频繁，请稍后再试',
  INTERNAL_ERROR: '服务器开小差了，请稍后重试',
  BAD_REQUEST: '请求有误，请检查输入',
  CSRF_FAILED: '安全校验失败，请刷新页面后重试',
}

interface RetryQueueItem {
  resolve: () => void
  reject: (reason?: unknown) => void
}

// 并发 401 去重标志：避免多个请求同时刷新导致风暴
let isRefreshing = false
let refreshQueue: RetryQueueItem[] = []

async function handleUnauthorized(config: AxiosRequestConfig) {
  const auth = useAuthStore()
  // 尝试静默刷新会话
  if (!isRefreshing) {
    isRefreshing = true
    try {
      await auth.refreshSession()
      const queued = [...refreshQueue]
      refreshQueue = []
      queued.forEach((item) => item.resolve())
    } catch {
      const queued = [...refreshQueue]
      refreshQueue = []
      queued.forEach((item) => item.reject(new ApiError('登录已失效', 'UNAUTHORIZED')))
      auth.reset()
      const path = router.currentRoute.value.path
      if (path.startsWith('/admin') && path !== '/admin/login') {
        router.push({ path: '/login', query: { redirect: router.currentRoute.value.fullPath } })
      }
    } finally {
      isRefreshing = false
    }
  }
  // 若正在刷新，排队等待
  await new Promise<void>((resolve, reject) => {
    refreshQueue.push({ resolve, reject })
  })
  if (config.url && !config.url.includes('/auth/me')) {
    return http.request(config)
  }
  return undefined
}

http.interceptors.response.use(
  (response) => response,
  async (error: AxiosError<ApiErrorBody>) => {
    const status = error.response?.status
    const body = error.response?.data
    const requestId = body?.request_id
    const code = body?.error?.code ?? 'UNKNOWN_ERROR'

    // 401：会话失效，尝试刷新后重放。
    // 统一登录页上的 401（密码 / MFA 验证码错误）必须原样交给页面展示，
    // 不能触发会话刷新与跳转，否则会吞掉真实错误。
    const currentPath = router.currentRoute.value.path
    if (
      status === 401 &&
      currentPath !== '/login' &&
      currentPath !== '/admin/login'
    ) {
      try {
        const retried = await handleUnauthorized(error.config!)
        return retried
      } catch {
        // 刷新失败已在上层处理
      }
    }

    const message =
      CODE_MESSAGES[code] ??
      body?.error?.message ??
      (status ? `请求失败（HTTP ${status}）` : '网络连接异常，请检查网络后重试')

    // 网络层错误
    if (!error.response) {
      return Promise.reject(
        new ApiError('网络连接异常，请检查网络后重试', 'NETWORK_ERROR', {
          requestId,
          status,
        }),
      )
    }

    // 表单校验字段错误透传给调用方（页面级展示）
    return Promise.reject(
      new ApiError(message, code, {
        details: body?.error?.details,
        requestId,
        status,
      }),
    )
  },
)

/** 分页列表响应断言：data 为数组、meta 带分页 */
export interface PagedData<T> {
  items: T[]
  total: number
  page: number
  pageSize: number
  totalPages: number
  /** 无结果搜索时后端给出的可尝试关键词（TODO §5），其它列表接口为 undefined。 */
  suggestions: string[]
}

/** 统一解析分页响应 */
export function toPaged<T>(res: { data: ApiResponse<T[]> }): PagedData<T> {
  const meta = res.data.meta
  return {
    items: res.data.data ?? [],
    total: meta?.total ?? 0,
    page: meta?.page ?? 1,
    pageSize: meta?.page_size ?? 0,
    totalPages: meta?.total_pages ?? 0,
    suggestions: meta?.suggestions ?? [],
  }
}

export function mapApiPage<T>(raw: PagedData<T>, page: number, pageSize: number) {
  return {
    items: raw.items,
    total: raw.total,
    page: raw.page || page,
    pageSize: raw.pageSize || pageSize,
    totalPages: raw.totalPages || Math.ceil(raw.total / pageSize),
  }
}

// 业务前端自带 Toast 反馈；HTTP 层不主动弹窗，避免噪音。
export function useHttpError(): { getMessage: (err: unknown) => string } {
  return {
    getMessage: (err: unknown) => {
      if (err instanceof ApiError) return err.message
      return '操作失败，请稍后重试'
    },
  }
}

export default http