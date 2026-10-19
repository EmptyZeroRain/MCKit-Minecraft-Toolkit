// Package adminaccount 提供后台账号管理（需求 §6.1 身份认证 / §6.2 权限）。
// 路由层用 RequirePermission(accounts.admins) 限制。
//
// 统一账号（迁移 0022）后 admins 与 users 合并为 accounts：
//   - 本包只管理「后台账号」（role <> 'user'），所有查询都显式排除普通用户，
//     否则普通用户会出现在管理员列表里，甚至被重置密码 / 删除；
//   - 内置角色 user 属普通用户，不允许经本接口创建（改由 /user/register 或
//     「普通用户管理」维护）。
//
// 所有写操作使用参数化查询；密码使用 Argon2id 哈希，绝不落库明文。
package adminaccount

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"mckit/backend/internal/auth"
	"mckit/backend/internal/domain"
)

// MinPasswordLength 管理员账号创建/重置密码的最小长度。
// 与初始化命令 cmd/create-admin 保持一致（>用户自助改密的 8 位）。
const MinPasswordLength = 12

var (
	// ErrNotFound 目标管理员不存在。
	ErrNotFound = errors.New("管理员不存在")
	// ErrUsernameTaken 用户名已被占用。
	ErrUsernameTaken = errors.New("用户名已被占用")
	// ErrBadRole 角色非法。
	ErrBadRole = errors.New("角色不存在，请先在「角色管理」中创建")
	// ErrReservedRole 内置角色 user 为普通用户身份，不经后台账号接口维护。
	ErrReservedRole = errors.New("内置角色 user 属普通用户，请在「普通用户管理」中维护")
	// ErrWeakPassword 密码强度不足。
	ErrWeakPassword = fmt.Errorf("密码长度至少 %d 位", MinPasswordLength)
	// ErrBadUsername 用户名格式非法。
	ErrBadUsername = errors.New("用户名长度需在 1-80 之间")
	// ErrLastAdmin 会造成没有任何启用状态 admin 账号，拒绝以保证系统可管理。
	ErrLastAdmin = errors.New("必须保留至少一个启用状态的 admin 账号")
)

// Service 管理员账号服务。
type Service struct {
	db *gorm.DB
}

// NewService 创建账号服务。
func NewService(db *gorm.DB) *Service { return &Service{db: db} }

// CreateInput 创建管理员入参。
type CreateInput struct {
	Username string
	Password string
	Role     string
}

// UpdateInput 更新管理员入参（仅白名单字段，指针区分「未提供」与「置零」）。
type UpdateInput struct {
	Role     *string
	IsActive *bool
}

