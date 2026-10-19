// 数据质量展示辅助纯函数（可单测，不含副作用）
import type {
  CheckSeverity,
  DataQualityCheck,
  DataImportJob,
  QualityIssueStatus,
} from '@/types/data-quality'

/** 严重级别展示文案 */
const SEVERITY_LABELS: Record<CheckSeverity, string> = {
  error: '严重',
  warning: '提醒',
  info: '统计',
}

/** 严重级别排序权重（越小越靠前） */
const SEVERITY_RANK: Record<CheckSeverity, number> = {
  error: 0,
  warning: 1,
  info: 2,
}

export function severityLabel(severity: CheckSeverity): string {
  return SEVERITY_LABELS[severity] ?? severity
}

/** 严重级别对应的徽标样式类 */
export function severityClass(severity: CheckSeverity): string {
  switch (severity) {
    case 'error':
      return 'bg-danger-mc/15 text-danger-mc'
    case 'warning':
      return 'bg-amber-500/15 text-amber-300'
    default:
      return 'bg-ink-700 text-text-muted'
  }
}

/**
 * 计算问题占比（0~100 的整数百分比）。
 * total 为 0 或 count 不小于 total 时按边界处理，避免出现 NaN / 超过 100。
 */
export function problemRatio(count: number, total: number): number {
  if (!total || total <= 0) return 0
  const ratio = Math.round((count / total) * 100)
  if (ratio < 0) return 0
  return ratio > 100 ? 100 : ratio
}

/**
 * 过滤出「需要关注的检查项」：严重与提醒级别且命中数 > 0。
 * 用于概览页突出显示，统计类（info）不参与。
 */
export function actionableChecks(checks: DataQualityCheck[]): DataQualityCheck[] {
  return checks
    .filter((c) => c.severity !== 'info' && c.count > 0)
    .sort((a, b) => (SEVERITY_RANK[a.severity] ?? 9) - (SEVERITY_RANK[b.severity] ?? 9))
}

/** 按类别分组，类别内保持「严重在前」的稳定顺序 */
export function groupChecksByCategory(checks: DataQualityCheck[]): Array<{ category: string; checks: DataQualityCheck[] }> {
  const order: string[] = []
  const map = new Map<string, DataQualityCheck[]>()
  for (const c of checks) {
    if (!map.has(c.category)) {
      map.set(c.category, [])
      order.push(c.category)
    }
    map.get(c.category)!.push(c)
  }
  return order.map((category) => ({
    category,
    checks: [...map.get(category)!].sort(
      (a, b) => (SEVERITY_RANK[a.severity] ?? 9) - (SEVERITY_RANK[b.severity] ?? 9),
    ),
  }))
}

/** 类别展示名 */
const CATEGORY_LABELS: Record<string, string> = {
  items: '物品资料库',
  recipes: '配方资料',
  commands: '指令资料',
  potions: '药水与酿造',
  villagers: '村民交易',
  mobs: '生物资料',
  redstone: '红石资料',
  mods: 'Mod 与整合包',
  dev_docs: '开发文档与映射',
  import: '数据导入',
}

export function categoryLabel(category: string): string {
  return CATEGORY_LABELS[category] ?? category
}

/** 导入任务是否失败（有失败行） */
export function isJobFailed(job: Pick<DataImportJob, 'failed_count'>): boolean {
  return job.failed_count > 0
}

// ===== 数据质量问题处理状态（工单） =====

/** 工单状态展示文案 */
export const ISSUE_STATUS_LABELS: Record<QualityIssueStatus, string> = {
  open: '待处理',
  assigned: '已指派',
  resolved: '已解决',
  ignored: '已忽略',
}

/** 工单状态对应的徽标样式类 */
export function issueStatusClass(status: QualityIssueStatus): string {
  switch (status) {
    case 'assigned':
      return 'bg-blue-500/15 text-blue-300'
    case 'resolved':
      return 'bg-grass-500/15 text-grass-300'
    case 'ignored':
      return 'bg-ink-700 text-text-muted'
    default:
      return 'bg-amber-500/15 text-amber-300'
  }
}

/**
 * 检查项 → 可直接跳转的页面。
 *
 * 数据质量检查覆盖的是「资料库原始数据」，多数没有独立的后台编辑页，
 * 因此直接跳到公开资料页或对应审核工作台，让运营看到实际数据再决定怎么修。
 */
export function checkActionPath(category: string): string {
  switch (category) {
    case 'items':
      return '/items'
    case 'recipes':
      return '/recipes'
    case 'commands':
      return '/commands'
    case 'potions':
      return '/potions'
    case 'dev_docs':
      return '/dev'
    case 'villagers':
      return '/villagers'
    case 'mobs':
      return '/mobs'
    case 'redstone':
      return '/redstone'
    case 'mods':
      return '/admin/review?tab=mods'
    case 'import':
      return '/admin/data-quality#import-jobs'
    default:
      return ''
  }
}

/** 工单是否处于「已处理完」状态（解决 / 忽略） */
export function isIssueClosed(status?: QualityIssueStatus): boolean {
  return status === 'resolved' || status === 'ignored'
}
