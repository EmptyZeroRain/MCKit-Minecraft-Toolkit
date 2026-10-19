package domain

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"mckit/backend/internal/repository/types"
)

// 村民职业类别（TODO §9B）。
//   - normal：可交易职业（13 种，拥有工作站与交易表）
//   - wandering：流浪商人（无工作站、无职业等级，交易分「收购 / 特殊 / 普通」三类）
//   - non_trading：不参与交易（傻子、失业者）
const (
	VillagerKindNormal     = "normal"
	VillagerKindWandering  = "wandering"
	VillagerKindNonTrading = "non_trading"
)

// VillagerKinds 全部合法职业类别，顺序即前端筛选展示顺序。
var VillagerKinds = []string{
	VillagerKindNormal,
	VillagerKindWandering,
	VillagerKindNonTrading,
}

// IsValidVillagerKind 判断职业类别是否合法。
func IsValidVillagerKind(kind string) bool {
	for _, k := range VillagerKinds {
		if k == kind {
			return true
		}
	}
	return false
}

// 交易分组（TODO §9B）。
//   - level：随职业等级解锁（普通职业的 1–5 级交易）
//   - purchase：流浪商人收购玩家物品
//   - special：流浪商人的特殊交易（无法从普通职业获得）
//   - ordinary：流浪商人的普通售卖项
const (
	VillagerTradeGroupLevel    = "level"
	VillagerTradeGroupPurchase = "purchase"
	VillagerTradeGroupSpecial  = "special"
	VillagerTradeGroupOrdinary = "ordinary"
)

// VillagerTradeGroups 全部合法交易分组，顺序即前端分组展示顺序。
var VillagerTradeGroups = []string{
	VillagerTradeGroupLevel,
	VillagerTradeGroupPurchase,
	VillagerTradeGroupSpecial,
	VillagerTradeGroupOrdinary,
}

// IsValidVillagerTradeGroup 判断交易分组是否合法。
func IsValidVillagerTradeGroup(group string) bool {
	for _, g := range VillagerTradeGroups {
		if g == group {
			return true
		}
	}
	return false
}

// VillagerCostItem 交易中「村民想要」的单条物品。
// 数量用区间表达，以忠实还原 Minecraft Wiki 的「19–33 个绿宝石」这类浮动定价，
// 固定数量时 CountMin == CountMax。
type VillagerCostItem struct {
	ItemID   string `json:"item_id"`
	NameZh   string `json:"name_zh"`
	CountMin int    `json:"count_min"`
	CountMax int    `json:"count_max"`
}

// VillagerCostItems 交易成本清单，作为 jsonb 数组持久化。
type VillagerCostItems []VillagerCostItem

