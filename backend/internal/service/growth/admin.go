package growth

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"mckit/backend/internal/domain"
)

// 本文件实现成长系统的后台管理能力（jingyan.md §10 / §11）：
//   - 概览统计；
//   - 经验规则版本化维护；
//   - 等级阈值维护与历史重算；
//   - 全局经验流水查询、人工调整与冲正；
//   - 每日任务 / 成就定义维护与历史补算；
//   - 成长审计查询。
//
// 所有会改变用户经验或定义的操作都写 growth_audit_logs，并保证：
//   - 人工调整 / 冲正通过账本幂等键防重复；
//   - 冲正只以负向流水表达，绝不物理删除原始流水；
//   - 人工负向调整不会使余额为负（applyLedger 兜底）。

// AdminOverview 后台成长概览（含等级分布）。
type AdminOverview struct {
	Timezone string `json:"timezone"`
	Date     string `json:"date"`
	*RepoStatsView
}

// RepoStatsView 后台统计视图（避免直接暴露仓储结构体，便于契约稳定）。
type RepoStatsView struct {
	ActiveUsers           int64         `json:"active_users"`
	XPIssuedToday         int64         `json:"xp_issued_today"`
	XPIssued7Days         int64         `json:"xp_issued_7days"`
	LedgerToday           int64         `json:"ledger_today"`
	ReversedTotal         int64         `json:"reversed_total"`
	ManualAdjustCount     int64         `json:"manual_adjust_count"`
	UnlockedTotal         int64         `json:"unlocked_total"`
	AchievementCount      int64         `json:"achievement_count"`
	TasksCompletedToday   int64         `json:"tasks_completed_today"`
	TaskParticipantsToday int64         `json:"task_participants_today"`
	LevelDistribution     []LevelBucket `json:"level_distribution"`
	TotalRules            int64         `json:"total_rules"`
	TotalTasks            int64         `json:"total_tasks"`
}

// GetAdminOverview 汇总后台成长概览。
func (s *Service) GetAdminOverview(ctx context.Context) (*AdminOverview, error) {
	date := businessDate(s.now(), s.loc)
	dayStart, dayEnd, err := dayRange(date, s.loc)
	if err != nil {
		return nil, err
	}
	weekStart := dayStart.AddDate(0, 0, -6)
	stats, err := s.repo.Stats(ctx, date, dayStart, dayEnd, weekStart)
	if err != nil {
		return nil, err
	}
	view := &RepoStatsView{
		ActiveUsers:           stats.ActiveUsers,
		XPIssuedToday:         stats.XPIssuedToday,
		XPIssued7Days:         stats.XPIssued7Days,
		LedgerToday:           stats.LedgerToday,
		ReversedTotal:         stats.ReversedTotal,
		ManualAdjustCount:     stats.ManualAdjustCount,
		UnlockedTotal:         stats.UnlockedTotal,
		AchievementCount:      stats.AchievementCount,
		TasksCompletedToday:   stats.TasksCompletedToday,
		TaskParticipantsToday: stats.TaskParticipantsToday,
		LevelDistribution:     stats.LevelDistribution,
	}
	if view.LevelDistribution == nil {
		view.LevelDistribution = []LevelBucket{}
	}
	if err := s.db.WithContext(ctx).Model(&domain.GrowthRule{}).Count(&view.TotalRules).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&domain.DailyTaskDefinition{}).Count(&view.TotalTasks).Error; err != nil {
		return nil, err
	}
	return &AdminOverview{Timezone: s.loc.String(), Date: date, RepoStatsView: view}, nil
}

// ---- 经验规则 ----

// RuleInput 经验规则创建 / 更新入参。
type RuleInput struct {
	Code            string     `json:"code"`
	Name            string     `json:"name"`
	EventType       string     `json:"event_type"`
	XPAmount        int        `json:"xp_amount"`
	DailyLimit      *int       `json:"daily_limit"`
	PerSubjectLimit *int       `json:"per_subject_limit"`
	Enabled         bool       `json:"enabled"`
	EffectiveFrom   *time.Time `json:"effective_from"`
	EffectiveUntil  *time.Time `json:"effective_until"`
}

// ListRules 返回全部经验规则（版本降序）。
func (s *Service) ListRules(ctx context.Context) ([]domain.GrowthRule, error) {
	return s.repo.ListRules(ctx)
}

// CreateRule 新建一条经验规则（版本 1）。编码必须唯一且事件类型合法。
func (s *Service) CreateRule(ctx context.Context, in RuleInput, operatorID string) (*domain.GrowthRule, error) {
	code, err := normalizeRuleCode(in.Code)
	if err != nil {
		return nil, err
	}
	if err := validateRuleInput(in); err != nil {
		return nil, err
	}
	var existing int64
	if err := s.db.WithContext(ctx).Model(&domain.GrowthRule{}).Where("code = ?", code).Count(&existing).Error; err != nil {
		return nil, err
	}
	if existing > 0 {
		return nil, ErrRuleCodeTaken
	}
	rule := &domain.GrowthRule{
		Code: code, Version: 1, Name: strings.TrimSpace(in.Name),
		EventType: in.EventType, XPAmount: in.XPAmount,
		DailyLimit: in.DailyLimit, PerSubjectLimit: in.PerSubjectLimit,
		Enabled: in.Enabled, EffectiveFrom: in.EffectiveFrom, EffectiveUntil: in.EffectiveUntil,
		CreatedBy: parseUUIDPtr(operatorID), UpdatedBy: parseUUIDPtr(operatorID),
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(rule).Error; err != nil {
			return err
		}
		return writeAudit(tx, auditEntry{
			OperatorID: operatorID, Action: "rule.create", EntityType: "growth_rule", EntityID: rule.ID.String(),
			Reason: "新建经验规则 " + code, After: ruleSnapshot(rule),
		})
	})
	if err != nil {
		return nil, err
	}
	return rule, nil
}

// UpdateRule 以「新版本」方式更新规则：保留历史版本，新版本号 = 该编码当前最大版本 + 1。
//
// 不原地改写历史含义，保证已发奖流水的 rule_version 仍可追溯。
func (s *Service) UpdateRule(ctx context.Context, id string, in RuleInput, operatorID string) (*domain.GrowthRule, error) {
	old, err := s.repo.GetRule(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := validateRuleInput(in); err != nil {
		return nil, err
	}
	var maxVersion int
	if err := s.db.WithContext(ctx).Model(&domain.GrowthRule{}).
		Where("code = ?", old.Code).Select("COALESCE(MAX(version),0)").Scan(&maxVersion).Error; err != nil {
		return nil, err
	}
	rule := &domain.GrowthRule{
		Code: old.Code, Version: maxVersion + 1, Name: strings.TrimSpace(in.Name),
		EventType: in.EventType, XPAmount: in.XPAmount,
		DailyLimit: in.DailyLimit, PerSubjectLimit: in.PerSubjectLimit,
		Enabled: in.Enabled, EffectiveFrom: in.EffectiveFrom, EffectiveUntil: in.EffectiveUntil,
		CreatedBy: old.CreatedBy, UpdatedBy: parseUUIDPtr(operatorID),
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(rule).Error; err != nil {
			return err
		}
		// 新版关闭时，连同该编码的历史版本一并停用，避免旧版意外继续生效。
		if !rule.Enabled {
			if err := tx.Model(&domain.GrowthRule{}).Where("code = ?", old.Code).
				Update("enabled", false).Error; err != nil {
				return err
			}
		}
		return writeAudit(tx, auditEntry{
			OperatorID: operatorID, Action: "rule.update", EntityType: "growth_rule", EntityID: rule.ID.String(),
			Reason: "更新经验规则 " + old.Code + " 至版本 " + itoa(rule.Version),
			Before: ruleSnapshot(old), After: ruleSnapshot(rule),
		})
	})
	if err != nil {
		return nil, err
	}
	return rule, nil
}

