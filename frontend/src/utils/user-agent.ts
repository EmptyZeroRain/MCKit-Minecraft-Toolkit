// User-Agent 友好化：把原始 UA 归纳成「浏览器 · 系统」，供登录历史 / 安全日志展示。
//
// 设计取舍：
//   - 不引入 ua-parser 之类的依赖：这里只需要「能认出是哪台设备」，
//     不追求版本号精确，UA 本身也允许为空（历史审计数据没有该字段）；
//   - 识别顺序有讲究：Edge / Opera / Chrome 的 UA 都含 Safari 与 Chrome 字样，
//     必须先匹配更具体的，再退化到通用内核。

export interface DeviceInfo {
  /** 系统 / 平台，如 macOS、Windows、iOS、Android */
  os: string
  /** 浏览器 / 客户端，如 Chrome、Safari、Edge */
  browser: string
}

/** 解析 User-Agent；无法识别时返回 null，由调用方决定展示文案。 */
export function parseUserAgent(ua: string | null | undefined): DeviceInfo | null {
  const raw = (ua ?? '').trim()
  if (!raw) return null

  return { os: detectOS(raw), browser: detectBrowser(raw) }
}

function detectOS(ua: string): string {
  // iOS 的 UA 同时含 "like Mac OS X"，必须先于 macOS 判断。
  if (/iPhone|iPad|iPod/i.test(ua)) return 'iOS'
  if (/Android/i.test(ua)) return 'Android'
  if (/Windows NT/i.test(ua)) return 'Windows'
  if (/Mac OS X|Macintosh/i.test(ua)) return 'macOS'
  // ChromeOS 的 UA 是 X11 + CrOS，需在 Linux 之前判断。
  if (/CrOS/i.test(ua)) return 'ChromeOS'
  if (/Linux|X11/i.test(ua)) return 'Linux'
  return '未知系统'
}

function detectBrowser(ua: string): string {
  // Edge（含旧版 Edg/ 与新版 EdgA/）与 Opera 都伪装成 Chrome，先匹配。
  if (/Edg(e|A|iOS)?\//i.test(ua)) return 'Edge'
  if (/OPR\/|Opera/i.test(ua)) return 'Opera'
  if (/Firefox\/|FxiOS\//i.test(ua)) return 'Firefox'
  if (/Chrome\/|CriOS\//i.test(ua)) return 'Chrome'
  if (/Safari\//i.test(ua)) return 'Safari'
  // 命令行 / 脚本客户端（curl、Python requests 等）直接暴露首段标识。
  const token = rawToken(ua)
  return token || '未知客户端'
}

/** UA 首段（如 "curl/8.4.0"）作为兜底标识，去掉过长的注释部分。 */
function rawToken(ua: string): string {
  const first = ua.split(/[\s(]/)[0] ?? ''
  return first.length > 40 ? `${first.slice(0, 40)}…` : first
}

/** 展示文案：识别失败时区分「无数据」与「无法识别」。 */
export function describeUserAgent(ua: string | null | undefined): string {
  const info = parseUserAgent(ua)
  if (!info) return '未知设备'
  return `${info.browser} · ${info.os}`
}
