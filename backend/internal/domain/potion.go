package domain

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"mckit/backend/internal/repository/types"
)

// 药水资料类别（TODO §9A）。
//   - water：水瓶（所有药水的起点）
//   - base：基础/前置药水（粗制、平淡、浓稠）
//   - positive / negative / mixed：增益 / 负面 / 混合效果药水
//   - other：不可通过酿造获得者（不可合成的药水等）
const (
	PotionCategoryWater    = "water"
	PotionCategoryBase     = "base"
	PotionCategoryPositive = "positive"
	PotionCategoryNegative = "negative"
	PotionCategoryMixed    = "mixed"
	PotionCategoryOther    = "other"
)

// PotionCategories 全部合法类别，顺序即前端筛选展示顺序。
var PotionCategories = []string{
	PotionCategoryWater,
	PotionCategoryBase,
	PotionCategoryPositive,
	PotionCategoryNegative,
	PotionCategoryMixed,
	PotionCategoryOther,
}

// IsValidPotionCategory 判断药水类别是否合法。
func IsValidPotionCategory(category string) bool {
	for _, c := range PotionCategories {
		if c == category {
			return true
		}
	}
	return false
}

// 酿造步骤类型（TODO §9A）。
//   - base：水瓶 + 原料 → 基础药水
//   - effect：粗制药水 + 效果原料 → 效果药水
//   - corrupt：药水 + 发酵蛛眼 → 改变效果
//   - modifier：作用于任意适用药水的通用修饰（红石延长 / 萤石增强）
//   - form：作用于任意适用药水的形态转换（火药喷溅 / 龙息滞留）
const (
	BrewStepBase     = "base"
	BrewStepEffect   = "effect"
	BrewStepCorrupt  = "corrupt"
	BrewStepModifier = "modifier"
	BrewStepForm     = "form"
)

// BrewStepKinds 全部合法酿造步骤类型，顺序即前端分组展示顺序。
var BrewStepKinds = []string{
	BrewStepBase,
	BrewStepEffect,
	BrewStepCorrupt,
	BrewStepModifier,
	BrewStepForm,
}

// IsValidBrewStepKind 判断酿造步骤类型是否合法。
func IsValidBrewStepKind(kind string) bool {
	for _, k := range BrewStepKinds {
		if k == kind {
			return true
		}
	}
	return false
}

// 效果变体（时长口径）：normal 普通 / extended 红石延长 / enhanced 萤石增强。
const (
	PotionVariantNormal   = "normal"
	PotionVariantExtended = "extended"
	PotionVariantEnhanced = "enhanced"
)

// PotionEffect 药水单条效果。同一药水在不同变体（普通/延长/增强）下分别记录一条，
// 前端据此渲染"普通/延长/增强"三列，避免在展示层推算规则。
type PotionEffect struct {
	Variant         string `json:"variant"`          // normal | extended | enhanced
	EffectID        string `json:"effect_id"`        // 效果命名空间 ID，如 minecraft:speed
	NameZh          string `json:"name_zh"`          // 效果中文名
	NameEn          string `json:"name_en"`          // 效果英文名
	Amplifier       int    `json:"amplifier"`        // 等级：1 表示 I 级
	DurationSeconds int    `json:"duration_seconds"` // 持续时间（秒）；0 表示瞬间效果
	Description     string `json:"description"`      // 效果说明（如"恢复 4 点生命值"）
}

// PotionEffects 效果集合，作为 jsonb 数组持久化。
type PotionEffects []PotionEffect

// Value 实现 driver.Valuer。
func (p PotionEffects) Value() (driver.Value, error) {
	if p == nil {
		p = PotionEffects{}
	}
	b, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// Scan 实现 sql.Scanner。
func (p *PotionEffects) Scan(value any) error {
	if value == nil {
		*p = nil
		return nil
	}
	var raw []byte
	switch v := value.(type) {
	case string:
		raw = []byte(v)
	case []byte:
		raw = v
	default:
		return fmt.Errorf("无法扫描类型 %T 到 PotionEffects", value)
	}
	if len(raw) == 0 {
		*p = nil
		return nil
	}
	var out []PotionEffect
	if err := json.Unmarshal(raw, &out); err != nil {
		return err
	}
	*p = out
	return nil
}

// MarshalJSON 直接输出原生数组。
func (p PotionEffects) MarshalJSON() ([]byte, error) {
	if p == nil {
		return []byte("[]"), nil
	}
	return json.Marshal([]PotionEffect(p))
}

// Potion 药水资料（TODO §9A）。
type Potion struct {
	ID                uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Slug              string          `gorm:"uniqueIndex;size:160;not null" json:"slug"`
	NameZh            string          `gorm:"size:160;not null" json:"name_zh"`
	NameEn            string          `gorm:"size:160" json:"name_en"`
	Aliases           types.JSONSlice `gorm:"type:jsonb;default:'[]'" json:"aliases"`
	Category          string          `gorm:"size:40;not null;default:effect" json:"category"`
	Summary           string          `gorm:"type:text" json:"summary"`
	EffectNote        string          `gorm:"column:effect_note;type:text" json:"effect_note"`
	IsBrewable        bool            `gorm:"not null;default:true" json:"is_brewable"`
	IsExtendable      bool            `gorm:"not null;default:false" json:"is_extendable"`
	IsEnhanceable     bool            `gorm:"not null;default:false" json:"is_enhanceable"`
	Effects           PotionEffects   `gorm:"type:jsonb;default:'[]'" json:"effects"`
	GameEdition       string          `gorm:"size:20;not null;default:java" json:"game_edition"`
	SupportedVersions types.JSONSlice `gorm:"type:jsonb;default:'[]'" json:"supported_versions"`
	SourceURL         string          `gorm:"type:text" json:"source_url"`
	DataVersion       string          `gorm:"size:80" json:"data_version"`
	LastVerifiedAt    *time.Time      `json:"last_verified_at"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

// TableName 固定表名，避免 GORM 复数化推测。
func (Potion) TableName() string { return "potions" }

// BrewingRecipe 酿造步骤（TODO §9A）。
// input_potion_slug 为 NULL 表示从水瓶起步；result_potion_slug 为 NULL 表示通用修饰/形态规则。
type BrewingRecipe struct {
	ID                uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	RecipeKey         string          `gorm:"uniqueIndex;size:240;not null" json:"recipe_key"`
	StepKind          string          `gorm:"size:40;not null" json:"step_kind"`
	InputPotionSlug   *string         `gorm:"size:160" json:"input_potion_slug"`
	IngredientItemID  string          `gorm:"size:200;not null" json:"ingredient_item_id"`
	IngredientNameZh  string          `gorm:"size:160;not null" json:"ingredient_name_zh"`
	ResultPotionSlug  *string         `gorm:"size:160" json:"result_potion_slug"`
	Note              string          `gorm:"type:text" json:"note"`
	GameEdition       string          `gorm:"size:20;not null;default:java" json:"game_edition"`
	SupportedVersions types.JSONSlice `gorm:"type:jsonb;default:'[]'" json:"supported_versions"`
	SourceURL         string          `gorm:"type:text" json:"source_url"`
	SortOrder         int             `gorm:"not null;default:0" json:"sort_order"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

// TableName 固定表名，避免 GORM 复数化推测。
func (BrewingRecipe) TableName() string { return "brewing_recipes" }
