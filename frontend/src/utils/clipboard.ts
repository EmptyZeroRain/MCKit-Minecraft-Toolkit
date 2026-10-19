// ===== 剪贴板复制：Clipboard API + 降级方案（需求 9.2）=====

/**
 * 复制文本到剪贴板。
 * 优先使用 Clipboard API；不可用时使用隐藏 textarea + execCommand 降级。
 * 返回成功与否，供组件层做轻提示。
 */
export async function copyToClipboard(text: string): Promise<boolean> {
  if (!text) return false

  // 主流现代浏览器路径
  if (navigator.clipboard && window.isSecureContext) {
    try {
      await navigator.clipboard.writeText(text)
      return true
    } catch {
      // 继续尝试降级方案
    }
  }

  // 降级：textarea + execCommand（兼容 http 本地开发与非安全上下文）
  try {
    const textarea = document.createElement('textarea')
    textarea.value = text
    textarea.setAttribute('readonly', '')
    textarea.style.position = 'fixed'
    textarea.style.top = '-9999px'
    textarea.style.opacity = '0'
    document.body.appendChild(textarea)
    textarea.select()
    textarea.setSelectionRange(0, text.length)
    const ok = document.execCommand('copy')
    document.body.removeChild(textarea)
    return ok
  } catch {
    return false
  }
}