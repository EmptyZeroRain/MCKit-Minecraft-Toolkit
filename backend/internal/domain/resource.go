package domain

import (
	"time"

	"github.com/google/uuid"

	"mckit/backend/internal/repository/types"
)

// 资源状态枚举。
const (
	StatusDraft     = "draft"
	StatusPublished = "published"
	StatusHidden    = "hidden"
	StatusRemoved   = "removed"
)

// 资源类型枚举（需求 §5.7）。
const (
	ResourceTypeMod          = "mod"
	ResourceTypeModpack      = "modpack"
	ResourceTypeResourcePack = "resource_pack"
	ResourceTypeShader       = "shader"
	ResourceTypeWorld        = "world"
	ResourceTypeDatapack     = "datapack"
	ResourceTypePlugin       = "plugin"
)

// 链接检查状态（需求 §5.8）。
const (
	LinkStatusUnchecked  = "unchecked"
	LinkStatusOK         = "ok"
	LinkStatusRedirected = "redirected"
	LinkStatusBroken     = "broken"
	LinkStatusBlocked    = "blocked"
	LinkStatusError      = "error"
)

// ResourceTypes 全部合法资源类型（TODO §6 统一字段，导入与接口共用）。
var ResourceTypes = []string{
	ResourceTypeMod, ResourceTypeModpack, ResourceTypeResourcePack,
	ResourceTypeShader, ResourceTypeWorld, ResourceTypeDatapack, ResourceTypePlugin,
}

// 加载器枚举（TODO §6 统一字段，与前端 LOADER_TYPES 对齐）。
const (
	LoaderForge    = "forge"
	LoaderNeoForge = "neoforge"
	LoaderFabric   = "fabric"
	LoaderQuilt    = "quilt"
	LoaderVanilla  = "vanilla"
	LoaderPaper    = "paper"
	LoaderSpigot   = "spigot"
	LoaderPurpur   = "purpur"
	LoaderOther    = "other"
)

// LoaderTypes 全部合法加载器。
var LoaderTypes = []string{
	LoaderForge, LoaderNeoForge, LoaderFabric, LoaderQuilt,
	LoaderVanilla, LoaderPaper, LoaderSpigot, LoaderPurpur, LoaderOther,
}

// 依赖关系类型（TODO §6：记录依赖、前置 Mod 与已知冲突）。
const (
	// DependencyTypeDependency 运行依赖（缺少则无法工作）。
	DependencyTypeDependency = "dependency"
	// DependencyTypePrerequisite 前置 Mod / 前置资源（安装顺序要求）。
	DependencyTypePrerequisite = "prerequisite"
	// DependencyTypeConflict 已知冲突（同时安装会出问题）。
	DependencyTypeConflict = "conflict"
)

// DependencyTypes 全部合法依赖关系类型。
var DependencyTypes = []string{
	DependencyTypeDependency, DependencyTypePrerequisite, DependencyTypeConflict,
}

// 游戏版本枚举（与前端 GameEdition 对齐）。
const (
	GameEditionJava    = "java"
	GameEditionBedrock = "bedrock"
)

// Resource 资源（需求 §7.2）。
// JSONB 字段使用自定义类型，保证存储与序列化均为原生 JSON 结构。
type Resource struct {
	ID                uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Slug              string          `gorm:"uniqueIndex;size:180;not null" json:"slug"`
	Name              string          `gorm:"size:200;not null" json:"name"`
	Summary           string          `gorm:"size:500;not null" json:"summary"`
	Description       string          `gorm:"type:text" json:"description"`
	ResourceType      string          `gorm:"size:40;not null" json:"resource_type"`
	CoverImageURL     string          `gorm:"type:text" json:"cover_image_url"`
	AuthorName        string          `gorm:"size:160" json:"author_name"`
	SourcePlatform    string          `gorm:"size:100" json:"source_platform"`
	SourceURL         string          `gorm:"type:text;not null" json:"source_url"`
	License           string          `gorm:"size:160" json:"license"`
	GameEdition       string          `gorm:"size:20;not null;default:java" json:"game_edition"`
	SupportedVersions types.JSONSlice `gorm:"type:jsonb;default:'[]'" json:"supported_versions"`
	LoaderTypes       types.JSONSlice `gorm:"type:jsonb;default:'[]'" json:"loader_types"`
	Dependencies      types.JSONB     `gorm:"type:jsonb;default:'[]'" json:"dependencies"`
	InstallGuide      string          `gorm:"type:text" json:"install_guide"`
	Tags              types.JSONSlice `gorm:"type:jsonb;default:'[]'" json:"tags"`
	// CompatibilityNote 人工核验备注（可选）：说明兼容性结论的依据或例外情况。
	CompatibilityNote string     `gorm:"type:text;not null;default:''" json:"compatibility_note"`
	Status            string     `gorm:"size:20;not null;default:draft" json:"status"`
	LinkStatus        string     `gorm:"size:20;not null;default:unchecked" json:"link_status"`
	LastCheckedAt     *time.Time `json:"last_checked_at"`
	PublishedAt       *time.Time `json:"published_at"`
	CreatedBy         *uuid.UUID `gorm:"type:uuid" json:"-"`
	UpdatedBy         *uuid.UUID `gorm:"type:uuid" json:"-"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	DeletedAt         *time.Time `json:"-"`
}

// TableName 指定表名。
func (Resource) TableName() string { return "resources" }

// ResourceRelease 资源发布版本（TODO §6）。
//
// 同一资源的多个发布版本通过 ResourceID 归属同一资源；
// 每个版本可各自声明适用游戏版本与加载器（例如同一 Mod 分别发布 1.20.1 / 1.21 版本），
// 因此兼容性结论必须结合 releases 推导，不能只看资源级字段。
type ResourceRelease struct {
	ID           uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ResourceID   uuid.UUID       `gorm:"type:uuid;not null;index" json:"resource_id"`
	VersionLabel string          `gorm:"size:120;not null" json:"version_label"`
	GameVersions types.JSONSlice `gorm:"type:jsonb;default:'[]'" json:"game_versions"`
	LoaderTypes  types.JSONSlice `gorm:"type:jsonb;default:'[]'" json:"loader_types"`
	SourceURL    string          `gorm:"type:text;not null" json:"source_url"`
	ReleasedAt   *time.Time      `json:"released_at"`
	Notes        string          `gorm:"type:text;not null;default:''" json:"notes"`
	IsLatest     bool            `gorm:"not null;default:false" json:"is_latest"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// TableName 指定表名。
func (ResourceRelease) TableName() string { return "resource_releases" }
