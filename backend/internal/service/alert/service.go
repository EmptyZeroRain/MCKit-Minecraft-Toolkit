// Package alert 提供运营告警：规则评估、去重落库、确认 / 关闭与历史查询。
//
// 设计取舍：
//   - 不做「告警通道」（邮件 / Webhook）：当前部署没有可靠的外发通道，
//     做成半成品反而会让人误以为通知可达；这里先把「发现问题 + 有人跟进」闭环。
//   - 统一用后台页面 + 轮询呈现，规则评估既可由后台定时触发，也可手动触发。
//   - 同一规则未关闭时只保留一条告警，指标持续异常只刷新 last_seen_at。
package alert

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"mckit/backend/internal/domain"
	"mckit/backend/internal/repository/types"
)

// ErrNotFound 告警不存在。
var ErrNotFound = errors.New("告警不存在")

// ErrNotOpen 告警已关闭，不能重复确认。
var ErrNotOpen = errors.New("告警已关闭")

// ErrBadInput 入参非法。
var ErrBadInput = errors.New("参数非法")

// Rule 一条告警规则。
//
// Check 返回是否触发、可读原因与指标快照；error 只用于记录日志，
// 单条规则失败不影响其它规则评估（避免一条 SQL 报错导致全站告警失效）。
type Rule struct {
	Key      string
	Severity string
	Title    string
	Check    func(ctx context.Context) (firing bool, detail string, metric map[string]any, err error)
}

// EvaluateSummary 一次评估的结果。
type EvaluateSummary struct {
	Evaluated  int      `json:"evaluated"`
	Triggered  int      `json:"triggered"`
	Refreshed  int      `json:"refreshed"`
	AutoClosed int      `json:"auto_closed"`
	Errors     []string `json:"errors,omitempty"`
}

// Service 告警服务。
type Service struct {
	db    *gorm.DB
	mu    sync.RWMutex
	rules []Rule

	stopOnce sync.Once
	stop     chan struct{}
	wg       sync.WaitGroup
}

// NewService 创建告警服务，并注册内置规则（依赖数据库的指标查询）。
func NewService(db *gorm.DB) *Service {
	s := &Service{db: db, stop: make(chan struct{})}
	s.registerBuiltinRules()
	return s
}

// RegisterRule 注册自定义规则（由 main 注入跨服务指标，如数据质量）。
func (s *Service) RegisterRule(r Rule) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rules = append(s.rules, r)
}

// Rules 返回已注册规则的只读快照。
func (s *Service) Rules() []Rule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Rule(nil), s.rules...)
}

// registerBuiltinRules 注册只依赖数据库的规则。
func (s *Service) registerBuiltinRules() {
	s.RegisterRule(Rule{
		Key:      "linkhealth.broken_pending",
		Severity: domain.AlertSeverityError,
		Title:    "存在较多未处理的失效链接",
		Check: func(ctx context.Context) (bool, string, map[string]any, error) {
			var total, broken int64
			if err := s.db.WithContext(ctx).Model(&domain.LinkCheck{}).
				Where("status <> ?", domain.LinkStatusUnchecked).Count(&total).Error; err != nil {
				return false, "", nil, err
			}
			if err := s.db.WithContext(ctx).Model(&domain.LinkCheck{}).
				Where("status = ? AND review_state = ?", domain.LinkStatusBroken, domain.ReviewPending).
				Count(&broken).Error; err != nil {
				return false, "", nil, err
			}
			ratio := 0.0
			if total > 0 {
				ratio = float64(broken) / float64(total)
			}
			metric := map[string]any{"broken": broken, "checked": total, "ratio": ratio}
			if broken < 5 || ratio < 0.1 {
				return false, "", metric, nil
			}
			return true, fmt.Sprintf("%d/%d 条已复查链接失效（%.1f%%），且未被人工确认", broken, total, ratio*100), metric, nil
		},
	})

	s.RegisterRule(Rule{
		Key:      "import.failed_recent",
		Severity: domain.AlertSeverityError,
		Title:    "近 24 小时存在失败的导入任务",
		Check: func(ctx context.Context) (bool, string, map[string]any, error) {
			var count int64
			if err := s.db.WithContext(ctx).Model(&domain.DataImportJob{}).
				Where("failed_count > 0 AND created_at > now() - interval '24 hours'").
				Count(&count).Error; err != nil {
				return false, "", nil, err
			}
			metric := map[string]any{"failed_jobs": count}
			if count == 0 {
				return false, "", metric, nil
			}
			return true, fmt.Sprintf("近 24 小时有 %d 个导入任务存在失败条目，请查看错误摘要", count), metric, nil
		},
	})

	s.RegisterRule(Rule{
		Key:      "jobs.failed_recent",
		Severity: domain.AlertSeverityWarning,
		Title:    "近 24 小时存在失败的后台任务",
		Check: func(ctx context.Context) (bool, string, map[string]any, error) {
			var count int64
			if err := s.db.WithContext(ctx).Model(&domain.AdminJob{}).
				Where("status = ? AND finished_at > now() - interval '24 hours'", domain.JobStatusFailed).
				Count(&count).Error; err != nil {
				return false, "", nil, err
			}
			metric := map[string]any{"failed_jobs": count}
			if count == 0 {
				return false, "", metric, nil
			}
			return true, fmt.Sprintf("近 24 小时有 %d 个后台任务在自动重试后仍失败，可在「后台任务」中重试", count), metric, nil
		},
	})
}

