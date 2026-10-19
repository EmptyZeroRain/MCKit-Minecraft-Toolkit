package domain

import (
	"sort"
	"time"

	"mckit/backend/internal/repository/types"
)

// Role 角色（后台 RBAC 底座）。
// name 为主键：稳定标识，供 admins.role / accounts.role 外键引用，创建后不可改名。
type Role struct {
	Name        string          `gorm:"primaryKey;size:40" json:"name"`
	Label       string          `gorm:"size:80;not null" json:"label"`
	Description string          `gorm:"not null;default:''" json:"description"`
	Permissions types.JSONSlice `gorm:"type:jsonb;not null;default:'[]'" json:"permissions"`
	IsSystem    bool            `gorm:"not null;default:false" json:"is_system"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// TableName 指定表名。
func (Role) TableName() string { return "roles" }

// 权限点常量。命名规则：<域>.<能力>，与后台导航分组保持一致。
const (
	PermContentResources  = "content.resources"
	PermContentTools      = "content.tools"
	PermContentWiki       = "content.wiki"
	PermContentCategories = "content.categories"
	PermContentTags       = "content.tags"
	PermContentFeedback   = "content.feedback"

	PermReviewRedstone = "review.redstone"
	PermReviewMods     = "review.mods"

	PermAccountsAdmins = "accounts.admins"
	PermAccountsUsers  = "accounts.users"
	PermRolesManage    = "roles.manage"

	PermOpsAudit       = "ops.audit"
	PermOpsDataQuality = "ops.dataquality"
	PermOpsSearchStats = "ops.searchstats"
	PermOpsAnalytics   = "ops.analytics"
	PermOpsLinkHealth  = "ops.linkhealth"
	PermOpsMCsrc       = "ops.mcsrc"
)

// Permission 单个权限点（key 为存储值，label 为中文展示名）。
type Permission struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// PermissionGroup 权限点分组（用于后台「角色管理」按组勾选）。
type PermissionGroup struct {
	Key         string       `json:"key"`
	Label       string       `json:"label"`
	Permissions []Permission `json:"permissions"`
}

// PermissionGroups 权限点目录（顺序即前端展示顺序）。
var PermissionGroups = []PermissionGroup{
	{
		Key:   "content",
		Label: "内容管理",
		Permissions: []Permission{
			{PermContentResources, "资源管理"},
			{PermContentTools, "工具管理"},
			{PermContentWiki, "百科管理"},
			{PermContentCategories, "分类管理"},
			{PermContentTags, "标签管理"},
			{PermContentFeedback, "反馈管理"},
		},
	},
	{
		Key:   "review",
		Label: "内容审核",
		Permissions: []Permission{
			{PermReviewRedstone, "红石设计审核"},
			{PermReviewMods, "Mod / 整合包审核"},
		},
	},
	{
		Key:   "accounts",
		Label: "账号与权限",
		Permissions: []Permission{
			{PermAccountsAdmins, "管理员账号"},
			{PermAccountsUsers, "普通用户管理"},
			{PermRolesManage, "角色管理"},
		},
	},
	{
		Key:   "ops",
		Label: "系统与运营",
		Permissions: []Permission{
			{PermOpsAudit, "操作日志"},
			{PermOpsDataQuality, "数据质量"},
			{PermOpsSearchStats, "搜索词统计"},
			{PermOpsAnalytics, "运营看板"},
			{PermOpsLinkHealth, "链接健康"},
			{PermOpsMCsrc, "MC 源码库"},
		},
	},
}

// AllPermissions 全部权限点（内置 admin 角色的权限集合）。
func AllPermissions() []string {
	out := make([]string, 0, 17)
	for _, g := range PermissionGroups {
		for _, p := range g.Permissions {
			out = append(out, p.Key)
		}
	}
	return out
}

// IsValidPermission 判断权限点是否为目录内的合法值。
func IsValidPermission(key string) bool {
	for _, g := range PermissionGroups {
		for _, p := range g.Permissions {
			if p.Key == key {
				return true
			}
		}
	}
	return false
}

// builtinRolePermissions 内置角色的默认权限。
// 用途有二：迁移种子数据的对照，以及 roles 表缺失该行时的兜底
// （避免角色数据异常导致后台完全不可管理）。
var builtinRolePermissions = map[string][]string{
	RoleAdmin:  AllPermissions(),
	RoleEditor: {PermContentResources, PermContentTools, PermContentWiki, PermContentCategories, PermContentTags, PermContentFeedback},
	RoleUser:   {},
}

// BuiltinRolePermissions 返回内置角色的默认权限；非内置角色返回 ok=false。
func BuiltinRolePermissions(role string) ([]string, bool) {
	perms, ok := builtinRolePermissions[role]
	return perms, ok
}

// NormalizePermissions 去重并排序权限点，保证入库结果稳定可比对。
func NormalizePermissions(perms []string) []string {
	seen := make(map[string]struct{}, len(perms))
	out := make([]string, 0, len(perms))
	for _, p := range perms {
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}
