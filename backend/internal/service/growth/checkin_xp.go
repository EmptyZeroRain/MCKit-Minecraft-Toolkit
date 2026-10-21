package growth

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"mckit/backend/internal/domain"
)

// checkinXP 签到经验事件参数。
//
// 事件由礼物服务在签到事务内产生，成长模块只负责校验、入账和审计；
// 签到算法（基础 / 连续 / 里程碑 / 断签惩罚）由礼物服务依据后台配置计算。
type checkinXP struct {
	Date        string
	Kind        string
	RequestKey  string
	StreakDay   int
	BaseXP      int64
	StreakXP    int64
	MilestoneXP int64
	PenaltyXP   int64
}

// applyCheckinXP 处理签到 / 补签事件的成长入账。
//
// 正向经验与断签惩罚分别写入流水，便于后台审计；惩罚最多扣到 0，
// 不会因为余额不足导致签到失败。经验明细同时写入 gift_checkin_xp，
// 供补签卡查询最近漏签日和前台展示。
func (s *Service) applyCheckinXP(tx *gorm.DB, ev domain.GrowthEvent) error {
	in, err := s.parseCheckinXP(ev)
	if err != nil {
		return err
	}

	gross := in.BaseXP + in.StreakXP + in.MilestoneXP
	if gross > 0 {
		reason := fmt.Sprintf("每日签到（连续 %d 天）", in.StreakDay)
		ruleCode := "checkin.actual"
		if in.Kind == domain.CheckinXPKindMakeup {
			reason = fmt.Sprintf("补签 %s", in.Date)
			ruleCode = "checkin.makeup"
		}
		if _, err := s.applyLedger(tx, ledgerInput{
			UserID:     ev.ActorUserID,
			DeltaXP:    gross,
			SourceType: domain.LedgerSourceCheckin,
			SourceID:   in.Date,
			EventID:    &ev.EventID,
			Key:        fmt.Sprintf("checkin:%s:%s:xp", in.Kind, in.Date),
			RuleCode:   ruleCode,
			Reason:     reason,
		}); err != nil {
			return err
		}
	}

	// 断签惩罚：只扣当前可用经验，避免经验为负。
	appliedPenalty := int64(0)
	if in.PenaltyXP > 0 {
		g, err := lockGrowth(tx, ev.ActorUserID)
		if err != nil {
			return err
		}
		appliedPenalty = in.PenaltyXP
		if appliedPenalty > g.CurrentXP {
			appliedPenalty = g.CurrentXP
		}
		if appliedPenalty > 0 {
			if _, err := s.applyLedger(tx, ledgerInput{
				UserID:     ev.ActorUserID,
				DeltaXP:    -appliedPenalty,
				SourceType: domain.LedgerSourceCheckin,
				SourceID:   in.Date,
				EventID:    &ev.EventID,
				Key:        fmt.Sprintf("checkin:break:%s:penalty", in.Date),
				RuleCode:   "checkin.break",
				Reason:     "断签惩罚",
			}); err != nil {
				return err
			}
		}
	}

	rows := []domain.GiftCheckinXP{{
		UserID:      ev.ActorUserID,
		XpDate:      mustDate(in.Date, s.loc),
		Kind:        in.Kind,
		RequestKey:  in.RequestKey,
		StreakDay:   in.StreakDay,
		BaseXP:      in.BaseXP,
		StreakXP:    in.StreakXP,
		MilestoneXP: in.MilestoneXP,
		PenaltyXP:   0,
		TotalXP:     gross,
	}}
	if appliedPenalty > 0 {
		rows = append(rows, domain.GiftCheckinXP{
			UserID:    ev.ActorUserID,
			XpDate:    mustDate(in.Date, s.loc),
			Kind:      domain.CheckinXPKindBreak,
			StreakDay: in.StreakDay,
			PenaltyXP: appliedPenalty,
			TotalXP:   -appliedPenalty,
		})
	}
	if err := tx.Create(&rows).Error; err != nil {
		return err
	}
	return nil
}

