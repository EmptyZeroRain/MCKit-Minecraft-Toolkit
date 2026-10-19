package handler

import (
	"context"
	"encoding/xml"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mckit/backend/internal/service/sitemap"
)

// stubSitemapSource 站点地图动态来源测试替身。
type stubSitemapSource struct {
	entries []sitemap.Entry
	err     error
}

func (s stubSitemapSource) Entries(context.Context) ([]sitemap.Entry, error) {
	return s.entries, s.err
}

func parseSitemap(t *testing.T, body string) []string {
	t.Helper()
	var set struct {
		URLs []struct {
			Loc string `xml:"loc"`
		} `xml:"url"`
	}
	if err := xml.Unmarshal([]byte(body), &set); err != nil {
		t.Fatalf("sitemap XML 解析失败: %v\n%s", err, body)
	}
	locs := make([]string, 0, len(set.URLs))
	for _, u := range set.URLs {
		locs = append(locs, u.Loc)
	}
	return locs
}

// 静态页必须包含 5 个内置工具落地页，且不含 noindex 的 /search。
func TestSitemapIndex_StaticPages(t *testing.T) {
	h := NewSEOHandler("https://example.com", nil)
	rec := httptest.NewRecorder()
	h.SitemapIndex(rec, httptest.NewRequest("GET", "/sitemap.xml", nil))

	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "xml") {
		t.Fatalf("Content-Type 应为 xml，实际 %q", ct)
	}
	locs := parseSitemap(t, rec.Body.String())
	joined := strings.Join(locs, " ")

	for _, want := range []string{
		"https://example.com/",
		"https://example.com/seed-map",
		"https://example.com/seed-tools",
		"https://example.com/region-inspect",
		"https://example.com/block-ids",
		"https://example.com/versions",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("静态 sitemap 缺少 %s\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "/search") {
		t.Fatalf("noindex 的 /search 不应进入 sitemap\n%s", joined)
	}
}

// 动态详情页需并入，且与静态页去重。
func TestSitemapIndex_DynamicEntries(t *testing.T) {
	updated := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	src := stubSitemapSource{entries: []sitemap.Entry{
		{Path: "/resources/jei", LastMod: updated},
		{Path: "/wiki/交易"},
		{Path: "/tools/my-tool"},
		{Path: "/tools"},          // 与静态页重复，应去重
		{Path: ""},                // 空路径忽略
	}}
	h := NewSEOHandler("https://example.com", src)
	rec := httptest.NewRecorder()
	h.SitemapIndex(rec, httptest.NewRequest("GET", "/sitemap.xml", nil))

	body := rec.Body.String()
	for _, want := range []string{
		"https://example.com/resources/jei",
		"https://example.com/wiki/交易",
		"https://example.com/tools/my-tool",
		"<lastmod>2026-01-02T03:04:05Z</lastmod>",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("动态 sitemap 缺少 %s\n%s", want, body)
		}
	}

	// /tools 静态页只出现一次（去重生效）。
	if n := strings.Count(body, "<loc>https://example.com/tools</loc>"); n != 1 {
		t.Fatalf("/tools 应去重为 1 条，实际 %d", n)
	}
}

// 动态来源报错时降级，仍返回静态页。
func TestSitemapIndex_SourceErrorFallsBack(t *testing.T) {
	src := stubSitemapSource{err: context.DeadlineExceeded}
	h := NewSEOHandler("https://example.com", src)
	rec := httptest.NewRecorder()
	h.SitemapIndex(rec, httptest.NewRequest("GET", "/sitemap.xml", nil))

	if rec.Code != 200 {
		t.Fatalf("降级时状态码应为 200，实际 %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "https://example.com/seed-map") {
		t.Fatalf("降级后仍应包含静态页\n%s", rec.Body.String())
	}
}
