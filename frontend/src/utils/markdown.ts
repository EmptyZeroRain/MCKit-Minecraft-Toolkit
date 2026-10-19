// ===== 安全的轻量 Markdown 渲染（禁用原始 HTML，防 XSS）=====
// 支持：标题、段落、粗体/斜体/行内代码、链接、图片引用（仅占位链接）、
//       无序/有序列表、引用、代码块、分割线、表格基础支持。
// 所有输出都经 HTML 转义；链接仅允许 http/https 协议。

function escapeHtml(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

function safeUrl(raw: string): string | null {
  const t = raw.trim()
  if (/^https?:\/\//i.test(t)) return t
  return null
}

/** 转义行内内容，并渲染行内 Markdown（粗体/斜体/行内代码/链接） */
function renderInline(line: string): string {
  let out = escapeHtml(line)

  // 行内代码 `code`
  out = out.replace(/`([^`]+)`/g, (_m, code: string) => `<code>${code}</code>`)

  // 链接 [text](url)
  out = out.replace(
    /\[([^\]]+)\]\(([^)\s]+)\)/g,
    (_m, text: string, url: string) => {
      const safe = safeUrl(url)
      if (!safe) return `<em>${text}</em>（非法链接已忽略）`
      const t = renderInline(text)
      return `<a href="${safe}" rel="noopener noreferrer nofollow" target="_blank">${t}</a>`
    },
  )

  // 粗体 **text**
  out = out.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
  // 斜体 *text*
  out = out.replace(/(^|[^*])\*([^*\n]+)\*(?!\*)/g, '$1<em>$2</em>')

  return out
}

function linkify(text: string): string {
  // 链接 token 已在上层处理；此处仅兜底裸链接 -> 文本
  return text
}

// 表格单元格内的转义竖线占位符（抓取内容会用 \| 表示单元格内的字面竖线）
const ESCAPED_PIPE = '\u0000PIPE\u0000'

/** 拆分 Markdown 表格行：保留内部空单元格，仅去除首尾空列，并正确处理转义竖线。 */
function splitTableRow(line: string): string[] {
  const normalized = line.replace(/\\\|/g, ESCAPED_PIPE)
  const cells = normalized.split('|').map((c) => c.trim().split(ESCAPED_PIPE).join('|'))
  if (cells.length && cells[0] === '') cells.shift()
  if (cells.length && cells[cells.length - 1] === '') cells.pop()
  return cells
}

/** 是否为 Markdown 表格分隔行（如 `| --- | :--: |`）。 */
function isTableSeparator(line: string): boolean {
  if (!line.includes('|')) return false
  const cells = splitTableRow(line)
  return cells.length > 0 && cells.every((c) => /^:?-{2,}:?$/.test(c))
}

export function renderSafeMarkdown(source: string): string {
  const lines = source.replace(/\r\n/g, '\n').split('\n')
  const out: string[] = []
  let inCode = false
  let codeBuf: string[] = []
  let inList: 'ul' | 'ol' | null = null
  let inQuote = false
  let tableBuf: string[][] = []
  let inTable = false

  const closeList = () => {
    if (inList) {
      out.push(`</${inList}>`)
      inList = null
    }
  }
  const closeQuote = () => {
    if (inQuote) {
      out.push('</blockquote>')
      inQuote = false
    }
  }
  // 表格必须在「离开表格的那一行」之前结算：否则表格会被推迟到文末输出。
  const flushTable = () => {
    if (inTable) {
      out.push(renderTable(tableBuf))
      tableBuf = []
      inTable = false
    }
  }

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]

    // 代码块 ```lang（围栏内一律按字面处理）
    if (line.trimStart().startsWith('```')) {
      if (!inCode) {
        closeList()
        closeQuote()
        flushTable()
        inCode = true
        codeBuf = []
      } else {
        inCode = false
        out.push(`<pre><code>${escapeHtml(codeBuf.join('\n'))}</code></pre>`)
      }
      continue
    }
    if (inCode) {
      codeBuf.push(line)
      continue
    }

    const trimmed = line.trim()

    // 分割线
    if (/^(-{3,}|\*{3,}|_{3,})$/.test(trimmed)) {
      closeList()
      closeQuote()
      flushTable()
      out.push('<hr/>')
      continue
    }

    // 表格：仅当「当前行含 |」且「下一行是分隔行」时才开始，
    // 避免把正文里偶然含 | 的句子误判为表格。
    const startsTable = line.includes('|') && isTableSeparator(lines[i + 1] ?? '')
    if (inTable && startsTable) flushTable() // 相邻两张表：先结算上一张
    if (!inTable && startsTable) {
      closeList()
      closeQuote()
      tableBuf = [splitTableRow(line)]
      inTable = true
      continue
    }
    if (inTable) {
      if (line.includes('|')) {
        if (!isTableSeparator(line)) tableBuf.push(splitTableRow(line))
        continue
      }
      flushTable()
    }

    // 标题
    const heading = trimmed.match(/^(#{1,6})\s+(.*)$/)
    if (heading) {
      closeList()
      closeQuote()
      const level = heading[1].length
      out.push(`<h${level}>${renderInline(heading[2])}</h${level}>`)
      continue
    }

    // 引用
    const quote = trimmed.match(/^>\s?(.*)$/)
    if (quote) {
      const content = renderInline(quote[1])
      if (!inQuote) {
        closeList()
        out.push('<blockquote>')
        inQuote = true
      }
      out.push(`<p>${content}</p>`)
      continue
    }
    closeQuote()

    // 无序列表
    const ul = trimmed.match(/^[-*+]\s+(.*)$/)
    if (ul) {
      if (inList !== 'ul') {
        closeList()
        out.push('<ul>')
        inList = 'ul'
      }
      out.push(`<li>${renderInline(ul[1])}</li>`)
      continue
    }

    // 有序列表
    const ol = trimmed.match(/^\d+[.、]\s+(.*)$/)
    if (ol) {
      if (inList !== 'ol') {
        closeList()
        out.push('<ol>')
        inList = 'ol'
      }
      out.push(`<li>${renderInline(ol[1])}</li>`)
      continue
    }

    // 空行
    if (trimmed === '') {
      closeList()
      continue
    }

    closeList()
    out.push(`<p>${renderInline(linkify(trimmed))}</p>`)
  }

  if (inCode) {
    out.push(`<pre><code>${escapeHtml(codeBuf.join('\n'))}</code></pre>`)
  }
  flushTable()
  closeList()
  closeQuote()

  return out.join('\n')
}

function renderTable(rows: string[][]): string {
  if (rows.length === 0) return ''
  const maxCol = Math.max(...rows.map((r) => r.length))
  let html = '<table><thead><tr>'
  for (let c = 0; c < maxCol; c++) {
    html += `<th>${renderInline(rows[0][c] ?? '')}</th>`
  }
  html += '</tr></thead><tbody>'
  for (let r = 1; r < rows.length; r++) {
    html += '<tr>'
    for (let c = 0; c < maxCol; c++) {
      html += `<td>${renderInline(rows[r][c] ?? '')}</td>`
    }
    html += '</tr>'
  }
  html += '</tbody></table>'
  return html
}