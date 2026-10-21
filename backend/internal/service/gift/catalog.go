package gift

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"mckit/backend/internal/domain"
)

// giftCodeRe 礼物编码：小写字母开头，仅含小写字母、数字、下划线。
var giftCodeRe = regexp.MustCompile(`^[a-z][a-z0-9_]{2,63}$`)

const (
	maxGiftPrice     int64 = 1_000_000
	maxGiftStock     int64 = 1_000_000_000
	maxSortOrder           = 100000
	maxCheckinReward int64 = 1_000_000
	maxCheckinXP     int64 = 1_000_000
)

var (
	ErrGiftCodeTaken = errors.New("礼物编码已存在")
	ErrGiftCodeBad   = errors.New("礼物编码需以小写字母开头，仅含小写字母、数字、下划线，长度 3-64")
	ErrGiftNameBad   = errors.New("礼物名称需为 1-80 个字符")
	ErrGiftDescBad   = errors.New("礼物描述不能超过 500 个字符")
	ErrGiftIconBad   = errors.New("礼物图标不能超过 16 个字符")
	ErrGiftPriceBad  = errors.New("礼物价格需在 1-1000000 之间")
	ErrGiftStockBad  = errors.New("礼物库存需在 0-1000000000 之间（0 表示不限）")
	ErrGiftSortBad   = errors.New("排序值超出允许范围")
	ErrCheckinReward = errors.New("签到奖励需在 0-1000000 之间")
)

