package domain

import (
	"time"

	"github.com/google/uuid"
)

// Account 统一账号（迁移 0022 合并原 admins / users 两表）。
//
// 身份由 Role 决定：内置角色 'user' 为普通用户（不含任何后台权限点），
// 其余角色（admin / editor / 自定义角色）为后台账号。
// 密码仅存 Argon2id 哈希，绝不落库明文。
type Account struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Username     string    `gorm:"uniqueIndex;size:80;not null" json:"username"`
	PasswordHash string    `gorm:"not null" json:"-"`
	// Role 外键引用 roles(name)；默认 'user'，注册普通用户时无需显式指定。
	Role        string     `gorm:"size:40;not null;default:user" json:"role"`
	IsActive    bool       `gorm:"not null;default:true" json:"is_active"`
	LastLoginAt *time.Time `json:"last_login_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// TableName 指定表名。
func (Account) TableName() string { return "accounts" }

// Admin 后台账号：统一账号的类型别名（保留历史命名，避免大范围改名）。
//
// 语义约定：后台账号 = Role 非 RoleUser 的账号；所有后台侧查询必须显式
// 过滤 role <> 'user'，否则普通用户会混进管理员列表甚至被重置密码。
type Admin = Account

// User 普通用户账号：统一账号的类型别名。
//
// 语义约定：普通用户 = Role 为 RoleUser 的账号；用户侧查询必须显式过滤
// role = 'user'，否则管理员账号会被当成普通用户登录 / 管理。
type User = Account
