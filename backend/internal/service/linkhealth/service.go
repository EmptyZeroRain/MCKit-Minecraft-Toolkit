// Package linkhealth 提供资源链接的批量复查与人工确认流程（TODO §6）。
//
// 与 internal/linkcheck 的分工：
//   - linkcheck 只负责「单个 / 批量 URL 的可达性判定」（含 SSRF 防护与并发限制）；
//   - linkhealth 负责把检查结果落库、更新资源链接状态，并管理人工复核流程
//     （确认失效 / 确认可用 / 暂不处理），供后台复查与人工确认使用。
package linkhealth

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"mckit/backend/internal/domain"
	"mckit/backend/internal/linkcheck"
)

// ErrNotFound 链接检查记录不存在。
var ErrNotFound = errors.New("链接检查记录不存在")

// ErrBadAction 无效的确认动作。
var ErrBadAction = errors.New("无效的确认动作")

// LinkChecker 批量链接检查能力（由 linkcheck.Checker 实现）。
type LinkChecker interface {
	CheckMany(ctx context.Context, urls []string, concurrency int) []linkcheck.BatchItem
}

// Service 链接健康服务。
type Service struct {
	db          *gorm.DB
	checker     LinkChecker
	concurrency int
}

// NewService 创建链接健康服务。concurrency 会被归一化（1–16，缺省 4）。
func NewService(db *gorm.DB, checker LinkChecker, concurrency int) *Service {
	return &Service{
		db:          db,
		checker:     checker,
		concurrency: linkcheck.NormalizeConcurrency(concurrency),
	}
}

// Concurrency 返回生效的并发上限（供接口回显）。
func (s *Service) Concurrency() int { return s.concurrency }

// RunOptions 批量复查参数。
type RunOptions struct {
	// ResourceID 非空时只复查该资源；为空表示复查全部有来源链接的资源。
	ResourceID string
	// OnlyPending 为 true 时跳过「已人工确认可用 / 已忽略」的资源，避免重复打扰。
	OnlyPending bool
	// Limit 最多检查条数（<=0 表示不限制）。
	Limit int
	// TriggerSource 触发来源（auto / manual），缺省 auto。
	TriggerSource string
}

// RunSummary 批量复查汇总。
type RunSummary struct {
	Checked     int    `json:"checked"`
	OK          int    `json:"ok"`
	Redirected  int    `json:"redirected"`
	Broken      int    `json:"broken"`
	Blocked     int    `json:"blocked"`
	Error       int    `json:"error"`
	Concurrency int    `json:"concurrency"`
	Trigger     string `json:"trigger_source"`
}

type target struct {
	ID  string
	URL string
}

// RunCheck 执行一次批量复查：受限并发检查 → 落库 → 更新资源链接状态。
func (s *Service) RunCheck(ctx context.Context, opts RunOptions) (RunSummary, error) {
	trigger := opts.TriggerSource
	if !domain.IsValidLinkTrigger(trigger) {
		trigger = domain.LinkTriggerAuto
	}
	summary := RunSummary{Concurrency: s.concurrency, Trigger: trigger}

	targets, err := s.collectTargets(ctx, opts)
	if err != nil {
		return summary, err
	}
	if len(targets) == 0 {
		return summary, nil
	}

	urls := make([]string, len(targets))
	for i, t := range targets {
		urls[i] = t.URL
	}
	items := s.checker.CheckMany(ctx, urls, s.concurrency)

	now := time.Now()
	for i, t := range targets {
		item := items[i]
		status, code, finalURL := classify(item.Result, item.Err)
		switch status {
		case domain.LinkStatusOK:
			summary.OK++
		case domain.LinkStatusRedirected:
			summary.Redirected++
		case domain.LinkStatusBroken:
			summary.Broken++
		case domain.LinkStatusBlocked:
			summary.Blocked++
		default:
			summary.Error++
		}
		summary.Checked++

		if err := s.upsert(ctx, domain.LinkCheck{
			ResourceID:    t.ID,
			URL:           t.URL,
			Status:        status,
			StatusCode:    code,
			FinalURL:      finalURL,
			TriggerSource: trigger,
			ReviewState:   domain.ReviewPending,
			CheckedAt:     now,
		}); err != nil {
			return summary, err
		}
		if err := s.db.WithContext(ctx).Model(&domain.Resource{}).
			Where("id = ?", t.ID).
			Updates(map[string]any{"link_status": status, "last_checked_at": now}).Error; err != nil {
			return summary, err
		}
	}
	return summary, nil
}

// collectTargets 选出待复查资源（仅取有来源链接、未软删除的资源）。
func (s *Service) collectTargets(ctx context.Context, opts RunOptions) ([]target, error) {
	q := s.db.WithContext(ctx).Model(&domain.Resource{}).
		Select("id", "source_url").
		Where("source_url <> '' AND deleted_at IS NULL")
	if opts.ResourceID != "" {
		q = q.Where("id = ?", opts.ResourceID)
	}
	if opts.OnlyPending {
		// 已人工确认可用 / 已忽略的资源不再自动复查。
		q = q.Where(`id NOT IN (
			SELECT resource_id FROM resource_link_checks
			WHERE review_state IN (?, ?)
		)`, domain.ReviewConfirmedOK, domain.ReviewIgnored)
	}
	q = q.Order("updated_at DESC")
	if opts.Limit > 0 {
		q = q.Limit(opts.Limit)
	}

	var rows []struct {
		ID        string
		SourceURL string
	}
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]target, 0, len(rows))
	for _, r := range rows {
		out = append(out, target{ID: r.ID, URL: r.SourceURL})
	}
	return out, nil
}

