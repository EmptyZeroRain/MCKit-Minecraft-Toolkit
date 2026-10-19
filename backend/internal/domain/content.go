package domain

import (
	"time"

	"github.com/google/uuid"

	"mckit/backend/internal/repository/types"
)

// 工具分类枚举（需求 §5.2，中文展示值与前端契约一致）。
const (
	ToolCategoryQuery       = "查询类"
	ToolCategoryCalculation = "计算类"
	ToolCategoryGeneration  = "生成类"
	ToolCategoryEditing     = "编辑类"
	ToolCategoryAuxiliary   = "辅助类"
)

// Tool 工具（需求 §7.3）。
type Tool struct {
	ID                uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Slug              string          `gorm:"uniqueIndex;size:180;not null" json:"slug"`
	Name              string          `gorm:"size:160;not null" json:"name"`
	Summary           string          `gorm:"size:500;not null" json:"summary"`
	Description       string          `gorm:"type:text" json:"description"`
	Category          string          `gorm:"size:40;not null" json:"category"`
	Icon              string          `gorm:"size:80" json:"icon"`
	Tags              types.JSONSlice `gorm:"type:jsonb;default:'[]'" json:"tags"`
	SupportedVersions types.JSONSlice `gorm:"type:jsonb;default:'[]'" json:"supported_versions"`
	InputSchema       types.JSONB     `gorm:"type:jsonb;default:'[]'" json:"input_schema"`
	OutputSchema      types.JSONB     `gorm:"type:jsonb;default:'[]'" json:"output_schema"`
	IsFeatured        bool            `gorm:"not null;default:false" json:"is_featured"`
	SortOrder         int             `gorm:"not null;default:0" json:"sort_order"`
	Status            string          `gorm:"size:20;not null;default:published" json:"status"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

// TableName 指定表名。
func (Tool) TableName() string { return "tools" }

// WikiEntry 百科条目（需求 §7.4）。
type WikiEntry struct {
	ID               uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Slug             string          `gorm:"uniqueIndex;size:180;not null" json:"slug"`
	Title            string          `gorm:"size:240;not null" json:"title"`
	Summary          string          `gorm:"size:500;not null;default:''" json:"summary"`
	Content          string          `gorm:"type:text;not null" json:"content"`
	ContentFormat    string          `gorm:"size:20;not null;default:markdown" json:"content_format"`
	ContentType      string          `gorm:"size:40;not null" json:"content_type"`
	GameEdition      string          `gorm:"size:20;not null;default:java" json:"game_edition"`
	GameVersions     types.JSONSlice `gorm:"type:jsonb;default:'[]'" json:"game_versions"`
	Tags             types.JSONSlice `gorm:"type:jsonb;default:'[]'" json:"tags"`
	SourceReferences types.JSONB     `gorm:"type:jsonb;default:'[]'" json:"source_references"`
	Status           string          `gorm:"size:20;not null;default:draft" json:"status"`
	PublishedAt      *time.Time      `json:"published_at"`
	CreatedBy        *uuid.UUID      `gorm:"type:uuid" json:"-"`
	UpdatedBy        *uuid.UUID      `gorm:"type:uuid" json:"-"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
	DeletedAt        *time.Time      `json:"-"`
}

// TableName 指定表名。
func (WikiEntry) TableName() string { return "wiki_entries" }

