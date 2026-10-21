// Package gift 实现礼物币钱包、签到、礼物目录与兑换。
//
// 设计约束（GiftCoins.md）：
//   - 余额只能通过 gift_ledger 流水变更，禁止直接改余额；
//   - 扣减不得使余额为负；
//   - 同一用户 + 幂等键只产生一笔流水；
//   - 后台调整必须填写原因并记录操作人；
//   - 签到币与通用币分账：礼物可按是否允许签到币兑换配置扣减顺序。
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

// MaxAdjustAmount 单次后台调整上限，避免误操作产生极端数值。
const MaxAdjustAmount int64 = 1_000_000

// 业务时区默认值（与成长系统保持一致）。
const defaultTimezone = "Asia/Shanghai"

var (
	ErrBadInput            = errors.New("输入不合法")
	ErrReasonRequired      = errors.New("必须填写操作原因")
	ErrUnknownUser         = errors.New("目标用户不存在")
	ErrInsufficientBalance = errors.New("礼物币余额不足")
	ErrAmountLimit         = errors.New("调整金额超出允许范围")
	ErrBadCoinType         = errors.New("礼物币类型不合法")
	ErrGiftNotFound        = errors.New("礼物不存在")
	ErrGiftDisabled        = errors.New("该礼物已下架")
	ErrGiftSoldOut         = errors.New("礼物库存不足")
	ErrGiftInUse           = errors.New("该礼物已有兑换记录，只能停用，不能删除")
	ErrAlreadyCheckedIn    = errors.New("今天已经签到过了")
	ErrMakeupCardNotEnough = errors.New("补签卡不足")
	ErrNoMissedCheckin     = errors.New("没有可补签的漏签日")
	ErrCheckinConfig       = errors.New("签到经验配置不合法：经验值需在 0-1000000 之间，补签卡价格需在 1-1000000 之间")
	ErrMilestoneConfig     = errors.New("里程碑配置不合法：连续天数需为 1-1000000 的唯一整数，奖励经验需在 0-1000000 之间")
)

// GrowthEventTxSink 在调用方事务内处理成长事件。
//
// 签到服务需要把签到币、签到记录与签到经验放在同一数据库事务中；
// 由 main 注入 growth.Service 实现，避免礼物包直接依赖成长包内部实现。
type GrowthEventTxSink interface {
	HandleEventTx(ctx context.Context, tx *gorm.DB, ev domain.GrowthEvent) error
}

// Service 礼物币钱包服务。
type Service struct {
	db     *gorm.DB
	now    func() time.Time
	loc    *time.Location
	growth GrowthEventTxSink
}

// NewService 创建礼物币服务。
func NewService(db *gorm.DB) *Service {
	return &Service{db: db, now: time.Now, loc: loadLocation(defaultTimezone)}
}

// WithTimezone 覆盖签到业务时区（空 / 非法时回退 Asia/Shanghai）。
func (s *Service) WithTimezone(name string) *Service {
	s.loc = loadLocation(name)
	return s
}

// WithGrowthSink 注入成长事件接收端；未注入时签到仍可用，但不发放签到经验。
func (s *Service) WithGrowthSink(sink GrowthEventTxSink) *Service {
	s.growth = sink
	return s
}

func loadLocation(name string) *time.Location {
	name = strings.TrimSpace(name)
	if name == "" {
		name = defaultTimezone
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		loc, err = time.LoadLocation(defaultTimezone)
		if err != nil {
			return time.UTC
		}
	}
	return loc
}

// WalletView 钱包展示信息。
type WalletView struct {
	UserID         string     `json:"user_id"`
	UserNo         string     `json:"user_no"`
	Username       string     `json:"username"`
	GeneralBalance int64      `json:"general_balance"`
	CheckinBalance int64      `json:"checkin_balance"`
	TotalBalance   int64      `json:"total_balance"`
	UpdatedAt      *time.Time `json:"updated_at"`
}