// Evaluate 评估全部规则：触发的开 / 刷新告警，恢复的自动关闭。
//
// 单条规则报错只记录在 Errors 中，不影响其它规则。
func (s *Service) Evaluate(ctx context.Context) (EvaluateSummary, error) {
	summary := EvaluateSummary{}
	now := time.Now()

	for _, rule := range s.Rules() {
		summary.Evaluated++
		firing, detail, metric, err := rule.Check(ctx)
		if err != nil {
			summary.Errors = append(summary.Errors, rule.Key+": "+err.Error())
			continue
		}
		if !firing {
			closed, err := s.resolveActive(ctx, rule.Key, "system", "指标已恢复，自动关闭", now)
			if err != nil {
				summary.Errors = append(summary.Errors, rule.Key+": "+err.Error())
				continue
			}
			summary.AutoClosed += int(closed)
			continue
		}

		metricJSON, _ := json.Marshal(metric)
		refreshed, err := s.upsertActive(ctx, rule, detail, types.JSONB(metricJSON), now)
		if err != nil {
			summary.Errors = append(summary.Errors, rule.Key+": "+err.Error())
			continue
		}
		if refreshed {
			summary.Refreshed++
		} else {
			summary.Triggered++
		}
	}
	return summary, nil
}

// upsertActive 刷新或新建未关闭告警；refreshed=true 表示复用了已有告警。
func (s *Service) upsertActive(ctx context.Context, rule Rule, detail string, metric types.JSONB, now time.Time) (bool, error) {
	var existing domain.OpsAlert
	err := s.db.WithContext(ctx).
		Where("dedupe_key = ? AND status IN ?", rule.Key, []string{domain.AlertStatusOpen, domain.AlertStatusAcked}).
		First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		alert := domain.OpsAlert{
			RuleKey:     rule.Key,
			Severity:    rule.Severity,
			Status:      domain.AlertStatusOpen,
			Title:       rule.Title,
			Detail:      detail,
			Metric:      metric,
			DedupeKey:   rule.Key,
			TriggeredAt: now,
			LastSeenAt:  now,
		}
		return false, s.db.WithContext(ctx).Create(&alert).Error
	}
	if err != nil {
		return false, err
	}
	return true, s.db.WithContext(ctx).Model(&domain.OpsAlert{}).
		Where("id = ?", existing.ID).
		Updates(map[string]any{
			"severity":     rule.Severity,
			"title":        rule.Title,
			"detail":       detail,
			"metric":       metric,
			"last_seen_at": now,
			"updated_at":   now,
		}).Error
}

// resolveActive 关闭某规则下所有未关闭告警。
func (s *Service) resolveActive(ctx context.Context, ruleKey, operator, note string, now time.Time) (int64, error) {
	res := s.db.WithContext(ctx).Model(&domain.OpsAlert{}).
		Where("dedupe_key = ? AND status IN ?", ruleKey,
			[]string{domain.AlertStatusOpen, domain.AlertStatusAcked}).
		Updates(map[string]any{
			"status":      domain.AlertStatusResolved,
			"resolved_by": operator,
			"resolved_at": now,
			"detail":      gorm.Expr("CASE WHEN ? <> '' THEN detail || ' · ' || ? ELSE detail END", note, note),
			"updated_at":  now,
		})
	return res.RowsAffected, res.Error
}

// ListFilter 告警列表筛选。
type ListFilter struct {
	Status   string
	Severity string
	RuleKey  string
	Page     int
	PageSize int
}

