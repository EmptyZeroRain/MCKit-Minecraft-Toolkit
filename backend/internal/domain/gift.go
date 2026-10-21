package domain

import (
	"time"

	"github.com/google/uuid"
)

// 礼物币账本事件类型。
const (
	// GiftEventAdminAdjustment 后台受控增减。
	GiftEventAdminAdjustment = "admin_adjustment"
	// GiftEventDailyCheckin 每日签到发放签到币。
	GiftEventDailyCheckin = "daily_checkin"
	// GiftEventGiftRedeem 使用礼物币兑换礼物。
	GiftEventGiftRedeem = "gift_redeem"
	// GiftEventMakeupCardPurchase 使用签到礼物币购买补签卡。
	GiftEventMakeupCardPurchase = "makeup_card_purchase"
)

// 签到经验明细类型。
const (
	// CheckinXPKindActual 真实签到。
	CheckinXPKindActual = "actual"
	// CheckinXPKindMakeup 补签（只补经验，不计签到天数）。
	CheckinXPKindMakeup = "makeup"
	// CheckinXPKindBreak 断签惩罚。
	CheckinXPKindBreak = "break"
)

// 礼物币币种：签到获得的币与通用币分账管理。
const (
	// GiftCoinGeneral 通用礼物币（活动 / 后台补偿等来源）。
	GiftCoinGeneral = "general"
	// GiftCoinCheckin 签到礼物币（每日签到获得，仅可兑换允许签到币的礼物）。
	GiftCoinCheckin = "checkin"
)

// 礼物目录项类型。
const (
	// GiftKindGift 普通礼物：可配置库存与签到币兑换，兑换后生成兑换记录。
	GiftKindGift = "gift"
	// GiftKindMakeupCard 补签卡等签到道具：只能签到币购买，购买后进入个人库存。
	GiftKindMakeupCard = "makeup_card"
)