// Category 分类（需求 §7.8）。
type Category struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name        string    `gorm:"size:120;not null" json:"name"`
	Slug        string    `gorm:"size:180;not null" json:"slug"`
	ContentType string    `gorm:"size:40;not null" json:"content_type"` // tool | resource | wiki
	SortOrder   int       `gorm:"not null;default:0" json:"sort_order"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TableName 指定表名。
func (Category) TableName() string { return "categories" }

// Tag 标签（需求 §7.8）。
type Tag struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name      string    `gorm:"size:120;not null" json:"name"`
	Slug      string    `gorm:"uniqueIndex;size:180;not null" json:"slug"`
	CreatedAt time.Time `json:"created_at"`
}

// TableName 指定表名。
func (Tag) TableName() string { return "tags" }

// 反馈 / 投稿类型（TODO §5.10、§17）。
//   - bug：功能错误反馈
//   - resource_broken：资源失效 / 链接失效
//   - infringement：侵权举报
//   - data_correction：资料纠错（本站资料有误）
//   - resource_supplement：资源补充信息（补充来源 / 版本 / 许可等）
//   - other：其他
const (
	FeedbackTypeBug                = "bug"
	FeedbackTypeResourceBroken     = "resource_broken"
	FeedbackTypeInfringement       = "infringement"
	FeedbackTypeDataCorrection     = "data_correction"
	FeedbackTypeResourceSupplement = "resource_supplement"
	FeedbackTypeOther              = "other"
)

// FeedbackTypes 全部合法反馈 / 投稿类型。
var FeedbackTypes = []string{
	FeedbackTypeBug,
	FeedbackTypeResourceBroken,
	FeedbackTypeInfringement,
	FeedbackTypeDataCorrection,
	FeedbackTypeResourceSupplement,
	FeedbackTypeOther,
}

// IsValidFeedbackType 判断反馈类型是否合法。
func IsValidFeedbackType(t string) bool {
	for _, v := range FeedbackTypes {
		if v == t {
			return true
		}
	}
	return false
}

// 反馈 / 投稿关联的资料条目类型（TODO §17：投稿需定位到具体资料）。
const (
	FeedbackTargetResource = "resource"
	FeedbackTargetItem     = "item"
	FeedbackTargetDevDoc   = "dev_doc"
	FeedbackTargetMob      = "mob"
	FeedbackTargetVillager = "villager"
	FeedbackTargetPotion   = "potion"
	FeedbackTargetTool     = "tool"
	FeedbackTargetOther    = "other"
)

// FeedbackTargetKinds 全部合法关联资料类型。
var FeedbackTargetKinds = []string{
	FeedbackTargetResource,
	FeedbackTargetItem,
	FeedbackTargetDevDoc,
	FeedbackTargetMob,
	FeedbackTargetVillager,
	FeedbackTargetPotion,
	FeedbackTargetTool,
	FeedbackTargetOther,
}

// IsValidFeedbackTargetKind 判断关联资料类型是否合法。
func IsValidFeedbackTargetKind(k string) bool {
	for _, v := range FeedbackTargetKinds {
		if v == k {
			return true
		}
	}
	return false
}

// 反馈 / 投稿状态（待审核 → 处理中 → 已采纳 / 已忽略）。
const (
	FeedbackStatusNew        = "new"
	FeedbackStatusProcessing = "processing"
	FeedbackStatusResolved   = "resolved"
	FeedbackStatusIgnored    = "ignored"
)

// FeedbackStatuses 全部合法状态。
var FeedbackStatuses = []string{
	FeedbackStatusNew,
	FeedbackStatusProcessing,
	FeedbackStatusResolved,
	FeedbackStatusIgnored,
}

// IsValidFeedbackStatus 判断状态是否合法。
func IsValidFeedbackStatus(s string) bool {
	for _, v := range FeedbackStatuses {
		if v == s {
			return true
		}
	}
	return false
}

// FeedbackLicenseVersion 当前投稿授权条款版本。
//
// 投稿内容默认授权本站按开源许可整理、转载与再发布；
// 版本号变化时需重新确认，历史投稿保留其当时接受的版本。
const FeedbackLicenseVersion = "2026-10-18"

// Feedback 反馈 / 投稿（需求 §5.10、§7.9、§17）。
//
// 投稿（data_correction / resource_supplement）与举报共用一张表：
// 均默认进入待审核状态，审核结果写入 Status / Reviewer / ReviewedAt / ReviewNote，
// 完整流转历史见 FeedbackReview。
type Feedback struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	FeedbackType string    `gorm:"size:40;not null" json:"feedback_type"`
	Title        string    `gorm:"size:200;not null" json:"title"`
	Content      string    `gorm:"type:text;not null" json:"content"`
	RelatedURL   string    `gorm:"type:text" json:"related_url"`
	Contact      string    `gorm:"type:text" json:"contact"`
	Status       string    `gorm:"size:20;not null;default:new" json:"status"`
	// TargetKind / TargetRef 关联的资料条目类型与标识（slug / item_id）。
	TargetKind string `gorm:"column:target_kind;size:40;not null;default:other" json:"target_kind"`
	TargetRef  string `gorm:"column:target_ref;size:200;not null;default:''" json:"target_ref"`
	// Reviewer / ReviewedAt / ReviewNote 最近一次审核的审核人、时间与结论。
	Reviewer   string     `gorm:"size:120;not null;default:''" json:"reviewer"`
	ReviewedAt *time.Time `gorm:"column:reviewed_at" json:"reviewed_at"`
	ReviewNote string     `gorm:"column:review_note;type:text;not null;default:''" json:"review_note"`
	// LicenseVersion / LicenseAcceptedAt 投稿时接受的授权条款版本与时间。
	LicenseVersion    string     `gorm:"column:license_version;size:40;not null;default:''" json:"license_version"`
	LicenseAcceptedAt *time.Time `gorm:"column:license_accepted_at" json:"license_accepted_at"`
	// DedupHash 反垃圾摘要（类型 + 目标 + 标题 + 正文），用于时间窗内查重。
	DedupHash string    `gorm:"column:dedup_hash;size:64;not null;default:''" json:"-"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName 指定表名。
