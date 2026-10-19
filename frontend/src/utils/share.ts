// ===== 分享链接构造与查询参数读取（TODO §14）=====
//
// 设计目标：
//   1) **稳定可复现**：分享 URL 只编码「可分享的查询 / 配置状态」，任何人打开都能得到同一结果；
//   2) **绝不泄漏机密**：链接取自地址栏（Cookie / CSRF 从不进入 URL），并额外剥离疑似机密的
//      查询参数与 hash 片段，防止有人把 token 之类拼进链接后被二次转发；
//   3) **保持简洁**：只写「与默认值不同」的字段，URL 可读、便于人工分享与排障。

/** 疑似机密参数名（小写子串匹配）：命中即从分享链接中剥离 */
const SECRET_KEY_PATTERN = /(token|csrf|secret|session|cookie|password|passwd|auth|ticket|signature|apikey|api_key)/

/** 判断查询参数名是否疑似机密信息（用于防护性剥离，而非业务判断）。 */
export function isSecretParam(key: string): boolean {
  return SECRET_KEY_PATTERN.test(key.toLowerCase())
}

/** 过滤查询对象：去掉空值与疑似机密参数，并统一转为字符串。 */
export function sanitizeShareQuery(
  query: Record<string, unknown> = {},
): Record<string, string> {
  const out: Record<string, string> = {}
  for (const [key, value] of Object.entries(query)) {
    if (value === undefined || value === null || value === '') continue
    if (isSecretParam(key)) continue
    out[key] = String(value)
  }
  return out
}

/** 只保留「与默认值不同」的字段，避免把默认状态写进 URL（值统一转字符串）。 */
export function changedOnly(
  values: Record<string, unknown>,
  defaults: Record<string, unknown> = {},
): Record<string, string> {
  const out: Record<string, string> = {}
  for (const [key, value] of Object.entries(values)) {
    if (value === undefined || value === null || value === '') continue
    if (value === defaults[key]) continue
    out[key] = String(value)
  }
  return out
}

/**
 * 构造绝对分享 URL。
 *
 * `path` 上自带的查询串与 hash 会被丢弃：分享链接**只由调用方显式给出的状态**构成，
 * 避免把当前页面上已有的（可能含机密的）参数顺手带出去。
 */
export function buildShareUrl(
  path: string,
  query: Record<string, unknown> = {},
  origin?: string,
): string {
  const base =
    origin ?? (typeof window !== 'undefined' ? window.location.origin : 'http://localhost')
  const url = new URL(path, base)
  url.search = ''
  url.hash = ''
  for (const [key, value] of Object.entries(sanitizeShareQuery(query))) {
    url.searchParams.set(key, value)
  }
  return url.toString()
}

/**
 * 清理一个完整 URL：剥离疑似机密的查询参数，并清理 hash 中的机密片段
 * （OAuth 隐式授权会把 token 放在 `#access_token=...`，此处一并防护）。
 * 解析失败时原样返回，避免因分享功能影响主流程。
 */
export function sanitizeShareUrl(rawUrl: string): string {
  let url: URL
  try {
    url = new URL(rawUrl)
  } catch {
    return rawUrl
  }

  const pairs = url.searchParams
  const kept = [...pairs.entries()].filter(([key]) => !isSecretParam(key))
  url.search = ''
  for (const [key, value] of kept) url.searchParams.append(key, value)

  if (url.hash && url.hash.includes('=')) {
    const raw = url.hash.replace(/^#/, '')
    const keptHash = raw
      .split('&')
      .filter((part) => !isSecretParam(part.split('=')[0] ?? ''))
      .join('&')
    url.hash = keptHash ? `#${keptHash}` : ''
  }

  return url.toString()
}

/** 从路由 query 读取字符串（非字符串或空串回退默认值）。 */
export function queryString(value: unknown, fallback = ''): string {
  return typeof value === 'string' && value !== '' ? value : fallback
}

/** 从路由 query 读取数字（非法值回退默认值）。 */
export function queryNumber(value: unknown, fallback: number): number {
  const raw = Array.isArray(value) ? value[0] : value
  if (typeof raw !== 'string' || raw.trim() === '') return fallback
  const num = Number(raw)
  return Number.isFinite(num) ? num : fallback
}

/** 从路由 query 读取布尔（`1`/`true` 为真）。 */
export function queryBool(value: unknown, fallback = false): boolean {
  const raw = Array.isArray(value) ? value[0] : value
  if (raw === undefined) return fallback
  if (typeof raw !== 'string') return fallback
  return raw === '1' || raw.toLowerCase() === 'true'
}
