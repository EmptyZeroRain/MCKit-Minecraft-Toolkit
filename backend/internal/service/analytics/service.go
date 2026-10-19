// Package analytics 提供后台运营看板的只读聚合查询（TODO §13）。
//
// 隐私立场：本包只读取 metrics 采集的按天聚合数据与公开内容统计，
// 不涉及也不返回任何个人身份信息；返回值均为计数、比率与近似分位数。
package analytics

import (
	"context"
	"sort"
	"time"

	"gorm.io/gorm"

	"mckit/backend/internal/domain"
	"mckit/backend/internal/service/metrics"
)

// Service 运营看板查询服务。
type Service struct{ db *gorm.DB }

// NewService 创建服务。
func NewService(db *gorm.DB) *Service { return &Service{db: db} }

// Overview 运营概览（给定时间范围内）。
type Overview struct {
	From string `json:"from"`
	To   string `json:"to"`

	ToolVisits   int64 `json:"tool_visits"`
	ToolUses     int64 `json:"tool_uses"`
	ToolFailures int64 `json:"tool_failures"`
	// ToolFailureRate 失败率 = failures / uses（无使用量时为 0）。
	ToolFailureRate float64 `json:"tool_failure_rate"`
	ActiveTools     int64   `json:"active_tools"`

	ResourceViews   int64 `json:"resource_views"`
	ActiveResources int64 `json:"active_resources"`
	// BrokenLinks 当前失效链接总数（实时查询，非时间范围内累加）。
	BrokenLinks int64 `json:"broken_links"`

	SearchQueries int64 `json:"search_queries"`
	SearchZero    int64 `json:"search_zero_result"`
	SearchFailed  int64 `json:"search_failed"`

	APITotal       int64   `json:"api_total"`
	APIErrors      int64   `json:"api_errors"`
	APIClientErrs  int64   `json:"api_client_errors"`
	APIRateLimited int64   `json:"api_rate_limited"`
	APIErrorRate   float64 `json:"api_error_rate"`
	AvgLatencyMs   float64 `json:"avg_latency_ms"`
	P95LatencyMs   int64   `json:"p95_latency_ms"`
}

// ToolStat 单个工具的统计。
type ToolStat struct {
	Slug     string  `json:"slug"`
	Visits   int64   `json:"visits"`
	Uses     int64   `json:"uses"`
	Failures int64   `json:"failures"`
	FailRate float64 `json:"fail_rate"`
}

// ResourceStat 单个资源的访问统计。
type ResourceStat struct {
	Slug  string `json:"slug"`
	Views int64  `json:"views"`
}

// APIStat 单个路由模板的指标。
type APIStat struct {
	Route        string  `json:"route"`
	Method       string  `json:"method"`
	Total        int64   `json:"total"`
	Errors       int64   `json:"errors"`
	ClientErrors int64   `json:"client_errors"`
	RateLimited  int64   `json:"rate_limited"`
	AvgMs        float64 `json:"avg_ms"`
	MaxMs        int64   `json:"max_ms"`
	P95Ms        int64   `json:"p95_ms"`
}

// TrendPoint 某一天的合并趋势点。
type TrendPoint struct {
	Date          string `json:"date"`
	ToolVisits    int64  `json:"tool_visits"`
	ToolUses      int64  `json:"tool_uses"`
	ToolFailures  int64  `json:"tool_failures"`
	ResourceViews int64  `json:"resource_views"`
	SearchQueries int64  `json:"search_queries"`
	APITotal      int64  `json:"api_total"`
	APIErrors     int64  `json:"api_errors"`
	APIRateLimit  int64  `json:"api_rate_limited"`
}

// latRow 延迟分桶聚合行。
type latRow struct {
	Total        int64
	Errors       int64
	ClientErrors int64
	RateLimited  int64
	LatencySum   int64
	LatencyMax   int64
	Le50         int64
	Le200        int64
	Le500        int64
	Le1000       int64
	Le3000       int64
	Gt3000       int64
}

