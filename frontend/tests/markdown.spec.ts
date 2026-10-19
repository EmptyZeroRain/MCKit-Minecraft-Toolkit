// Markdown 安全渲染测试（需求 11.1：禁用原始 HTML，防 XSS）
import { describe, expect, it } from 'vitest'
import { renderSafeMarkdown } from '@/utils/markdown'

describe('安全 Markdown 渲染', () => {
  it('渲染标题、段落与列表', () => {
    const html = renderSafeMarkdown('# 标题\n\n正文\n\n- 列表项')
    expect(html).toContain('<h1>标题</h1>')
    expect(html).toContain('<p>正文</p>')
    expect(html).toContain('<ul>')
    expect(html).toContain('<li>列表项</li>')
  })

  it('原始 HTML 被转义，不注入脚本', () => {
    const payload = '<script>alert("xss")</script>'
    const html = renderSafeMarkdown(payload)
    expect(html).not.toContain('<script>')
    expect(html).toContain('&lt;script&gt;')
  })

  it('事件属性被转义，不产生真实标签', () => {
    const html = renderSafeMarkdown('<img src=x onerror=alert(1)>')
    expect(html).toContain('&lt;img')
    expect(html).not.toContain('<img src=x')
    // 内容作为纯文本出现，不会成为可解析的属性
    expect(html).toContain('onerror')
    expect(html).not.toMatch(/<img[^>]*onerror/i)
  })

  it('非法协议的链接被忽略', () => {
    const html = renderSafeMarkdown('[点我](javascript:alert(1))')
    expect(html).toContain('非法链接已忽略')
    expect(html).not.toContain('href="javascript:')
  })

  it('合法 https 链接保留且带安全属性', () => {
    const html = renderSafeMarkdown('[官方](https://www.minecraft.net/)')
    expect(html).toContain('href="https://www.minecraft.net/"')
    expect(html).toContain('rel="noopener noreferrer nofollow"')
    expect(html).toContain('target="_blank"')
  })

  it('代码块不被当行内解析', () => {
    const html = renderSafeMarkdown('```\n<div>hi</div>\n```')
    expect(html).toContain('<pre><code>')
    expect(html).toContain('&lt;div&gt;hi&lt;/div&gt;')
  })

  it('行内代码不执行', () => {
    const html = renderSafeMarkdown('`<b onclick>`')
    expect(html).not.toContain('<b onclick>')
    expect(html).toContain('&lt;b onclick&gt;')
  })

  it('表格保留内部空单元格，不因过滤而错位', () => {
    const html = renderSafeMarkdown('| 名称 | 等级 |\n| --- | --- |\n| 保护 |  |')
    expect(html).toContain('<table>')
    expect(html).toContain('<th>名称</th>')
    expect(html).toContain('<td>保护</td>')
    // 内部空单元格应保留为 <td></td>
    expect(html).toMatch(/<td>保护<\/td><td><\/td>/)
  })

  it('表格单元格内的转义竖线不被当作分列', () => {
    const html = renderSafeMarkdown('| 表达式 | 结果 |\n| --- | --- |\n| a\\|b | V |')
    expect(html).toContain('<td>a|b</td>')
    expect(html).toContain('<td>V</td>')
  })

  it('相邻代码块各自独立成块，不粘连（Java / TOML 两段示例）', () => {
    const html = renderSafeMarkdown(
      '```java\npublic class ExampleMod {}\n```\n\n```toml\nmodLoader = "javafml"\n```',
    )
    // 两段各自是完整代码块
    expect(html.match(/<pre><code>/g)).toHaveLength(2)
    expect(html.match(/<\/code><\/pre>/g)).toHaveLength(2)
    expect(html).toContain('public class ExampleMod {}')
    expect(html).toContain('modLoader = &quot;javafml&quot;')
  })

  it('表格在离开表格处结算，不会跑到文末', () => {
    const html = renderSafeMarkdown(
      '## 常用运行任务\n\n| 任务 | 用途 |\n| --- | --- |\n| runClient | 启动客户端 |\n\n## 注意\n\n- 事项一',
    )
    const tableIdx = html.indexOf('<table>')
    const noteIdx = html.indexOf('<h2>注意</h2>')
    expect(tableIdx).toBeGreaterThan(-1)
    expect(noteIdx).toBeGreaterThan(-1)
    // 表格应出现在「注意」标题之前（原位），而不是文末
    expect(tableIdx).toBeLessThan(noteIdx)
  })

  it('文末的列表与引用被正确闭合', () => {
    expect(renderSafeMarkdown('- 一\n- 二')).toContain('</ul>')
    expect(renderSafeMarkdown('1. 一\n2. 二')).toContain('</ol>')
    expect(renderSafeMarkdown('> 引用')).toContain('</blockquote>')
  })

  it('正文中偶然含竖线的句子不会被误判为表格', () => {
    const html = renderSafeMarkdown('命令 A | B 都可执行')
    expect(html).not.toContain('<table>')
    expect(html).toContain('<p>命令 A | B 都可执行</p>')
  })
})