// ===== 搜索关键词高亮（需求 §5.1）=====
// 纯函数：把文本按查询词切分为片段，交由视图用 <mark> 渲染。
// 不使用 v-html，全部走文本插值，避免 XSS（URL/剪贴板/数据源文本一律按不可信处理）。

export interface HighlightSegment {
  /** 片段文本（原样，不做任何 HTML 转义外的加工） */
  text: string
  /** 是否为命中的关键词 */
  match: boolean
}

/** 转义正则元字符，避免用户输入被当作正则执行。 */
function escapeRegExp(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

/**
 * 将 text 按 query 切分为片段；不区分大小写，命中所有出现位置。
 * query 为空或 text 为空时返回单段（不命中）。
 */
export function highlightSegments(text: string, query: string): HighlightSegment[] {
  const src = text ?? ''
  if (src === '') return []
  const q = (query ?? '').trim()
  if (q === '') return [{ text: src, match: false }]

  const re = new RegExp(escapeRegExp(q), 'gi')
  const out: HighlightSegment[] = []
  let last = 0
  for (const m of src.matchAll(re)) {
    const idx = m.index ?? 0
    if (idx > last) out.push({ text: src.slice(last, idx), match: false })
    out.push({ text: m[0], match: true })
    last = idx + m[0].length
  }
  if (last < src.length) out.push({ text: src.slice(last), match: false })
  return out
}