// SetRuleEnabled 启用 / 停用某编码的全部版本。
func (s *Service) SetRuleEnabled(ctx context.Context, id string, enabled bool, operatorID string) error {
	rule, err := s.repo.GetRule(ctx, id)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&domain.GrowthRule{}).Where("code = ?", rule.Code).
			Update("enabled", enabled).Error; err != nil {
			return err
		}
		action := "rule.disable"
		if enabled {
			action = "rule.enable"
		}
		return writeAudit(tx, auditEntry{
			OperatorID: operatorID, Action: action, EntityType: "growth_rule", EntityID: rule.ID.String(),
			Reason: "切换经验规则 " + rule.Code + " 启用状态",
			After:  map[string]any{"code": rule.Code, "enabled": enabled},
		})
	})
}

// DeleteRule 删除某编码的全部版本；已被账本引用时拒绝删除，保证审计可追溯。
func (s *Service) DeleteRule(ctx context.Context, id string, operatorID string) error {
	rule, err := s.repo.GetRule(ctx, id)
	if err != nil {
		return err
	}
	var used int64
	if err := s.db.WithContext(ctx).Model(&domain.ExperienceLedger{}).
		Where("rule_code = ?", rule.Code).Count(&used).Error; err != nil {
		return err
	}
	if used > 0 {
		return ErrRuleInUse
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("code = ?", rule.Code).Delete(&domain.GrowthRule{}).Error; err != nil {
			return err
		}
		return writeAudit(tx, auditEntry{
			OperatorID: operatorID, Action: "rule.delete", EntityType: "growth_rule", EntityID: rule.ID.String(),
			Reason: "删除经验规则 " + rule.Code, Before: ruleSnapshot(rule),
		})
	})
}

// ---- 等级 ----

// ListLevels 返回全部等级阈值。
func (s *Service) ListLevels(ctx context.Context) ([]domain.GrowthLevel, error) {
	return s.repo.ListLevels(ctx)
}

// UpdateLevel 更新单个等级阈值 / 标题 / 启用状态。
// 校验：阈值非负且全局唯一、按等级严格递增，Lv.1 必须为 0 XP。
func (s *Service) UpdateLevel(ctx context.Context, in domain.GrowthLevel, operatorID string) error {
	if in.Level < 1 || in.ThresholdXP < 0 || strings.TrimSpace(in.Title) == "" || len(in.Title) > 100 {
		return ErrLevelThreshold
	}
	levels, err := s.repo.ListLevels(ctx)
	if err != nil {
		return err
	}
	var target *domain.GrowthLevel
	for i := range levels {
		if levels[i].Level == in.Level {
			target = &levels[i]
		}
	}
	if target == nil {
		return ErrLevelNotFound
	}
	for _, l := range levels {
		if l.Level == in.Level {
			continue
		}
		if l.ThresholdXP == in.ThresholdXP {
			return ErrLevelThreshold
		}
	}
	if in.Level == 1 && in.ThresholdXP != 0 {
		return ErrLevelThreshold
	}
	for _, l := range levels {
		if l.Level < in.Level && l.ThresholdXP >= in.ThresholdXP {
			return ErrLevelThreshold
		}
		if l.Level > in.Level && l.ThresholdXP <= in.ThresholdXP {
			return ErrLevelThreshold
		}
	}
	before := levelSnapshot(target)
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&domain.GrowthLevel{}).Where("level = ?", in.Level).
			Updates(map[string]any{
				"threshold_xp": in.ThresholdXP,
				"title":        strings.TrimSpace(in.Title),
				"enabled":      in.Enabled,
				"updated_at":   s.now(),
			}).Error; err != nil {
			return err
		}
		return writeAudit(tx, auditEntry{
			OperatorID: operatorID, Action: "level.update", EntityType: "growth_level", EntityID: itoa(in.Level),
			Reason: "更新等级 Lv." + itoa(in.Level) + " 阈值", Before: before,
			After: map[string]any{"level": in.Level, "threshold_xp": in.ThresholdXP, "title": in.Title, "enabled": in.Enabled},
		})
	})
}

// CreateLevelInput 新增等级入参（等级号由服务端追加）。
type CreateLevelInput struct {
	ThresholdXP int64  `json:"threshold_xp"`
	Title       string `json:"title"`
	Enabled     bool   `json:"enabled"`
}

// CreateLevel 在现有等级序列末尾追加一个等级。
//
// 等级号自动取当前最大等级 + 1；新增等级的所需经验必须严格大于当前最高等级，
// 以保证阈值按等级递增，不影响已有用户等级计算。
func (s *Service) CreateLevel(ctx context.Context, in CreateLevelInput, operatorID string) (*domain.GrowthLevel, error) {
	title := strings.TrimSpace(in.Title)
	if in.ThresholdXP < 0 || title == "" || len(title) > 100 {
		return nil, ErrLevelThreshold
	}
	var created *domain.GrowthLevel
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var levels []domain.GrowthLevel
		if err := tx.Order("level ASC").Find(&levels).Error; err != nil {
			return err
		}
		level := 1
		if len(levels) > 0 {
			last := levels[len(levels)-1]
			if in.ThresholdXP <= last.ThresholdXP {
				return ErrLevelNewThreshold
			}
			level = last.Level + 1
		} else if in.ThresholdXP != 0 {
			return ErrLevelNewThreshold
		}
		lvl := &domain.GrowthLevel{
			Level: level, ThresholdXP: in.ThresholdXP, Title: title, Enabled: in.Enabled,
		}
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(lvl)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrLevelNewThreshold
		}
		if err := writeAudit(tx, auditEntry{
			OperatorID: operatorID, Action: "level.create", EntityType: "growth_level", EntityID: itoa(level),
			Reason: "新增等级 Lv." + itoa(level), After: levelSnapshot(lvl),
		}); err != nil {
			return err
		}
		created = lvl
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// DeleteLevel 删除最高等级，并重算所有用户等级。
//
// 只允许删除最高等级：保持等级序列连续，避免中间等级消失后成就条件与等级号错位。
// Lv.1 永远保留；删除后用户展示等级按剩余阈值重算，人工等级偏移同步收敛到新范围。
func (s *Service) DeleteLevel(ctx context.Context, level int, operatorID string) error {
	if level <= 1 {
		return ErrLevelDeleteTopOnly
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var levels []domain.GrowthLevel
		if err := tx.Order("level ASC").Find(&levels).Error; err != nil {
			return err
		}
		var target *domain.GrowthLevel
		for i := range levels {
			if levels[i].Level == level {
				target = &levels[i]
				break
			}
		}
		if target == nil {
			return ErrLevelNotFound
		}
		if levels[len(levels)-1].Level != level {
			return ErrLevelDeleteTopOnly
		}
		res := tx.Where("level = ?", level).Delete(&domain.GrowthLevel{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrLevelNotFound
		}
		if err := writeAudit(tx, auditEntry{
			OperatorID: operatorID, Action: "level.delete", EntityType: "growth_level", EntityID: itoa(level),
			Reason: "删除等级 Lv." + itoa(level), Before: levelSnapshot(target),
		}); err != nil {
			return err
		}
		_, err := recalculateUserLevels(tx, levels[:len(levels)-1], true)
		return err
	})
}

