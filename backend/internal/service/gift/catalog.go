package gift

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"mckit/backend/internal/domain"
)

// giftCodeRe 礼物编码：小写字母开头，仅含小写字母、数字、下划线。
var giftCodeRe = regexp.MustCompile(`^[a-z][a-z0-9_]{2,63}$`)

const (
	maxGiftPrice     int64 = 1_000_000
	maxSortOrder           = 100000
	maxCheckinReward int64 = 1_000_000
)

var (
	ErrGiftCodeTaken = errors.New("礼物编码已存在")
	ErrGiftCodeBad   = errors.New("礼物编码需以小写字母开头，仅含小写字母、数字、下划线，长度 3-64")
	ErrGiftNameBad   = errors.New("礼物名称需为 1-80 个字符")
	ErrGiftDescBad   = errors.New("礼物描述不能超过 500 个字符")
	ErrGiftIconBad   = errors.New("礼物图标不能超过 16 个字符")
	ErrGiftPriceBad  = errors.New("礼物价格需在 1-1000000 之间")
	ErrGiftSortBad   = errors.New("排序值超出允许范围")
	ErrCheckinReward = errors.New("签到奖励需在 0-1000000 之间")
)

// DefinitionView 礼物目录视图。
type DefinitionView struct {
	ID            string    `json:"id"`
	Code          string    `json:"code"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	Icon          string    `json:"icon"`
	Price         int64     `json:"price"`
	AllowCheckin  bool      `json:"allow_checkin"`
	SortOrder     int       `json:"sort_order"`
	Enabled       bool      `json:"enabled"`
	RedeemedCount int64     `json:"redeemed_count"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// CatalogItem 用户侧礼物目录条目（附带当前账号是否买得起）。
type CatalogItem struct {
	DefinitionView
	CanAfford bool `json:"can_afford"`
}

// DefinitionInput 新建 / 编辑礼物目录入参。
//
// Code 只在新建时使用；编辑时编码不可变，避免历史快照与外部引用失配。
type DefinitionInput struct {
	Code         string
	Name         string
	Description  string
	Icon         string
	Price        int64
	AllowCheckin bool
	SortOrder    int
	Enabled      bool
}

// SettingsView 礼物中心全局配置。
type SettingsView struct {
	CheckinReward int64     `json:"checkin_reward"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// ListDefinitions 后台礼物目录（含停用项与兑换次数）。
func (s *Service) ListDefinitions(ctx context.Context) ([]DefinitionView, error) {
	var defs []domain.GiftDefinition
	if err := s.db.WithContext(ctx).Order("sort_order ASC, created_at ASC").Find(&defs).Error; err != nil {
		return nil, err
	}
	counts := map[string]int64{}
	var rows []struct {
		GiftID uuid.UUID
		N      int64
	}
	if err := s.db.WithContext(ctx).Model(&domain.GiftRedemption{}).
		Select("gift_id, count(*) AS n").Group("gift_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		counts[r.GiftID.String()] = r.N
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
	out := make([]CatalogItem, 0, len(defs))
	for _, d := range defs {
		available := wallet.GeneralBalance
		if d.AllowCheckin {
			available += wallet.CheckinBalance
		}
		out = append(out, CatalogItem{
			DefinitionView: definitionView(d, 0),
			CanAfford:      available >= d.Price,
		})
	}
	return out, nil
}

// CreateDefinition 新建礼物。
func (s *Service) CreateDefinition(ctx context.Context, in DefinitionInput) (*DefinitionView, error) {
	code, name, desc, icon, err := validateDefinition(in, true)
	if err != nil {
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
		Name:         name,
		Description:  desc,
		Icon:         icon,
		Price:        in.Price,
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
	_, name, desc, icon, err := validateDefinition(in, false)
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
	updates := map[string]any{
		"name":          name,
		"description":   desc,
		"icon":          icon,
		"price":         in.Price,
		"allow_checkin": in.AllowCheckin,
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
	v := definitionView(d, 0)
	return &v, nil
}

// DeleteDefinition 删除礼物；已有兑换记录时只允许停用。
func (s *Service) DeleteDefinition(ctx context.Context, id string) error {
	parsed, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return ErrGiftNotFound
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

// GetSettings 读取礼物中心配置；缺失时返回默认值。
func (s *Service) GetSettings(ctx context.Context) (*SettingsView, error) {
	var row domain.GiftSettings
	err := s.db.WithContext(ctx).Where("id = true").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &SettingsView{CheckinReward: 1}, nil
	}
	if err != nil {
		return nil, err
	}
	return &SettingsView{CheckinReward: row.CheckinReward, UpdatedAt: row.UpdatedAt}, nil
}

// UpdateSettings 更新签到奖励。
func (s *Service) UpdateSettings(ctx context.Context, checkinReward int64) (*SettingsView, error) {
	if checkinReward < 0 || checkinReward > maxCheckinReward {
		return nil, ErrCheckinReward
	}
	updates := map[string]any{
		"checkin_reward": checkinReward,
		"updated_at":     s.now(),
	}
	if err := s.db.WithContext(ctx).Model(&domain.GiftSettings{}).
		Where("id = true").Updates(updates).Error; err != nil {
		return nil, err
	}
	var row domain.GiftSettings
	if err := s.db.WithContext(ctx).Where("id = true").First(&row).Error; err != nil {
		return nil, err
	}
	return &SettingsView{CheckinReward: row.CheckinReward, UpdatedAt: row.UpdatedAt}, nil
}

// checkinReward 读取当前签到奖励（事务内外均可使用）。
func (s *Service) checkinReward(ctx context.Context) (int64, error) {
	var row domain.GiftSettings
	err := s.db.WithContext(ctx).Where("id = true").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 1, nil
	}
	if err != nil {
		return 0, err
	}
	return row.CheckinReward, nil
}

func validateDefinition(in DefinitionInput, requireCode bool) (code, name, desc, icon string, err error) {
	code = strings.TrimSpace(in.Code)
	if requireCode {
		if !giftCodeRe.MatchString(code) {
			return "", "", "", "", ErrGiftCodeBad
		}
	}
	name = strings.TrimSpace(in.Name)
	if n := len([]rune(name)); n == 0 || n > 80 {
		return "", "", "", "", ErrGiftNameBad
	}
	desc = strings.TrimSpace(in.Description)
	if len([]rune(desc)) > 500 {
		return "", "", "", "", ErrGiftDescBad
	}
	icon = strings.TrimSpace(in.Icon)
	if icon == "" {
		icon = "🎁"
	}
	if len([]rune(icon)) > 16 {
		return "", "", "", "", ErrGiftIconBad
	}
	if in.Price <= 0 || in.Price > maxGiftPrice {
		return "", "", "", "", ErrGiftPriceBad
	}
	if in.SortOrder < -maxSortOrder || in.SortOrder > maxSortOrder {
		return "", "", "", "", ErrGiftSortBad
	}
	return code, name, desc, icon, nil
}

func definitionView(d domain.GiftDefinition, redeemed int64) DefinitionView {
	return DefinitionView{
		ID:            d.ID.String(),
		Code:          d.Code,
		Name:          d.Name,
		Description:   d.Description,
		Icon:          d.Icon,
		Price:         d.Price,
		AllowCheckin:  d.AllowCheckin,
		SortOrder:     d.SortOrder,
		Enabled:       d.Enabled,
		RedeemedCount: redeemed,
		CreatedAt:     d.CreatedAt,
		UpdatedAt:     d.UpdatedAt,
	}
}
