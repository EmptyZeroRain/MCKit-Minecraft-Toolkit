// Package wikicrawl 从中文 Minecraft Wiki 抓取公开条目内容并转换为 Markdown。
//
// 合规约束（必须遵守）：
//   - 内容授权：CC BY-NC-SA 3.0（署名-非商业-相同方式共享）
//   - 遵守 robots.txt：仅抓取 /w/ 普通条目页；不抓 File/Special/User/Draft/Bucket
//     等被禁止的命名空间，也不使用被 robots.txt 禁止的 api.php / rest_v1。
//   - 限速：请求间隔可配置（默认 1 秒），失败重试带线性退避，避免给站点压力。
//   - 标注来源：每条记录保存原始 URL 与许可信息。
//
// 抓取内容仅用于本站非商业展示，并保留来源署名。
package wikicrawl

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// Source 数据来源标识。
const (
	SourceName    = "Minecraft Wiki（中文）"
	SourceLicense = "CC BY-NC-SA 3.0"
	SourceURL     = "https://zh.minecraft.wiki"
)

// defaultUserAgent 抓取器标识（便于站点管理员识别与联系）。
const defaultUserAgent = "mckit-bot/0.1 (Minecraft Wiki content indexer; CC BY-NC-SA 3.0)"

// maxBodyBytes 单页最大读取字节数，避免异常页面拖垮内存。
const maxBodyBytes = 8 << 20

// ErrDisallowed 页面被 robots.txt 或抓取规则禁止。
var ErrDisallowed = fmt.Errorf("该页面不允许抓取")

// disallowedNamespaces 禁止抓取的命名空间前缀（与 robots.txt 保持一致，并补充非正文命名空间）。
var disallowedNamespaces = []string{
	"File:", "Special:", "User:", "Draft:", "Bucket:",
	"Template:", "Module:", "MediaWiki:", "Category:", "Help:", "Forum:", "Talk:",
}

// Crawler Wiki 抓取器（非并发安全，单实例顺序使用）。
type Crawler struct {
	client    *http.Client
	baseURL   string
	interval  time.Duration
	retries   int
	lastReq   time.Time
	userAgent string
}

// NewCrawler 创建抓取器。interval 为请求间隔（小于 1s 会被提升为 1s）。
func NewCrawler(interval time.Duration) *Crawler {
	if interval < time.Second {
		interval = time.Second
	}
	return &Crawler{
		client: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(_ *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return fmt.Errorf("重定向次数过多")
				}
				return nil
			},
		},
		baseURL:   SourceURL,
		interval:  interval,
		retries:   3,
		userAgent: defaultUserAgent,
	}
}

// SetUserAgent 覆盖默认 User-Agent（如需要附带联系邮箱）。
func (c *Crawler) SetUserAgent(ua string) {
	if strings.TrimSpace(ua) != "" {
		c.userAgent = ua
	}
}

// Article 抓取并转换后的条目。
type Article struct {
	Slug       string `json:"slug"`        // 由标题生成的稳定 slug
	Title      string `json:"title"`       // 页面真实标题（重定向后）
	Summary    string `json:"summary"`     // 首个段落摘要
	Content    string `json:"content"`     // Markdown 正文
	URL        string `json:"url"`         // 最终（重定向后）页面 URL
	Source     string `json:"source"`      // 来源名称
	License    string `json:"license"`     // 许可协议
	Requested  string `json:"requested"`   // 请求时的原始标题
	Redirected bool   `json:"redirected"`  // 是否发生了重定向
}

