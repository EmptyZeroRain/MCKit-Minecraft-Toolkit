package gift

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

// RedeemInput 兑换礼物入参。
type RedeemInput struct {
	UserID     string
	GiftID     string
	RequestKey string
}

// RedemptionView 兑换记录视图。
type RedemptionView struct {
	ID           string    `json:"id"`
	GiftID       string    `json:"gift_id"`
	GiftCode     string    `json:"gift_code"`
	GiftName     string    `json:"gift_name"`
	GiftIcon     string    `json:"gift_icon"`
	Price        int64     `json:"price"`
	CheckinSpent int64     `json:"checkin_spent"`
	GeneralSpent int64     `json:"general_spent"`
	CreatedAt    time.Time `json:"created_at"`
}

// RedeemResult 兑换结果（含最新钱包）。
type RedeemResult struct {
	Redemption RedemptionView `json:"redemption"`
	Wallet     *WalletView    `json:"wallet"`
}

// Redeem 使用礼物币兑换一个礼物。
//
// 扣减顺序：
//   - 礼物允许签到币（allow_checkin = true）：先扣签到币，不足部分扣通用币；
//   - 不允许：只能扣通用币。
//
// 同一用户 + request_key 重复提交只会兑换一次。
func (s *Service) Redeem(ctx context.Context, in RedeemInput) (*RedeemResult, error) {
	userID, err := uuid.Parse(strings.TrimSpace(in.UserID))
	if err != nil {
		return nil, ErrBadInput
	}
	giftID, err := uuid.Parse(strings.TrimSpace(in.GiftID))
	if err != nil {
		return nil, ErrGiftNotFound
	}
	if err := s.ensureUser(ctx, userID); err != nil {
		return nil, err
	}
	requestKey := strings.TrimSpace(in.RequestKey)
	if requestKey == "" {
		requestKey = uuid.NewString()
	}
	if len(requestKey) > 100 {
		return nil, ErrBadInput
	}

	var redemption *domain.GiftRedemption
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		w, err := lockWallet(tx, userID)
		if err != nil {
			return err
		}
		existing, err := findRedemptionByKey(tx, userID, requestKey)
		if err != nil {
			return err
		}
		if existing != nil {
			redemption = existing
			return nil
		}

		// 锁定礼物行，串行化同一礼物的库存校验，避免并发超兑。
		var gift domain.GiftDefinition
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", giftID).First(&gift).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrGiftNotFound
			}
			return err
		}
		if !gift.Enabled {
			return ErrGiftDisabled
		}
		if gift.Stock > 0 {
			var redeemed int64
			if err := tx.Model(&domain.GiftRedemption{}).
				Where("gift_id = ?", gift.ID).Count(&redeemed).Error; err != nil {
				return err
			}
			if redeemed >= gift.Stock {
				return ErrGiftSoldOut
			}
		}

		checkinSpent, generalSpent := splitPayment(&gift, w)
		if w.GeneralBalance < generalSpent {
			return ErrInsufficientBalance
		}
		newCheckin := w.CheckinBalance - checkinSpent
		newGeneral := w.GeneralBalance - generalSpent
		if err := tx.Model(&domain.GiftWallet{}).Where("user_id = ?", userID).
			Updates(map[string]any{
				"checkin_balance": newCheckin,
				"general_balance": newGeneral,
				"version":         gorm.Expr("version + 1"),
				"updated_at":      s.now(),
			}).Error; err != nil {
			return err
		}

		row := domain.GiftRedemption{
			ID:               uuid.New(),
			UserID:           userID,
			GiftID:           gift.ID,
			GiftCodeSnapshot: gift.Code,
			GiftNameSnapshot: gift.Name,
			GiftIconSnapshot: gift.Icon,
			Price:            gift.Price,
			CheckinSpent:     checkinSpent,
			GeneralSpent:     generalSpent,
			IdempotencyKey:   requestKey,
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		redemption = &row

		reason := "兑换礼物：" + gift.Name
		if checkinSpent > 0 {
			if err := tx.Create(&domain.GiftLedger{
				UserID:           userID,
				Amount:           -checkinSpent,
				CoinType:         domain.GiftCoinCheckin,
				CoinBalanceAfter: newCheckin,
				EventType:        domain.GiftEventGiftRedeem,
				SourceType:       "gift_redemption",
				SourceID:         row.ID.String(),
				IdempotencyKey:   "redeem:" + requestKey + ":checkin",
				Reason:           reason,
			}).Error; err != nil {
				return err
			}
		}
		if generalSpent > 0 {
			if err := tx.Create(&domain.GiftLedger{
				UserID:           userID,
				Amount:           -generalSpent,
				CoinType:         domain.GiftCoinGeneral,
				CoinBalanceAfter: newGeneral,
				EventType:        domain.GiftEventGiftRedeem,
				SourceType:       "gift_redemption",
				SourceID:         row.ID.String(),
				IdempotencyKey:   "redeem:" + requestKey + ":general",
				Reason:           reason,
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	wallet, err := s.GetWallet(ctx, userID.String())
	if err != nil {
		return nil, err
	}
	return &RedeemResult{Redemption: redemptionView(*redemption), Wallet: wallet}, nil
}

// ListSelfRedemptions 返回本人兑换记录（分页）。
func (s *Service) ListSelfRedemptions(ctx context.Context, userID string, page, pageSize int) ([]RedemptionView, int64, error) {
	id, err := uuid.Parse(strings.TrimSpace(userID))
	if err != nil {
		return nil, 0, ErrBadInput
	}
	page, pageSize = normalizePage(page, pageSize)
	q := s.db.WithContext(ctx).Model(&domain.GiftRedemption{}).Where("user_id = ?", id)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []domain.GiftRedemption
	if err := q.Order("created_at DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]RedemptionView, 0, len(rows))
	for _, row := range rows {
		out = append(out, redemptionView(row))
	}
	return out, total, nil
}

// splitPayment 计算签到币 / 通用币分摊；通用币不足由调用方统一报错。
func splitPayment(gift *domain.GiftDefinition, w *domain.GiftWallet) (checkinSpent, generalSpent int64) {
	if !gift.AllowCheckin {
		return 0, gift.Price
	}
	checkinSpent = w.CheckinBalance
	if checkinSpent > gift.Price {
		checkinSpent = gift.Price
	}
	return checkinSpent, gift.Price - checkinSpent
}

func findRedemptionByKey(tx *gorm.DB, userID uuid.UUID, key string) (*domain.GiftRedemption, error) {
	var row domain.GiftRedemption
	err := tx.Where("user_id = ? AND idempotency_key = ?", userID, key).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func redemptionView(row domain.GiftRedemption) RedemptionView {
	return RedemptionView{
		ID:           row.ID.String(),
		GiftID:       row.GiftID.String(),
		GiftCode:     row.GiftCodeSnapshot,
		GiftName:     row.GiftNameSnapshot,
		GiftIcon:     row.GiftIconSnapshot,
		Price:        row.Price,
		CheckinSpent: row.CheckinSpent,
		GeneralSpent: row.GeneralSpent,
		CreatedAt:    row.CreatedAt,
	}
}
