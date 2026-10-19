package handler

import (
	"context"
	"encoding/xml"
	"log/slog"
	"net/http"
	"time"

	"mckit/backend/internal/service/sitemap"
)

// SitemapSource 提供站点地图中的动态详情页路径（便于测试替换；可为 nil）。
type SitemapSource interface {
	Entries(ctx context.Context) ([]sitemap.Entry, error)
}

// SEOHandler 提供 robots.txt 与 sitemap.xml（需求 §9.3）。
// 草稿、管理页、登录页不进入站点地图。
type SEOHandler struct {
	baseURL string
	source  SitemapSource
}

// NewSEOHandler 创建 SEO 处理器；source 可为 nil（仅输出静态页）。
func NewSEOHandler(baseURL string, source SitemapSource) *SEOHandler {
	return &SEOHandler{baseURL: baseURL, source: source}
}

// RobotsTxt 输出 robots.txt。
func (h *SEOHandler) RobotsTxt(w http.ResponseWriter, r *http.Request) {
	body := `User-agent: *
Allow: /
Disallow: /api/
Disallow: /admin/

Sitemap: ` + h.baseURL + `/sitemap.xml
`
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}

// staticPage 静态页路径与其抓取提示。
type staticPage struct {
	path     string
	priority string
	change   string
}

// staticPages 公开静态页清单。
// 注意：/search 为 noindex（结果页含查询参数），不收录；内置工具页有独立落地页，单独列出。
var staticPages = []staticPage{
	{"/", "1.0", "daily"},
	{"/tools", "0.9", "weekly"},
	{"/recipes", "0.9", "weekly"},
	{"/items", "0.8", "weekly"},
	{"/potions", "0.8", "weekly"},
	{"/dev", "0.8", "weekly"},
	{"/mappings", "0.7", "weekly"},
	{"/commands", "0.8", "weekly"},
	{"/command-generator", "0.8", "monthly"},
	{"/enchant-calculator", "0.8", "monthly"},
	{"/coordinate", "0.8", "monthly"},
	{"/resources", "0.9", "weekly"},
	{"/versions", "0.7", "weekly"},
	{"/block-ids", "0.7", "monthly"},
	{"/seed-map", "0.7", "weekly"},
	{"/seed-tools", "0.7", "weekly"},
	{"/region-inspect", "0.6", "monthly"},
	{"/about", "0.5", "monthly"},
	{"/privacy", "0.3", "yearly"},
	{"/copyright", "0.3", "yearly"},
	{"/feedback", "0.5", "monthly"},
}

// sitemapURL 单条 <url> 记录。
type sitemapURL struct {
	Loc        string `xml:"loc"`
	LastMod    string `xml:"lastmod,omitempty"`
	ChangeFreq string `xml:"changefreq,omitempty"`
	Priority   string `xml:"priority,omitempty"`
}

// urlset <urlset> 根节点。
type urlset struct {
	XMLName xml.Name     `xml:"urlset"`
	Xmlns   string       `xml:"xmlns,attr"`
	URLs    []sitemapURL `xml:"url"`
}

// SitemapIndex 输出站点地图：静态页 + 数据库中的资源/百科/工具详情页。
// 动态条目查询失败时降级为仅输出静态页，保证 sitemap 始终可用。
func (h *SEOHandler) SitemapIndex(w http.ResponseWriter, r *http.Request) {
	schema := urlset{Xmlns: "http://www.sitemaps.org/schemas/sitemap/0.9"}

	seen := make(map[string]bool, len(staticPages))
	for _, p := range staticPages {
		seen[p.path] = true
		schema.URLs = append(schema.URLs, sitemapURL{
			Loc:        h.baseURL + p.path,
			ChangeFreq: p.change,
			Priority:   p.priority,
		})
	}

	if h.source != nil {
		entries, err := h.source.Entries(r.Context())
		if err != nil {
			slog.Warn("sitemap 动态条目查询失败，降级为静态页", "err", err)
		} else {
			for _, e := range entries {
				if e.Path == "" || seen[e.Path] {
					continue
				}
				seen[e.Path] = true
				u := sitemapURL{Loc: h.baseURL + e.Path, ChangeFreq: "weekly", Priority: "0.6"}
				if !e.LastMod.IsZero() {
					u.LastMod = e.LastMod.UTC().Format(time.RFC3339)
				}
				schema.URLs = append(schema.URLs, u)
			}
		}
	}

	body, err := xml.MarshalIndent(schema, "", "  ")
	if err != nil {
		http.Error(w, "sitemap 生成失败", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	_, _ = w.Write([]byte(xml.Header))
	_, _ = w.Write(body)
	_, _ = w.Write([]byte("\n"))
}
