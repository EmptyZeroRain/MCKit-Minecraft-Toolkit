// ===== 用户接口专用 axios 实例 =====
// 与管理员实例（api/http.ts）分离的原因：
//   1) CSRF token 是「会话绑定」的，管理员与用户各自独立，共用变量会互相覆盖；
//   2) 401 处理不同：用户接口失效只应清理用户会话并跳转用户登录页，
//      绝不能触发管理员会话刷新（反之亦然）。
import axios, { AxiosError, type InternalAxiosRequestConfig } from 'axios'
import type { ApiErrorBody } from '@/types/api'
import { ApiError } from '@/types/api'

const BASE_URL = import.meta.env.VITE_API_BASE_URL || '/api/v1'

const userHttp = axios.create({
  baseURL: BASE_URL,
  timeout: 15000,
  headers: { 'Content-Type': 'application/json' },
  withCredentials: true, // 用户会话 Cookie（mc_user_session）
})

let csrfToken = ''
export function setUserCsrfToken(token: string) {
  csrfToken = token
}

userHttp.interceptors.request.use((config: InternalAxiosRequestConfig) => {
  if (csrfToken && config.method !== 'get') {
    config.headers.set('X-CSRF-Token', csrfToken)
  }
  return config
})

const CODE_MESSAGES: Record<string, string> = {
  VALIDATION_ERROR: '提交的内容不符合要求，请检查后重试',
  UNAUTHORIZED: '登录状态已失效，请重新登录',
  FORBIDDEN: '没有权限执行该操作',
  CONFLICT: '该用户名已被占用',
  RATE_LIMITED: '操作过于频繁，请稍后再试',
  LOGIN_LOCKED: '登录尝试次数过多，请稍后再试',
  INTERNAL_ERROR: '服务器开小差了，请稍后重试',
  BAD_REQUEST: '请求有误，请检查输入',
  CSRF_INVALID: '安全校验失败，请刷新页面后重试',
}

userHttp.interceptors.response.use(
  (response) => response,
  (error: AxiosError<ApiErrorBody>) => {
    const status = error.response?.status
    const body = error.response?.data
    const code = body?.error?.code ?? 'UNKNOWN_ERROR'
    const requestId = body?.request_id

    if (!error.response) {
      return Promise.reject(
        new ApiError('网络连接异常，请检查网络后重试', 'NETWORK_ERROR', { status }),
      )
    }

    const message =
      body?.error?.message ?? CODE_MESSAGES[code] ?? (status ? `请求失败（HTTP ${status}）` : '请求失败')

    return Promise.reject(
      new ApiError(message, code, { details: body?.error?.details, requestId, status }),
    )
  },
)

export default userHttp