// List 分页查询告警（历史只增不减）。
func (s *Service) List(ctx context.Context, f ListFilter) ([]domain.OpsAlert, int64, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 || f.PageSize > 100 {
		f.PageSize = 20
	}
	tx := s.db.WithContext(ctx).Model(&domain.OpsAlert{})
	if st := strings.TrimSpace(f.Status); st != "" {
		if !domain.IsValidAlertStatus(st) {
			return nil, 0, fmt.Errorf("%w: 状态非法", ErrBadInput)
		}
		tx = tx.Where("status = ?", st)
	}
	if sev := strings.TrimSpace(f.Severity); sev != "" {
		if sev != domain.AlertSeverityWarning && sev != domain.AlertSeverityError {
			return nil, 0, fmt.Errorf("%w: 级别非法", ErrBadInput)
		}
		tx = tx.Where("severity = ?", sev)
	}
	if rk := strings.TrimSpace(f.RuleKey); rk != "" {
		tx = tx.Where("rule_key = ?", rk)
	}

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := make([]domain.OpsAlert, 0, f.PageSize)
	if err := tx.Order("triggered_at DESC").
		Offset((f.Page - 1) * f.PageSize).
		Limit(f.PageSize).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// Summary 未关闭告警计数（按级别），用于页面顶部提示。
type Summary struct {
	Open  int64 `json:"open"`
	Acked int64 `json:"acked"`
	Error int64 `json:"error"`
	Warn  int64 `json:"warning"`
}

// Summary 统计未关闭告警。
func (s *Service) Summary(ctx context.Context) (Summary, error) {
	var out Summary
	db := s.db.WithContext(ctx).Model(&domain.OpsAlert{})
	if err := db.Where("status = ?", domain.AlertStatusOpen).Count(&out.Open).Error; err != nil {
		return out, err
	}
	if err := s.db.WithContext(ctx).Model(&domain.OpsAlert{}).
		Where("status = ?", domain.AlertStatusAcked).Count(&out.Acked).Error; err != nil {
		return out, err
	}
	if err := s.db.WithContext(ctx).Model(&domain.OpsAlert{}).
		Where("status IN ? AND severity = ?", []string{domain.AlertStatusOpen, domain.AlertStatusAcked}, domain.AlertSeverityError).
		Count(&out.Error).Error; err != nil {
		return out, err
	}
	if err := s.db.WithContext(ctx).Model(&domain.OpsAlert{}).
		Where("status IN ? AND severity = ?", []string{domain.AlertStatusOpen, domain.AlertStatusAcked}, domain.AlertSeverityWarning).
		Count(&out.Warn).Error; err != nil {
		return out, err
	}
	return out, nil
}

// Ack 确认告警（open → acked）。
func (s *Service) Ack(ctx context.Context, id uuid.UUID, operator string) (*domain.OpsAlert, error) {
	var alert domain.OpsAlert
	if err := s.db.WithContext(ctx).First(&alert, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if alert.Status != domain.AlertStatusOpen {
		return nil, ErrNotOpen
	}
	now := time.Now()
	if err := s.db.WithContext(ctx).Model(&domain.OpsAlert{}).
		Where("id = ? AND status = ?", id, domain.AlertStatusOpen).
		Updates(map[string]any{
			"status":     domain.AlertStatusAcked,
			"acked_by":   operator,
			"acked_at":   now,
			"updated_at": now,
		}).Error; err != nil {
		return nil, err
	}
	return s.get(ctx, id)
}

// Resolve 关闭告警（open / acked → resolved）。
func (s *Service) Resolve(ctx context.Context, id uuid.UUID, operator, note string) (*domain.OpsAlert, error) {
	var alert domain.OpsAlert
	if err := s.db.WithContext(ctx).First(&alert, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if alert.Status == domain.AlertStatusResolved {
		return nil, ErrNotOpen
	}
	now := time.Now()
	if err := s.db.WithContext(ctx).Model(&domain.OpsAlert{}).
		Where("id = ? AND status <> ?", id, domain.AlertStatusResolved).
		Updates(map[string]any{
			"status":      domain.AlertStatusResolved,
			"resolved_by": operator,
			"resolved_at": now,
			"detail":      gorm.Expr("CASE WHEN ? <> '' THEN detail || ' · ' || ? ELSE detail END", note, note),
			"updated_at":  now,
		}).Error; err != nil {
		return nil, err
	}
	return s.get(ctx, id)
}

func (s *Service) get(ctx context.Context, id uuid.UUID) (*domain.OpsAlert, error) {
	var alert domain.OpsAlert
	if err := s.db.WithContext(ctx).First(&alert, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &alert, nil
}

// Start 定时评估（后台常驻）。
func (s *Service) Start(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-s.stop:
				return
			case <-ticker.C:
				// 用独立超时，避免单条慢查询拖住整个周期。
				runCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
				_, _ = s.Evaluate(runCtx)
				cancel()
			}
		}
	}()
}

// Stop 停止定时评估。
func (s *Service) Stop() {
	s.stopOnce.Do(func() { close(s.stop) })
	s.wg.Wait()
}
