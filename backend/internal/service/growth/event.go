package growth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"mckit/backend/internal/domain"
	"mckit/backend/internal/repository/types"
)

// EmitGrowthEvent 实现 domain.GrowthEventSink：业务模块投递已确认事实。
func (s *Service) EmitGrowthEvent(ctx context.Context, ev domain.GrowthEvent) error {
	return s.HandleEvent(ctx, ev)
}

// HandleEvent 处理一个成长事件（jingyan.md §12.1）。
//
// 在单个事务内完成：事件幂等登记 → 规则发奖 → 每日任务推进 → 等级重算 → 成就解锁。
// 任一步失败整体回滚；并发 / 序列化失败做有限次安全重试。
func (s *Service) HandleEvent(ctx context.Context, ev domain.GrowthEvent) error {
	if err := validateEvent(&ev); err != nil {
		return err
	}
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		err = s.handleEventOnce(ctx, ev)
		if err == nil {
			return nil
		}
		if !isRetryable(err) {
			return err
		}
	}
	return err
}

// validateEvent 校验事件必要字段，并补默认发生时间。
func validateEvent(ev *domain.GrowthEvent) error {
	if ev.EventID == uuid.Nil || ev.EventType == "" || ev.SourceModule == "" || ev.ActorUserID == uuid.Nil {
		return ErrInvalidEvent
	}
	if ev.OccurredAt.IsZero() {
		ev.OccurredAt = time.Now()
	}
	if ev.Payload == nil {
		ev.Payload = map[string]any{}
	}
	return nil
}

// handleEventOnce 单事务处理事件。
func (s *Service) handleEventOnce(ctx context.Context, ev domain.GrowthEvent) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. 幂等登记：同一 EventID 重复投递直接返回成功。
		now := s.now()
		rec := domain.GrowthEventRecord{
			EventID:      ev.EventID,
			EventType:    ev.EventType,
			ActorUserID:  &ev.ActorUserID,
			SubjectType:  ev.SubjectType,
			SubjectID:    ev.SubjectID,
			SourceModule: ev.SourceModule,
			OccurredAt:   ev.OccurredAt,
			ProcessedAt:  &now,
			Status:       domain.GrowthEventProcessed,
			Payload:      mustJSONB(ev.Payload),
		}
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rec)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil // 已处理过，幂等成功
		}

		// 1.1 记录活跃日：同一业务日期只计一次，满 7 天后不再重复统计。
		if err := s.refreshActiveDays(tx, ev.ActorUserID); err != nil {
			return err
		}

		date := businessDate(ev.OccurredAt, s.loc)

		// 2. 加载当前生效规则并发放行为经验。
		rules, err := loadActiveRules(tx, ev.EventType, ev.OccurredAt)
		if err != nil {
			return err
		}
		granted := map[string]bool{}
		for _, rule := range rules {
			key, skip := s.ruleKey(rule, ev, date)
			if skip {
				continue
			}
			ok, err := ledgerAbsent(tx, key)
			if err != nil {
				return err
			}
			if !ok {
				continue // 已发过（幂等）
			}
			if rule.DailyLimit != nil {
				cnt, err := countDailyLedger(tx, ev.ActorUserID, rule.Code, date, s.loc)
				if err != nil {
					return err
				}
				if cnt >= int64(*rule.DailyLimit) {
					continue
				}
			}
			if _, err := s.applyLedger(tx, ledgerInput{
				UserID:      ev.ActorUserID,
				DeltaXP:     int64(rule.XPAmount),
				SourceType:  domain.LedgerSourceRule,
				SourceID:    ev.SubjectID,
				EventID:     &ev.EventID,
				Key:         key,
				RuleCode:    rule.Code,
				RuleVersion: rule.Version,
				Reason:      rule.Name,
			}); err != nil {
				return err
			}
			granted[rule.Code] = true
			granted[ev.EventType] = true
		}

		// 3. 推进每日任务（含自动发奖）。
		if err := s.applyDailyTasks(tx, ev, date, len(rules) > 0, granted); err != nil {
			return err
		}

		// 4. 依据最新经验重算等级。
		if err := s.recalcLevel(tx, ev.ActorUserID); err != nil {
			return err
		}

		// 5. 评估成就解锁（含等级成就，需在等级重算之后）。
		if err := s.applyAchievements(tx, ev); err != nil {
			return err
		}
		return nil
	})
}