// Fetch 抓取指定标题的条目（标题为 wiki 页面名，如 "村庄"、"附魔"）。
// 会自动处理重定向，并返回重定向后的真实标题与 URL。
func (c *Crawler) Fetch(ctx context.Context, title string) (*Article, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, fmt.Errorf("标题不能为空")
	}
	if isDisallowed(title) {
		return nil, ErrDisallowed
	}

	pageURL := c.baseURL + "/w/" + strings.ReplaceAll(title, " ", "_")

	var lastErr error
	for attempt := 0; attempt <= c.retries; attempt++ {
		if attempt > 0 {
			// 线性退避：1×interval、2×interval …
			if err := sleepCtx(ctx, time.Duration(attempt)*c.interval); err != nil {
				return nil, err
			}
		}
		if err := c.wait(ctx); err != nil {
			return nil, err
		}

		art, retryable, err := c.fetchOnce(ctx, pageURL, title)
		if err == nil {
			return art, nil
		}
		lastErr = err
		if !retryable {
			return nil, err
		}
	}
	return nil, fmt.Errorf("重试 %d 次后仍失败: %w", c.retries, lastErr)
}

// fetchOnce 执行一次抓取；retryable 表示失败是否可重试（网络错误 / 5xx / 429）。
func (c *Crawler) fetchOnce(ctx context.Context, pageURL, requested string) (*Article, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")

	resp, err := c.client.Do(req)
	c.lastReq = time.Now()
	if err != nil {
		return nil, true, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		retryable := resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests
		return nil, retryable, fmt.Errorf("页面返回状态 %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, true, fmt.Errorf("读取响应失败: %w", err)
	}

	finalURL := pageURL
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}

	art := extractArticle(string(body), requested, finalURL)
	art.Redirected = pageTitleFromURL(finalURL) != requested
	return art, false, nil
}

// wait 执行请求间隔限速。
func (c *Crawler) wait(ctx context.Context) error {
	if c.lastReq.IsZero() {
		return nil
	}
	if wait := c.interval - time.Since(c.lastReq); wait > 0 {
		return sleepCtx(ctx, wait)
	}
	return nil
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func isDisallowed(title string) bool {
	for _, ns := range disallowedNamespaces {
		if strings.HasPrefix(title, ns) {
			return true
		}
	}
	return false
}

// pageTitleFromURL 从 /w/xxx 形式 URL 还原页面标题（用于判断重定向）。
func pageTitleFromURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	p := strings.TrimPrefix(u.Path, "/w/")
	if p == "" {
		return ""
	}
	if decoded, err := url.PathUnescape(p); err == nil {
		p = decoded
	}
	return strings.ReplaceAll(p, "_", " ")
}

// ---- HTML → Markdown ----

// extractArticle 从整页 HTML 中提取标题、摘要与 Markdown 正文。
func extractArticle(raw, requested, finalURL string) *Article {
	doc, err := html.Parse(strings.NewReader(raw))
	if err != nil {
		return &Article{
			Slug:      slugifyTitle(requested),
			Title:     requested,
			Content:   "",
			URL:       finalURL,
			Source:    SourceName,
			License:   SourceLicense,
			Requested: requested,
		}
	}

	title := extractTitle(doc)
	if title == "" {
		title = pageTitleFromURL(finalURL)
	}
	if title == "" {
		title = requested
	}

	content := ""
	summary := ""
	if node := findContent(doc); node != nil {
		r := &renderer{}
		content = cleanMarkdown(r.renderChildren(node))
		summary = firstParagraphText(node)
	}
	if summary == "" {
		summary = summarize(content)
	}

	return &Article{
		Slug:      slugifyTitle(title),
		Title:     title,
		Summary:   summary,
		Content:   content,
		URL:       finalURL,
		Source:    SourceName,
		License:   SourceLicense,
		Requested: requested,
	}
}

// findContent 定位正文容器：优先 #mw-content-text 下的 .mw-parser-output。
func findContent(doc *html.Node) *html.Node {
	var contentText *html.Node
	walk(doc, func(n *html.Node) bool {
		if n.Type == html.ElementNode && attr(n, "id") == "mw-content-text" {
			contentText = n
			return false
		}
		return true
	})

	search := doc
	if contentText != nil {
		search = contentText
	}
	var parserOutput *html.Node
	walk(search, func(n *html.Node) bool {
		if n.Type == html.ElementNode && hasClass(n, "mw-parser-output") {
			parserOutput = n
			return false
		}
		return true
	})
	if parserOutput != nil {
		return parserOutput
	}
	return contentText
}

