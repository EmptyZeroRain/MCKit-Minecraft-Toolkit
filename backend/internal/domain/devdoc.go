package domain

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"mckit/backend/internal/repository/types"
)

// 开发文档所属平台 / 加载器（TODO §21 我的世界开发栏目）。
//   - neoforge / forge / fabric：三个主流模组加载器
//   - vanilla：原版数据包 / 资源包 / 协议等不依赖加载器的内容
//   - tooling：映射、版本元数据等开发工具链
//   - multi：跨加载器的通用主题（如映射体系总览）
const (
	DevLoaderNeoForge = "neoforge"
	DevLoaderForge    = "forge"
	DevLoaderFabric   = "fabric"
	DevLoaderVanilla  = "vanilla"
	DevLoaderTooling  = "tooling"
	DevLoaderMulti    = "multi"
)

// DevLoaders 全部合法平台取值，顺序即前端分组展示顺序。
var DevLoaders = []string{
	DevLoaderNeoForge,
	DevLoaderForge,
	DevLoaderFabric,
	DevLoaderVanilla,
	DevLoaderTooling,
	DevLoaderMulti,
}

// IsValidDevLoader 判断平台取值是否合法。
func IsValidDevLoader(loader string) bool {
	for _, l := range DevLoaders {
		if l == loader {
			return true
		}
	}
	return false
}

// 开发文档主题分类（TODO §21）。
const (
	DevCategoryGettingStarted = "getting_started" // 入门与项目结构
	DevCategoryEvents         = "events"          // 事件与生命周期
	DevCategoryRegistry       = "registry"        // 注册表与内容注册
	DevCategoryNetworking     = "networking"      // 网络与数据包
	DevCategoryDatagen        = "datagen"         // 数据生成
	DevCategoryResources      = "resources"       // 资源与数据文件
	DevCategoryRendering      = "rendering"       // 渲染与界面
	DevCategorySerialization  = "serialization"   // 序列化与数据存储
	DevCategoryAPIReference   = "api_reference"   // API 速查
	DevCategoryMapping        = "mapping"         // 映射与混淆
	DevCategoryToolchain      = "toolchain"       // 工具链与元数据
)

// DevCategories 全部合法主题分类，顺序即前端筛选展示顺序。
var DevCategories = []string{
	DevCategoryGettingStarted,
	DevCategoryEvents,
	DevCategoryRegistry,
	DevCategoryNetworking,
	DevCategoryDatagen,
	DevCategoryResources,
	DevCategoryRendering,
	DevCategorySerialization,
	DevCategoryAPIReference,
	DevCategoryMapping,
	DevCategoryToolchain,
}

// IsValidDevCategory 判断主题分类是否合法。
func IsValidDevCategory(category string) bool {
	for _, c := range DevCategories {
		if c == category {
			return true
		}
	}
	return false
}

// DevApiEntry 开发文档涉及的关键接口（类 / 方法 / 事件 / 字段）。
// 单独结构化存储，使「接口速查」可检索、可复制，而不是埋在正文里。
type DevApiEntry struct {
	// Name 全限定名，如 net.fabricmc.api.ModInitializer#onInitialize
	Name string `json:"name"`
	// Kind 类型：class | method | event | field | command | file
	Kind string `json:"kind"`
	// Signature 签名 / 用法片段（可留空）。
	Signature string `json:"signature"`
	// Note 一句话说明。
	Note string `json:"note"`
}

// DevApiEntries 接口清单，作为 jsonb 数组持久化。
type DevApiEntries []DevApiEntry

