package domain

import (
	"time"

	"github.com/google/uuid"

	"mckit/backend/internal/repository/types"
)

// Mod 加载器（TODO §16）。DB 不做 CHECK，由导入器校验（加载器生态仍在演进）。
//   - fabric：Fabric
//   - forge：Forge（历史版本主流）
//   - neoforge：NeoForge（Forge 分叉）
//   - quilt：Quilt（Fabric 分叉）
const (
	ModLoaderFabric   = "fabric"
	ModLoaderForge    = "forge"
	ModLoaderNeoForge = "neoforge"
	ModLoaderQuilt    = "quilt"
)

// ModLoaders 全部合法加载器，顺序即前端筛选与展示顺序。
var ModLoaders = []string{
	ModLoaderFabric,
	ModLoaderForge,
	ModLoaderNeoForge,
	ModLoaderQuilt,
}

// IsValidModLoader 判断加载器是否合法。
func IsValidModLoader(loader string) bool {
	for _, l := range ModLoaders {
		if l == loader {
			return true
		}
	}
	return false
}

// Mod 功能类别（TODO §16）。
//   - ui：界面 / 信息显示
//   - optimization：性能优化
//   - technology：科技 / 自动化
//   - magic：魔法
//   - world：世界生成 / 地图
//   - adventure：冒险 / 战斗
//   - library：前置库 / API
//   - storage：仓储 / 背包
//   - decoration：装饰
//   - other：其他 / 未归类
const (
	ModCategoryUI           = "ui"
	ModCategoryOptimization = "optimization"
	ModCategoryTechnology   = "technology"
	ModCategoryMagic        = "magic"
	ModCategoryWorld        = "world"
	ModCategoryAdventure    = "adventure"
	ModCategoryLibrary      = "library"
	ModCategoryStorage      = "storage"
	ModCategoryDecoration   = "decoration"
	ModCategoryOther        = "other"
)

// ModCategories 全部合法类别，顺序即前端筛选与展示顺序。
var ModCategories = []string{
	ModCategoryUI,
	ModCategoryOptimization,
	ModCategoryTechnology,
	ModCategoryMagic,
	ModCategoryWorld,
	ModCategoryAdventure,
	ModCategoryLibrary,
	ModCategoryStorage,
	ModCategoryDecoration,
	ModCategoryOther,
}

// IsValidModCategory 判断类别是否合法。
func IsValidModCategory(category string) bool {
	for _, c := range ModCategories {
		if c == category {
			return true
		}
	}
	return false
}

// Mod 发布版本类型（TODO §16）。
const (
	ModReleaseTypeRelease = "release"
	ModReleaseTypeBeta    = "beta"
	ModReleaseTypeAlpha   = "alpha"
)

// ModReleaseTypes 全部合法发布版本类型，顺序即前端筛选与展示顺序。
var ModReleaseTypes = []string{
	ModReleaseTypeRelease,
	ModReleaseTypeBeta,
	ModReleaseTypeAlpha,
}

// IsValidModReleaseType 判断发布版本类型是否合法。
func IsValidModReleaseType(rt string) bool {
	for _, r := range ModReleaseTypes {
		if r == rt {
			return true
		}
	}
	return false
}

// Mod 依赖 / 冲突类型（TODO §16）。本站只呈现**已记录**的关系。
//   - required：必需前置
//   - optional：可选前置
//   - embedded：内嵌 / 捆绑（如 Create 与 Flywheel）
//   - incompatible：明确记录的不兼容
const (
	ModDependencyRequired     = "required"
	ModDependencyOptional     = "optional"
	ModDependencyEmbedded     = "embedded"
	ModDependencyIncompatible = "incompatible"
)

// ModDependencyKinds 全部合法依赖类型，顺序即前端分组展示顺序。
var ModDependencyKinds = []string{
	ModDependencyRequired,
	ModDependencyOptional,
	ModDependencyEmbedded,
	ModDependencyIncompatible,
}