// extractTitle 提取页面标题：优先 <h1 id="firstHeading">，回退 <title>。
func extractTitle(doc *html.Node) string {
	var title string
	walk(doc, func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Data == "h1" && attr(n, "id") == "firstHeading" {
			title = strings.TrimSpace(nodeText(n))
			return false
		}
		return true
	})
	if title != "" {
		return title
	}
	walk(doc, func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Data == "title" {
			title = strings.TrimSpace(nodeText(n))
			return false
		}
		return true
	})
	for _, suffix := range []string{" - 中文 Minecraft Wiki", " - Minecraft Wiki"} {
		title = strings.TrimSuffix(title, suffix)
	}
	return strings.TrimSpace(title)
}

// renderer 将 DOM 子树渲染为 Markdown。
type renderer struct{}

// skipTags 完全不渲染的标签。
var skipTags = map[string]bool{
	"script": true, "style": true, "noscript": true, "link": true, "meta": true,
	"iframe": true, "svg": true, "audio": true, "video": true, "map": true,
	"button": true, "input": true, "select": true, "textarea": true,
	"figure": true, "figcaption": true, "nav": true, "aside": true,
}

// skipClassSet 需要跳过的 class token（编辑链接、目录、导航框、引用、提示框等噪音）。
var skipClassSet = map[string]bool{
	"mw-editsection": true, "mw-editsection-bracket": true, "mw-editsection-divider": true,
	"navbox": true, "navbox-styles": true, "toc": true, "toctitle": true,
	"hatnote": true, "metadata": true, "noprint": true, "mw-empty-elt": true,
	"mw-references-wrap": true, "references": true, "reference": true, "reference-text": true,
	"thumb": true, "gallery": true, "mw-collapsible-toggle": true, "vector-toc": true,
	"shortdescription": true, "mw-indicators": true, "printfooter": true, "catlinks": true,
	"mw-jump-link": true, "navigation-not-searchable": true, "side-box": true, "portal": true,
	"mw-cite-backlink": true, "error": true, "mw-message-box": true, "sitenotice": true,
	// 维护/提示模板（ambox/mbox 系列）不应进入正文与摘要
	"ambox": true, "ambox-content": true, "mbox": true, "messagebox": true, "msgbox": true,
	"notice": true, "notice-metadata": true, "article-message-box": true, "cmbox": true,
	"tmbox": true, "ombox": true, "imbox": true, "dmbox": true, "fmbox": true,
	"cleanup": true, "stub": true, "expand": true, "update": true, "disambig": true,
}

// shouldSkip 判断节点是否属于正文噪音（编辑链接、目录、导航框、图片说明、提示模板等）。
func shouldSkip(n *html.Node) bool {
	if skipTags[n.Data] {
		return true
	}
	for _, c := range classList(n) {
		if skipClassSet[c] {
			return true
		}
	}
	return isHidden(n)
}

// renderChildren 渲染节点的全部子节点。
func (r *renderer) renderChildren(n *html.Node) string {
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		b.WriteString(r.renderNode(c))
	}
	return b.String()
}

