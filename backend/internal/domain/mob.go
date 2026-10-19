package domain

import (
	"time"

	"github.com/google/uuid"

	"mckit/backend/internal/repository/types"
)

// 生物类别（TODO §9C）。
//   - passive：被动型（不主动攻击玩家）
//   - neutral：中立型（被攻击或触发条件后才反击）
//   - hostile：敌对型（主动攻击玩家）
//   - boss：Boss（末影龙、凋灵等）
//   - utility：功能型 / 可交互（村民、铁傀儡等以交互为主）
//   - other：其他 / 未归类
const (
	MobCategoryPassive = "passive"
	MobCategoryNeutral = "neutral"
	MobCategoryHostile = "hostile"
	MobCategoryBoss    = "boss"
	MobCategoryUtility = "utility"
	MobCategoryOther   = "other"
)

// MobCategories 全部合法生物类别，顺序即前端筛选与展示顺序。
var MobCategories = []string{
	MobCategoryPassive,
	MobCategoryNeutral,
	MobCategoryHostile,
	MobCategoryBoss,
	MobCategoryUtility,
	MobCategoryOther,
}

// IsValidMobCategory 判断生物类别是否合法。
func IsValidMobCategory(category string) bool {
	for _, c := range MobCategories {
		if c == category {
			return true
		}
	}
	return false
}

// 维度（生成 / 出现范围）。
const (
	MobDimensionOverworld = "overworld"
	MobDimensionNether    = "nether"
	MobDimensionEnd       = "end"
)

// MobDimensions 全部合法维度，顺序即前端展示顺序。
var MobDimensions = []string{
	MobDimensionOverworld,
	MobDimensionNether,
	MobDimensionEnd,
}

// IsValidMobDimension 判断维度是否合法。
func IsValidMobDimension(dim string) bool {
	for _, d := range MobDimensions {
		if d == dim {
			return true
		}
	}
	return false
}

// Mob 生物资料（TODO §9C）。
//
// 血量 health 使用 Java 版生命值（1 点 = 半颗心），无法确定时为 nil；
// 攻击伤害因难度 / 版本差异较大，改用文字说明，不写死单一数字。
type Mob struct {
	ID                uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Slug              string          `gorm:"uniqueIndex;size:120;not null" json:"slug"`
	NameZh            string          `gorm:"size:120;not null" json:"name_zh"`
	NameEn            string          `gorm:"size:120" json:"name_en"`
	Aliases           types.JSONSlice `gorm:"type:jsonb;default:'[]'" json:"aliases"`
	Category          string          `gorm:"size:30;not null;default:other" json:"category"`
	Health            *float64        `gorm:"type:numeric(6,1)" json:"health"`
	AttackNote        string          `gorm:"column:attack_note;type:text" json:"attack_note"`
	BehaviorNote      string          `gorm:"column:behavior_note;type:text" json:"behavior_note"`
	SpawnNote         string          `gorm:"column:spawn_note;type:text" json:"spawn_note"`
	TamingNote        string          `gorm:"column:taming_note;type:text" json:"taming_note"`
	BreedingNote      string          `gorm:"column:breeding_note;type:text" json:"breeding_note"`
	Dimensions        types.JSONSlice `gorm:"type:jsonb;default:'[]'" json:"dimensions"`
	BiomesNote        string          `gorm:"column:biomes_note;type:text" json:"biomes_note"`
	Summary           string          `gorm:"type:text" json:"summary"`
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
func (Mob) TableName() string { return "mobs" }

// MobDrop 一条生物掉落（TODO §9C）。
// 数量用区间表达（count_min / count_max），概率用 chance（0–1，1 表示必定掉落）。
type MobDrop struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	DropKey    string    `gorm:"column:drop_key;uniqueIndex;size:240;not null" json:"drop_key"`
	MobSlug    string    `gorm:"column:mob_slug;size:120;not null" json:"mob_slug"`
	ItemID     string    `gorm:"column:item_id;size:200;not null" json:"item_id"`
	ItemNameZh string    `gorm:"column:item_name_zh;size:160;not null" json:"item_name_zh"`
	CountMin   int       `gorm:"column:count_min;not null" json:"count_min"`
	CountMax   int       `gorm:"column:count_max;not null" json:"count_max"`
	// Chance 不加 default 标签：GORM 会省略零值字段而改用数据库默认值，
	// 导致显式写入的数值被覆盖；此处必须保证写入值如实落库。
	Chance     float64   `gorm:"type:numeric(5,4);not null" json:"chance"`
	Conditions string    `gorm:"type:text" json:"conditions"`
	Note       string    `gorm:"type:text" json:"note"`
	SortOrder  int       `gorm:"not null;default:0" json:"sort_order"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// TableName 固定表名，避免 GORM 复数化推测。
func (MobDrop) TableName() string { return "mob_drops" }
