package mcsrc

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// 分页默认与上限。
const (
	defaultPageSize = 50
	maxPageSize     = 200
)

// ClassEntry 索引中的一个类。
type ClassEntry struct {
	// Name 全限定名，如 net.minecraft.server.MinecraftServer。
	Name string `json:"name"`
	// Simple 简单名，如 MinecraftServer。
	Simple string `json:"simple"`
	// Pkg 包名，如 net.minecraft.server。
	Pkg string `json:"pkg"`
	// Inner 是否为内部类（简单名含 $）。
	Inner bool `json:"inner"`
	// Size class 文件未压缩字节数。
	Size int64 `json:"size"`
}

// Index 单个 jar 的类索引。
type Index struct {
	Version   string       `json:"version"`
	Side      string       `json:"side"`
	SHA1      string       `json:"sha1"`
	BuiltAt   time.Time    `json:"built_at"`
	Truncated bool         `json:"truncated"`
	Classes   []ClassEntry `json:"classes"`
}

// SearchQuery 类检索条件。
type SearchQuery struct {
	Q            string
	Pkg          string
	IncludeInner bool
	Page         int
	PageSize     int
}

// SearchResult 分页检索结果。
type SearchResult struct {
	Items    []ClassEntry `json:"items"`
	Total    int          `json:"total"`
	Page     int          `json:"page"`
	PageSize int          `json:"page_size"`
}

// BuildIndex 读取 jar 的条目清单并生成类索引。
// 只读取 zip 中央目录（不解析 class 字节码），因此对数百 MB 的 jar 也很快。
func BuildIndex(ctx context.Context, jarPath, version, side, sha1sum string, maxClasses int) (*Index, error) {
	if maxClasses <= 0 {
		maxClasses = 60000
	}
	zr, err := zip.OpenReader(jarPath)
	if err != nil {
		return nil, fmt.Errorf("打开 jar 失败: %w", err)
	}
	defer func() { _ = zr.Close() }()

	idx := &Index{
		Version: version,
		Side:    side,
		SHA1:    sha1sum,
		BuiltAt: time.Now().UTC(),
		Classes: make([]ClassEntry, 0, 4096),
	}

	for _, f := range zr.File {
		// 每若干条目检查一次取消，避免超大 jar 长时间不可中断。
		if len(idx.Classes)%2048 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		name := f.Name
		if !strings.HasSuffix(strings.ToLower(name), ".class") {
			continue
		}
		// 跳过签名与模块描述符。
		if strings.HasPrefix(name, "META-INF/") || strings.HasSuffix(name, "module-info.class") {
			continue
		}
		full := strings.TrimSuffix(name, ".class")
		full = strings.ReplaceAll(full, "/", ".")
		if full == "" {
			continue
		}
		pkg, simple := splitClassName(full)
		idx.Classes = append(idx.Classes, ClassEntry{
			Name:   full,
			Simple: simple,
			Pkg:    pkg,
			Inner:  strings.Contains(simple, "$"),
			Size:   int64(f.UncompressedSize64),
		})
		if len(idx.Classes) >= maxClasses {
			idx.Truncated = true
			break
		}
	}

	sort.Slice(idx.Classes, func(i, j int) bool { return idx.Classes[i].Name < idx.Classes[j].Name })
	return idx, nil
}

// splitClassName 拆分全限定名为包名与简单名。
func splitClassName(full string) (pkg, simple string) {
	if i := strings.LastIndex(full, "."); i >= 0 {
		return full[:i], full[i+1:]
	}
	return "", full
}

// Search 在索引中检索类。
// 匹配规则（大小写不敏感）：简单名相等 > 简单名前缀 > 简单名包含 > 全限定名包含。
func (idx *Index) Search(q SearchQuery) SearchResult {
	page := q.Page
	if page < 1 {
		page = 1
	}
	pageSize := q.PageSize
	if pageSize < 1 || pageSize > maxPageSize {
		pageSize = defaultPageSize
	}

	keyword := strings.ToLower(strings.TrimSpace(q.Q))
	pkg := strings.TrimSpace(q.Pkg)

	type scored struct {
		entry ClassEntry
		rank  int
	}
	matches := make([]scored, 0, 64)
	for _, c := range idx.Classes {
		if c.Inner && !q.IncludeInner {
			continue
		}
		if pkg != "" && c.Pkg != pkg && !strings.HasPrefix(c.Pkg, pkg+".") {
			continue
		}
		if keyword == "" {
			matches = append(matches, scored{c, 3})
			continue
		}
		simple := strings.ToLower(c.Simple)
		full := strings.ToLower(c.Name)
		switch {
		case simple == keyword:
			matches = append(matches, scored{c, 0})
		case strings.HasPrefix(simple, keyword):
			matches = append(matches, scored{c, 1})
		case strings.Contains(simple, keyword):
			matches = append(matches, scored{c, 2})
		case strings.Contains(full, keyword):
			matches = append(matches, scored{c, 3})
		}
	}

	// 相关度优先，其次按全限定名升序（稳定、可复现）。
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].rank != matches[j].rank {
			return matches[i].rank < matches[j].rank
		}
		return matches[i].entry.Name < matches[j].entry.Name
	})

	total := len(matches)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	items := make([]ClassEntry, 0, end-start)
	for _, m := range matches[start:end] {
		items = append(items, m.entry)
	}

	return SearchResult{Items: items, Total: total, Page: page, PageSize: pageSize}
}

// ErrEmptyIndex 索引中没有可用类（多为异常 jar）。
var ErrEmptyIndex = errors.New("索引为空")