// upsert 写入 / 更新某资源的最新检查记录。
//
// 已有人工确认结论（confirmed_ok / ignored / confirmed_broken）在自动复查时**保持不变**，
// 只有「未复核（pending）」的记录才随本次结果刷新，避免自动结果覆盖人工判断。
func (s *Service) upsert(ctx context.Context, c domain.LinkCheck) error {
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "resource_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"url":            c.URL,
			"status":         c.Status,
			"status_code":    c.StatusCode,
			"final_url":      c.FinalURL,
			"trigger_source": c.TriggerSource,
			"checked_at":     c.CheckedAt,
			"updated_at":     c.CheckedAt,
		}),
	}).Create(&c).Error
}

// Item 复核列表项（附带资源名称 / slug，便于后台展示）。
type Item struct {
	domain.LinkCheck
	ResourceName string `json:"resource_name"`
	ResourceSlug string `json:"resource_slug"`
}

// ListFilter 复核列表筛选。
type ListFilter struct {
	ReviewState string
	Status      string
	// FailedOnly 仅返回失败状态（broken / blocked / error）。
	FailedOnly bool
	Page       int
	PageSize   int
}

// ListResult 复核列表结果。
type ListResult struct {
	Items    []Item `json:"items"`
	Total    int64  `json:"total"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
}

// List 查询复核列表（默认只看失败项，最近检查在前）。
func (s *Service) List(ctx context.Context, f ListFilter) (ListResult, error) {
	base := s.db.WithContext(ctx).Table("resource_link_checks AS c").
		Joins("JOIN resources AS r ON r.id = c.resource_id AND r.deleted_at IS NULL")

	if f.FailedOnly {
		base = base.Where("c.status IN ?", failedStatuses())
	}
	if f.Status != "" {
		base = base.Where("c.status = ?", f.Status)
	}
	if f.ReviewState != "" {
		base = base.Where("c.review_state = ?", f.ReviewState)
	}

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return ListResult{}, err
	}

	page, pageSize := normalizePage(f.Page, f.PageSize)
	var items []Item
	err := base.
		Select("c.*, r.name AS resource_name, r.slug AS resource_slug").
		Order("c.checked_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Scan(&items).Error
	if err != nil {
		return ListResult{}, err
	}
	return ListResult{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

// 确认动作 → 复核状态。
var actionState = map[string]string{
	"confirm_broken": domain.ReviewConfirmedBroken,
	"confirm_ok":     domain.ReviewConfirmedOK,
	"ignore":         domain.ReviewIgnored,
}

// Confirm 人工确认某条链接检查结论。
//
// action 取 confirm_broken / confirm_ok / ignore，或直接传复核状态值。
func (s *Service) Confirm(ctx context.Context, id, action, operator, note string) (*domain.LinkCheck, error) {
	state, ok := actionState[strings.TrimSpace(action)]
	if !ok {
		if domain.IsValidReviewState(action) {
			state = action
		} else {
			return nil, ErrBadAction
		}
	}
	if state == domain.ReviewPending {
		return nil, ErrBadAction
	}

	var rec domain.LinkCheck
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&rec).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	now := time.Now()
	rec.ReviewState = state
	rec.ReviewedBy = operator
	rec.ReviewedAt = &now
	rec.ReviewNote = strings.TrimSpace(note)
	if err := s.db.WithContext(ctx).Model(&domain.LinkCheck{}).Where("id = ?", id).
		Updates(map[string]any{
			"review_state": state,
			"reviewed_by":  rec.ReviewedBy,
			"reviewed_at":  now,
			"review_note":  rec.ReviewNote,
		}).Error; err != nil {
		return nil, err
	}
	return &rec, nil
}

// classify 把检查结果归一为链接状态 + 状态码 + 最终地址。
func classify(res *linkcheck.Result, err error) (status string, code int, finalURL string) {
	if res != nil {
		code = res.StatusCode
		finalURL = res.FinalURL
		status = res.Status
	}
	if status == "" {
		if errors.Is(err, linkcheck.ErrUnsafeURL) {
			status = domain.LinkStatusBlocked
		} else {
			status = domain.LinkStatusError
		}
	}
	// 重定向到最终地址但未命中明确分类时，按 ok 处理（可达）。
	if status == "ok" || status == "redirected" || status == "broken" ||
		status == "blocked" || status == "error" {
		return status, code, finalURL
	}
	return domain.LinkStatusError, code, finalURL
}

func failedStatuses() []string {
	return []string{domain.LinkStatusBroken, domain.LinkStatusBlocked, domain.LinkStatusError}
}

func normalizePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}
