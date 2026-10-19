// Package search 提供全站搜索（需求 §5.1）。
// 搜索范围：工具、资源、百科、物品、开发文档。标题优先，其次别名、摘要和正文。
package search

import (
	"context"
	"sort"
	"strings"

	"gorm.io/gorm"

	"mckit/backend/internal/domain"
	"mckit/backend/internal/versioning"
)

// versionFilter 构造版本筛选条件（列名 + 参数）。
//
// 语义与资源列表一致：按声明文本匹配；声明「所有版本」（all）的内容
// 对任何具体版本筛选都应命中。column 由调用方给出（内部常量，不接受外部输入）。
func versionFilter(column, gameVersion string) (string, []any) {
	allPattern := "%\"" + versioning.AllKeyword + "\"%"
	if gameVersion == versioning.AllKeyword {
		return column + "::text LIKE ?", []any{allPattern}
	}
	return "(" + column + "::text LIKE ? OR " + column + "::text LIKE ?)",
		[]any{"%" + gameVersion + "%", allPattern}
}

// ResultItem 单个搜索结果。
type ResultItem struct {
	Type    string `json:"type"` // tool | resource | wiki | item | devdoc
	ID      string `json:"id"`
	Slug    string `json:"slug,omitempty"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
	ItemID  string `json:"item_id,omitempty"`
	// URL 前端详情页路径，供搜索结果直接跳转（公开页；物品详情页 TODO §1）。
	URL       string `json:"url"`
	UpdatedAt string `json:"updated_at"`

	// score 为内部相关度，不参与 JSON 序列化。
	score int `json:"-"`
}

// Service 搜索服务。
type Service struct {
	db *gorm.DB
}

// NewService 创建搜索服务。
func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// Filter 搜索筛选。
type Filter struct {
	Q           string
	Type        string // 可选：tool | resource | wiki | item | devdoc
	GameVersion string
	// Sort 排序方式："" / "relevance"（默认，相关度优先）或 "updated_desc"（最近更新优先）。
	Sort     string
	Page     int
	PageSize int
}

// MaxQueryLength 搜索关键词长度上限（以字符计）。
// 超长输入在处理器层被拒绝，避免超大 ILIKE / tsquery 拖垮数据库（TODO §5 输入防护）。
const MaxQueryLength = 80

// Result 搜索结果。
type Result struct {
	Items    []ResultItem `json:"items"`
	Total    int64        `json:"total"`
	Page     int          `json:"page"`
	PageSize int          `json:"page_size"`
}

// maxFetchPerType 单类型最大预取行数，避免大页码导致无界查询。
const maxFetchPerType = 200

// Search 执行跨类型搜索。
// 空关键词直接返回空结果，不执行全库模糊扫描（需求 §5.1）。
// 结果先按相关度排序，再按分页切片，保证 total 与分页一致。
func (s *Service) Search(ctx context.Context, f Filter) (*Result, error) {
	page, pageSize := normalizePage(f.Page, f.PageSize)
	out := &Result{Items: []ResultItem{}, Page: page, PageSize: pageSize}

	q := strings.TrimSpace(f.Q)
	if q == "" {
		return out, nil
	}

	// 预取量随页码增长，但设上限防止滥用。
	fetch := page * pageSize
	if fetch > maxFetchPerType {
		fetch = maxFetchPerType
	}
	if fetch < pageSize {
		fetch = pageSize
	}

	var merged []ResultItem
	var total int64

	if f.Type == "" || f.Type == "tool" {
		items, n, err := s.searchTool(ctx, q, f.GameVersion, fetch)
		if err != nil {
			return nil, err
		}
		merged = append(merged, items...)
		total += n
	}
	if f.Type == "" || f.Type == "resource" {
		items, n, err := s.searchResource(ctx, q, f.GameVersion, fetch)
		if err != nil {
			return nil, err
		}
		merged = append(merged, items...)
		total += n
	}
	if f.Type == "" || f.Type == "wiki" {
		items, n, err := s.searchWiki(ctx, q, f.GameVersion, fetch)
		if err != nil {
			return nil, err
		}
		merged = append(merged, items...)
		total += n
	}
	if f.Type == "" || f.Type == "item" {
		items, n, err := s.searchItem(ctx, q, f.GameVersion, fetch)
		if err != nil {
			return nil, err
		}
		merged = append(merged, items...)
		total += n
	}
	if f.Type == "" || f.Type == "devdoc" {
		items, n, err := s.searchDevDoc(ctx, q, f.GameVersion, fetch)
		if err != nil {
			return nil, err
		}
		merged = append(merged, items...)
		total += n
	}

	// 排序：默认相关度降序；updated_desc 按更新时间降序。两者都以标题升序兜底，保证稳定。
	switch f.Sort {
	case "updated_desc":
		sort.SliceStable(merged, func(i, j int) bool {
			if merged[i].UpdatedAt != merged[j].UpdatedAt {
				return merged[i].UpdatedAt > merged[j].UpdatedAt
			}
			return merged[i].Title < merged[j].Title
		})
	default:
		rankItems(merged)
	}

	out.Total = total
	out.Items = pageSlice(merged, page, pageSize)
	return out, nil
}

// rankItems 按相关度降序、标题升序排序（稳定排序，同分保持类型内原顺序）。
func rankItems(items []ResultItem) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].score != items[j].score {
			return items[i].score > items[j].score
		}
		return items[i].Title < items[j].Title
	})
}

// pageSlice 对已排序结果做分页切片；越界返回空切片（而非 nil），保证 JSON 为 []。
func pageSlice(items []ResultItem, page, pageSize int) []ResultItem {
	start := (page - 1) * pageSize
	if start >= len(items) {
		return []ResultItem{}
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}

// rankInput 相关度计算的输入字段。
// Identifier 为命名空间 ID（物品）或空；Aliases 为别名 / 标签。
type rankInput struct {
	Title      string
	Slug       string
	Identifier string
	Aliases    []string
	Summary    string
}

// relevance 计算相关度（TODO §5「优化中文名、英文名、命名空间 ID 和别名的搜索权重」）。
//
// 权重梯度（高 → 低）：
//   - 标题精确 100 / 前缀 80 / 包含 60
//   - 命名空间 ID 精确 95 / 包含 70（用户复制粘贴 ID 是很强的意图信号）
//   - 别名精确 75 / 包含 55
//   - slug 包含 45
//   - 摘要包含 20
//   - 其它（命中全文索引但字段不含关键词，如正文）5
func relevance(q string, in rankInput) int {
	lq := strings.ToLower(strings.TrimSpace(q))
	if lq == "" {
		return 0
	}
	lt := strings.ToLower(in.Title)
	switch {
	case lt == lq:
		return 100
	case strings.HasPrefix(lt, lq):
		return 80
	case strings.Contains(lt, lq):
		return 60
	}

	if id := strings.ToLower(in.Identifier); id != "" {
		switch {
		case id == lq:
			return 95
		case strings.Contains(id, lq):
			return 70
		}
	}

	for _, a := range in.Aliases {
		la := strings.ToLower(strings.TrimSpace(a))
		if la == "" {
			continue
		}
		if la == lq {
			return 75
		}
	}
	for _, a := range in.Aliases {
		if strings.Contains(strings.ToLower(a), lq) {
			return 55
		}
	}

	if in.Slug != "" && strings.Contains(strings.ToLower(in.Slug), lq) {
		return 45
	}
	if strings.Contains(strings.ToLower(in.Summary), lq) {
		return 20
	}
	return 5
}

func keywordPattern(q string) string {
	return "%" + q + "%"
}

// fullTextMatch 组合 tsvector 全文匹配与 ILIKE 兜底条件（需求 §5.1 / §7.11）。
//
// tsvector（search_vector 生成列 + GIN 索引）命中英文标识/空格分词的关键词；
// PostgreSQL 默认分词器对连续中文按整段切分，故保留 ilike 子句兜底中文子串与别名。
// 返回 SQL 片段；参数按 (q, <ilike 参数...>) 顺序传入。
func fullTextMatch(ilike string) string {
	return "search_vector @@ plainto_tsquery('simple', ?) OR (" + ilike + ")"
}

func (s *Service) searchTool(ctx context.Context, q, gameVersion string, limit int) ([]ResultItem, int64, error) {
	kw := keywordPattern(q)
	base := s.db.WithContext(ctx).Model(&domain.Tool{}).
		Where("status = ?", domain.StatusPublished).
		Where(fullTextMatch("name ILIKE ? OR slug ILIKE ? OR summary ILIKE ?"), q, kw, kw, kw)
	if gameVersion != "" {
		cond, args := versionFilter("supported_versions", gameVersion)
		base = base.Where(cond, args...)
	}

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var items []domain.Tool
	if err := base.Order("name ASC").Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	out := make([]ResultItem, 0, len(items))
	for _, it := range items {
		out = append(out, ResultItem{
			Type:      "tool",
			ID:        it.ID.String(),
			Slug:      it.Slug,
			Title:     it.Name,
			Summary:   it.Summary,
			URL:       "/tools/" + it.Slug,
			UpdatedAt: it.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
			score: relevance(q, rankInput{
				Title: it.Name, Slug: it.Slug, Aliases: it.Tags, Summary: it.Summary,
			}),
		})
	}
	return out, total, nil
}

func (s *Service) searchResource(ctx context.Context, q, gameVersion string, limit int) ([]ResultItem, int64, error) {
	kw := keywordPattern(q)
	base := s.db.WithContext(ctx).Model(&domain.Resource{}).
		Where("status = ? AND deleted_at IS NULL", domain.StatusPublished).
		Where(fullTextMatch("name ILIKE ? OR slug ILIKE ? OR summary ILIKE ? OR tags::text ILIKE ? OR description ILIKE ?"),
			q, kw, kw, kw, kw, kw)
	if gameVersion != "" {
		cond, args := versionFilter("supported_versions", gameVersion)
		base = base.Where(cond, args...)
	}

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var items []domain.Resource
	if err := base.Order("name ASC").Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	out := make([]ResultItem, 0, len(items))
	for _, it := range items {
		out = append(out, ResultItem{
			Type:      "resource",
			ID:        it.ID.String(),
			Slug:      it.Slug,
			Title:     it.Name,
			Summary:   it.Summary,
			URL:       "/resources/" + it.Slug,
			UpdatedAt: it.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
			score: relevance(q, rankInput{
				Title: it.Name, Slug: it.Slug, Aliases: it.Tags, Summary: it.Summary,
			}),
		})
	}
	return out, total, nil
}

func (s *Service) searchWiki(ctx context.Context, q, gameVersion string, limit int) ([]ResultItem, int64, error) {
	kw := keywordPattern(q)
	base := s.db.WithContext(ctx).Model(&domain.WikiEntry{}).
		Where("status = ? AND deleted_at IS NULL", domain.StatusPublished).
		Where(fullTextMatch("title ILIKE ? OR slug ILIKE ? OR summary ILIKE ?"), q, kw, kw, kw)
	if gameVersion != "" {
		cond, args := versionFilter("game_versions", gameVersion)
		base = base.Where(cond, args...)
	}

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var items []domain.WikiEntry
	if err := base.Order("title ASC").Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	out := make([]ResultItem, 0, len(items))
	for _, it := range items {
		out = append(out, ResultItem{
			Type:      "wiki",
			ID:        it.ID.String(),
			Slug:      it.Slug,
			Title:     it.Title,
			Summary:   it.Summary,
			URL:       "/wiki/" + it.Slug,
			UpdatedAt: it.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
			score: relevance(q, rankInput{
				Title: it.Title, Slug: it.Slug, Aliases: it.Tags, Summary: it.Summary,
			}),
		})
	}
	return out, total, nil
}

func (s *Service) searchItem(ctx context.Context, q, gameVersion string, limit int) ([]ResultItem, int64, error) {
	kw := keywordPattern(q)
	base := s.db.WithContext(ctx).Model(&domain.Item{}).
		Where(fullTextMatch("item_id ILIKE ? OR name_zh ILIKE ? OR COALESCE(name_en,'') ILIKE ? OR aliases::text ILIKE ?"),
			q, kw, kw, kw, kw)
	if gameVersion != "" {
		cond, args := versionFilter("supported_versions", gameVersion)
		base = base.Where(cond, args...)
	}

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var items []domain.Item
	if err := base.Order("name_zh ASC").Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	out := make([]ResultItem, 0, len(items))
	for _, it := range items {
		out = append(out, ResultItem{
			Type:      "item",
			ID:        it.ID.String(),
			ItemID:    it.ItemID,
			Title:     it.NameZh,
			Summary:   it.Description,
			URL:       "/items/" + it.ItemID,
			UpdatedAt: it.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
			score: relevance(q, rankInput{
				Title:      it.NameZh,
				Identifier: it.ItemID,
				Aliases:    append([]string{it.NameEn}, it.Aliases...),
				Summary:    it.Description,
			}),
		})
	}
	return out, total, nil
}

// searchDevDoc 检索开发文档（TODO §21）。
// 命中范围：中文/英文标题、slug、摘要、正文（body_md）与标签；
// 仅返回已发布（published）文档，正文命中用于「搜得到文档内容」这一诉求。
func (s *Service) searchDevDoc(ctx context.Context, q, gameVersion string, limit int) ([]ResultItem, int64, error) {
	kw := keywordPattern(q)
	base := s.db.WithContext(ctx).Model(&domain.DevDoc{}).
		Where("status = ?", domain.StatusPublished).
		Where(fullTextMatch("title_zh ILIKE ? OR COALESCE(title_en,'') ILIKE ? OR slug ILIKE ? OR summary ILIKE ? OR body_md ILIKE ? OR tags::text ILIKE ?"),
			q, kw, kw, kw, kw, kw, kw)
	if gameVersion != "" {
		cond, args := versionFilter("mc_versions", gameVersion)
		base = base.Where(cond, args...)
	}

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var items []domain.DevDoc
	if err := base.Order("title_zh ASC").Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	out := make([]ResultItem, 0, len(items))
	for _, it := range items {
		out = append(out, ResultItem{
			Type:      "devdoc",
			ID:        it.ID.String(),
			Slug:      it.Slug,
			Title:     it.TitleZh,
			Summary:   it.Summary,
			URL:       "/dev/" + it.Slug,
			UpdatedAt: it.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
			score: relevance(q, rankInput{
				Title:   it.TitleZh,
				Slug:    it.Slug,
				Aliases: append([]string{it.TitleEn}, it.Tags...),
				Summary: it.Summary,
			}),
		})
	}
	return out, total, nil
}

// normalizePage 归一化分页：page 从 1 起，page_size 默认 20、最大 50（需求 §8.1）。
func normalizePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 50 {
		pageSize = 50
	}
	return page, pageSize
}