// renderNode 渲染单个节点。
func (r *renderer) renderNode(n *html.Node) string {
	switch n.Type {
	case html.TextNode:
		return normalizeSpace(n.Data)
	case html.CommentNode, html.DoctypeNode:
		return ""
	case html.ElementNode:
		// 继续处理
	default:
		return ""
	}

	if shouldSkip(n) {
		return ""
	}

	switch n.Data {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		return r.renderHeading(n)
	case "p":
		txt := strings.TrimSpace(r.renderChildren(n))
		if txt == "" {
			return ""
		}
		return "\n\n" + txt + "\n\n"
	case "br":
		return "\n"
	case "hr":
		return "\n\n---\n\n"
	case "b", "strong":
		txt := strings.TrimSpace(r.renderChildren(n))
		if txt == "" {
			return ""
		}
		return "**" + txt + "**"
	case "i", "em":
		txt := strings.TrimSpace(r.renderChildren(n))
		if txt == "" {
			return ""
		}
		return "*" + txt + "*"
	case "code":
		txt := nodeText(n)
		if strings.Contains(txt, "\n") {
			return "\n\n```\n" + txt + "\n```\n\n"
		}
		return "`" + txt + "`"
	case "pre":
		txt := strings.TrimRight(nodeText(n), "\n")
		if txt == "" {
			return ""
		}
		return "\n\n```\n" + txt + "\n```\n\n"
	case "a":
		return r.renderLink(n)
	case "img":
		return "" // 不渲染图片，仅保留文字（图片版权与热链合规）
	case "ul", "ol":
		return r.renderList(n, n.Data == "ol")
	case "li", "dt", "dd":
		return r.renderChildren(n)
	case "table":
		return r.renderTable(n)
	case "blockquote":
		txt := strings.TrimSpace(r.renderChildren(n))
		if txt == "" {
			return ""
		}
		lines := strings.Split(txt, "\n")
		for i := range lines {
			lines[i] = "> " + strings.TrimSpace(lines[i])
		}
		return "\n\n" + strings.Join(lines, "\n") + "\n\n"
	default:
		// div / span / section / tbody / tr / td … 递归处理
		return r.renderChildren(n)
	}
}

func (r *renderer) renderHeading(n *html.Node) string {
	txt := strings.TrimSpace(r.renderChildren(n))
	if txt == "" {
		return ""
	}
	level := int(n.Data[1] - '0')
	if level < 2 {
		level = 2 // 正文中不使用 h1（页面标题已单独展示）
	}
	return "\n\n" + strings.Repeat("#", level) + " " + txt + "\n\n"
}

func (r *renderer) renderLink(n *html.Node) string {
	text := strings.TrimSpace(r.renderChildren(n))
	if text == "" {
		return ""
	}
	href := attr(n, "href")
	if href == "" || strings.HasPrefix(href, "#") || hasClass(n, "new") || isEditLink(href) {
		return text
	}
	// 仅保留指向 wiki 条目的内部链接；外链与特殊命名空间降级为纯文本。
	if !strings.HasPrefix(href, "/w/") || isDisallowed(pageTitleFromURL(href)) {
		return text
	}
	abs := SourceURL + href
	return "[" + text + "](" + abs + ")"
}

func (r *renderer) renderList(n *html.Node, ordered bool) string {
	var b strings.Builder
	b.WriteString("\n\n")
	idx := 1
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode || c.Data != "li" {
			continue
		}
		txt := flatten(r.renderChildren(c))
		if txt == "" {
			continue
		}
		if ordered {
			b.WriteString(fmt.Sprintf("%d. %s\n", idx, txt))
			idx++
		} else {
			b.WriteString("- " + txt + "\n")
		}
	}
	b.WriteString("\n")
	return b.String()
}

func (r *renderer) renderTable(n *html.Node) string {
	var rows [][]string
	var collect func(*html.Node)
	collect = func(node *html.Node) {
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			if c.Type != html.ElementNode {
				continue
			}
			switch c.Data {
			case "tr":
				cells := r.collectCells(c)
				if len(cells) > 0 {
					rows = append(rows, cells)
				}
			case "table":
				// 跳过嵌套表格，避免结构错乱
			case "caption", "colgroup", "col":
				// 忽略
			default:
				collect(c)
			}
		}
	}
	collect(n)
	if len(rows) == 0 {
		return ""
	}

	maxCol := 0
	for _, row := range rows {
		if len(row) > maxCol {
			maxCol = len(row)
		}
	}

	var b strings.Builder
	b.WriteString("\n\n")
	for i, row := range rows {
		b.WriteString("|")
		for c := 0; c < maxCol; c++ {
			cell := ""
			if c < len(row) {
				cell = row[c]
			}
			b.WriteString(" " + cell + " |")
		}
		b.WriteString("\n")
		if i == 0 {
			b.WriteString("|")
			for c := 0; c < maxCol; c++ {
				b.WriteString(" --- |")
			}
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	return b.String()
}

func (r *renderer) collectCells(tr *html.Node) []string {
	var cells []string
	for c := tr.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode {
			continue
		}
		if c.Data != "td" && c.Data != "th" {
			continue
		}
		txt := flatten(r.renderChildren(c))
		txt = strings.ReplaceAll(txt, "|", "\\|")
		cells = append(cells, txt)
	}
	return cells
}