// AdjustLevelInput 单用户等级调整入参。
type AdjustLevelInput struct {
	TargetUserID string `json:"target_user_id"`
	DeltaLevel   int    `json:"delta_level"`
	Reason       string `json:"reason"`
	RequestKey   string `json:"request_key"`
	OperatorID   string `json:"-"`
	RequestID    string `json:"-"`
}

// LevelAdjustResult 单用户等级调整结果。
type LevelAdjustResult struct {
	UserID      string `json:"user_id"`
	BaseLevel   int    `json:"base_level"`
	LevelOffset int    `json:"level_offset"`
	Level       int    `json:"level"`
	LevelTitle  string `json:"level_title"`
	DeltaLevel  int    `json:"delta_level"`
}

// AdjustLevel 直接调整单个用户的展示等级。
//
// 等级本身由经验阈值推导，因此这里调整的是 user_growth.level_offset：
// 有效等级 = 经验等级 + 偏移，后续经验变化仍会平滑叠加，不会把人工调整清零。
// 只允许调整现有启用等级范围内的目标；等级定义本身由 CreateLevel / DeleteLevel 管理。
func (s *Service) AdjustLevel(ctx context.Context, in AdjustLevelInput) (*LevelAdjustResult, error) {
	targetID, err := s.resolveUserID(ctx, in.TargetUserID)
	if err != nil {
		return nil, err
	}
	if in.DeltaLevel == 0 || in.DeltaLevel < -100 || in.DeltaLevel > 100 {
		return nil, ErrBadInput
	}
	reason := strings.TrimSpace(in.Reason)
	if reason == "" {
		return nil, ErrReasonRequired
	}
	levels, err := s.repo.ListLevels(ctx)
	if err != nil {
		return nil, err
	}
	enabled := enabledLevels(levels)
	if len(enabled) == 0 {
		return nil, ErrLevelNotFound
	}
	minLevel, maxLevel := enabled[0].Level, enabled[len(enabled)-1].Level
	enabledSet := make(map[int]bool, len(enabled))
	for _, l := range enabled {
		enabledSet[l.Level] = true
	}

	var result *LevelAdjustResult
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		g, err := lockGrowth(tx, targetID)
		if err != nil {
			return err
		}
		baseLevel := computeLevel(levels, g.CurrentXP).Level
		currentLevel := effectiveLevel(levels, g.CurrentXP, g.LevelOffset)
		targetLevel := currentLevel + in.DeltaLevel
		if targetLevel < minLevel || targetLevel > maxLevel || !enabledSet[targetLevel] {
			return ErrLevelRange
		}
		newOffset := targetLevel - baseLevel
		if err := tx.Model(&domain.UserGrowth{}).Where("user_id = ?", targetID).
			Updates(map[string]any{
				"level":        targetLevel,
				"level_offset": newOffset,
				"version":      gorm.Expr("version + 1"),
				"updated_at":   s.now(),
			}).Error; err != nil {
			return err
		}
		if err := writeAudit(tx, auditEntry{
			OperatorID: in.OperatorID, Action: "level.adjust", TargetUser: targetID.String(),
			EntityType: "user_growth", EntityID: targetID.String(), Reason: reason,
			Before: map[string]any{
				"base_level":   baseLevel,
				"level":        currentLevel,
				"level_offset": g.LevelOffset,
			},
			After: map[string]any{
				"base_level":   baseLevel,
				"level":        targetLevel,
				"level_offset": newOffset,
				"delta_level":  in.DeltaLevel,
				"request_key":  strings.TrimSpace(in.RequestKey),
			},
			RequestID: in.RequestID,
		}); err != nil {
			return err
		}
		result = &LevelAdjustResult{
			UserID:      targetID.String(),
			BaseLevel:   baseLevel,
			LevelOffset: newOffset,
			Level:       targetLevel,
			LevelTitle:  levelTitle(levels, targetLevel),
			DeltaLevel:  in.DeltaLevel,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// RecalculateLevels 依据当前阈值表重算所有用户等级（等级仅展示用，不影响权限）。
// 返回被修正的用户数。
func (s *Service) RecalculateLevels(ctx context.Context) (int, error) {
	levels, err := s.repo.ListLevels(ctx)
	if err != nil {
		return 0, err
	}
	return recalculateUserLevels(s.db.WithContext(ctx), levels, false)
}

// recalculateUserLevels 按给定等级表重算用户展示等级。
//
// clampOffset 为 true 时会把 level_offset 收敛到新等级范围内；
// 删除最高等级后必须收敛，否则偏移会在未来新增等级时产生意外跳级。
func recalculateUserLevels(db *gorm.DB, levels []domain.GrowthLevel, clampOffset bool) (int, error) {
	var rows []domain.UserGrowth
	if err := db.Select("user_id, current_xp, level, level_offset").Find(&rows).Error; err != nil {
		return 0, err
	}
	fixed := 0
	for _, g := range rows {
		want := effectiveLevel(levels, g.CurrentXP, g.LevelOffset)
		updates := map[string]any{}
		if want != g.Level {
			updates["level"] = want
		}
		if clampOffset {
			base := computeLevel(levels, g.CurrentXP).Level
			if newOffset := want - base; newOffset != g.LevelOffset {
				updates["level_offset"] = newOffset
			}
		}
		if len(updates) == 0 {
			continue
		}
		if err := db.Model(&domain.UserGrowth{}).
			Where("user_id = ?", g.UserID).Updates(updates).Error; err != nil {
			return fixed, err
		}
		fixed++
	}
	return fixed, nil
}

// ---- 全局流水 / 人工调整 / 冲正 ----

// AdminListLedger 分页查询全局经验流水。
func (s *Service) AdminListLedger(ctx context.Context, f LedgerFilter) ([]LedgerItem, int64, error) {
	if strings.TrimSpace(f.UserID) != "" {
		id, err := s.resolveUserID(ctx, f.UserID)
		if err != nil {
			return nil, 0, err
		}
		f.UserID = id.String()
	}
	rows, total, err := s.repo.ListLedger(ctx, f)
	if err != nil {
		return nil, 0, err
	}
	return toLedgerItems(rows), total, nil
}

// AdjustInput 人工调整入参。
type AdjustInput struct {
	TargetUserID string `json:"target_user_id"`
	DeltaXP      int64  `json:"delta_xp"`
	Reason       string `json:"reason"`
	// RequestKey 客户端幂等键；为空时服务端生成随机键。
	RequestKey string `json:"request_key"`
	OperatorID string `json:"-"`
	RequestID  string `json:"-"`
}

// Adjust 人工调整用户经验（正负均可），写账本与审计。
// 以「目标用户 + 请求键」为幂等键，重复提交不会重复调整。
func (s *Service) Adjust(ctx context.Context, in AdjustInput) (*domain.ExperienceLedger, error) {
	targetID, err := s.resolveUserID(ctx, in.TargetUserID)
	if err != nil {
		return nil, err
	}
	if in.DeltaXP == 0 {
		return nil, ErrBadInput
	}
	reason := strings.TrimSpace(in.Reason)
	if reason == "" {
		return nil, ErrReasonRequired
	}
	requestKey := strings.TrimSpace(in.RequestKey)
	if requestKey == "" {
		requestKey = uuid.NewString()
	}
	key := manualAdjustKey(targetID.String(), requestKey)
	var entry *domain.ExperienceLedger
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		existing, err := findLedgerByKey(tx, key)
		if err != nil {
			return err
		}
		if existing != nil {
			entry = existing // 幂等：重复提交直接返回原流水
			return nil
		}
		e, err := s.applyLedger(tx, ledgerInput{
			UserID:     targetID,
			DeltaXP:    in.DeltaXP,
			SourceType: domain.LedgerSourceManual,
			SourceID:   requestKey,
			Key:        key,
			Reason:     reason,
			OperatorID: parseUUIDPtr(in.OperatorID),
		})
		if err != nil {
			return err
		}
		if err := s.recalcLevel(tx, targetID); err != nil {
			return err
		}
		if err := writeAudit(tx, auditEntry{
			OperatorID: in.OperatorID, Action: "xp.adjust", TargetUser: targetID.String(),
			EntityType: "experience_ledger", EntityID: e.ID.String(), Reason: reason,
			After:     map[string]any{"delta_xp": in.DeltaXP, "balance_after": e.BalanceAfter},
			RequestID: in.RequestID,
		}); err != nil {
			return err
		}
		entry = e
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entry, nil
}

// ReverseInput 冲正入参。
type ReverseInput struct {
	LedgerID   string `json:"id"`
	Reason     string `json:"reason"`
	OperatorID string `json:"-"`
	RequestID  string `json:"-"`
}

// Reverse 冲正一笔正向经验流水：生成等额负向流水，原始流水保持不变。
// 以「原流水 ID」为幂等键，同一流水只能被冲正一次。
func (s *Service) Reverse(ctx context.Context, in ReverseInput) (*domain.ExperienceLedger, error) {
	ledgerID := strings.TrimSpace(in.LedgerID)
	if ledgerID == "" {
		return nil, ErrBadInput
	}
	reason := strings.TrimSpace(in.Reason)
	if reason == "" {
		return nil, ErrReasonRequired
	}
	key := reversalKey(ledgerID)
	var entry *domain.ExperienceLedger
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		existing, err := findLedgerByKey(tx, key)
		if err != nil {
			return err
		}
		if existing != nil {
			entry = existing // 幂等：已冲正
			return nil
		}
		orig, err := findLedgerByID(tx, ledgerID)
		if err != nil {
			return err
		}
		if orig.SourceType == domain.LedgerSourceReversal {
			return ErrCannotReverseReversal
		}
		if orig.DeltaXP == 0 {
			return ErrBadInput
		}
		e, err := s.applyLedger(tx, ledgerInput{
			UserID:      orig.UserID,
			DeltaXP:     -orig.DeltaXP,
			SourceType:  domain.LedgerSourceReversal,
			SourceID:    orig.ID.String(),
			Key:         key,
			RuleCode:    orig.RuleCode,
			RuleVersion: orig.RuleVersion,
			Reason:      "冲正：" + reason,
			OperatorID:  parseUUIDPtr(in.OperatorID),
		})
		if err != nil {
			return err
		}
		if err := s.recalcLevel(tx, orig.UserID); err != nil {
			return err
		}
		if err := writeAudit(tx, auditEntry{
			OperatorID: in.OperatorID, Action: "xp.reverse", TargetUser: orig.UserID.String(),
			EntityType: "experience_ledger", EntityID: orig.ID.String(), Reason: reason,
			Before:    map[string]any{"delta_xp": orig.DeltaXP, "source_type": orig.SourceType},
			After:     map[string]any{"reversed_delta_xp": e.DeltaXP, "reversal_ledger_id": e.ID.String()},
			RequestID: in.RequestID,
		}); err != nil {
			return err
		}
		entry = e
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entry, nil
}

// ---- 每日任务定义 ----

// TaskDefInput 每日任务定义创建 / 更新入参。
type TaskDefInput struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
	EventType   string `json:"event_type"`
	TargetCount int    `json:"target_count"`
	RewardXP    int    `json:"reward_xp"`
	Enabled     bool   `json:"enabled"`
	SortOrder   int    `json:"sort_order"`
}

// ListTaskDefs 返回全部每日任务定义（含停用项）。
func (s *Service) ListTaskDefs(ctx context.Context) ([]domain.DailyTaskDefinition, error) {
	return s.repo.ListTaskDefs(ctx, false)
}

// CreateTaskDef 新建每日任务定义。
func (s *Service) CreateTaskDef(ctx context.Context, in TaskDefInput, operatorID string) (*domain.DailyTaskDefinition, error) {
	code, err := normalizeRuleCode(in.Code)
	if err != nil {
		return nil, err
	}
	if err := validateTaskDefInput(in); err != nil {
		return nil, err
	}
	var existing int64
	if err := s.db.WithContext(ctx).Model(&domain.DailyTaskDefinition{}).Where("code = ?", code).Count(&existing).Error; err != nil {
		return nil, err
	}
	if existing > 0 {
		return nil, ErrTaskCodeTaken
	}
	def := &domain.DailyTaskDefinition{
		Code: code, Name: strings.TrimSpace(in.Name), Description: strings.TrimSpace(in.Description),
		EventType: in.EventType, TargetCount: in.TargetCount, RewardXP: in.RewardXP,
		Enabled: in.Enabled, SortOrder: in.SortOrder, Version: 1,
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(def).Error; err != nil {
			return err
		}
		return writeAudit(tx, auditEntry{
			OperatorID: operatorID, Action: "task.create", EntityType: "daily_task", EntityID: def.ID.String(),
			Reason: "新建每日任务 " + code, After: taskSnapshot(def),
		})
	})
	if err != nil {
		return nil, err
	}
	return def, nil
}