func (Feedback) TableName() string { return "feedback" }

// FeedbackReview 一次审核流转记录（TODO §17：记录审核人、审核结果和变更历史）。
type FeedbackReview struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	FeedbackID uuid.UUID `gorm:"column:feedback_id;type:uuid;not null" json:"feedback_id"`
	FromStatus string    `gorm:"column:from_status;size:20;not null" json:"from_status"`
	ToStatus   string    `gorm:"column:to_status;size:20;not null" json:"to_status"`
	Reviewer   string    `gorm:"size:120;not null;default:''" json:"reviewer"`
	Note       string    `gorm:"type:text;not null;default:''" json:"note"`
	CreatedAt  time.Time `json:"created_at"`
}

// TableName 指定表名。
func (FeedbackReview) TableName() string { return "feedback_reviews" }

// 物品资料类别（与前端 ITEM_CATEGORIES 契约一致，TODO §1）。
// 类别决定可选属性（如耐久度）的适用范围。
const (
	ItemCategoryBlock    = "block"    // 方块
	ItemCategoryItem     = "item"     // 物品
	ItemCategoryTool     = "tool"     // 工具
	ItemCategoryWeapon   = "weapon"   // 武器
	ItemCategoryArmor    = "armor"    // 防具
	ItemCategoryFood     = "food"     // 食物
	ItemCategoryMaterial = "material" // 材料
	ItemCategoryOther    = "other"    // 其他
)

// ItemCategories 全部合法类别（顺序与前端一致）。
var ItemCategories = []string{
	ItemCategoryBlock, ItemCategoryItem, ItemCategoryTool, ItemCategoryWeapon,
	ItemCategoryArmor, ItemCategoryFood, ItemCategoryMaterial, ItemCategoryOther,
}

// IsValidItemCategory 校验物品类别枚举。
func IsValidItemCategory(c string) bool {
	for _, v := range ItemCategories {
		if v == c {
			return true
		}
	}
	return false
}

