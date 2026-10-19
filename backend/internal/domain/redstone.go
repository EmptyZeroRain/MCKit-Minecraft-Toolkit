package domain

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"mckit/backend/internal/repository/types"
)

// 红石元件类别（TODO §15）。
//   - source：信号源（红石火把、拉杆、按钮、压力板、侦测器等）
//   - transmission：传输（红石粉、红石中继器、红石比较器等）
//   - logic：逻辑（与 / 或 / 非 / 锁存等逻辑门元件）
//   - actuator：执行（活塞、粘性活塞、发射器、投掷器、红石灯、TNT 等）
//   - sensor：传感（侦测器、阳光探测器、标靶、幽匿感测体等）
//   - other：其他 / 未归类
const (
	RedstoneComponentCategorySource       = "source"
	RedstoneComponentCategoryTransmission = "transmission"
	RedstoneComponentCategoryLogic        = "logic"
	RedstoneComponentCategoryActuator     = "actuator"
	RedstoneComponentCategorySensor       = "sensor"
	RedstoneComponentCategoryOther        = "other"
)

// RedstoneComponentCategories 全部合法元件类别，顺序即前端筛选与展示顺序。
var RedstoneComponentCategories = []string{
	RedstoneComponentCategorySource,
	RedstoneComponentCategoryTransmission,
	RedstoneComponentCategoryLogic,
	RedstoneComponentCategoryActuator,
	RedstoneComponentCategorySensor,
	RedstoneComponentCategoryOther,
}

// IsValidRedstoneComponentCategory 判断元件类别是否合法。
func IsValidRedstoneComponentCategory(category string) bool {
	for _, c := range RedstoneComponentCategories {
		if c == category {
			return true
		}
	}
	return false
}

// 机器设计用途（TODO §15）。
//   - farm：农场（作物 / 生物 / 刷怪等自动化产出）
//   - sorter：分类机（物品分类与仓储）
//   - transport：运输（物品 / 玩家 / 生物的位移系统）
//   - door：门与机关（隐藏门、陷阱、安全机关）
//   - logic：逻辑电路（计数器、计时器、编码 / 解码等）
//   - utility：其他实用装置（照明、开关、自动门等）
//   - other：其他 / 未归类
const (
	RedstoneDesignPurposeFarm      = "farm"
	RedstoneDesignPurposeSorter    = "sorter"
	RedstoneDesignPurposeTransport = "transport"
	RedstoneDesignPurposeDoor      = "door"
	RedstoneDesignPurposeLogic     = "logic"
	RedstoneDesignPurposeUtility   = "utility"
	RedstoneDesignPurposeOther     = "other"
)

// RedstoneDesignPurposes 全部合法设计用途，顺序即前端筛选与展示顺序。
var RedstoneDesignPurposes = []string{
	RedstoneDesignPurposeFarm,
	RedstoneDesignPurposeSorter,
	RedstoneDesignPurposeTransport,
	RedstoneDesignPurposeDoor,
	RedstoneDesignPurposeLogic,
	RedstoneDesignPurposeUtility,
	RedstoneDesignPurposeOther,
}

// IsValidRedstoneDesignPurpose 判断设计用途是否合法。
func IsValidRedstoneDesignPurpose(purpose string) bool {
	for _, p := range RedstoneDesignPurposes {
		if p == purpose {
			return true
		}
	}
	return false
}

// 设计难度（TODO §15）。
const (
	RedstoneDesignDifficultyEasy   = "easy"
	RedstoneDesignDifficultyMedium = "medium"
	RedstoneDesignDifficultyHard   = "hard"
)

// RedstoneDesignDifficulties 全部合法难度，顺序即前端筛选与展示顺序。
var RedstoneDesignDifficulties = []string{
	RedstoneDesignDifficultyEasy,
	RedstoneDesignDifficultyMedium,
	RedstoneDesignDifficultyHard,
}

// IsValidRedstoneDesignDifficulty 判断设计难度是否合法。
func IsValidRedstoneDesignDifficulty(difficulty string) bool {
	for _, d := range RedstoneDesignDifficulties {
		if d == difficulty {
			return true
		}
	}
	return false
}

// 内容审核状态（TODO §15）。
//   - pending：待审核（默认，不对公开接口可见）
//   - published：已发布（仅此状态对公开接口可见）
//   - rejected：已驳回
const (
	RedstoneReviewPending   = "pending"
	RedstoneReviewPublished = "published"
	RedstoneReviewRejected  = "rejected"
)

// RedstoneReviewStatuses 全部合法审核状态，顺序即前端筛选与展示顺序。
var RedstoneReviewStatuses = []string{
	RedstoneReviewPending,
	RedstoneReviewPublished,
	RedstoneReviewRejected,
}

// IsValidRedstoneReviewStatus 判断审核状态是否合法。
func IsValidRedstoneReviewStatus(status string) bool {
	for _, s := range RedstoneReviewStatuses {
		if s == status {
			return true
		}
	}
	return false
}

// RedstoneMaterial 机器设计的单条材料。
// 数量用区间表达（count_max 缺省等于 count_min）；ItemID 为空串表示未收录物品，
// 此时 NameZh 必填，前端回退为纯文本展示。
type RedstoneMaterial struct {
	ItemID   string `json:"item_id"`
	NameZh   string `json:"name_zh"`
	CountMin int    `json:"count_min"`
	CountMax int    `json:"count_max"`
}