// List 分页查询后台账号（按用户名模糊匹配，排除普通用户）。
func (s *Service) List(ctx context.Context, q string, page, pageSize int) ([]domain.Admin, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	tx := s.db.WithContext(ctx).Model(&domain.Admin{}).Where("role <> ?", domain.RoleUser)
	if q = strings.TrimSpace(q); q != "" {
		tx = tx.Where("username ILIKE ?", "%"+q+"%")
	}

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var items []domain.Admin
	if err := tx.Order("created_at ASC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// GetByID 按 ID 查询。
func (s *Service) GetByID(ctx context.Context, id string) (*domain.Admin, error) {
	if id == "" {
		return nil, ErrNotFound
	}
	return s.findByID(ctx, id)
}

// Create 创建后台账号：校验用户名唯一、角色合法（非普通用户）与密码强度。
func (s *Service) Create(ctx context.Context, in CreateInput) (*domain.Admin, error) {
	username := strings.TrimSpace(in.Username)
	if username == "" || len(username) > 80 {
		return nil, ErrBadUsername
	}
	if strings.TrimSpace(in.Role) == domain.RoleUser {
		return nil, ErrReservedRole
	}
	if err := s.validateRole(ctx, in.Role); err != nil {
		return nil, err
	}
	if len(in.Password) < MinPasswordLength {
		return nil, ErrWeakPassword
	}

	// 用户名在 accounts 中全局唯一，需与普通用户一并查重。
	var existing int64
	if err := s.db.WithContext(ctx).Model(&domain.Admin{}).
		Where("username = ?", username).Count(&existing).Error; err != nil {
		return nil, err
	}
	if existing > 0 {
		return nil, ErrUsernameTaken
	}

	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return nil, err
	}
	admin := &domain.Admin{
		Username:     username,
		PasswordHash: hash,
		Role:         in.Role,
		IsActive:     true,
	}
	if err := s.db.WithContext(ctx).Create(admin).Error; err != nil {
		return nil, err
	}
	return admin, nil
}

// Update 更新角色与启用状态；拒绝把系统最后一名启用 admin 降级或停用。
func (s *Service) Update(ctx context.Context, id string, in UpdateInput) (*domain.Admin, error) {
	admin, err := s.findByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if admin == nil {
		return nil, ErrNotFound
	}

	updates := map[string]any{}

	if in.Role != nil && *in.Role != admin.Role {
		if strings.TrimSpace(*in.Role) == domain.RoleUser {
			return nil, ErrReservedRole
		}
		if err := s.validateRole(ctx, *in.Role); err != nil {
			return nil, err
		}
		// 降级最后一名启用 admin 同样会导致无人可管理，需拦截。
		if admin.Role == domain.RoleAdmin && admin.IsActive {
			if n, err := s.countActiveAdmins(ctx); err != nil {
				return nil, err
			} else if n <= 1 {
				return nil, ErrLastAdmin
			}
		}
		updates["role"] = *in.Role
	}

	if in.IsActive != nil && *in.IsActive != admin.IsActive {
		if !*in.IsActive && admin.Role == domain.RoleAdmin {
			if n, err := s.countActiveAdmins(ctx); err != nil {
				return nil, err
			} else if n <= 1 {
				return nil, ErrLastAdmin
			}
		}
		updates["is_active"] = *in.IsActive
	}

	if len(updates) == 0 {
		return admin, nil
	}
	updates["updated_at"] = gorm.Expr("now()")
	if err := s.db.WithContext(ctx).Model(&domain.Admin{}).
		Where("id = ? AND role <> ?", id, domain.RoleUser).Updates(updates).Error; err != nil {
		return nil, err
	}
	return s.findByID(ctx, id)
}

// ResetPassword 重置指定管理员密码（不需旧密码，由 admin 执行）。
func (s *Service) ResetPassword(ctx context.Context, id, newPassword string) error {
	if len(newPassword) < MinPasswordLength {
		return ErrWeakPassword
	}
	admin, err := s.findByID(ctx, id)
	if err != nil {
		return err
	}
	if admin == nil {
		return ErrNotFound
	}

	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Model(&domain.Admin{}).
		Where("id = ? AND role <> ?", id, domain.RoleUser).
		Updates(map[string]any{
			"password_hash": hash,
			"updated_at":    gorm.Expr("now()"),
		}).Error
}

// Delete 删除后台账号；拒绝删除系统最后一名启用 admin。
// accounts 表采用物理删除，调用方（handler）需叠加「不可删除自己」的约束。
func (s *Service) Delete(ctx context.Context, id string) error {
	admin, err := s.findByID(ctx, id)
	if err != nil {
		return err
	}
	if admin == nil {
		return ErrNotFound
	}
	if admin.Role == domain.RoleAdmin && admin.IsActive {
		if n, err := s.countActiveAdmins(ctx); err != nil {
			return err
		} else if n <= 1 {
			return ErrLastAdmin
		}
	}
	return s.db.WithContext(ctx).
		Where("id = ? AND role <> ?", id, domain.RoleUser).
		Delete(&domain.Admin{}).Error
}

// findByID 按主键查询后台账号（排除普通用户），未命中返回 (nil, nil)。
func (s *Service) findByID(ctx context.Context, id string) (*domain.Admin, error) {
	var admin domain.Admin
	err := s.db.WithContext(ctx).Where("id = ? AND role <> ?", id, domain.RoleUser).First(&admin).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &admin, nil
}

// countActiveAdmins 统计启用状态的 admin 账号数。
func (s *Service) countActiveAdmins(ctx context.Context) (int64, error) {
	var n int64
	err := s.db.WithContext(ctx).Model(&domain.Admin{}).
		Where("role = ? AND is_active = true", domain.RoleAdmin).
		Count(&n).Error
	return n, err
}

// validateRole 校验角色存在（以 roles 表为准，支持自定义角色）。
// accounts.role 是 roles(name) 的外键，因此这里必须要求角色行真实存在。
func (s *Service) validateRole(ctx context.Context, role string) error {
	name := strings.TrimSpace(role)
	if name == "" {
		return ErrBadRole
	}
	var n int64
	if err := s.db.WithContext(ctx).Model(&domain.Role{}).
		Where("name = ?", name).Count(&n).Error; err != nil {
		return err
	}
	if n == 0 {
		return ErrBadRole
	}
	return nil
}
