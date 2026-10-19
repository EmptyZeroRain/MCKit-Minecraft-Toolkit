// Package domain 定义数据库实体模型（GORM）。
package domain

import (
	"time"

	"github.com/google/uuid"
)

// Admin 管理员账号。
type Admin struct {
	ID           uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Username     string     `gorm:"uniqueIndex;size:80;not null" json:"username"`
	PasswordHash string     `gorm:"not null" json:"-"`
	Role         string     `gorm:"size:20;not null" json:"role"` // admin | editor
	IsActive     bool       `gorm:"not null;default:true" json:"is_active"`
	LastLoginAt  *time.Time `json:"last_login_at"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// 角色常量。
const (
	RoleAdmin  = "admin"
	RoleEditor = "editor"
)

// TableName 指定表名。
func (Admin) TableName() string { return "admins" }