// RedstoneMaterials 材料清单，作为 jsonb 数组持久化。
type RedstoneMaterials []RedstoneMaterial

// Value 实现 driver.Valuer。
func (m RedstoneMaterials) Value() (driver.Value, error) {
	if m == nil {
		m = RedstoneMaterials{}
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// Scan 实现 sql.Scanner。
func (m *RedstoneMaterials) Scan(value any) error {
	if value == nil {
		*m = nil
		return nil
	}
	var raw []byte
	switch v := value.(type) {
	case string:
		raw = []byte(v)
	case []byte:
		raw = v
	default:
		return fmt.Errorf("无法扫描类型 %T 到 RedstoneMaterials", value)
	}
	if len(raw) == 0 {
		*m = nil
		return nil
	}
	var out []RedstoneMaterial
	if err := json.Unmarshal(raw, &out); err != nil {
		return err
	}
	*m = out
	return nil
}

// MarshalJSON 直接输出原生数组。
func (m RedstoneMaterials) MarshalJSON() ([]byte, error) {
	if m == nil {
		return []byte("[]"), nil
	}
	return json.Marshal([]RedstoneMaterial(m))
}

// RedstoneComponent 红石元件资料（TODO §15）。
// 信号强度 / 延迟等数值不确定，统一用文字说明，不写死未经核验的数字。
type RedstoneComponent struct {
	ID                uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Slug              string          `gorm:"uniqueIndex;size:120;not null" json:"slug"`
	NameZh            string          `gorm:"size:120;not null" json:"name_zh"`
	NameEn            string          `gorm:"size:120" json:"name_en"`
	Aliases           types.JSONSlice `gorm:"type:jsonb;default:'[]'" json:"aliases"`
	Category          string          `gorm:"size:30;not null;default:other" json:"category"`
	MechanismNote     string          `gorm:"column:mechanism_note;type:text" json:"mechanism_note"`
	SignalNote        string          `gorm:"column:signal_note;type:text" json:"signal_note"`
	Summary           string          `gorm:"type:text" json:"summary"`
	GameEdition       string          `gorm:"size:20;not null;default:java" json:"game_edition"`
	SupportedVersions types.JSONSlice `gorm:"type:jsonb;default:'[]'" json:"supported_versions"`
	SourceURL         string          `gorm:"type:text" json:"source_url"`
	DataVersion       string          `gorm:"size:60" json:"data_version"`
	LastVerifiedAt    *time.Time      `json:"last_verified_at"`
	SortOrder         int             `gorm:"not null;default:0" json:"sort_order"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

// TableName 固定表名，避免 GORM 复数化推测。
func (RedstoneComponent) TableName() string { return "redstone_components" }

// RedstoneDesign 机器设计资料（TODO §15）。
// 仅 review_status = published 的记录对公开接口可见。
type RedstoneDesign struct {
	ID                uuid.UUID         `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Slug              string            `gorm:"uniqueIndex;size:120;not null" json:"slug"`
	NameZh            string            `gorm:"size:120;not null" json:"name_zh"`
	NameEn            string            `gorm:"size:120" json:"name_en"`
	Aliases           types.JSONSlice   `gorm:"type:jsonb;default:'[]'" json:"aliases"`
	Purpose           string            `gorm:"size:30;not null;default:other" json:"purpose"`
	Difficulty        string            `gorm:"size:20;not null;default:medium" json:"difficulty"`
	Summary           string            `gorm:"type:text" json:"summary"`
	StepsMd           string            `gorm:"column:steps_md;type:text" json:"steps_md"`
	Materials         RedstoneMaterials `gorm:"type:jsonb;default:'[]'" json:"materials"`
	Author            string            `gorm:"size:120" json:"author"`
	AuthorURL         string            `gorm:"column:author_url;type:text" json:"author_url"`
	ScreenshotURL     string            `gorm:"column:screenshot_url;type:text" json:"screenshot_url"`
	VideoURL          string            `gorm:"column:video_url;type:text" json:"video_url"`
	WorldDownloadURL  string            `gorm:"column:world_download_url;type:text" json:"world_download_url"`
	WorldDownloadNote string            `gorm:"column:world_download_note;type:text" json:"world_download_note"`
	License           string            `gorm:"size:80" json:"license"`
	LicenseNote       string            `gorm:"column:license_note;type:text" json:"license_note"`
	SupportedVersions types.JSONSlice   `gorm:"type:jsonb;default:'[]'" json:"supported_versions"`
	GameEdition       string            `gorm:"size:20;not null;default:java" json:"game_edition"`
	SourceURL         string            `gorm:"type:text" json:"source_url"`
	DataVersion       string            `gorm:"size:60" json:"data_version"`
	LastVerifiedAt    *time.Time        `json:"last_verified_at"`
	ReviewStatus      string            `gorm:"column:review_status;size:20;not null;default:pending" json:"review_status"`
	Reviewer          string            `gorm:"size:120" json:"reviewer"`
	ReviewedAt        *time.Time        `json:"reviewed_at"`
	ReviewNote        string            `gorm:"column:review_note;type:text" json:"review_note"`
	SortOrder         int               `gorm:"not null;default:0" json:"sort_order"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
}

// TableName 固定表名，避免 GORM 复数化推测。
func (RedstoneDesign) TableName() string { return "redstone_designs" }