func (r latRow) buckets() []int64 {
	return []int64{r.Le50, r.Le200, r.Le500, r.Le1000, r.Le3000, r.Gt3000}
}

const apiLatencySelect = `COALESCE(SUM(total),0) AS total,
	COALESCE(SUM(errors),0) AS errors,
	COALESCE(SUM(client_errors),0) AS client_errors,
	COALESCE(SUM(rate_limited),0) AS rate_limited,
	COALESCE(SUM(latency_ms_sum),0) AS latency_sum,
	COALESCE(MAX(latency_ms_max),0) AS latency_max,
	COALESCE(SUM(latency_le_50),0) AS le50,
	COALESCE(SUM(latency_le_200),0) AS le200,
	COALESCE(SUM(latency_le_500),0) AS le500,
	COALESCE(SUM(latency_le_1000),0) AS le1000,
	COALESCE(SUM(latency_le_3000),0) AS le3000,
	COALESCE(SUM(latency_gt_3000),0) AS gt3000`

// Overview 汇总给定范围内（含首尾日期）的全部指标。
func (s *Service) Overview(ctx context.Context, from, to time.Time) (Overview, error) {
	out := Overview{From: dateStr(from), To: dateStr(to)}
	if s == nil || s.db == nil {
		return out, nil
	}
	db := s.db.WithContext(ctx)
	rng := []any{from, to}

	var tool struct {
		Visits   int64
		Uses     int64
		Failures int64
		Slugs    int64
	}
	if err := db.Model(&domain.ToolStatDaily{}).
		Select(`COALESCE(SUM(visits),0) AS visits, COALESCE(SUM(uses),0) AS uses,
			COALESCE(SUM(failures),0) AS failures, COUNT(DISTINCT tool_slug) AS slugs`).
		Where("stat_date BETWEEN ? AND ?", rng...).Scan(&tool).Error; err != nil {
		return out, err
	}
	out.ToolVisits, out.ToolUses, out.ToolFailures, out.ActiveTools = tool.Visits, tool.Uses, tool.Failures, tool.Slugs
	if tool.Uses > 0 {
		out.ToolFailureRate = float64(tool.Failures) / float64(tool.Uses)
	}

	var res struct {
		Views int64
		Slugs int64
	}
	if err := db.Model(&domain.ResourceStatDaily{}).
		Select("COALESCE(SUM(views),0) AS views, COUNT(DISTINCT resource_slug) AS slugs").
		Where("stat_date BETWEEN ? AND ?", rng...).Scan(&res).Error; err != nil {
		return out, err
	}
	out.ResourceViews, out.ActiveResources = res.Views, res.Slugs

	var search struct {
		Queries    int64
		ZeroResult int64
		Failed     int64
	}
	if err := db.Model(&domain.SearchStatDaily{}).
		Select(`COALESCE(SUM(queries),0) AS queries, COALESCE(SUM(zero_result),0) AS zero_result,
			COALESCE(SUM(failed),0) AS failed`).
		Where("stat_date BETWEEN ? AND ?", rng...).Scan(&search).Error; err != nil {
		return out, err
	}
	out.SearchQueries, out.SearchZero, out.SearchFailed = search.Queries, search.ZeroResult, search.Failed

	var lat latRow
	if err := db.Model(&domain.APIStatDaily{}).
		Select(apiLatencySelect).Where("stat_date BETWEEN ? AND ?", rng...).Scan(&lat).Error; err != nil {
		return out, err
	}
	out.APITotal, out.APIErrors, out.APIClientErrs, out.APIRateLimited = lat.Total, lat.Errors, lat.ClientErrors, lat.RateLimited
	if lat.Total > 0 {
		out.APIErrorRate = float64(lat.Errors) / float64(lat.Total)
		out.AvgLatencyMs = float64(lat.LatencySum) / float64(lat.Total)
	}
	out.P95LatencyMs = metrics.PercentileFromBuckets(lat.buckets(), metrics.LatencyBucketBounds, lat.Total, 0.95)

	broken, err := s.BrokenLinkCount(ctx)
	if err != nil {
		return out, err
	}
	out.BrokenLinks = broken
	return out, nil
}

