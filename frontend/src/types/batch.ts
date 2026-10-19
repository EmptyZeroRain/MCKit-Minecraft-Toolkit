// 批量操作的通用返回结构（后端 domain.BatchReviewResult）。
//
// 批量接口逐条独立执行：单条失败不影响其余记录，因此前端必须展示
// 「成功 X / 失败 Y」以及失败原因，而不是简单的成功或失败。

/** 单条处理结果 */
export interface BatchItemResult {
  id: string
  /** 可读名称（资源名 / Mod 名等），为空时回退到 id */
  label?: string
  ok: boolean
  /** 失败原因（后端已脱敏为可直接展示的文案） */
  error?: string
}

/** 批量操作汇总 */
export interface BatchReviewResult {
  requested: number
  succeeded: number
  failed: number
  items: BatchItemResult[]
}

/** 单次批量操作的最大条数（与后端 domain.MaxBatchReview 一致） */
export const MAX_BATCH_REVIEW = 50

/** 把批量结果压缩成一条可展示的提示文案。 */
export function describeBatchResult(result: BatchReviewResult, action: string): string {
  if (result.failed === 0) return `${action}成功：${result.succeeded} 条`
  const reasons = result.items
    .filter((i) => !i.ok)
    .slice(0, 3)
    .map((i) => `${i.label || i.id}（${i.error || '未知原因'}）`)
    .join('；')
  const more = result.failed > 3 ? ` 等 ${result.failed} 条` : ''
  return `${action}完成：成功 ${result.succeeded} 条，失败 ${result.failed} 条 —— ${reasons}${more}`
}