// parseCheckinXP 解析并校验签到事件 payload。
func (s *Service) parseCheckinXP(ev domain.GrowthEvent) (checkinXP, error) {
	var in checkinXP
	in.Date, _ = payloadString(ev.Payload, "xp_date")
	in.Kind, _ = payloadString(ev.Payload, "kind")
	in.RequestKey, _ = payloadString(ev.Payload, "request_key")
	streakDay, ok := payloadInt64(ev.Payload, "streak_day")
	if !ok {
		return in, ErrInvalidEvent
	}
	in.StreakDay = int(streakDay)
	in.BaseXP, _ = payloadInt64(ev.Payload, "base_xp")
	in.StreakXP, _ = payloadInt64(ev.Payload, "streak_xp")
	in.MilestoneXP, _ = payloadInt64(ev.Payload, "milestone_xp")
	in.PenaltyXP, _ = payloadInt64(ev.Payload, "penalty_xp")

	if in.Date == "" || in.StreakDay < 1 || in.StreakDay > 1_000_000 || len(in.RequestKey) > 128 {
		return in, ErrInvalidEvent
	}
	if in.BaseXP < 0 || in.BaseXP > 1_000_000 ||
		in.StreakXP < 0 || in.StreakXP > 1_000_000 ||
		in.MilestoneXP < 0 || in.MilestoneXP > 1_000_000 ||
		in.PenaltyXP < 0 || in.PenaltyXP > 1_000_000 {
		return in, ErrInvalidEvent
	}
	if _, err := time.ParseInLocation("2006-01-02", in.Date, s.loc); err != nil {
		return in, ErrInvalidEvent
	}
	today := businessDate(s.now(), s.loc)
	switch ev.EventType {
	case domain.EventGiftCheckin:
		if in.Kind != domain.CheckinXPKindActual {
			return in, ErrInvalidEvent
		}
		if in.Date != businessDate(ev.OccurredAt, s.loc) {
			return in, ErrInvalidEvent
		}
	case domain.EventGiftMakeupCheckin:
		if in.Kind != domain.CheckinXPKindMakeup || in.PenaltyXP != 0 {
			return in, ErrInvalidEvent
		}
		if in.Date >= today {
			return in, ErrInvalidEvent
		}
	default:
		return in, ErrInvalidEvent
	}
	return in, nil
}

// payloadString 读取字符串字段。
func payloadString(m map[string]any, key string) (string, bool) {
	if m == nil {
		return "", false
	}
	v, ok := m[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return strings.TrimSpace(s), ok
}

// payloadInt64 读取整数字段，兼容 Go 原生整数、JSON float64 与 json.Number。
func payloadInt64(m map[string]any, key string) (int64, bool) {
	if m == nil {
		return 0, false
	}
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int8:
		return int64(n), true
	case int16:
		return int64(n), true
	case int32:
		return int64(n), true
	case int64:
		return n, true
	case uint:
		return int64(n), true
	case uint8:
		return int64(n), true
	case uint16:
		return int64(n), true
	case uint32:
		return int64(n), true
	case uint64:
		if n > uint64(^uint64(0)>>1) {
			return 0, false
		}
		return int64(n), true
	case float64:
		if n != float64(int64(n)) {
			return 0, false
		}
		return int64(n), true
	case json.Number:
		i, err := n.Int64()
		return i, err == nil
	case string:
		i, err := strconv.ParseInt(strings.TrimSpace(n), 10, 64)
		return i, err == nil
	default:
		return 0, false
	}
}

// mustDate 将 YYYY-MM-DD 转为业务时区日期；调用前已完成格式校验。
func mustDate(date string, loc *time.Location) time.Time {
	t, err := time.ParseInLocation("2006-01-02", date, loc)
	if err != nil {
		return time.Time{}
	}
	return t
}
