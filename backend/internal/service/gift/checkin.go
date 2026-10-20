package gift

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"mckit/backend/internal/domain"
)

// CheckinStatus 今日签到状态。
type CheckinStatus struct {
	Date      string `json:"checkin_date"`
	Reward    int64  `json:"checkin_reward"`
	CheckedIn bool   `json:"checked_in_today"`
}

// CheckinResult 签到结果（含签到后的钱包）。
type CheckinResult struct {
	Date      string      `json:"checkin_date"`
	Reward    int64       `json:"reward"`
	CheckedIn bool        `json:"checked_in"`
	Wallet    *WalletView `json:"wallet"`
}

// CheckinStatus 查询当前用户今日签到状态。
func (s *Service) CheckinStatus(ctx context.Context, userID string) (*CheckinStatus, error) {
	id, err := uuid.Parse(strings.TrimSpace(userID))
	if err != nil {
		return nil, ErrBadInput
	}
	return s.checkinStatus(ctx, id)
}

// checkinStatus 基于账号 UUID 查询今日签到状态（供 Overview 复用）。
func (s *Service) checkinStatus(ctx context.Context, userID uuid.UUID) (*CheckinStatus, error) {
	date := s.businessDate()
	reward, err := s.checkinReward(ctx)
	if err != nil {
		return nil, err
	}
	var cnt int64
	if err := s.db.WithContext(ctx).Model(&domain.GiftCheckin{}).
		Where("user_id = ? AND checkin_date = ?", userID, date).
		Count(&cnt).Error; err != nil {
		return nil, err
	}
	return &CheckinStatus{Date: date.Format("2006-01-02"), Reward: reward, CheckedIn: cnt > 0}, nil
}

// Checkin 每日签到：每人每天一条记录，发放签到礼物币并写账本。
//
// 重复签到返回 ErrAlreadyCheckedIn；奖励为 0 时仍记录签到，便于运营统计。
func (s *Service) Checkin(ctx context.Context, userID string) (*CheckinResult, error) {
	id, err := uuid.Parse(strings.TrimSpace(userID))
	if err != nil {
		return nil, ErrBadInput
	}
	if err := s.ensureUser(ctx, id); err != nil {
		return nil, err
	}
	date := s.businessDate()
	reward, err := s.checkinReward(ctx)
	if err != nil {
		return nil, err
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&domain.GiftCheckin{
			UserID:      id,
			CheckinDate: date,
			Reward:      reward,
		})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrAlreadyCheckedIn
		}
		if reward <= 0 {
			return nil
		}
		w, err := lockWallet(tx, id)
		if err != nil {
			return err
		}
		newBalance := w.CheckinBalance + reward
		if err := updateCoinBalance(tx, id, domain.GiftCoinCheckin, newBalance, s.now()); err != nil {
			return err
		}
		row := domain.GiftLedger{
			UserID:           id,
			Amount:           reward,
			CoinType:         domain.GiftCoinCheckin,
			CoinBalanceAfter: newBalance,
			EventType:        domain.GiftEventDailyCheckin,
			SourceType:       "daily_checkin",
			SourceID:         date.Format("2006-01-02"),
			IdempotencyKey:   "checkin:" + date.Format("2006-01-02"),
			Reason:           "每日签到奖励",
			RequestID:        "",
		}
		return tx.Create(&row).Error
	})
	if err != nil {
		return nil, err
	}
	wallet, err := s.GetWallet(ctx, id.String())
	if err != nil {
		return nil, err
	}
	return &CheckinResult{
		Date:      date.Format("2006-01-02"),
		Reward:    reward,
		CheckedIn: true,
		Wallet:    wallet,
	}, nil
}

// businessDate 返回站点业务时区下的「今天」零点（用于 DATE 列）。
func (s *Service) businessDate() time.Time {
	now := s.now().In(s.loc)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}