// ToolStats 按访问 + 使用总量降序返回工具统计。
func (s *Service) ToolStats(ctx context.Context, from, to time.Time, limit int) ([]ToolStat, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	var rows []ToolStat
	err := s.db.WithContext(ctx).Model(&domain.ToolStatDaily{}).
		Select(`tool_slug AS slug, COALESCE(SUM(visits),0) AS visits,
			COALESCE(SUM(uses),0) AS uses, COALESCE(SUM(failures),0) AS failures`).
		Where("stat_date BETWEEN ? AND ?", from, to).
		Group("tool_slug").
		Order("SUM(visits) + SUM(uses) DESC").
		Limit(clampLimit(limit, 50)).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for i := range rows {
		if rows[i].Uses > 0 {
			rows[i].FailRate = float64(rows[i].Failures) / float64(rows[i].Uses)
		}
	}
	return rows, nil
}

// ResourceStats 按访问量降序返回资源统计。
func (s *Service) ResourceStats(ctx context.Context, from, to time.Time, limit int) ([]ResourceStat, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	var rows []ResourceStat
	err := s.db.WithContext(ctx).Model(&domain.ResourceStatDaily{}).
		Select("resource_slug AS slug, COALESCE(SUM(views),0) AS views").
		Where("stat_date BETWEEN ? AND ?", from, to).
		Group("resource_slug").
		Order("SUM(views) DESC").
		Limit(clampLimit(limit, 50)).
		Scan(&rows).Error
	return rows, err
}

// APIStats 返回路由模板级指标（含近似 P95），按请求量降序。
func (s *Service) APIStats(ctx context.Context, from, to time.Time, limit int) ([]APIStat, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	var rows []struct {
		Route        string
		Method       string
		Total        int64
		Errors       int64
		ClientErrors int64
		RateLimited  int64
		LatencySum   int64
		LatencyMax   int64
		Le50         int64
		Le200        int64
		Le500        int64
		Le1000       int64
		Le3000       int64
		Gt3000       int64
	}
	err := s.db.WithContext(ctx).Model(&domain.APIStatDaily{}).
		Select("route, method, "+apiLatencySelect).
		Where("stat_date BETWEEN ? AND ?", from, to).
		Group("route, method").
		Order("SUM(total) DESC").
		Limit(clampLimit(limit, 100)).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]APIStat, 0, len(rows))
	for _, r := range rows {
		buckets := []int64{r.Le50, r.Le200, r.Le500, r.Le1000, r.Le3000, r.Gt3000}
		st := APIStat{
			Route: r.Route, Method: r.Method,
			Total: r.Total, Errors: r.Errors, ClientErrors: r.ClientErrors,
			RateLimited: r.RateLimited, MaxMs: r.LatencyMax,
			P95Ms: metrics.PercentileFromBuckets(buckets, metrics.LatencyBucketBounds, r.Total, 0.95),
		}
		if r.Total > 0 {
			st.AvgMs = float64(r.LatencySum) / float64(r.Total)
		}
		out = append(out, st)
	}
	return out, nil
}