// DefinitionView 礼物目录视图。
type DefinitionView struct {
	ID   string `json:"id"`
	Code string `json:"code"`
	// Kind 目录项类型：gift / makeup_card。
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	Price       int64  `json:"price"`
	// Stock 总库存；0 表示不限库存。
	Stock         int64 `json:"stock"`
	AllowCheckin  bool  `json:"allow_checkin"`
	SortOrder     int   `json:"sort_order"`
	Enabled       bool  `json:"enabled"`
	RedeemedCount int64 `json:"redeemed_count"`
	// Remaining 剩余库存；不限库存时为 null。
	Remaining *int64    `json:"remaining"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CatalogItem 用户侧礼物目录条目（附带当前账号是否买得起）。
type CatalogItem struct {
	DefinitionView
	CanAfford bool `json:"can_afford"`
}

// DefinitionInput 新建 / 编辑礼物目录入参。
//
// 编码由系统自动生成，管理员不可指定，也不在后台展示。
type DefinitionInput struct {
	Name        string
	Description string
	Icon        string
	Price       int64
	// Stock 总库存；0 表示不限库存。
	Stock        int64
	AllowCheckin bool
	SortOrder    int
	Enabled      bool
}

// SettingsView 礼物中心全局配置（含签到经验算法与里程碑）。
type SettingsView struct {
	CheckinReward     int64           `json:"checkin_reward"`
	BaseXP            int64           `json:"base_xp"`
	StreakStepXP      int64           `json:"streak_step_xp"`
	StreakBonusCap    int64           `json:"streak_bonus_cap"`
	MilestoneBonusCap int64           `json:"milestone_bonus_cap"`
	BreakPenaltyXP    int64           `json:"break_penalty_xp"`
	Milestones        []MilestoneView `json:"milestones"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

// MilestoneView 连续签到里程碑。
type MilestoneView struct {
	ID         string    `json:"id"`
	StreakDays int       `json:"streak_days"`
	BonusXP    int64     `json:"bonus_xp"`
	Enabled    bool      `json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// MilestoneInput 后台保存里程碑的入参。
type MilestoneInput struct {
	StreakDays int   `json:"streak_days"`
	BonusXP    int64 `json:"bonus_xp"`
	Enabled    bool  `json:"enabled"`
}

// CheckinSettingsInput 后台更新签到配置；nil 字段表示保持不变。
type CheckinSettingsInput struct {
	CheckinReward     *int64
	BaseXP            *int64
	StreakStepXP      *int64
	StreakBonusCap    *int64
	MilestoneBonusCap *int64
	BreakPenaltyXP    *int64
	Milestones        *[]MilestoneInput
}

// ListDefinitions 后台礼物目录（含停用项与兑换次数）。
func (s *Service) ListDefinitions(ctx context.Context) ([]DefinitionView, error) {
	var defs []domain.GiftDefinition
	if err := s.db.WithContext(ctx).Order("sort_order ASC, created_at ASC").Find(&defs).Error; err != nil {
		return nil, err
	}
	counts, err := s.redemptionCounts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]DefinitionView, 0, len(defs))
	for _, d := range defs {
		out = append(out, definitionView(d, counts[d.ID.String()]))
	}
	return out, nil
}

// Catalog 用户可兑换礼物（仅启用项，按排序值升序）。
func (s *Service) Catalog(ctx context.Context, userID string) ([]CatalogItem, error) {
	id, err := uuid.Parse(strings.TrimSpace(userID))
	if err != nil {
		return nil, ErrBadInput
	}
	wallet, err := s.GetWallet(ctx, id.String())
	if err != nil {
		return nil, err
	}
	var defs []domain.GiftDefinition
	if err := s.db.WithContext(ctx).Where("enabled = true").
		Order("sort_order ASC, created_at ASC").Find(&defs).Error; err != nil {
		return nil, err
	}
	counts, err := s.redemptionCounts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]CatalogItem, 0, len(defs))
	for _, d := range defs {
		redeemed := counts[d.ID.String()]
		// 已兑完的限量礼物不再出现在用户目录；0 表示不限库存。
		if d.Stock > 0 && redeemed >= d.Stock {
			continue
		}
		available := wallet.GeneralBalance
		if d.Kind == domain.GiftKindMakeupCard {
			// 签到道具只能用签到礼物币购买，通用币不参与。
			available = wallet.CheckinBalance
		} else if d.AllowCheckin {
			available += wallet.CheckinBalance
		}
		out = append(out, CatalogItem{
			DefinitionView: definitionView(d, redeemed),
			CanAfford:      available >= d.Price,
		})
	}
	return out, nil
}

// generateGiftCode 生成后台创建礼物的内部编码；管理员不可指定。
func generateGiftCode() string {
	return "gift_" + strings.ReplaceAll(uuid.NewString(), "-", "")
}

// CreateDefinition 新建礼物。编码由系统生成。
func (s *Service) CreateDefinition(ctx context.Context, in DefinitionInput) (*DefinitionView, error) {
	code := generateGiftCode()
	if !giftCodeRe.MatchString(code) {
		return nil, ErrGiftCodeBad
	}
	name, desc, icon, err := validateDefinition(in)
	if err != nil {
		return nil, err
	}
	if err := validateStock(in.Stock); err != nil {
		return nil, err
	}
	var existing int64
	if err := s.db.WithContext(ctx).Model(&domain.GiftDefinition{}).
		Where("code = ?", code).Count(&existing).Error; err != nil {
		return nil, err
	}
	if existing > 0 {
		return nil, ErrGiftCodeTaken
	}
	d := &domain.GiftDefinition{
		Code:         code,
		Kind:         domain.GiftKindGift,
		Name:         name,
		Description:  desc,
		Icon:         icon,
		Price:        in.Price,
		Stock:        in.Stock,
		AllowCheckin: in.AllowCheckin,
		SortOrder:    in.SortOrder,
		Enabled:      in.Enabled,
	}
	if err := s.db.WithContext(ctx).Create(d).Error; err != nil {
		return nil, err
	}
	v := definitionView(*d, 0)
	return &v, nil
}

// UpdateDefinition 编辑礼物（编码不可变）。
func (s *Service) UpdateDefinition(ctx context.Context, id string, in DefinitionInput) (*DefinitionView, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return nil, ErrGiftNotFound
	}
	name, desc, icon, err := validateDefinition(in)
	if err != nil {
		return nil, err
	}
	var d domain.GiftDefinition
	if err := s.db.WithContext(ctx).Where("id = ?", parsed).First(&d).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrGiftNotFound
		}
		return nil, err
	}
	stock := in.Stock
	allowCheckin := in.AllowCheckin
	if d.Kind == domain.GiftKindMakeupCard {
		// 签到道具固定不限库存、只允许签到币，避免被后台误配。
		stock = 0
		allowCheckin = true
	} else if err := validateStock(stock); err != nil {
		return nil, err
	}
	updates := map[string]any{
		"name":          name,
		"description":   desc,
		"icon":          icon,
		"price":         in.Price,
		"stock":         stock,
		"allow_checkin": allowCheckin,
		"sort_order":    in.SortOrder,
		"enabled":       in.Enabled,
		"updated_at":    s.now(),
	}
	if err := s.db.WithContext(ctx).Model(&domain.GiftDefinition{}).
		Where("id = ?", parsed).Updates(updates).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Where("id = ?", parsed).First(&d).Error; err != nil {
		return nil, err
	}
	var redeemed int64
	if err := s.db.WithContext(ctx).Model(&domain.GiftRedemption{}).
		Where("gift_id = ?", parsed).Count(&redeemed).Error; err != nil {
		return nil, err
	}
	v := definitionView(d, redeemed)
	return &v, nil
}

// DeleteDefinition 删除礼物；签到道具与已有兑换记录的礼物只允许停用。
func (s *Service) DeleteDefinition(ctx context.Context, id string) error {
	parsed, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return ErrGiftNotFound
	}
	var d domain.GiftDefinition
	if err := s.db.WithContext(ctx).Where("id = ?", parsed).First(&d).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrGiftNotFound
		}
		return err
	}
	if d.Kind == domain.GiftKindMakeupCard {
		return ErrGiftSystemItem
	}
	var cnt int64
	if err := s.db.WithContext(ctx).Model(&domain.GiftRedemption{}).
		Where("gift_id = ?", parsed).Count(&cnt).Error; err != nil {
		return err
	}
	if cnt > 0 {
		return ErrGiftInUse
	}
	res := s.db.WithContext(ctx).Where("id = ?", parsed).Delete(&domain.GiftDefinition{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrGiftNotFound
	}
	return nil
}

// makeupCardDefinitionTx 读取唯一的补签卡目录项；尚未初始化时返回 nil。
func (s *Service) makeupCardDefinitionTx(tx *gorm.DB) (*domain.GiftDefinition, error) {
	var d domain.GiftDefinition
	err := tx.Where("kind = ?", domain.GiftKindMakeupCard).First(&d).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// makeupCardDefinition 读取补签卡目录项。
func (s *Service) makeupCardDefinition(ctx context.Context) (*domain.GiftDefinition, error) {
	return s.makeupCardDefinitionTx(s.db.WithContext(ctx))
}

// defaultGiftSettings 返回与 0048 迁移一致的默认签到配置。
func defaultGiftSettings() domain.GiftSettings {
	return domain.GiftSettings{
		ID:                true,
		CheckinReward:     1,
		BaseXP:            10,
		StreakStepXP:      2,
		StreakBonusCap:    100,
		MilestoneBonusCap: 500,
		BreakPenaltyXP:    5,
	}
}

// GetSettings 读取礼物中心配置；缺失时返回默认值。
func (s *Service) GetSettings(ctx context.Context) (*SettingsView, error) {
	db := s.db.WithContext(ctx)
	var row domain.GiftSettings
	err := db.Where("id = true").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row = defaultGiftSettings()
	} else if err != nil {
		return nil, err
	}
	var milestones []domain.GiftCheckinMilestone
	if err := db.Order("streak_days ASC").Find(&milestones).Error; err != nil {
		return nil, err
	}
	return settingsView(row, milestones), nil
}

// UpdateSettings 兼容旧后台：只更新每日签到礼物币。
func (s *Service) UpdateSettings(ctx context.Context, checkinReward int64) (*SettingsView, error) {
	if checkinReward < 0 || checkinReward > maxCheckinReward {
		return nil, ErrCheckinReward
	}
	return s.UpdateCheckinSettings(ctx, CheckinSettingsInput{CheckinReward: &checkinReward})
}

// UpdateCheckinSettings 更新签到奖励、经验算法与里程碑列表。
//
// 里程碑使用「整表替换」语义：传入 nil 表示不改动，传入空数组表示清空。
func (s *Service) UpdateCheckinSettings(ctx context.Context, in CheckinSettingsInput) (*SettingsView, error) {
	if in.CheckinReward == nil && in.BaseXP == nil && in.StreakStepXP == nil &&
		in.StreakBonusCap == nil && in.MilestoneBonusCap == nil &&
		in.BreakPenaltyXP == nil && in.Milestones == nil {
		return nil, ErrBadInput
	}
	if err := validateCheckinSettings(in); err != nil {
		return nil, err
	}
	return s.updateCheckinSettings(ctx, in)
}

// updateCheckinSettings 实际落库，独立出来便于测试替换事务外查询。
func (s *Service) updateCheckinSettings(ctx context.Context, in CheckinSettingsInput) (*SettingsView, error) {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		def := defaultGiftSettings()
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&def).Error; err != nil {
			return err
		}
		updates := map[string]any{"updated_at": s.now()}
		if in.CheckinReward != nil {
			updates["checkin_reward"] = *in.CheckinReward
		}
		if in.BaseXP != nil {
			updates["base_xp"] = *in.BaseXP
		}
		if in.StreakStepXP != nil {
			updates["streak_step_xp"] = *in.StreakStepXP
		}
		if in.StreakBonusCap != nil {
			updates["streak_bonus_cap"] = *in.StreakBonusCap
		}
		if in.MilestoneBonusCap != nil {
			updates["milestone_bonus_cap"] = *in.MilestoneBonusCap
		}
		if in.BreakPenaltyXP != nil {
			updates["break_penalty_xp"] = *in.BreakPenaltyXP
		}
		if err := tx.Model(&domain.GiftSettings{}).Where("id = true").Updates(updates).Error; err != nil {
			return err
		}
		if in.Milestones != nil {
			if err := tx.Where("1 = 1").Delete(&domain.GiftCheckinMilestone{}).Error; err != nil {
				return err
			}
			if len(*in.Milestones) > 0 {
				rows := make([]domain.GiftCheckinMilestone, 0, len(*in.Milestones))
				for _, m := range *in.Milestones {
					rows = append(rows, domain.GiftCheckinMilestone{
						StreakDays: m.StreakDays,
						BonusXP:    m.BonusXP,
						Enabled:    m.Enabled,
					})
				}
				if err := tx.Create(&rows).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.GetSettings(ctx)
}

func validateCheckinSettings(in CheckinSettingsInput) error {
	check := func(v *int64, min, max int64) bool {
		return v == nil || (*v >= min && *v <= max)
	}
	if !check(in.CheckinReward, 0, maxCheckinReward) ||
		!check(in.BaseXP, 0, maxCheckinXP) ||
		!check(in.StreakStepXP, 0, maxCheckinXP) ||
		!check(in.StreakBonusCap, 0, maxCheckinXP) ||
		!check(in.MilestoneBonusCap, 0, maxCheckinXP) ||
		!check(in.BreakPenaltyXP, 0, maxCheckinXP) {
		return ErrCheckinConfig
	}
	if in.Milestones == nil {
		return nil
	}
	seen := make(map[int]bool, len(*in.Milestones))
	for _, m := range *in.Milestones {
		if m.StreakDays < 1 || m.StreakDays > 1_000_000 || m.BonusXP < 0 || m.BonusXP > maxCheckinXP || seen[m.StreakDays] {
			return ErrMilestoneConfig
		}
		seen[m.StreakDays] = true
	}
	return nil
}

func settingsView(row domain.GiftSettings, milestones []domain.GiftCheckinMilestone) *SettingsView {
	out := &SettingsView{
		CheckinReward:     row.CheckinReward,
		BaseXP:            row.BaseXP,
		StreakStepXP:      row.StreakStepXP,
		StreakBonusCap:    row.StreakBonusCap,
		MilestoneBonusCap: row.MilestoneBonusCap,
		BreakPenaltyXP:    row.BreakPenaltyXP,
		Milestones:        make([]MilestoneView, 0, len(milestones)),
		UpdatedAt:         row.UpdatedAt,
	}
	for _, m := range milestones {
		out.Milestones = append(out.Milestones, MilestoneView{
			ID:         m.ID.String(),
			StreakDays: m.StreakDays,
			BonusXP:    m.BonusXP,
			Enabled:    m.Enabled,
			CreatedAt:  m.CreatedAt,
			UpdatedAt:  m.UpdatedAt,
		})
	}
	return out
}

// loadSettingsTx 在事务内读取签到配置；缺失时创建默认行。
func (s *Service) loadSettingsTx(tx *gorm.DB) (*domain.GiftSettings, error) {
	def := defaultGiftSettings()
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&def).Error; err != nil {
		return nil, err
	}
	var row domain.GiftSettings
	if err := tx.Where("id = true").First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

// checkinReward 读取当前签到奖励（事务内外均可使用）。
func (s *Service) checkinReward(ctx context.Context) (int64, error) {
	var row domain.GiftSettings
	err := s.db.WithContext(ctx).Where("id = true").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return defaultGiftSettings().CheckinReward, nil
	}
	if err != nil {
		return 0, err
	}
	return row.CheckinReward, nil
}

func validateStock(stock int64) error {
	if stock < 0 || stock > maxGiftStock {
		return ErrGiftStockBad
	}
	return nil
}

// redemptionCounts 统计每个礼物的累计兑换次数（按礼物 ID 索引）。
func (s *Service) redemptionCounts(ctx context.Context) (map[string]int64, error) {
	var rows []struct {
		GiftID uuid.UUID
		N      int64
	}
	if err := s.db.WithContext(ctx).Model(&domain.GiftRedemption{}).
		Select("gift_id, count(*) AS n").Group("gift_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	counts := make(map[string]int64, len(rows))
	for _, r := range rows {
		counts[r.GiftID.String()] = r.N
	}
	return counts, nil
}

// remainingStock 计算剩余库存；0 表示不限，返回 nil。
func remainingStock(stock, redeemed int64) *int64 {
	if stock <= 0 {
		return nil
	}
	left := stock - redeemed
	if left < 0 {
		left = 0
	}
	return &left
}

func validateDefinition(in DefinitionInput) (name, desc, icon string, err error) {
	name = strings.TrimSpace(in.Name)
	if n := len([]rune(name)); n == 0 || n > 80 {
		return "", "", "", ErrGiftNameBad
	}
	desc = strings.TrimSpace(in.Description)
	if len([]rune(desc)) > 500 {
		return "", "", "", ErrGiftDescBad
	}
	icon = strings.TrimSpace(in.Icon)
	if icon == "" {
		icon = "🎁"
	}
	if len([]rune(icon)) > 16 {
		return "", "", "", ErrGiftIconBad
	}
	if in.Price <= 0 || in.Price > maxGiftPrice {
		return "", "", "", ErrGiftPriceBad
	}
	if in.SortOrder < -maxSortOrder || in.SortOrder > maxSortOrder {
		return "", "", "", ErrGiftSortBad
	}
	return name, desc, icon, nil
}

func definitionView(d domain.GiftDefinition, redeemed int64) DefinitionView {
	return DefinitionView{
		ID:            d.ID.String(),
		Code:          d.Code,
		Kind:          d.Kind,
		Name:          d.Name,
		Description:   d.Description,
		Icon:          d.Icon,
		Price:         d.Price,
		Stock:         d.Stock,
		AllowCheckin:  d.AllowCheckin,
		SortOrder:     d.SortOrder,
		Enabled:       d.Enabled,
		RedeemedCount: redeemed,
		Remaining:     remainingStock(d.Stock, redeemed),
		CreatedAt:     d.CreatedAt,
		UpdatedAt:     d.UpdatedAt,
	}
}
