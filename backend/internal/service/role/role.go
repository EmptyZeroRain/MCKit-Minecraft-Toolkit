// Package role 提供后台角色管理（RBAC 底座）。
//
// 设计说明：
//   - roles.name 为主键且创建后不可改名（admins.role / accounts.role 以外键引用它）；
//   - 内置角色（is_system）不可删除、不可改名；权限可调整，唯 admin 必须保留全量权限，
//     避免把系统改到无人可管理；
//   - PermissionsForRole 供鉴权中间件解析当前角色权限；roles 行缺失时回落到内置默认值，
//     保证角色数据异常也不会让后台完全不可用。
package role

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"gorm.io/gorm"

	"mckit/backend/internal/domain"
)

var (
	// ErrNotFound 角色不存在。
	ErrNotFound = errors.New("角色不存在")
	// ErrNameTaken 角色标识已被占用。
	ErrNameTaken = errors.New("角色标识已被占用")
	// ErrBadName 角色标识格式非法。
	ErrBadName = errors.New("角色标识需以字母开头，仅含小写字母、数字、下划线或连字符，长度 1-40")
	// ErrBadLabel 角色名称非法。
	ErrBadLabel = errors.New("角色名称长度需在 1-80 之间")
	// ErrBadPermission 权限点非法。
	ErrBadPermission = errors.New("包含未知权限点")
	// ErrSystemRole 内置角色不可删除。
	ErrSystemRole = errors.New("内置角色不可删除")
	// ErrRoleInUse 角色仍被账号引用。
	ErrRoleInUse = errors.New("该角色仍被账号使用，无法删除")
	// ErrAdminMustKeepAll 内置 admin 角色必须保留全量权限。
	ErrAdminMustKeepAll = errors.New("内置 admin 角色必须保留全部权限")
)

// namePattern 角色标识白名单：小写字母开头，仅含小写字母、数字、下划线、连字符。
var namePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,39}$`)

// Service 角色服务。
type Service struct {
	db *gorm.DB
}

// NewService 创建角色服务。
func NewService(db *gorm.DB) *Service { return &Service{db: db} }

// CreateInput 新建角色入参。
type CreateInput struct {
	Name        string
	Label       string
	Description string
	Permissions []string
}

// UpdateInput 更新角色入参（name 不可改，故不含该字段）。
type UpdateInput struct {
	Label       *string
	Description *string
	Permissions *[]string
}

// List 返回全部角色：内置角色在前，其后按标识升序。
func (s *Service) List(ctx context.Context) ([]domain.Role, error) {
	var roles []domain.Role
	if err := s.db.WithContext(ctx).
		Order("is_system DESC, name ASC").
		Find(&roles).Error; err != nil {
		return nil, err
	}
	return roles, nil
}

// Get 按标识查询角色。
func (s *Service) Get(ctx context.Context, name string) (*domain.Role, error) {
	var r domain.Role
	err := s.db.WithContext(ctx).Where("name = ?", strings.TrimSpace(name)).First(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// Create 新建自定义角色（内置标识已被占用，因此无需额外判断）。
func (s *Service) Create(ctx context.Context, in CreateInput) (*domain.Role, error) {
	name := strings.TrimSpace(in.Name)
	label := strings.TrimSpace(in.Label)
	if !namePattern.MatchString(name) {
		return nil, ErrBadName
	}
	if label == "" || len([]rune(label)) > 80 {
		return nil, ErrBadLabel
	}
	perms, err := validatePermissions(in.Permissions)
	if err != nil {
		return nil, err
	}

	var existing int64
	if err := s.db.WithContext(ctx).Model(&domain.Role{}).Where("name = ?", name).Count(&existing).Error; err != nil {
		return nil, err
	}
	if existing > 0 {
		return nil, ErrNameTaken
	}

	r := domain.Role{
		Name:        name,
		Label:       label,
		Description: strings.TrimSpace(in.Description),
		Permissions: perms,
		IsSystem:    false,
	}
	if err := s.db.WithContext(ctx).Create(&r).Error; err != nil {
		return nil, err
	}
	return &r, nil
}

// Update 更新角色展示信息与权限（标识不可改）。
func (s *Service) Update(ctx context.Context, name string, in UpdateInput) (*domain.Role, error) {
	r, err := s.Get(ctx, name)
	if err != nil {
		return nil, err
	}

	updates := map[string]any{}
	if in.Label != nil {
		label := strings.TrimSpace(*in.Label)
		if label == "" || len([]rune(label)) > 80 {
			return nil, ErrBadLabel
		}
		updates["label"] = label
	}
	if in.Description != nil {
		updates["description"] = strings.TrimSpace(*in.Description)
	}
	if in.Permissions != nil {
		perms, err := validatePermissions(*in.Permissions)
		if err != nil {
			return nil, err
		}
		// 内置 admin 必须始终保留全量权限，否则可能出现无人可管理的死锁。
		if r.Name == domain.RoleAdmin && len(perms) != len(domain.AllPermissions()) {
			return nil, ErrAdminMustKeepAll
		}
		updates["permissions"] = perms
	}

	if len(updates) > 0 {
		updates["updated_at"] = gorm.Expr("now()")
		if err := s.db.WithContext(ctx).Model(&domain.Role{}).Where("name = ?", r.Name).
			Updates(updates).Error; err != nil {
			return nil, err
		}
	}
	return s.Get(ctx, r.Name)
}

// Delete 删除自定义角色；内置角色与仍被账号引用的角色会被拒绝。
func (s *Service) Delete(ctx context.Context, name string) error {
	r, err := s.Get(ctx, name)
	if err != nil {
		return err
	}
	if r.IsSystem {
		return ErrSystemRole
	}

	var used int64
	if err := s.db.WithContext(ctx).Model(&domain.Admin{}).
		Where("role = ?", r.Name).Count(&used).Error; err != nil {
		return err
	}
	if used > 0 {
		return ErrRoleInUse
	}

	return s.db.WithContext(ctx).Where("name = ?", r.Name).Delete(&domain.Role{}).Error
}

// PermissionsForRole 解析角色权限，供鉴权中间件使用。
// 角色行不存在时回落到内置默认值；未知角色返回空集合（即无任何后台权限）。
func (s *Service) PermissionsForRole(ctx context.Context, name string) ([]string, error) {
	roleName := strings.TrimSpace(name)
	if roleName == "" {
		return nil, nil
	}
	var r domain.Role
	err := s.db.WithContext(ctx).Select("name", "permissions").
		Where("name = ?", roleName).First(&r).Error
	if err == nil {
		return r.Permissions, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if perms, ok := domain.BuiltinRolePermissions(roleName); ok {
		return perms, nil
	}
	return nil, nil
}

// validatePermissions 校验权限点并归一化（去重 + 排序）。
func validatePermissions(perms []string) ([]string, error) {
	normalized := domain.NormalizePermissions(perms)
	for _, p := range normalized {
		if !domain.IsValidPermission(p) {
			return nil, fmt.Errorf("%w: %s", ErrBadPermission, p)
		}
	}
	if normalized == nil {
		normalized = []string{}
	}
	return normalized, nil
}