// UpdateTaskDef 更新每日任务定义（原地更新并递增版本；已产生的进度保留快照）。
func (s *Service) UpdateTaskDef(ctx context.Context, id string, in TaskDefInput, operatorID string) (*domain.DailyTaskDefinition, error) {
	old, err := s.repo.GetTaskDef(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := validateTaskDefInput(in); err != nil {
		return nil, err
	}
	before := taskSnapshot(old)
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&domain.DailyTaskDefinition{}).Where("id = ?", old.ID).
			Updates(map[string]any{
				"name":         strings.TrimSpace(in.Name),
				"description":  strings.TrimSpace(in.Description),
				"event_type":   in.EventType,
				"target_count": in.TargetCount,
				"reward_xp":    in.RewardXP,
				"enabled":      in.Enabled,
				"sort_order":   in.SortOrder,
				"version":      gorm.Expr("version + 1"),
				"updated_at":   s.now(),
			}).Error; err != nil {
			return err
		}
		return writeAudit(tx, auditEntry{
			OperatorID: operatorID, Action: "task.update", EntityType: "daily_task", EntityID: old.ID.String(),
			Reason: "更新每日任务 " + old.Code, Before: before,
			After: map[string]any{"name": in.Name, "target_count": in.TargetCount, "reward_xp": in.RewardXP, "enabled": in.Enabled},
		})
	})
	if err != nil {
		return nil, err
	}
	return s.repo.GetTaskDef(ctx, id)
}