// ruleKey 依据规则限制策略生成幂等键；skip=true 表示该规则无法应用（缺少目标对象）。
func (s *Service) ruleKey(rule domain.GrowthRule, ev domain.GrowthEvent, date string) (string, bool) {
	actor := ev.ActorUserID.String()
	switch {
	case rule.PerSubjectLimit != nil:
		if ev.SubjectID == "" {
			return "", true // 单对象规则必须有目标对象
		}
		return ruleKeyForSubject(rule.Code, actor, ev.SubjectID), false
	case rule.DailyLimit != nil:
		return ruleKeyForDay(rule.Code, actor, date), false
	default:
		return ruleKeyForEvent(rule.Code, actor, ev.EventID.String()), false
	}
}

// ledgerInput 写入一笔经验流水并更新投影的入参。
type ledgerInput struct {
	UserID      uuid.UUID
	DeltaXP     int64
	SourceType  string
	SourceID    string
	EventID     *uuid.UUID
	Key         string
	RuleCode    string
	RuleVersion int
	Reason      string
	OperatorID  *uuid.UUID
}

// applyLedger 在事务内写入经验流水并更新 user_growth 投影（行锁保护）。
// 经验余额不得低于 0；若负向调整会导致余额为负，返回 ErrBadInput。
func (s *Service) applyLedger(tx *gorm.DB, in ledgerInput) (*domain.ExperienceLedger, error) {
	g, err := lockGrowth(tx, in.UserID)
	if err != nil {
		return nil, err
	}
	newXP := g.CurrentXP + in.DeltaXP
	if newXP < 0 {
		return nil, ErrBadInput
	}
	if in.DeltaXP > 0 {
		g.LifetimeEarnedXP += in.DeltaXP
	} else if in.DeltaXP < 0 {
		g.LifetimeReversedXP += -in.DeltaXP
	}
	if err := tx.Model(&domain.UserGrowth{}).Where("user_id = ?", in.UserID).
		Updates(map[string]any{
			"current_xp":           newXP,
			"lifetime_earned_xp":   g.LifetimeEarnedXP,
			"lifetime_reversed_xp": g.LifetimeReversedXP,
			"version":              gorm.Expr("version + 1"),
			"updated_at":           s.now(),
		}).Error; err != nil {
		return nil, err
	}
	entry := domain.ExperienceLedger{
		UserID:         in.UserID,
		DeltaXP:        in.DeltaXP,
		BalanceAfter:   newXP,
		SourceType:     in.SourceType,
		SourceID:       in.SourceID,
		EventID:        in.EventID,
		IdempotencyKey: in.Key,
		RuleCode:       in.RuleCode,
		RuleVersion:    in.RuleVersion,
		Reason:         in.Reason,
		OperatorUserID: in.OperatorID,
	}
	if err := tx.Create(&entry).Error; err != nil {
		return nil, err
	}
	return &entry, nil
}

// lockGrowth 确保成长投影行存在并加行锁（SELECT ... FOR UPDATE）。
func lockGrowth(tx *gorm.DB, userID uuid.UUID) (*domain.UserGrowth, error) {
	// 先幂等建行（并发下 ON CONFLICT DO NOTHING 不报错），再加锁读取。
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).
		Create(&domain.UserGrowth{UserID: userID}).Error; err != nil {
		return nil, err
	}
	var g domain.UserGrowth
	if err := tx.Raw(`SELECT user_id, current_xp, lifetime_earned_xp, lifetime_reversed_xp,
		level, level_override, active_days, version
		FROM user_growth WHERE user_id = ? FOR UPDATE`, userID).Scan(&g).Error; err != nil {
		return nil, err
	}
	if g.UserID == uuid.Nil {
		return nil, errors.New("成长投影行缺失")
	}
	return &g, nil
}