// Value 实现 driver.Valuer。
func (c VillagerCostItems) Value() (driver.Value, error) {
	if c == nil {
		c = VillagerCostItems{}
	}
	b, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// Scan 实现 sql.Scanner。
func (c *VillagerCostItems) Scan(value any) error {
	if value == nil {
		*c = nil
		return nil
	}
	var raw []byte
	switch v := value.(type) {
	case string:
		raw = []byte(v)
	case []byte:
		raw = v
	default:
		return fmt.Errorf("无法扫描类型 %T 到 VillagerCostItems", value)
	}
	if len(raw) == 0 {
		*c = nil
		return nil
	}
	var out []VillagerCostItem
	if err := json.Unmarshal(raw, &out); err != nil {
		return err
	}
	*c = out
	return nil
}

// MarshalJSON 直接输出原生数组。
func (c VillagerCostItems) MarshalJSON() ([]byte, error) {
	if c == nil {
		return []byte("[]"), nil
	}
	return json.Marshal([]VillagerCostItem(c))
}

// VillagerProfession 村民职业资料（TODO §9B）。
type VillagerProfession struct {
	ID                uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Slug              string          `gorm:"uniqueIndex;size:120;not null" json:"slug"`
	NameZh            string          `gorm:"size:120;not null" json:"name_zh"`
	NameEn            string          `gorm:"size:120" json:"name_en"`
	Aliases           types.JSONSlice `gorm:"type:jsonb;default:'[]'" json:"aliases"`
	Kind              string          `gorm:"size:30;not null;default:normal" json:"kind"`
	WorkstationItemID string          `gorm:"column:workstation_item_id;size:200" json:"workstation_item_id"`
	WorkstationNameZh string          `gorm:"column:workstation_name_zh;size:160" json:"workstation_name_zh"`
	Summary           string          `gorm:"type:text" json:"summary"`
	Description       string          `gorm:"type:text" json:"description"`
	TradeNote         string          `gorm:"column:trade_note;type:text" json:"trade_note"`
	GameEdition       string          `gorm:"size:20;not null;default:java" json:"game_edition"`
	SupportedVersions types.JSONSlice `gorm:"type:jsonb;default:'[]'" json:"supported_versions"`
	SourceURL         string          `gorm:"type:text" json:"source_url"`
	DataVersion       string          `gorm:"size:80" json:"data_version"`
	LastVerifiedAt    *time.Time      `json:"last_verified_at"`
	SortOrder         int             `gorm:"not null;default:0" json:"sort_order"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

// TableName 固定表名，避免 GORM 复数化推测。
func (VillagerProfession) TableName() string { return "villager_professions" }

// VillagerTradeLevel 职业等级定义（通用规则，存库不硬编码）。
type VillagerTradeLevel struct {
	ID                 uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Level              int       `gorm:"uniqueIndex;not null" json:"level"`
	Slug               string    `gorm:"uniqueIndex;size:40;not null" json:"slug"`
	NameZh             string    `gorm:"size:40;not null" json:"name_zh"`
	NameEn             string    `gorm:"size:40" json:"name_en"`
	RequiredExperience int       `gorm:"column:required_experience;not null;default:0" json:"required_experience"`
	BadgeNote          string    `gorm:"column:badge_note;type:text" json:"badge_note"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// TableName 固定表名，避免 GORM 复数化推测。
func (VillagerTradeLevel) TableName() string { return "villager_trade_levels" }

// VillagerTrade 一条村民交易（TODO §9B）。
// 方向不单列：cost_items 为「村民想要」，result 为「玩家获得」，
// 哪一侧是绿宝石即可判断买卖，与 Minecraft Wiki 交易表两列一一对应。
type VillagerTrade struct {
	ID                 uuid.UUID         `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TradeKey           string            `gorm:"column:trade_key;uniqueIndex;size:240;not null" json:"trade_key"`
	ProfessionSlug     string            `gorm:"column:profession_slug;size:120;not null" json:"profession_slug"`
	Level              int               `gorm:"not null;default:0" json:"level"`
	TradeGroup         string            `gorm:"column:trade_group;size:30;not null;default:level" json:"trade_group"`
	CostItems          VillagerCostItems `gorm:"column:cost_items;type:jsonb;default:'[]'" json:"cost_items"`
	ResultItemID       string            `gorm:"column:result_item_id;size:200;not null" json:"result_item_id"`
	ResultItemNameZh   string            `gorm:"column:result_item_name_zh;size:160;not null" json:"result_item_name_zh"`
	ResultCount        int               `gorm:"column:result_count;not null;default:1" json:"result_count"`
	MaxUses            int               `gorm:"column:max_uses;not null;default:12" json:"max_uses"`
	VillagerExperience int               `gorm:"column:villager_experience;not null;default:0" json:"villager_experience"`
	PriceMultiplier    float64           `gorm:"column:price_multiplier;type:numeric(4,2);not null;default:0.05" json:"price_multiplier"`
	VariantNote        string            `gorm:"column:variant_note;type:text" json:"variant_note"`
	Note               string            `gorm:"type:text" json:"note"`
	GameEdition        string            `gorm:"size:20;not null;default:java" json:"game_edition"`
	SupportedVersions  types.JSONSlice   `gorm:"type:jsonb;default:'[]'" json:"supported_versions"`
	SourceURL          string            `gorm:"type:text" json:"source_url"`
	SortOrder          int               `gorm:"not null;default:0" json:"sort_order"`
	CreatedAt          time.Time         `json:"created_at"`
	UpdatedAt          time.Time         `json:"updated_at"`
}

// TableName 固定表名，避免 GORM 复数化推测。
func (VillagerTrade) TableName() string { return "villager_trades" }