// LedgerView 用户本人可见的流水视图（不暴露操作人、幂等键等内部字段）。
type LedgerView struct {
	ID           uuid.UUID `json:"id"`
	Amount       int64     `json:"amount"`
	CoinType     string    `json:"coin_type"`
	BalanceAfter int64     `json:"balance_after"`
	EventType    string    `json:"event_type"`
	Reason       string    `json:"reason"`
	CreatedAt    time.Time `json:"created_at"`
}

// Overview 用户本人钱包概览（含最近流水与今日签到状态）。
type Overview struct {
	WalletView
	Recent           []LedgerView `json:"recent"`
	CheckinReward    int64        `json:"checkin_reward"`
	CheckedIn        bool         `json:"checked_in_today"`
	CheckinDate      string       `json:"checkin_date"`
	CheckinStreak    int          `json:"checkin_streak"`
	LongestStreak    int          `json:"checkin_longest_streak"`
	NextStreak       int          `json:"next_checkin_streak"`
	TodayXP          int64        `json:"checkin_today_xp"`
	BaseXP           int64        `json:"checkin_base_xp"`
	StreakXP         int64        `json:"checkin_streak_xp"`
	MilestoneXP      int64        `json:"checkin_milestone_xp"`
	PenaltyXP        int64        `json:"checkin_penalty_xp"`
	MakeupCards      int64        `json:"makeup_cards"`
	MakeupCardPrice  int64        `json:"makeup_card_price"`
	LatestMissedDate string       `json:"latest_missed_date"`
}

// AdjustInput 后台调整入参。
type AdjustInput struct {
	TargetUserID string
	// CoinType 为空时按通用币处理，兼容旧调用。
	CoinType   string
	Amount     int64
	Reason     string
	RequestKey string
	OperatorID string
	RequestID  string
}

// LedgerFilter 礼物币流水筛选。
type LedgerFilter struct {
	UserID    string
	CoinType  string
	EventType string
	Page      int
	PageSize  int
}

// GetWallet 查询指定用户钱包；钱包不存在时返回 0 余额。
func (s *Service) GetWallet(ctx context.Context, userID string) (*WalletView, error) {
	id, err := s.resolveUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	var acc struct {
		UserNo   string
		Username string
	}
	if err := s.db.WithContext(ctx).Table("accounts").
		Select("user_no, username").
		Where("id = ?", id).Scan(&acc).Error; err != nil {
		return nil, err
	}
	if acc.Username == "" {
		return nil, ErrUnknownUser
	}
	view := &WalletView{UserID: id.String(), UserNo: acc.UserNo, Username: acc.Username}
	var w domain.GiftWallet
	err = s.db.WithContext(ctx).Where("user_id = ?", id).First(&w).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return view, nil
	}
	if err != nil {
		return nil, err
	}
	view.GeneralBalance = w.GeneralBalance
	view.CheckinBalance = w.CheckinBalance
	view.TotalBalance = w.GeneralBalance + w.CheckinBalance
	view.UpdatedAt = &w.UpdatedAt
	return view, nil
}

// SelfOverview 返回本人钱包、最近 5 条流水与今日签到状态。
func (s *Service) SelfOverview(ctx context.Context, userID string) (*Overview, error) {
	wallet, err := s.GetWallet(ctx, userID)
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(wallet.UserID)
	if err != nil {
		return nil, ErrBadInput
	}
	var rows []domain.GiftLedger
	if err := s.db.WithContext(ctx).Where("user_id = ?", id).
		Order("created_at DESC").Limit(5).Find(&rows).Error; err != nil {
		return nil, err
	}
	recent := make([]LedgerView, 0, len(rows))
	for _, row := range rows {
		recent = append(recent, publicLedger(row))
	}
	status, err := s.checkinStatus(ctx, id)
	if err != nil {
		return nil, err
	}
	return &Overview{
		WalletView:       *wallet,
		Recent:           recent,
		CheckinReward:    status.Reward,
		CheckedIn:        status.CheckedIn,
		CheckinDate:      status.Date,
		CheckinStreak:    status.Streak,
		LongestStreak:    status.LongestStreak,
		NextStreak:       status.NextStreak,
		TodayXP:          status.TodayXP,
		BaseXP:           status.BaseXP,
		StreakXP:         status.StreakXP,
		MilestoneXP:      status.MilestoneXP,
		PenaltyXP:        status.PenaltyXP,
		MakeupCards:      status.MakeupCards,
		MakeupCardPrice:  status.MakeupCardPrice,
		LatestMissedDate: status.LatestMissedDate,
	}, nil
}