// refreshActiveDays 统计用户累计活跃天数并写入投影。
//
// 活跃日口径：growth_events 中同一业务日期（站点时区）至少有一个已处理事件。
// 注册、登录、收藏、工具使用等都会产生事件；只读页面刷新不会。满 7 天后
// 不再查询历史，避免老用户每次事件都做一次去重统计。
func (s *Service) refreshActiveDays(tx *gorm.DB, userID uuid.UUID) error {
	g, err := lockGrowth(tx, userID)
	if err != nil {
		return err
	}
	if g.ActiveDays >= GuestActiveDaysRequired {
		return nil
	}
	var count int
	if err := tx.Raw(`SELECT COUNT(DISTINCT (occurred_at AT TIME ZONE ?)::date)
		FROM growth_events
		WHERE actor_user_id = ? AND status = ?`,
		s.loc.String(), userID, domain.GrowthEventProcessed).Scan(&count).Error; err != nil {
		return err
	}
	if count > GuestActiveDaysRequired {
		count = GuestActiveDaysRequired
	}
	if count <= g.ActiveDays {
		return nil
	}
	return tx.Model(&domain.UserGrowth{}).Where("user_id = ?", userID).
		Updates(map[string]any{"active_days": count, "updated_at": s.now()}).Error
}

// recalcLevel 依据当前经验 / 人工覆盖 / 活跃天数重算并写入等级。
func (s *Service) recalcLevel(tx *gorm.DB, userID uuid.UUID) error {
	g, err := lockGrowth(tx, userID)
	if err != nil {
		return err
	}
	levels, err := loadLevels(tx)
	if err != nil {
		return err
	}
	level := effectiveLevel(levels, g.CurrentXP, g.LevelOverride, g.ActiveDays)
	if level == g.Level {
		return nil
	}
	return tx.Model(&domain.UserGrowth{}).Where("user_id = ?", userID).
		Update("level", level).Error
}

// loadLevels 读取全部等级（按阈值升序、同阈值按等级号升序）。
func loadLevels(tx *gorm.DB) ([]domain.GrowthLevel, error) {
	var out []domain.GrowthLevel
	err := tx.Order("threshold_xp ASC, level ASC").Find(&out).Error
	return out, err
}

// loadActiveRules 加载某事件类型在指定时间生效的规则：每个编码取最高版本。
func loadActiveRules(tx *gorm.DB, eventType string, at time.Time) ([]domain.GrowthRule, error) {
	var rows []domain.GrowthRule
	if err := tx.Where("event_type = ? AND enabled = true", eventType).
		Order("code ASC, version DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := make([]domain.GrowthRule, 0, len(rows))
	for _, r := range rows {
		if seen[r.Code] {
			continue
		}
		if r.EffectiveFrom != nil && at.Before(*r.EffectiveFrom) {
			continue
		}
		if r.EffectiveUntil != nil && !at.Before(*r.EffectiveUntil) {
			continue
		}
		seen[r.Code] = true
		out = append(out, r)
	}
	return out, nil
}

// ledgerAbsent 判断幂等键是否尚未存在（true 表示可发奖）。
func ledgerAbsent(tx *gorm.DB, key string) (bool, error) {
	var cnt int64
	if err := tx.Model(&domain.ExperienceLedger{}).
		Where("idempotency_key = ?", key).Count(&cnt).Error; err != nil {
		return false, err
	}
	return cnt == 0, nil
}

// countDailyLedger 统计某规则在某业务日期的正向发奖笔数（用于每日上限）。
func countDailyLedger(tx *gorm.DB, userID uuid.UUID, ruleCode, date string, loc *time.Location) (int64, error) {
	start, end, err := dayRange(date, loc)
	if err != nil {
		return 0, err
	}
	var cnt int64
	err = tx.Model(&domain.ExperienceLedger{}).
		Where("user_id = ? AND rule_code = ? AND delta_xp > 0 AND created_at >= ? AND created_at < ?",
			userID, ruleCode, start, end).Count(&cnt).Error
	return cnt, err
}

// mustJSONB 将 payload 序列化为 JSONB。
func mustJSONB(m map[string]any) types.JSONB {
	if m == nil {
		return types.JSONB("{}")
	}
	b, err := types.JSONMap(m).Value()
	if err != nil {
		return types.JSONB("{}")
	}
	s, _ := b.(string)
	if s == "" {
		return types.JSONB("{}")
	}
	return types.JSONB(s)
}

// isRetryable 判断错误是否可安全重放（序列化失败 / 死锁）。
func isRetryable(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "40001" || pgErr.Code == "40P01"
	}
	return false
}