// GiftWallet 用户礼物币余额投影。
//
// 余额只能通过账本流水变更；禁止业务代码直接 UPDATE 余额而不写 gift_ledger。
type GiftWallet struct {
	UserID         uuid.UUID `gorm:"column:user_id;type:uuid;primaryKey" json:"user_id"`
	GeneralBalance int64     `gorm:"column:general_balance;not null;default:0" json:"general_balance"`
	CheckinBalance int64     `gorm:"column:checkin_balance;not null;default:0" json:"checkin_balance"`
	Version        int64     `gorm:"column:version;not null;default:0" json:"version"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// TableName 指定表名。
func (GiftWallet) TableName() string { return "gift_wallets" }

// GiftLedger 礼物币账本（只增不删；扣减为负数流水）。
//
// CoinBalanceAfter 表示该笔流水发生后「对应币种」的余额。
type GiftLedger struct {
	ID               uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID           uuid.UUID  `gorm:"column:user_id;type:uuid;not null" json:"user_id"`
	Amount           int64      `gorm:"column:amount;not null" json:"amount"`
	CoinType         string     `gorm:"column:coin_type;size:16;not null;default:general" json:"coin_type"`
	CoinBalanceAfter int64      `gorm:"column:coin_balance_after;not null" json:"balance_after"`
	EventType        string     `gorm:"column:event_type;size:32;not null" json:"event_type"`
	SourceType       string     `gorm:"column:source_type;size:32;not null;default:admin_action" json:"source_type"`
	SourceID         string     `gorm:"column:source_id;size:128;not null;default:''" json:"source_id"`
	IdempotencyKey   string     `gorm:"column:idempotency_key;size:128;not null" json:"idempotency_key"`
	Reason           string     `gorm:"column:reason;size:500;not null" json:"reason"`
	OperatorAdminID  *uuid.UUID `gorm:"column:operator_admin_id;type:uuid" json:"operator_admin_id"`
	RequestID        string     `gorm:"column:request_id;size:128;not null;default:''" json:"request_id"`
	CreatedAt        time.Time  `json:"created_at"`
}

// TableName 指定表名。
func (GiftLedger) TableName() string { return "gift_ledger" }

// GiftDefinition 礼物目录（后台可维护）。
type GiftDefinition struct {
	ID   uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Code string    `gorm:"size:64;not null;uniqueIndex" json:"code"`
	// Kind 目录项类型：gift（普通礼物）/ makeup_card（补签卡等签到道具）。
	Kind        string `gorm:"size:16;not null;default:gift" json:"kind"`
	Name        string `gorm:"size:80;not null" json:"name"`
	Description string `gorm:"size:500;not null;default:''" json:"description"`
	Icon        string `gorm:"size:32;not null;default:'🎁'" json:"icon"`
	Price       int64  `gorm:"not null" json:"price"`
	// Stock 总库存；0 表示不限库存，兑换时按已兑换记录数校验剩余库存。
	Stock        int64     `gorm:"not null;default:0" json:"stock"`
	AllowCheckin bool      `gorm:"column:allow_checkin;not null;default:false" json:"allow_checkin"`
	SortOrder    int       `gorm:"not null;default:0" json:"sort_order"`
	Enabled      bool      `gorm:"not null;default:true" json:"enabled"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// TableName 指定表名。
func (GiftDefinition) TableName() string { return "gift_definitions" }

// GiftRedemption 礼物兑换记录（保存成交时的名称 / 价格快照）。
type GiftRedemption struct {
	ID               uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID           uuid.UUID `gorm:"column:user_id;type:uuid;not null" json:"user_id"`
	GiftID           uuid.UUID `gorm:"column:gift_id;type:uuid;not null" json:"gift_id"`
	GiftCodeSnapshot string    `gorm:"column:gift_code_snapshot;size:64;not null" json:"gift_code"`
	GiftNameSnapshot string    `gorm:"column:gift_name_snapshot;size:80;not null" json:"gift_name"`
	GiftIconSnapshot string    `gorm:"column:gift_icon_snapshot;size:32;not null;default:'🎁'" json:"gift_icon"`
	Price            int64     `gorm:"not null" json:"price"`
	CheckinSpent     int64     `gorm:"column:checkin_spent;not null;default:0" json:"checkin_spent"`
	GeneralSpent     int64     `gorm:"column:general_spent;not null;default:0" json:"general_spent"`
	IdempotencyKey   string    `gorm:"column:idempotency_key;size:128;not null" json:"idempotency_key"`
	CreatedAt        time.Time `json:"created_at"`
}

// TableName 指定表名。
func (GiftRedemption) TableName() string { return "gift_redemptions" }

// GiftCheckin 每日签到记录（每人每天一条）。
type GiftCheckin struct {
	UserID      uuid.UUID `gorm:"column:user_id;type:uuid;primaryKey" json:"user_id"`
	CheckinDate time.Time `gorm:"column:checkin_date;type:date;primaryKey" json:"checkin_date"`
	Reward      int64     `gorm:"not null;default:0" json:"reward"`
	CreatedAt   time.Time `json:"created_at"`
}

// TableName 指定表名。
func (GiftCheckin) TableName() string { return "gift_checkins" }

// GiftSettings 礼物中心全局配置（单行表）。
type GiftSettings struct {
	ID            bool  `gorm:"primaryKey" json:"id"`
	CheckinReward int64 `gorm:"column:checkin_reward;not null;default:1" json:"checkin_reward"`
	// BaseXP 每次真实签到 / 补签的基础经验。
	BaseXP int64 `gorm:"column:base_xp;not null;default:10" json:"base_xp"`
	// StreakStepXP 连续签到每多 1 天增加的经验。
	StreakStepXP int64 `gorm:"column:streak_step_xp;not null;default:2" json:"streak_step_xp"`
	// StreakBonusCap 单次签到连续奖励上限。
	StreakBonusCap int64 `gorm:"column:streak_bonus_cap;not null;default:100" json:"streak_bonus_cap"`
	// MilestoneBonusCap 单次签到里程碑奖励合计上限。
	MilestoneBonusCap int64 `gorm:"column:milestone_bonus_cap;not null;default:500" json:"milestone_bonus_cap"`
	// BreakPenaltyXP 断签后首次真实签到扣除的经验。
	BreakPenaltyXP int64     `gorm:"column:break_penalty_xp;not null;default:5" json:"break_penalty_xp"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// TableName 指定表名。
func (GiftSettings) TableName() string { return "gift_settings" }

// GiftCheckinMilestone 连续签到里程碑：真实签到达到 streak_days 时发放 bonus_xp。
type GiftCheckinMilestone struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	StreakDays int       `gorm:"column:streak_days;not null;uniqueIndex" json:"streak_days"`
	BonusXP    int64     `gorm:"column:bonus_xp;not null;default:0" json:"bonus_xp"`
	Enabled    bool      `gorm:"not null;default:true" json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// TableName 指定表名。
func (GiftCheckinMilestone) TableName() string { return "gift_checkin_milestones" }

// GiftCheckinStreak 连续签到投影（可由 gift_checkins 重建）。
type GiftCheckinStreak struct {
	UserID          uuid.UUID  `gorm:"column:user_id;type:uuid;primaryKey" json:"user_id"`
	CurrentStreak   int        `gorm:"column:current_streak;not null;default:0" json:"current_streak"`
	LongestStreak   int        `gorm:"column:longest_streak;not null;default:0" json:"longest_streak"`
	LastCheckinDate *time.Time `gorm:"column:last_checkin_date;type:date" json:"last_checkin_date"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// TableName 指定表名。
func (GiftCheckinStreak) TableName() string { return "gift_checkin_streaks" }

// GiftMakeupCard 补签卡库存。
type GiftMakeupCard struct {
	UserID    uuid.UUID `gorm:"column:user_id;type:uuid;primaryKey" json:"user_id"`
	Quantity  int64     `gorm:"not null;default:0" json:"quantity"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName 指定表名。
func (GiftMakeupCard) TableName() string { return "gift_makeup_cards" }

// GiftCheckinXP 签到经验明细：保留每次签到 / 补签 / 断签惩罚的计算快照。
type GiftCheckinXP struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID      uuid.UUID `gorm:"column:user_id;type:uuid;not null" json:"user_id"`
	XpDate      time.Time `gorm:"column:xp_date;type:date;not null" json:"xp_date"`
	Kind        string    `gorm:"size:16;not null" json:"kind"`
	RequestKey  string    `gorm:"column:request_key;size:128;not null;default:''" json:"request_key"`
	StreakDay   int       `gorm:"column:streak_day;not null;default:0" json:"streak_day"`
	BaseXP      int64     `gorm:"column:base_xp;not null;default:0" json:"base_xp"`
	StreakXP    int64     `gorm:"column:streak_xp;not null;default:0" json:"streak_xp"`
	MilestoneXP int64     `gorm:"column:milestone_xp;not null;default:0" json:"milestone_xp"`
	PenaltyXP   int64     `gorm:"column:penalty_xp;not null;default:0" json:"penalty_xp"`
	TotalXP     int64     `gorm:"column:total_xp;not null" json:"total_xp"`
	CreatedAt   time.Time `json:"created_at"`
}

// TableName 指定表名。
func (GiftCheckinXP) TableName() string { return "gift_checkin_xp" }
