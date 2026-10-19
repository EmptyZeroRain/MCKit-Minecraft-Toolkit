// ===== 公开匿名使用统计上报（TODO §13）=====
// 独立 axios 实例（不复用 api/http.ts / api/user-http.ts）的原因：
//   1) 该接口公开且匿名，不需要也不应携带管理端 / 用户会话与 CSRF 头；
//   2) 上报是 best-effort，绝不能因 401 触发会话刷新或跳转登录页。
import axios from 'axios'

const BASE_URL = import.meta.env.VITE_API_BASE_URL || '/api/v1'

// timeout 取小值：上报不影响用户体验，超时即放弃。
const statsHttp = axios.create({
  baseURL: BASE_URL,
  timeout: 5000,
  headers: { 'Content-Type': 'application/json' },
})

/** 工具事件动作：visit 访问 / use 使用 / fail 失败 */
export type ToolStatAction = 'visit' | 'use' | 'fail'

/** 上报一次内置工具事件；失败由调用方静默处理（见 utils/tool-stats.ts）。 */
export function reportToolEvent(id: string, action: ToolStatAction): Promise<unknown> {
  return statsHttp.post('/stats/event', { scope: 'tool', id, action })
}