// SetTaskDefEnabled 启用 / 停用每日任务定义。
func (s *Service) SetTaskDefEnabled(ctx context.Context, id string, enabled bool, operatorID string) error {
	def, err := s.repo.GetTaskDef(ctx, id)
	if err != nil {
		return err
	}
	action := "task.disable"
	if enabled {
		action = "task.enable"
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&domain.DailyTaskDefinition{}).Where("id = ?", def.ID).
			Update("enabled", enabled).Error; err != nil {
			return err
		}
		return writeAudit(tx, auditEntry{
			OperatorID: operatorID, Action: action, EntityType: "daily_task", EntityID: def.ID.String(),
			Reason: "切换每日任务 " + def.Code + " 启用状态",
			After:  map[string]any{"code": def.Code, "enabled": enabled},
		})
	})
}

// DeleteTaskDef 删除每日任务定义；已产生用户进度的任务不允许删除（保留历史）。
func (s *Service) DeleteTaskDef(ctx context.Context, id string, operatorID string) error {
	def, err := s.repo.GetTaskDef(ctx, id)
	if err != nil {
		return err
	}
	var used int64
	if err := s.db.WithContext(ctx).Model(&domain.UserDailyTask{}).
		Where("task_code = ?", def.Code).Count(&used).Error; err != nil {
		return err
	}
	if used > 0 {
		return ErrTaskInUse
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", def.ID).Delete(&domain.DailyTaskDefinition{}).Error; err != nil {
			return err
		}
		return writeAudit(tx, auditEntry{
			OperatorID: operatorID, Action: "task.delete", EntityType: "daily_task", EntityID: def.ID.String(),
			Reason: "删除每日任务 " + def.Code, Before: taskSnapshot(def),
		})
	})
}

// ---- 成就定义 ----

// AchievementInput 成就定义创建 / 更新入参。
type AchievementInput struct {
	Code                 string `json:"code"`
	Name                 string `json:"name"`
	Description          string `json:"description"`
	Category             string `json:"category"`
	Rarity               string `json:"rarity"`
	IconURL              string `json:"icon_url"`
	ConditionType        string `json:"condition_type"`
	ConditionEventType   string `json:"condition_event_type"`
	ConditionSubjectType string `json:"condition_subject_type"`
	TargetValue          int64  `json:"target_value"`
	RewardXP             int    `json:"reward_xp"`
	IsHidden             bool   `json:"is_hidden"`
	Enabled              bool   `json:"enabled"`
	SortOrder            int    `json:"sort_order"`
}

// ListAchievementDefs 返回全部成就定义（含停用项）。
func (s *Service) ListAchievementDefs(ctx context.Context) ([]domain.AchievementDefinition, error) {
	return s.repo.ListAchievementDefs(ctx, false)
}

// CreateAchievement 新建成就定义。
func (s *Service) CreateAchievement(ctx context.Context, in AchievementInput, operatorID string) (*domain.AchievementDefinition, error) {
	code, err := normalizeRuleCode(in.Code)
	if err != nil {
		return nil, err
	}
	if err := validateAchievementInput(in); err != nil {
		return nil, err
	}
	var existing int64
	if err := s.db.WithContext(ctx).Model(&domain.AchievementDefinition{}).Where("code = ?", code).Count(&existing).Error; err != nil {
		return nil, err
	}
	if existing > 0 {
		return nil, ErrAchievementCodeTaken
	}
	def := &domain.AchievementDefinition{
		Code: code, Name: strings.TrimSpace(in.Name), Description: strings.TrimSpace(in.Description),
		Category: strings.TrimSpace(in.Category), Rarity: strings.TrimSpace(in.Rarity), IconURL: strings.TrimSpace(in.IconURL),
		ConditionType: in.ConditionType, ConditionEventType: in.ConditionEventType,
		ConditionSubjectType: in.ConditionSubjectType, TargetValue: in.TargetValue, RewardXP: in.RewardXP,
		IsHidden: in.IsHidden, Enabled: in.Enabled, SortOrder: in.SortOrder, Version: 1,
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(def).Error; err != nil {
			return err
		}
		return writeAudit(tx, auditEntry{
			OperatorID: operatorID, Action: "achievement.create", EntityType: "achievement", EntityID: def.ID.String(),
			Reason: "新建成就 " + code, After: achievementSnapshot(def),
		})
	})
	if err != nil {
		return nil, err
	}
	return def, nil
}