// ListSelfLedger 返回本人礼物币流水（已脱敏）。
func (s *Service) ListSelfLedger(ctx context.Context, userID string, f LedgerFilter) ([]LedgerView, int64, error) {
	id, err := uuid.Parse(strings.TrimSpace(userID))
	if err != nil {
		return nil, 0, ErrBadInput
	}
	f.UserID = id.String()
	rows, total, err := s.ListLedger(ctx, f)
	if err != nil {
		return nil, 0, err
	}
	out := make([]LedgerView, 0, len(rows))
	for _, row := range rows {
		out = append(out, publicLedger(row))
	}
	return out, total, nil
}

// Adjust 后台受控增减礼物币，写账本并返回流水。
//
// coin_type 省略时按通用币处理；签到币与通用币分别校验余额。
func (s *Service) Adjust(ctx context.Context, in AdjustInput) (*domain.GiftLedger, error) {
	targetID, err := s.resolveUserID(ctx, in.TargetUserID)
	if err != nil {
		return nil, err
	}
	if in.Amount == 0 {
		return nil, ErrBadInput
	}
	if in.Amount > MaxAdjustAmount || in.Amount < -MaxAdjustAmount {
		return nil, ErrAmountLimit
	}
	coinType, err := normalizeCoinType(in.CoinType)
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
	requestKey := strings.TrimSpace(in.RequestKey)
	if requestKey == "" {
		requestKey = uuid.NewString()
	}
	if len(requestKey) > 100 {
		return nil, ErrBadInput
	}
	if err := s.ensureUser(ctx, targetID); err != nil {
		return nil, err
	}

	// 币种进入幂等键：同一 request_key 用不同币种重试不会被误判为同一笔。
	key := "admin_adjust:" + coinType + ":" + requestKey
	var entry *domain.GiftLedger
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		w, err := lockWallet(tx, targetID)
		if err != nil {
			return err
		}
		existing, err := findLedgerByKey(tx, targetID, key)
		if err != nil {
			return err
		}
		if existing != nil {
			entry = existing
			return nil
		}
		newBalance := coinBalance(w, coinType) + in.Amount
		if newBalance < 0 {
			return ErrInsufficientBalance
		}
		if err := updateCoinBalance(tx, targetID, coinType, newBalance, s.now()); err != nil {
			return err
		}
		row := domain.GiftLedger{
			UserID:           targetID,
			Amount:           in.Amount,
			CoinType:         coinType,
			CoinBalanceAfter: newBalance,
			EventType:        domain.GiftEventAdminAdjustment,
			SourceType:       "admin_action",
			SourceID:         requestKey,
			IdempotencyKey:   key,
			Reason:           reason,
			OperatorAdminID:  parseUUIDPtr(in.OperatorID),
			RequestID:        in.RequestID,
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		entry = &row
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entry, nil
}

// ListLedger 分页查询礼物币流水。
func (s *Service) ListLedger(ctx context.Context, f LedgerFilter) ([]domain.GiftLedger, int64, error) {
	q := s.db.WithContext(ctx).Model(&domain.GiftLedger{})
	if strings.TrimSpace(f.UserID) != "" {
		id, err := s.resolveUserID(ctx, f.UserID)
		if err != nil {
			return nil, 0, err
		}
		q = q.Where("user_id = ?", id)
	}
	if strings.TrimSpace(f.CoinType) != "" {
		coinType, err := normalizeCoinType(f.CoinType)
		if err != nil {
			return nil, 0, err
		}
		q = q.Where("coin_type = ?", coinType)
	}
	if f.EventType != "" {
		q = q.Where("event_type = ?", f.EventType)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	page, pageSize := normalizePage(f.Page, f.PageSize)
	var rows []domain.GiftLedger
	if err := q.Order("created_at DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// resolveUserID 将后台输入解析为账号 UUID；同时接受内部 UUID 与对外 user_no。
func (s *Service) resolveUserID(ctx context.Context, raw string) (uuid.UUID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return uuid.Nil, ErrBadInput
	}
	if id, err := uuid.Parse(raw); err == nil {
		return id, nil
	}
	var row struct {
		ID uuid.UUID
	}
	if err := s.db.WithContext(ctx).Table("accounts").
		Select("id").
		Where("user_no = ?", raw).
		Scan(&row).Error; err != nil {
		return uuid.Nil, err
	}
	if row.ID == uuid.Nil {
		return uuid.Nil, ErrUnknownUser
	}
	return row.ID, nil
}

func (s *Service) ensureUser(ctx context.Context, userID uuid.UUID) error {
	var cnt int64
	if err := s.db.WithContext(ctx).Table("accounts").
		Where("id = ?", userID).Count(&cnt).Error; err != nil {
		return err
	}
	if cnt == 0 {
		return ErrUnknownUser
	}
	return nil
}

// lockWallet 幂等创建钱包行并加行锁，返回锁定后的余额。
func lockWallet(tx *gorm.DB, userID uuid.UUID) (*domain.GiftWallet, error) {
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).
		Create(&domain.GiftWallet{UserID: userID}).Error; err != nil {
		return nil, err
	}
	var w domain.GiftWallet
	if err := tx.Raw(`SELECT user_id, general_balance, checkin_balance, version, created_at, updated_at
		FROM gift_wallets WHERE user_id = ? FOR UPDATE`, userID).Scan(&w).Error; err != nil {
		return nil, err
	}
	if w.UserID == uuid.Nil {
		return nil, errors.New("礼物币钱包缺失")
	}
	return &w, nil
}

func normalizeCoinType(raw string) (string, error) {
	switch strings.TrimSpace(raw) {
	case "", domain.GiftCoinGeneral:
		return domain.GiftCoinGeneral, nil
	case domain.GiftCoinCheckin:
		return domain.GiftCoinCheckin, nil
	default:
		return "", ErrBadCoinType
	}
}

func coinBalance(w *domain.GiftWallet, coinType string) int64 {
	if coinType == domain.GiftCoinCheckin {
		return w.CheckinBalance
	}
	return w.GeneralBalance
}

func updateCoinBalance(tx *gorm.DB, userID uuid.UUID, coinType string, balance int64, now time.Time) error {
	column := "general_balance"
	if coinType == domain.GiftCoinCheckin {
		column = "checkin_balance"
	}
	return tx.Model(&domain.GiftWallet{}).Where("user_id = ?", userID).
		Updates(map[string]any{
			column:       balance,
			"version":    gorm.Expr("version + 1"),
			"updated_at": now,
		}).Error
}

func publicLedger(row domain.GiftLedger) LedgerView {
	return LedgerView{
		ID:           row.ID,
		Amount:       row.Amount,
		CoinType:     row.CoinType,
		BalanceAfter: row.CoinBalanceAfter,
		EventType:    row.EventType,
		Reason:       row.Reason,
		CreatedAt:    row.CreatedAt,
	}
}

func findLedgerByKey(tx *gorm.DB, userID uuid.UUID, key string) (*domain.GiftLedger, error) {
	var row domain.GiftLedger
	err := tx.Where("user_id = ? AND idempotency_key = ?", userID, key).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func parseUUIDPtr(s string) *uuid.UUID {
	if s == "" {
		return nil
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return nil
	}
	return &id
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