// firstParagraphText 返回正文中第一个有效段落（跳过提示框、图片说明、引用等噪音）。
func firstParagraphText(root *html.Node) string {
	r := &renderer{}
	var found string
	var rec func(n *html.Node) bool
	rec = func(n *html.Node) bool {
		if n.Type == html.ElementNode {
			if shouldSkip(n) {
				return true // 跳过整个子树
			}
			if n.Data == "p" {
				txt := stripInlineMarkdown(flatten(r.renderChildren(n)))
				if len([]rune(txt)) >= 15 {
					found = txt
					return false
				}
				return true
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if !rec(c) {
				return false
			}
		}
		return true
	}
	rec(root)
	return found
}

// ---- DOM / 文本辅助 ----

func walk(n *html.Node, fn func(*html.Node) bool) bool {
	if !fn(n) {
		return false
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if !walk(c, fn) {
			return false
		}
	}
	return true
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func classList(n *html.Node) []string {
	c := attr(n, "class")
	if c == "" {
		return nil
	}
	return strings.Fields(c)
}

func hasClass(n *html.Node, class string) bool {
	for _, c := range classList(n) {
		if c == class {
			return true
		}
	}
	return false
}

func isHidden(n *html.Node) bool {
	style := strings.ReplaceAll(attr(n, "style"), " ", "")
	if strings.Contains(style, "display:none") || strings.Contains(style, "visibility:hidden") {
		return true
	}
	return attr(n, "aria-hidden") == "true" && n.Data == "div"
}

func isEditLink(href string) bool {
	return strings.Contains(href, "action=edit") ||
		strings.Contains(href, "veaction=") ||
		strings.Contains(href, "redlink=1") ||
		strings.Contains(href, "&action=")
}

// nodeText 拼接子树中的全部文本（保留原始空白，用于 h1/title/pre/code）。
func nodeText(n *html.Node) string {
	var b strings.Builder
	walk(n, func(node *html.Node) bool {
		if node.Type == html.TextNode {
			b.WriteString(node.Data)
		}
		return true
	})
	return b.String()
}

var wsRe = regexp.MustCompile(`\s+`)

func normalizeSpace(s string) string {
	return wsRe.ReplaceAllString(s, " ")
}

// flatten 将多行文本压平为单行（用于列表项与表格单元格）。
func flatten(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// cleanMarkdown 规整最终 Markdown：压缩空行、清理行尾空白与残留标记。
func cleanMarkdown(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimRight(line, " \t")
		out = append(out, line)
	}
	s = strings.Join(out, "\n")
	s = blankLineRe.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

var blankLineRe = regexp.MustCompile(`\n{3,}`)

var (
	inlineCodeRe = regexp.MustCompile("`[^`]*`")
	inlineLinkRe = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	boldRe       = regexp.MustCompile(`\*{1,2}([^*]+)\*{1,2}`)
)

// summarize 从 Markdown 中取首个有效段落作为摘要。
func summarize(md string) string {
	for _, line := range strings.Split(md, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		switch line[0] {
		case '#', '-', '*', '|', '>', '`':
			continue
		}
		line = stripInlineMarkdown(line)
		if len([]rune(line)) < 10 {
			continue
		}
		if r := []rune(line); len(r) > 200 {
			return string(r[:200]) + "…"
		}
		return line
	}
	return ""
}

func stripInlineMarkdown(s string) string {
	s = inlineLinkRe.ReplaceAllString(s, "$1")
	s = inlineCodeRe.ReplaceAllString(s, "")
	s = boldRe.ReplaceAllString(s, "$1")
	return strings.TrimSpace(s)
}

// slugifyTitle 由标题生成 slug：保留 ASCII 字母数字与中日韩汉字，其余转为连字符。
func slugifyTitle(title string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(title)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r >= 0x4e00 && r <= 0x9fff, r >= 0x3400 && r <= 0x4dbf: // CJK 汉字
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