// Value 实现 driver.Valuer。
func (e DevApiEntries) Value() (driver.Value, error) {
	if e == nil {
		e = DevApiEntries{}
	}
	b, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// Scan 实现 sql.Scanner。
func (e *DevApiEntries) Scan(value any) error {
	if value == nil {
		*e = nil
		return nil
	}
	var raw []byte
	switch v := value.(type) {
	case string:
		raw = []byte(v)
	case []byte:
		raw = v
	default:
		return fmt.Errorf("无法扫描类型 %T 到 DevApiEntries", value)
	}
	if len(raw) == 0 {
		*e = nil
		return nil
	}
	var out []DevApiEntry
	if err := json.Unmarshal(raw, &out); err != nil {
		return err
	}
	*e = out
	return nil
}

// MarshalJSON 直接输出原生数组。
func (e DevApiEntries) MarshalJSON() ([]byte, error) {
	if e == nil {
		return []byte("[]"), nil
	}
	return json.Marshal([]DevApiEntry(e))
}

// DevDoc 开发文档条目（TODO §21）。
type DevDoc struct {
	ID             uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Slug           string          `gorm:"uniqueIndex;size:200;not null" json:"slug"`
	TitleZh        string          `gorm:"size:240;not null" json:"title_zh"`
	TitleEn        string          `gorm:"size:240" json:"title_en"`
	Loader         string          `gorm:"size:30;not null" json:"loader"`
	Category       string          `gorm:"size:40;not null" json:"category"`
	Summary        string          `gorm:"type:text" json:"summary"`
	BodyMd         string          `gorm:"column:body_md;type:text" json:"body_md"`
	APIEntries     DevApiEntries   `gorm:"column:api_entries;type:jsonb;default:'[]'" json:"api_entries"`
	OfficialURL    string          `gorm:"column:official_url;type:text" json:"official_url"`
	SourceURL      string          `gorm:"type:text" json:"source_url"`
	MCVersions     types.JSONSlice `gorm:"column:mc_versions;type:jsonb;default:'[]'" json:"mc_versions"`
	Loaders        types.JSONSlice `gorm:"type:jsonb;default:'[]'" json:"loaders"`
	Tags           types.JSONSlice `gorm:"type:jsonb;default:'[]'" json:"tags"`
	SortOrder      int             `gorm:"not null;default:0" json:"sort_order"`
	Status         string          `gorm:"size:20;not null;default:published" json:"status"`
	DataVersion    string          `gorm:"size:80" json:"data_version"`
	LastVerifiedAt *time.Time      `json:"last_verified_at"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// TableName 固定表名，避免 GORM 复数化推测。
func (DevDoc) TableName() string { return "dev_docs" }

// 映射数据集类型（TODO §21 版本映射与类 / 方法查询）。
const (
	MappingTypeMojang    = "mojang"    // Mojang 官方映射（ProGuard client.txt / server.txt）
	MappingTypeSRG       = "srg"       // SRG 名（历史 MCP 体系）
	MappingTypeTSRG      = "tsrg"      // TSRG / TSRG2（MCPConfig，Forge / NeoForge）
	MappingTypeYarn      = "yarn"      // Fabric Yarn（Tiny v2，official→intermediary→named）
	MappingTypeParchment = "parchment" // Parchment（在官方映射之上补参数名）
	MappingTypeMixed     = "mixed"     // 其它 / 混合来源
)

// MappingTypes 全部合法映射类型，顺序即前端筛选展示顺序。
var MappingTypes = []string{
	MappingTypeMojang,
	MappingTypeSRG,
	MappingTypeTSRG,
	MappingTypeYarn,
	MappingTypeParchment,
	MappingTypeMixed,
}

// IsValidMappingType 判断映射类型是否合法。
func IsValidMappingType(t string) bool {
	for _, v := range MappingTypes {
		if v == t {
			return true
		}
	}
	return false
}

// 成员类型。
const (
	MappingMemberMethod = "method"
	MappingMemberField  = "field"
	MappingMemberParam  = "param"
)

// MappingSet 一套映射数据集（版本 + 映射类型 + 渠道）。
// 不预置数据：必须由 cmd/import-mappings 从官方映射文件导入，避免收录猜测的混淆名。
type MappingSet struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	SetKey      string     `gorm:"column:set_key;uniqueIndex;size:200;not null" json:"set_key"`
	MCVersion   string     `gorm:"column:mc_version;size:60;not null" json:"mc_version"`
	MappingType string     `gorm:"column:mapping_type;size:40;not null" json:"mapping_type"`
	Channel     string     `gorm:"size:60" json:"channel"`
	BuildLabel  string     `gorm:"column:build_label;size:120" json:"build_label"`
	ClassCount  int        `gorm:"column:class_count;not null;default:0" json:"class_count"`
	MemberCount int        `gorm:"column:member_count;not null;default:0" json:"member_count"`
	SourceURL   string     `gorm:"type:text" json:"source_url"`
	License     string     `gorm:"size:160" json:"license"`
	Note        string     `gorm:"type:text" json:"note"`
	ImportedAt  *time.Time `gorm:"column:imported_at" json:"imported_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// TableName 固定表名。
func (MappingSet) TableName() string { return "mapping_sets" }

// MappingClass 类级映射。三个名字列按映射格式填空，未提供的留空。
type MappingClass struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	SetID       uuid.UUID `gorm:"column:set_id;type:uuid;not null" json:"set_id"`
	ObfName     string    `gorm:"column:obf_name;size:400;not null" json:"obf_name"`
	MidName     string    `gorm:"column:mid_name;size:400" json:"mid_name"`
	NamedName   string    `gorm:"column:named_name;size:400" json:"named_name"`
	Package     string    `gorm:"size:400" json:"package"`
	MemberCount int       `gorm:"column:member_count;not null;default:0" json:"member_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TableName 固定表名。
func (MappingClass) TableName() string { return "mapping_classes" }

// MappingMember 成员级映射（方法 / 字段 / 参数）。
// 描述符语义见 migrations/0013_dev_docs.sql：desc_key 仅用于区分重载，
// obf_desc 为第一命名空间描述符，named_desc 为可读类型描述符。
type MappingMember struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ClassID   uuid.UUID `gorm:"column:class_id;type:uuid;not null" json:"class_id"`
	Kind      string    `gorm:"size:20;not null" json:"kind"`
	ObfName   string    `gorm:"column:obf_name;size:400;not null" json:"obf_name"`
	MidName   string    `gorm:"column:mid_name;size:400" json:"mid_name"`
	NamedName string    `gorm:"column:named_name;size:400" json:"named_name"`
	DescKey   string    `gorm:"column:desc_key;size:1000" json:"-"`
	ObfDesc   string    `gorm:"column:obf_desc;size:1000" json:"obf_desc"`
	NamedDesc string    `gorm:"column:named_desc;size:1000" json:"named_desc"`
	Side      string    `gorm:"size:20" json:"side"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName 固定表名。
func (MappingMember) TableName() string { return "mapping_members" }
