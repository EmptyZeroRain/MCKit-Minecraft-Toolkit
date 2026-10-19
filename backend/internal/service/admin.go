// Package service 提供业务服务层。
package service

import (
	"errors"

	"gorm.io/gorm"

	"mckit/backend/internal/domain"
)

// AdminRepo 后台账号数据访问接口（便于测试隔离）。
//
// 统一账号（迁移 0022）后 admins 与 users 合并为 accounts，
// 本接口只服务后台登录入口，因此查询必须排除普通用户（role = 'user'）。
type AdminRepo interface {
	FindByUsername(username string) (*domain.Admin, error)
	FindByID(id string) (*domain.Admin, error)
	UpdateLastLogin(adminID string) error
	// ChangePassword 更新密码哈希。
	ChangePassword(adminID, newHash string) error
	// UpdateProfile 更新展示资料（昵称 / 头像 URL），仅限白名单字段。
	UpdateProfile(adminID, nickname, avatarURL string) error
}

// GORMAdminRepo 基于 GORM 的后台账号仓库实现。
type GORMAdminRepo struct {
	db *gorm.DB
}

// NewAdminRepo 创建管理员仓库。
func NewAdminRepo(db *gorm.DB) *GORMAdminRepo {
	return &GORMAdminRepo{db: db}
}

// FindByUsername 按用户名查询后台账号（排除 role = 'user' 的普通用户）。
// 排除普通用户是安全边界：普通用户不得从 /admin/auth/login 登录后台。
func (r *GORMAdminRepo) FindByUsername(username string) (*domain.Admin, error) {
	var admin domain.Admin
	err := r.db.Where("username = ? AND role <> ?", username, domain.RoleUser).First(&admin).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &admin, nil
}

// FindByID 按 ID 查询后台账号（同样排除普通用户）。
func (r *GORMAdminRepo) FindByID(id string) (*domain.Admin, error) {
	var admin domain.Admin
	err := r.db.Where("id = ? AND role <> ?", id, domain.RoleUser).First(&admin).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &admin, nil
}

// UpdateLastLogin 记录最后登录时间。
func (r *GORMAdminRepo) UpdateLastLogin(adminID string) error {
	return r.db.Model(&domain.Admin{}).
		Where("id = ?", adminID).
		Update("last_login_at", gorm.Expr("now()")).Error
}

// ChangePassword 更新密码哈希，并清空强制改密标记（账号已完成自助改密）。
func (r *GORMAdminRepo) ChangePassword(adminID, newHash string) error {
	return r.db.Model(&domain.Admin{}).
		Where("id = ?", adminID).
		Updates(map[string]any{"password_hash": newHash, "must_change_password": false}).Error
}

// UpdateProfile 更新展示资料（昵称 / 头像 URL）。
// 仅写入白名单字段，避免把请求体其余键带入 UPDATE（防字段注入）。
func (r *GORMAdminRepo) UpdateProfile(adminID, nickname, avatarURL string) error {
	return r.db.Model(&domain.Admin{}).
		Where("id = ?", adminID).
		Updates(map[string]any{"nickname": nickname, "avatar_url": avatarURL}).Error
}
