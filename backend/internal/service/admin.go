// Package service 提供业务服务层。
package service

import (
	"errors"

	"gorm.io/gorm"

	"mckit/backend/internal/domain"
)

// AdminRepo 管理员数据访问接口（便于测试隔离）。
type AdminRepo interface {
	FindByUsername(username string) (*domain.Admin, error)
	FindByID(id string) (*domain.Admin, error)
	UpdateLastLogin(adminID string) error
	// ChangePassword 更新密码哈希。
	ChangePassword(adminID, newHash string) error
}

// GORMAdminRepo 基于 GORM 的管理员仓库实现。
type GORMAdminRepo struct {
	db *gorm.DB
}

// NewAdminRepo 创建管理员仓库。
func NewAdminRepo(db *gorm.DB) *GORMAdminRepo {
	return &GORMAdminRepo{db: db}
}

// FindByUsername 按用户名查询，唯一索引保证至多一条。
func (r *GORMAdminRepo) FindByUsername(username string) (*domain.Admin, error) {
	var admin domain.Admin
	err := r.db.Where("username = ?", username).First(&admin).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &admin, nil
}

// FindByID 按 ID 查询。
func (r *GORMAdminRepo) FindByID(id string) (*domain.Admin, error) {
	var admin domain.Admin
	err := r.db.Where("id = ?", id).First(&admin).Error
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

// ChangePassword 更新密码哈希。
func (r *GORMAdminRepo) ChangePassword(adminID, newHash string) error {
	return r.db.Model(&domain.Admin{}).
		Where("id = ?", adminID).
		Update("password_hash", newHash).Error
}