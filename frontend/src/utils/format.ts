// ===== 通用格式化工具 =====

/** UTC RFC3339 时间 -> 本地时区展示（需求 2.2：接口 UTC，页面本地时区） */
export function formatDateTime(iso: string | null | undefined): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '—'
  return d.toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

/** 仅日期 */
export function formatDate(iso: string | null | undefined): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '—'
  return d.toLocaleDateString('zh-CN')
}

/** 相对时间（最近更新等场景） */
export function timeAgo(iso: string | null | undefined): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '—'
  const diff = Date.now() - d.getTime()
  const minute = 60 * 1000
  const hour = 60 * minute
  const day = 24 * hour
  if (diff < minute) return '刚刚'
  if (diff < hour) return `${Math.floor(diff / minute)} 分钟前`
  if (diff < day) return `${Math.floor(diff / hour)} 小时前`
  if (diff < 7 * day) return `${Math.floor(diff / day)} 天前`
  return formatDate(iso)
}

/** JSONB 数组转普通展示 */
export function joinList(
  items: string[] | null | undefined,
  sep = '、',
): string {
  if (!items || items.length === 0) return '—'
  return items.join(sep)
}

/** 截断长文本 */
export function truncate(text: string, max = 120): string {
  if (text.length <= max) return text
  return text.slice(0, max).trimEnd() + '…'
}

/** 将字符串安全地转为 URL 可读 slug（前端展示用） */
export function slugify(input: string): string {
  return input
    .trim()
    .toLowerCase()
    .replace(/[\s_]+/g, '-')
    .replace(/[^\w\u4e00-\u9fa5-]/g, '')
    .replace(/-+/g, '-')
}

export function isValidSlug(slug: string): boolean {
  return /^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(slug)
}

/** 字节大小格式化（移植自 CreeperCode） */
export function formatSize(bytes: number | null | undefined): string {
  if (bytes == null || !Number.isFinite(bytes)) return '-'
  if (bytes < 1024) return `${bytes} B`
  const units = ['KB', 'MB', 'GB']
  let v = bytes / 1024
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(1)} ${units[i]}`
}

/** SHA1 短展示 */
export function shortSha1(sha1: string | null | undefined): string {
  if (!sha1) return '—'
  return `${sha1.slice(0, 12)}…`
}

/**
 * 版本声明展示：`all` 表示「不受具体游戏版本限制」（versioning 的 AllKeyword），
 * 直接展示英文关键字会让中文界面显得割裂，故统一转为中文标签。
 */
export function formatVersionLabel(version: string): string {
  return version === 'all' ? '全版本' : version
}

/** 版本声明列表展示（用 / 连接，最多 max 项） */
export function formatVersionList(versions: string[] | null | undefined, max = 3): string {
  if (!versions || versions.length === 0) return '—'
  return versions.slice(0, max).map(formatVersionLabel).join(' / ')
}