// UpdateAchievement 更新成就定义（原地更新并递增版本；已解锁记录保留快照）。
func (s *Service) UpdateAchievement(ctx context.Context, id string, in AchievementInput, operatorID string) (*domain.AchievementDefinition, error) {
	old, err := s.repo.GetAchievementDef(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := validateAchievementInput(in); err != nil {
		return nil, err
	}
	before := achievementSnapshot(old)
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&domain.AchievementDefinition{}).Where("id = ?", old.ID).
			Updates(map[string]any{
				"name":                   strings.TrimSpace(in.Name),
				"description":            strings.TrimSpace(in.Description),
				"category":               strings.TrimSpace(in.Category),
				"rarity":                 strings.TrimSpace(in.Rarity),
				"icon_url":               strings.TrimSpace(in.IconURL),
				"condition_type":         in.ConditionType,
				"condition_event_type":   in.ConditionEventType,
				"condition_subject_type": in.ConditionSubjectType,
				"target_value":           in.TargetValue,
				"reward_xp":              in.RewardXP,
				"is_hidden":              in.IsHidden,
				"enabled":                in.Enabled,
				"sort_order":             in.SortOrder,
				"version":                gorm.Expr("version + 1"),
				"updated_at":             s.now(),
			}).Error; err != nil {
			return err
		}
		return writeAudit(tx, auditEntry{
			OperatorID: operatorID, Action: "achievement.update", EntityType: "achievement", EntityID: old.ID.String(),
			Reason: "更新成就 " + old.Code, Before: before,
			After: map[string]any{"name": in.Name, "condition_type": in.ConditionType, "target_value": in.TargetValue, "enabled": in.Enabled},
		})
	})
	if err != nil {
		return nil, err
	}
	return s.repo.GetAchievementDef(ctx, id)
}

// SetAchievementEnabled 启用 / 停用成就定义（停用不影响已解锁记录）。
func (s *Service) SetAchievementEnabled(ctx context.Context, id string, enabled bool, operatorID string) error {
	def, err := s.repo.GetAchievementDef(ctx, id)
	if err != nil {
		return err
	}
	action := "achievement.disable"
	if enabled {
		action = "achievement.enable"
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&domain.AchievementDefinition{}).Where("id = ?", def.ID).
			Update("enabled", enabled).Error; err != nil {
			return err
		}
		return writeAudit(tx, auditEntry{
			OperatorID: operatorID, Action: action, EntityType: "achievement", EntityID: def.ID.String(),
			Reason: "切换成就 " + def.Code + " 启用状态",
			After:  map[string]any{"code": def.Code, "enabled": enabled},
		})
	})
}

// RecalculateAchievements 重算成就：userID 为空时对全部有成长记录的用户执行。
// 返回本次新解锁的成就总数。用于新增 / 调整成就定义后的历史补算。
func (s *Service) RecalculateAchievements(ctx context.Context, userID, operatorID string) (int, error) {
	ids := []uuid.UUID{}
	if strings.TrimSpace(userID) != "" {
		id, err := uuid.Parse(strings.TrimSpace(userID))
		if err != nil {
			return 0, ErrBadInput
		}
		ids = append(ids, id)
	} else {
		var rows []uuid.UUID
		if err := s.db.WithContext(ctx).Model(&domain.UserGrowth{}).Pluck("user_id", &rows).Error; err != nil {
			return 0, err
		}
		ids = rows
	}
	total := 0
	for _, id := range ids {
		n, err := s.recalculateUser(ctx, id)
		if err != nil {
			return total, err
		}
		total += n
	}
	// 补算属批量运维动作，单独记一条审计。
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return writeAudit(tx, auditEntry{
			OperatorID: operatorID, Action: "achievement.recalculate", EntityType: "achievement",
			TargetUser: strings.TrimSpace(userID),
			Reason:     "重算成就", After: map[string]any{"users": len(ids), "unlocked": total},
		})
	}); err != nil {
		return total, err
	}
	return total, nil
}

// AchievementGrantInput 手动授予成就入参。
type AchievementGrantInput struct {
	TargetUserID  string `json:"target_user_id"`
	AchievementID string `json:"achievement_id"`
	Reason        string `json:"reason"`
	OperatorID    string `json:"-"`
	RequestID     string `json:"-"`
}

// AchievementRevokeInput 手动撤销成就入参。
type AchievementRevokeInput struct {
	TargetUserID  string `json:"target_user_id"`
	AchievementID string `json:"achievement_id"`
	Reason        string `json:"reason"`
	OperatorID    string `json:"-"`
	RequestID     string `json:"-"`
}

// AdminUserAchievementView 后台查看某用户成就状态。
type AdminUserAchievementView struct {
	AchievementID string     `json:"achievement_id"`
	Code          string     `json:"code"`
	Name          string     `json:"name"`
	Category      string     `json:"category"`
	Rarity        string     `json:"rarity"`
	Enabled       bool       `json:"enabled"`
	RewardXP      int        `json:"reward_xp"`
	Unlocked      bool       `json:"unlocked"`
	Revoked       bool       `json:"revoked"`
	GrantSource   string     `json:"grant_source"`
	UnlockedAt    *time.Time `json:"unlocked_at"`
	RevokedAt     *time.Time `json:"revoked_at"`
	RevokeReason  string     `json:"revoke_reason"`
}