// Item 物品（需求 §7.5；统一物品资料库 TODO §1）。
type Item struct {
	ID                uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ItemID            string          `gorm:"uniqueIndex;size:200;not null" json:"item_id"`
	NameZh            string          `gorm:"size:200;not null" json:"name_zh"`
	NameEn            string          `gorm:"size:200" json:"name_en"`
	Aliases           types.JSONSlice `gorm:"type:jsonb;default:'[]'" json:"aliases"`
	Description       string          `gorm:"type:text" json:"description"`
	ItemCategory      string          `gorm:"size:40;not null;default:other" json:"item_category"`
	StackSize         *int            `json:"stack_size"`
	Durability        *int            `json:"durability"`
	ObtainMethod      string          `gorm:"type:text" json:"obtain_method"`
	UsageNotes        string          `gorm:"column:usage_notes;type:text" json:"usage"`
	ImageURL          string          `gorm:"type:text" json:"image_url"`
	GameEdition       string          `gorm:"size:20;not null;default:java" json:"game_edition"`
	SupportedVersions types.JSONSlice `gorm:"type:jsonb;default:'[]'" json:"supported_versions"`
	SourceURL         string          `gorm:"type:text" json:"source_url"`
	DataVersion       string          `gorm:"size:80" json:"data_version"`
	LastVerifiedAt    *time.Time      `json:"last_verified_at"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

// TableName 指定表名。
func (Item) TableName() string { return "items" }

// Recipe 配方（需求 §7.6；版本化资料 TODO §2）。
type Recipe struct {
	ID                uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	RecipeKey         string          `gorm:"uniqueIndex;size:240;not null" json:"recipe_key"`
	RecipeType        string          `gorm:"size:40;not null" json:"recipe_type"`
	GameVersion       string          `gorm:"size:40;not null" json:"game_version"`
	ResultItemID      string          `gorm:"size:200;not null" json:"result_item_id"`
	ResultCount       int             `gorm:"not null;default:1" json:"result_count"`
	Pattern           types.JSONB     `gorm:"type:jsonb;default:'[]'" json:"pattern"`
	KeyMapping        types.JSONB     `gorm:"type:jsonb;default:'{}'" json:"key_mapping"`
	Ingredients       types.JSONB     `gorm:"type:jsonb;default:'[]'" json:"ingredients"`
	RequiresCrafting  bool            `gorm:"column:requires_crafting_table;not null;default:false" json:"requires_crafting_table"`
	SupportedVersions types.JSONSlice `gorm:"type:jsonb;default:'[]'" json:"supported_versions"`
	SourceURL         string          `gorm:"type:text" json:"source_url"`
	DataVersion       string          `gorm:"size:80" json:"data_version"`
	LastVerifiedAt    *time.Time      `json:"last_verified_at"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

// TableName 指定表名。
func (Recipe) TableName() string { return "recipes" }

// Command 指令（需求 §7.7；版本化资料 TODO §2）。
type Command struct {
	ID               uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Slug             string          `gorm:"uniqueIndex;size:180;not null" json:"slug"`
	Name             string          `gorm:"size:160;not null" json:"name"`
	Syntax           string          `gorm:"type:text;not null" json:"syntax"`
	Description      string          `gorm:"type:text;not null" json:"description"`
	Parameters       types.JSONB     `gorm:"type:jsonb;default:'[]'" json:"parameters"`
	Examples         types.JSONB     `gorm:"type:jsonb;default:'[]'" json:"examples"`
	PermissionNotes  string          `gorm:"type:text" json:"permission_notes"`
	GameVersions     types.JSONSlice `gorm:"type:jsonb;default:'[]'" json:"game_versions"`
	SourceReferences types.JSONB     `gorm:"type:jsonb;default:'[]'" json:"source_references"`
	SourceURL        string          `gorm:"type:text" json:"source_url"`
	LastVerifiedAt   *time.Time      `json:"last_verified_at"`
	Status           string          `gorm:"size:20;not null;default:published" json:"status"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

// TableName 指定表名。
func (Command) TableName() string { return "commands" }

// DataImportJob 一次数据导入任务的执行记录（TODO §4 数据质量监控）。
// 只记录「来源 / 数量 / 错误摘要」等运行元数据，不含被导入的业务内容本身。
type DataImportJob struct {
	ID            uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	DataType      string          `gorm:"size:40;not null" json:"data_type"`
	SourceFile    string          `gorm:"type:text" json:"source_file"`
	SourceVersion string          `gorm:"size:80" json:"source_version"`
	DryRun        bool            `gorm:"not null;default:false" json:"dry_run"`
	TotalCount    int             `gorm:"not null;default:0" json:"total_count"`
	CreatedCount  int             `gorm:"not null;default:0" json:"created_count"`
	UpdatedCount  int             `gorm:"not null;default:0" json:"updated_count"`
	SkippedCount  int             `gorm:"not null;default:0" json:"skipped_count"`
	FailedCount   int             `gorm:"not null;default:0" json:"failed_count"`
	ErrorSummary  types.JSONSlice `gorm:"type:jsonb;default:'[]'" json:"error_summary"`
	Operator      string          `gorm:"size:80" json:"operator"`
	StartedAt     time.Time       `gorm:"not null;default:now()" json:"started_at"`
	FinishedAt    *time.Time      `json:"finished_at"`
	CreatedAt     time.Time       `json:"created_at"`
}

// TableName 指定表名。
func (DataImportJob) TableName() string { return "data_import_jobs" }

// SearchTerm 搜索词聚合统计（TODO §5 搜索体验）。
//
// 隐私设计：仅保存归一化关键词与计数，不含 IP、会话、用户标识，也不含逐次时间序列。
type SearchTerm struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Term         string    `gorm:"uniqueIndex;size:64;not null" json:"term"`
	Hits         int64     `gorm:"not null;default:0" json:"hits"`
	NoResultHits int64     `gorm:"not null;default:0" json:"no_result_hits"`
	// FailedHits 查询本身失败的次数（超时 / 数据库错误等），与 NoResultHits（查询成功但无结果）区分。
	FailedHits      int64     `gorm:"not null;default:0" json:"failed_hits"`
	LastResultCount int       `gorm:"not null;default:0" json:"last_result_count"`
	FirstSeenAt     time.Time `json:"first_seen_at"`
	LastSeenAt      time.Time `json:"last_seen_at"`
}

// TableName 指定表名。
func (SearchTerm) TableName() string { return "search_terms" }