// Trend 返回按天合并的趋势点，缺失日期补零并对齐到完整区间。
func (s *Service) Trend(ctx context.Context, from, to time.Time) ([]TrendPoint, error) {
	points := map[string]*TrendPoint{}
	// 预填充区间内每一天，保证前端折线连续。
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		key := dateStr(d)
		points[key] = &TrendPoint{Date: key}
	}
	if s == nil || s.db == nil {
		return sortedTrend(points), nil
	}
	db := s.db.WithContext(ctx)
	rng := []any{from, to}

	var toolRows []struct {
		D        time.Time
		Visits   int64
		Uses     int64
		Failures int64
	}
	if err := db.Model(&domain.ToolStatDaily{}).
		Select(`stat_date AS d, COALESCE(SUM(visits),0) AS visits,
			COALESCE(SUM(uses),0) AS uses, COALESCE(SUM(failures),0) AS failures`).
		Where("stat_date BETWEEN ? AND ?", rng...).Group("stat_date").Scan(&toolRows).Error; err != nil {
		return nil, err
	}
	for _, r := range toolRows {
		if p := points[dateStr(r.D)]; p != nil {
			p.ToolVisits, p.ToolUses, p.ToolFailures = r.Visits, r.Uses, r.Failures
		}
	}

	var resRows []struct {
		D     time.Time
		Views int64
	}
	if err := db.Model(&domain.ResourceStatDaily{}).
		Select("stat_date AS d, COALESCE(SUM(views),0) AS views").
		Where("stat_date BETWEEN ? AND ?", rng...).Group("stat_date").Scan(&resRows).Error; err != nil {
		return nil, err
	}
	for _, r := range resRows {
		if p := points[dateStr(r.D)]; p != nil {
			p.ResourceViews = r.Views
		}
	}

	var searchRows []struct {
		D       time.Time
		Queries int64
	}
	if err := db.Model(&domain.SearchStatDaily{}).
		Select("stat_date AS d, COALESCE(SUM(queries),0) AS queries").
		Where("stat_date BETWEEN ? AND ?", rng...).Group("stat_date").Scan(&searchRows).Error; err != nil {
		return nil, err
	}
	for _, r := range searchRows {
		if p := points[dateStr(r.D)]; p != nil {
			p.SearchQueries = r.Queries
		}
	}

	var apiRows []struct {
		D           time.Time
		Total       int64
		Errors      int64
		RateLimited int64
	}
	if err := db.Model(&domain.APIStatDaily{}).
		Select(`stat_date AS d, COALESCE(SUM(total),0) AS total,
			COALESCE(SUM(errors),0) AS errors, COALESCE(SUM(rate_limited),0) AS rate_limited`).
		Where("stat_date BETWEEN ? AND ?", rng...).Group("stat_date").Scan(&apiRows).Error; err != nil {
		return nil, err
	}
	for _, r := range apiRows {
		if p := points[dateStr(r.D)]; p != nil {
			p.APITotal, p.APIErrors, p.APIRateLimit = r.Total, r.Errors, r.RateLimited
		}
	}
	return sortedTrend(points), nil
}

// ToolExists 判断工具 slug 是否存在于工具表（用于统计上报白名单校验）。
func (s *Service) ToolExists(ctx context.Context, slug string) (bool, error) {
	if s == nil || s.db == nil {
		return false, nil
	}
	var n int64
	err := s.db.WithContext(ctx).Model(&domain.Tool{}).Where("slug = ?", slug).Count(&n).Error
	return n > 0, err
}

// ResourceExists 判断资源 slug 是否存在（用于统计上报白名单校验）。
func (s *Service) ResourceExists(ctx context.Context, slug string) (bool, error) {
	if s == nil || s.db == nil {
		return false, nil
	}
	var n int64
	err := s.db.WithContext(ctx).Model(&domain.Resource{}).
		Where("slug = ? AND deleted_at IS NULL", slug).Count(&n).Error
	return n > 0, err
}

// BrokenLinkCount 统计当前失效链接资源数。
func (s *Service) BrokenLinkCount(ctx context.Context) (int64, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	var n int64
	err := s.db.WithContext(ctx).Model(&domain.Resource{}).
		Where("link_status = ? AND deleted_at IS NULL", domain.LinkStatusBroken).Count(&n).Error
	return n, err
}

func sortedTrend(points map[string]*TrendPoint) []TrendPoint {
	out := make([]TrendPoint, 0, len(points))
	for _, p := range points {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out
}

func dateStr(t time.Time) string { return t.Format("2006-01-02") }

func clampLimit(limit, def int) int {
	if limit <= 0 {
		return def
	}
	if limit > 500 {
		return 500
	}
	return limit
}