// IsValidModDependencyKind 判断依赖类型是否合法。
func IsValidModDependencyKind(kind string) bool {
	for _, k := range ModDependencyKinds {
		if k == kind {
			return true
		}
	}
	return false
}

// 内容审核状态（TODO §16）。仅 published 对公开接口可见。
const (
	ModReviewPending   = "pending"
	ModReviewPublished = "published"
	ModReviewRejected  = "rejected"
)

// ModReviewStatuses 全部合法审核状态，顺序即前端筛选与展示顺序。
var ModReviewStatuses = []string{
	ModReviewPending,
	ModReviewPublished,
	ModReviewRejected,
}

// IsValidModReviewStatus 判断审核状态是否合法。
func IsValidModReviewStatus(status string) bool {
	for _, s := range ModReviewStatuses {
		if s == status {
			return true
		}
	}
	return false
}

// Mod Mod 父条目（TODO §16）。仅 review_status = published 的记录对公开接口可见。
type Mod struct {
	ID             uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Slug           string          `gorm:"uniqueIndex;size:160;not null" json:"slug"`
	NameZh         string          `gorm:"size:160;not null" json:"name_zh"`
	NameEn         string          `gorm:"size:160" json:"name_en"`
	Aliases        types.JSONSlice `gorm:"type:jsonb;default:'[]'" json:"aliases"`
	Summary        string          `gorm:"type:text" json:"summary"`
	Description    string          `gorm:"type:text" json:"description"`
	Categories     types.JSONSlice `gorm:"type:jsonb;default:'[]'" json:"categories"`
	Loaders        types.JSONSlice `gorm:"type:jsonb;default:'[]'" json:"loaders"`
	MCVersions     types.JSONSlice `gorm:"column:mc_versions;type:jsonb;default:'[]'" json:"mc_versions"`
	HomepageURL    string          `gorm:"column:homepage_url;type:text" json:"homepage_url"`
	SourceURL      string          `gorm:"column:source_url;type:text" json:"source_url"`
	License        string          `gorm:"size:80" json:"license"`
	LicenseNote    string          `gorm:"column:license_note;type:text" json:"license_note"`
	GameEdition    string          `gorm:"size:20;not null;default:java" json:"game_edition"`
	DataVersion    string          `gorm:"size:60" json:"data_version"`
	LastVerifiedAt *time.Time      `json:"last_verified_at"`
	ReviewStatus   string          `gorm:"column:review_status;size:20;not null;default:pending" json:"review_status"`
	Reviewer       string          `gorm:"size:120" json:"reviewer"`
	ReviewedAt     *time.Time      `json:"reviewed_at"`
	ReviewNote     string          `gorm:"column:review_note;type:text" json:"review_note"`
	SortOrder      int             `gorm:"not null;default:0" json:"sort_order"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// TableName 固定表名，避免 GORM 复数化推测。
func (Mod) TableName() string { return "mods" }

// ModVersion 同一 Mod 的一个发布版本（TODO §16）。
// download_url 只允许官方 / 作者授权入口，不镜像分发；verified 表示验证状态。
type ModVersion struct {
	ID           uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ModID        uuid.UUID       `gorm:"column:mod_id;type:uuid;not null" json:"mod_id"`
	Version      string          `gorm:"size:80;not null" json:"version"`
	ReleaseType  string          `gorm:"column:release_type;size:20;not null;default:release" json:"release_type"`
	MCVersions   types.JSONSlice `gorm:"column:mc_versions;type:jsonb;default:'[]'" json:"mc_versions"`
	Loaders      types.JSONSlice `gorm:"type:jsonb;default:'[]'" json:"loaders"`
	DownloadURL  string          `gorm:"column:download_url;type:text" json:"download_url"`
	DownloadNote string          `gorm:"column:download_note;type:text" json:"download_note"`
	FileName     string          `gorm:"column:file_name;size:200" json:"file_name"`
	PublishedAt  *time.Time      `json:"published_at"`
	SourceURL    string          `gorm:"column:source_url;type:text" json:"source_url"`
	Verified     bool            `gorm:"not null;default:false" json:"verified"`
	VerifyNote   string          `gorm:"column:verify_note;type:text" json:"verify_note"`
	SortOrder    int             `gorm:"not null;default:0" json:"sort_order"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// TableName 固定表名。
func (ModVersion) TableName() string { return "mod_versions" }

// ModDependency 一条**已记录**的依赖或冲突关系（TODO §16）。
// TargetModID 指向已收录 Mod；未收录时回退 TargetModName（二者必居其一）。
type ModDependency struct {
	ID            uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ModID         uuid.UUID  `gorm:"column:mod_id;type:uuid;not null" json:"mod_id"`
	DepKind       string     `gorm:"column:dep_kind;size:20;not null" json:"dep_kind"`
	TargetModID   *uuid.UUID `gorm:"column:target_mod_id;type:uuid" json:"target_mod_id"`
	TargetModName string     `gorm:"column:target_mod_name;size:160" json:"target_mod_name"`
	VersionNote   string     `gorm:"column:version_note;type:text" json:"version_note"`
	Note          string     `gorm:"type:text" json:"note"`
	SourceURL     string     `gorm:"column:source_url;type:text" json:"source_url"`
	Verified      bool       `gorm:"not null;default:false" json:"verified"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// TableName 固定表名。
func (ModDependency) TableName() string { return "mod_dependencies" }

// Modpack 整合包父条目（TODO §16）。仅 review_status = published 的记录对公开接口可见。
type Modpack struct {
	ID             uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Slug           string          `gorm:"uniqueIndex;size:160;not null" json:"slug"`
	NameZh         string          `gorm:"size:160;not null" json:"name_zh"`
	NameEn         string          `gorm:"size:160" json:"name_en"`
	Aliases        types.JSONSlice `gorm:"type:jsonb;default:'[]'" json:"aliases"`
	Summary        string          `gorm:"type:text" json:"summary"`
	Description    string          `gorm:"type:text" json:"description"`
	MCVersion      string          `gorm:"column:mc_version;size:40" json:"mc_version"`
	Loader         string          `gorm:"size:20" json:"loader"`
	Author         string          `gorm:"size:120" json:"author"`
	HomepageURL    string          `gorm:"column:homepage_url;type:text" json:"homepage_url"`
	SourceURL      string          `gorm:"column:source_url;type:text" json:"source_url"`
	License        string          `gorm:"size:80" json:"license"`
	LicenseNote    string          `gorm:"column:license_note;type:text" json:"license_note"`
	DataVersion    string          `gorm:"size:60" json:"data_version"`
	LastVerifiedAt *time.Time      `json:"last_verified_at"`
	ReviewStatus   string          `gorm:"column:review_status;size:20;not null;default:pending" json:"review_status"`
	Reviewer       string          `gorm:"size:120" json:"reviewer"`
	ReviewedAt     *time.Time      `json:"reviewed_at"`
	ReviewNote     string          `gorm:"column:review_note;type:text" json:"review_note"`
	SortOrder      int             `gorm:"not null;default:0" json:"sort_order"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// TableName 固定表名。
func (Modpack) TableName() string { return "modpacks" }

// ModpackMod 整合包内的一条 Mod 清单项（TODO §16）。
// ModID 指向已收录 Mod；未收录时回退 ModName（前端回退纯文本，不生成死链）。
type ModpackMod struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ModpackID uuid.UUID  `gorm:"column:modpack_id;type:uuid;not null" json:"modpack_id"`
	ModID     *uuid.UUID `gorm:"column:mod_id;type:uuid" json:"mod_id"`
	ModName   string     `gorm:"column:mod_name;size:160" json:"mod_name"`
	Version   string     `gorm:"size:80" json:"version"`
	Note      string     `gorm:"type:text" json:"note"`
	SortOrder int        `gorm:"not null;default:0" json:"sort_order"`
	CreatedAt time.Time  `json:"created_at"`
}

// TableName 固定表名。
func (ModpackMod) TableName() string { return "modpack_mods" }