// GrantAchievement 手动授予用户成就。
//
// 授予会补发成就定义中的奖励经验（若尚未发过），并写入成长审计。
// 已撤销的成就可重新授予，但不会再次发放奖励经验（账本幂等键兜底）。
func (s *Service) GrantAchievement(ctx context.Context, in AchievementGrantInput) (*domain.UserAchievement, error) {
	targetID, err := s.resolveUserID(ctx, in.TargetUserID)
	if err != nil {
		return nil, err
	}
	def, err := s.repo.GetAchievementDef(ctx, strings.TrimSpace(in.AchievementID))
	if err != nil {
		return nil, err
	}
	reason := strings.TrimSpace(in.Reason)
	if reason == "" {
		return nil, ErrReasonRequired
	}
	if len([]rune(reason)) > 500 {
		return nil, ErrBadInput
	}

	var result domain.UserAchievement
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ua domain.UserAchievement
		if err := tx.Raw(`SELECT * FROM user_achievements
			WHERE user_id = ? AND achievement_id = ? FOR UPDATE`, targetID, def.ID).
			Scan(&ua).Error; err != nil {
			return err
		}
		if ua.ID != uuid.Nil && ua.RevokedAt == nil {
			return ErrAchievementAlreadyUnlocked
		}

		now := s.now()
		if ua.ID == uuid.Nil {
			ua = domain.UserAchievement{
				UserID:                  targetID,
				AchievementID:           def.ID,
				AchievementCodeSnapshot: def.Code,
				NameSnapshot:            def.Name,
				DescriptionSnapshot:     def.Description,
				DefinitionVersion:       def.Version,
				UnlockedAt:              now,
				IsPublic:                true,
				GrantSource:             "manual",
				GrantedByAdminID:        parseUUIDPtr(in.OperatorID),
			}
			if err := tx.Create(&ua).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Model(&domain.UserAchievement{}).Where("id = ?", ua.ID).
				Updates(map[string]any{
					"achievement_code_snapshot": def.Code,
					"name_snapshot":             def.Name,
					"description_snapshot":      def.Description,
					"definition_version":        def.Version,
					"unlocked_at":               now,
					"is_public":                 true,
					"grant_source":              "manual",
					"granted_by_admin_id":       parseUUIDPtr(in.OperatorID),
					"revoked_at":                nil,
					"revoked_by_admin_id":       nil,
					"revoke_reason":             "",
				}).Error; err != nil {
				return err
			}
			ua.AchievementCodeSnapshot = def.Code
			ua.NameSnapshot = def.Name
			ua.DescriptionSnapshot = def.Description
			ua.DefinitionVersion = def.Version
			ua.UnlockedAt = now
			ua.IsPublic = true
			ua.GrantSource = "manual"
			ua.GrantedByAdminID = parseUUIDPtr(in.OperatorID)
			ua.RevokedAt = nil
			ua.RevokedByAdminID = nil
			ua.RevokeReason = ""
		}

		// 成就奖励经验与自动解锁保持一致：同一成就只发一次，撤销不回收。
		if def.RewardXP > 0 {
			key := achievementRewardKey(def.Code, targetID.String())
			absent, err := ledgerAbsent(tx, key)
			if err != nil {
				return err
			}
			if absent {
				if _, err := s.applyLedger(tx, ledgerInput{
					UserID:      targetID,
					DeltaXP:     int64(def.RewardXP),
					SourceType:  domain.LedgerSourceAchievement,
					SourceID:    def.Code,
					Key:         key,
					RuleCode:    def.Code,
					RuleVersion: def.Version,
					Reason:      "手动授予成就：" + def.Name,
					OperatorID:  parseUUIDPtr(in.OperatorID),
				}); err != nil {
					return err
				}
				if err := s.recalcLevel(tx, targetID); err != nil {
					return err
				}
			}
		}

		if err := writeAudit(tx, auditEntry{
			OperatorID: in.OperatorID, Action: "achievement.grant", TargetUser: targetID.String(),
			EntityType: "achievement", EntityID: def.ID.String(), Reason: reason,
			After: map[string]any{
				"code":         def.Code,
				"name":         def.Name,
				"grant_source": "manual",
			},
			RequestID: in.RequestID,
		}); err != nil {
			return err
		}
		result = ua
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// RevokeAchievement 手动撤销用户成就（软删除）。
//
// 撤销保留原始解锁快照与原因，不回收已发放的经验；自动重算不会重新解锁，
// 需要后台再次手动授予。
func (s *Service) RevokeAchievement(ctx context.Context, in AchievementRevokeInput) (*domain.UserAchievement, error) {
	targetID, err := s.resolveUserID(ctx, in.TargetUserID)
	if err != nil {
		return nil, err
	}
	achievementID, err := uuid.Parse(strings.TrimSpace(in.AchievementID))
	if err != nil {
		return nil, ErrBadInput
	}
	reason := strings.TrimSpace(in.Reason)
	if reason == "" {
		return nil, ErrReasonRequired
	}
	if len([]rune(reason)) > 500 {
		return nil, ErrBadInput
	}

	var result domain.UserAchievement
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ua domain.UserAchievement
		if err := tx.Raw(`SELECT * FROM user_achievements
			WHERE user_id = ? AND achievement_id = ? AND revoked_at IS NULL FOR UPDATE`,
			targetID, achievementID).Scan(&ua).Error; err != nil {
			return err
		}
		if ua.ID == uuid.Nil {
			return ErrAchievementNotUnlocked
		}
		now := s.now()
		if err := tx.Model(&domain.UserAchievement{}).Where("id = ?", ua.ID).
			Updates(map[string]any{
				"revoked_at":          now,
				"revoked_by_admin_id": parseUUIDPtr(in.OperatorID),
				"revoke_reason":       reason,
			}).Error; err != nil {
			return err
		}
		ua.RevokedAt = &now
		ua.RevokedByAdminID = parseUUIDPtr(in.OperatorID)
		ua.RevokeReason = reason
		if err := writeAudit(tx, auditEntry{
			OperatorID: in.OperatorID, Action: "achievement.revoke", TargetUser: targetID.String(),
			EntityType: "achievement", EntityID: achievementID.String(), Reason: reason,
			Before: map[string]any{
				"code":         ua.AchievementCodeSnapshot,
				"name":         ua.NameSnapshot,
				"unlocked_at":  ua.UnlockedAt,
				"grant_source": ua.GrantSource,
			},
			After:     map[string]any{"revoked_at": now, "revoke_reason": reason},
			RequestID: in.RequestID,
		}); err != nil {
			return err
		}
		result = ua
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// ListAdminUserAchievements 返回某用户全部成就的解锁 / 撤销状态。
func (s *Service) ListAdminUserAchievements(ctx context.Context, userID string) ([]AdminUserAchievementView, error) {
	id, err := s.resolveUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	defs, err := s.repo.ListAchievementDefs(ctx, false)
	if err != nil {
		return nil, err
	}
	var rows []domain.UserAchievement
	if err := s.db.WithContext(ctx).Where("user_id = ?", id).Find(&rows).Error; err != nil {
		return nil, err
	}
	byAchievement := make(map[uuid.UUID]domain.UserAchievement, len(rows))
	for _, row := range rows {
		byAchievement[row.AchievementID] = row
	}
	out := make([]AdminUserAchievementView, 0, len(defs))
	for _, def := range defs {
		view := AdminUserAchievementView{
			AchievementID: def.ID.String(),
			Code:          def.Code,
			Name:          def.Name,
			Category:      def.Category,
			Rarity:        def.Rarity,
			Enabled:       def.Enabled,
			RewardXP:      def.RewardXP,
		}
		if ua, ok := byAchievement[def.ID]; ok {
			view.GrantSource = ua.GrantSource
			view.UnlockedAt = &ua.UnlockedAt
			if ua.RevokedAt != nil {
				view.Revoked = true
				view.RevokedAt = ua.RevokedAt
				view.RevokeReason = ua.RevokeReason
			} else {
				view.Unlocked = true
			}
		}
		out = append(out, view)
	}
	return out, nil
}

// recalculateUser 在单事务内重算单个用户成就。
func (s *Service) recalculateUser(ctx context.Context, userID uuid.UUID) (int, error) {
	var unlocked int
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		n, err := s.evaluateAchievements(tx, userID, nil)
		if err != nil {
			return err
		}
		unlocked = n
		return nil
	})
	return unlocked, err
}

// ---- 审计 ----

// ListAudit 分页查询成长审计日志。
func (s *Service) ListAudit(ctx context.Context, f AuditFilter) ([]domain.GrowthAuditLog, int64, error) {
	return s.repo.ListAudit(ctx, f)
}

// ---- 内部工具 ----

// resolveUserID 将后台输入解析为普通用户 UUID。
//
// 后台列表默认展示对外随机用户 ID（user_no），因此这里同时接受内部 UUID
// 与对外 ID，避免运营必须从数据库复制内部主键。
func (s *Service) resolveUserID(ctx context.Context, raw string) (uuid.UUID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return uuid.Nil, ErrBadInput
	}
	if id, err := uuid.Parse(raw); err == nil {
		if err := s.ensureUser(ctx, id); err != nil {
			return uuid.Nil, err
		}
		return id, nil
	}
	var row struct {
		ID uuid.UUID
	}
	if err := s.db.WithContext(ctx).Table("accounts").
		Select("id").
		Where("user_no = ? AND role = ?", raw, domain.RoleUser).
		Scan(&row).Error; err != nil {
		return uuid.Nil, err
	}
	if row.ID == uuid.Nil {
		return uuid.Nil, ErrUnknownUser
	}
	return row.ID, nil
}

// ensureUser 校验目标账号存在且为普通用户。
func (s *Service) ensureUser(ctx context.Context, userID uuid.UUID) error {
	var cnt int64
	if err := s.db.WithContext(ctx).Table("accounts").
		Where("id = ? AND role = ?", userID, domain.RoleUser).Count(&cnt).Error; err != nil {
		return err
	}
	if cnt == 0 {
		return ErrUnknownUser
	}
	return nil
}

// findLedgerByKey 按幂等键查询流水；不存在返回 (nil, nil)。
func findLedgerByKey(tx *gorm.DB, key string) (*domain.ExperienceLedger, error) {
	var l domain.ExperienceLedger
	err := tx.Where("idempotency_key = ?", key).First(&l).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

// findLedgerByID 按 ID 查询流水（加行锁，避免并发重复冲正）。
func findLedgerByID(tx *gorm.DB, id string) (*domain.ExperienceLedger, error) {
	var l domain.ExperienceLedger
	err := tx.Raw(`SELECT id, user_id, delta_xp, balance_after, source_type, source_id,
			rule_code, rule_version, idempotency_key
		FROM experience_ledger WHERE id = ? FOR UPDATE`, id).Scan(&l).Error
	if err != nil {
		return nil, err
	}
	if l.ID == uuid.Nil {
		return nil, ErrLedgerNotFound
	}
	return &l, nil
}

// normalizeRuleCode 校验并归一化规则 / 任务 / 成就编码。
func normalizeRuleCode(code string) (string, error) {
	code = strings.TrimSpace(code)
	if code == "" || len(code) > 100 {
		return "", ErrBadInput
	}
	for _, r := range code {
		if !(r == '_' || r == '-' || r == '.' ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return "", ErrBadInput
		}
	}
	return code, nil
}

// validateRuleInput 校验规则字段。
func validateRuleInput(in RuleInput) error {
	if strings.TrimSpace(in.Name) == "" || len(in.Name) > 150 {
		return ErrBadInput
	}
	if !domain.IsValidGrowthEventType(in.EventType) {
		return ErrInvalidEvent
	}
	if in.XPAmount < 0 {
		return ErrBadInput
	}
	if in.DailyLimit != nil && *in.DailyLimit <= 0 {
		return ErrBadInput
	}
	if in.PerSubjectLimit != nil && *in.PerSubjectLimit <= 0 {
		return ErrBadInput
	}
	if in.EffectiveFrom != nil && in.EffectiveUntil != nil && !in.EffectiveUntil.After(*in.EffectiveFrom) {
		return ErrBadInput
	}
	return nil
}

// validateTaskDefInput 校验每日任务定义字段。
func validateTaskDefInput(in TaskDefInput) error {
	if strings.TrimSpace(in.Name) == "" || len(in.Name) > 150 {
		return ErrBadInput
	}
	if !domain.IsValidGrowthEventType(in.EventType) {
		return ErrInvalidEvent
	}
	if in.TargetCount <= 0 || in.RewardXP < 0 {
		return ErrBadInput
	}
	return nil
}

// validateAchievementInput 校验成就定义字段。
func validateAchievementInput(in AchievementInput) error {
	if strings.TrimSpace(in.Name) == "" || len(in.Name) > 150 {
		return ErrBadInput
	}
	if strings.TrimSpace(in.Category) == "" || len(in.Category) > 50 {
		return ErrBadInput
	}
	if !domain.IsValidAchievementCondition(in.ConditionType) {
		return ErrBadCondition
	}
	// event_count / distinct_subject_count 必须指定统计的事件类型，避免全表扫描。
	if (in.ConditionType == domain.AchievementCondEventCount ||
		in.ConditionType == domain.AchievementCondDistinctSubjectCount) &&
		!domain.IsValidGrowthEventType(in.ConditionEventType) {
		return ErrInvalidEvent
	}
	if in.TargetValue <= 0 || in.RewardXP < 0 {
		return ErrBadInput
	}
	if err := validateIconURL(in.IconURL); err != nil {
		return err
	}
	return nil
}

// validateIconURL 限制图标地址：空串、https:// 开头或站内 / 开头路径。
func validateIconURL(url string) error {
	url = strings.TrimSpace(url)
	if url == "" {
		return nil
	}
	if len(url) > 500 {
		return ErrBadIconURL
	}
	if strings.HasPrefix(url, "/") && !strings.HasPrefix(url, "//") {
		return nil
	}
	if strings.HasPrefix(strings.ToLower(url), "https://") {
		return nil
	}
	return ErrBadIconURL
}

// ruleSnapshot 规则审计快照。
func ruleSnapshot(r *domain.GrowthRule) map[string]any {
	if r == nil {
		return nil
	}
	return map[string]any{
		"code": r.Code, "version": r.Version, "name": r.Name, "event_type": r.EventType,
		"xp_amount": r.XPAmount, "daily_limit": r.DailyLimit, "per_subject_limit": r.PerSubjectLimit,
		"enabled": r.Enabled,
	}
}

// taskSnapshot 任务定义审计快照。
func taskSnapshot(d *domain.DailyTaskDefinition) map[string]any {
	if d == nil {
		return nil
	}
	return map[string]any{
		"code": d.Code, "name": d.Name, "event_type": d.EventType,
		"target_count": d.TargetCount, "reward_xp": d.RewardXP, "enabled": d.Enabled, "version": d.Version,
	}
}

// achievementSnapshot 成就定义审计快照。
func achievementSnapshot(d *domain.AchievementDefinition) map[string]any {
	if d == nil {
		return nil
	}
	return map[string]any{
		"code": d.Code, "name": d.Name, "condition_type": d.ConditionType,
		"condition_event_type": d.ConditionEventType, "target_value": d.TargetValue,
		"reward_xp": d.RewardXP, "enabled": d.Enabled, "version": d.Version,
	}
}

// levelSnapshot 等级审计快照。
func levelSnapshot(l *domain.GrowthLevel) map[string]any {
	if l == nil {
		return nil
	}
	return map[string]any{"level": l.Level, "threshold_xp": l.ThresholdXP, "title": l.Title, "enabled": l.Enabled}
}

// itoa 整数转字符串（审计文本用，避免引入 strconv 到多处）。
func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	buf := make([]byte, 0, 12)
	for v > 0 {
		buf = append([]byte{byte('0' + v%10)}, buf...)
		v /= 10
	}
	if neg {
		return "-" + string(buf)
	}
	return string(buf)
}
