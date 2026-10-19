package domain

import (
	"time"

	"github.com/google/uuid"
)

// RoleUser 普通用户角色常量。
// 说明：普通用户与管理员表体系分离，此常量仅用于会话标记，不授予任何后台权限。
const RoleUser = "user"

// User 普通用户账号（需求 §12）。
// 与 Admin 分离，避免普通用户触碰后台；密码仅存 Argon2id 哈希。
type User struct {
	ID           uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Username     string     `gorm:"uniqueIndex;size:40;not null" json:"username"`
	PasswordHash string     `gorm:"not null" json:"-"`
	IsActive     bool       `gorm:"not null;default:true" json:"is_active"`
	LastLoginAt  *time.Time `json:"last_login_at"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// TableName 显式指定表名。
func (User) TableName() string { return "users" }

// UserFavorite 登录用户的收藏（工具 / 物品 / 资源 / 百科）。
type UserFavorite struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID    uuid.UUID `gorm:"type:uuid;not null" json:"-"`
	Kind      string    `gorm:"size:20;not null" json:"kind"`
	ObjectID  string    `gorm:"size:200;not null" json:"object_id"`
	Label     string    `gorm:"size:200;not null;default:''" json:"label"`
	URL       string    `gorm:"not null;default:''" json:"url"`
	CreatedAt time.Time `json:"created_at"`
}

// TableName 显式指定表名。
func (UserFavorite) TableName() string { return "user_favorites" }

// UserRecent 登录用户的最近使用记录。
type UserRecent struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID    uuid.UUID `gorm:"type:uuid;not null" json:"-"`
	Kind      string    `gorm:"size:20;not null" json:"kind"`
	ObjectID  string    `gorm:"size:200;not null" json:"object_id"`
	Label     string    `gorm:"size:200;not null;default:''" json:"label"`
	URL       string    `gorm:"not null;default:''" json:"url"`
	VisitedAt time.Time `json:"visited_at"`
}

// TableName 显式指定表名。
func (UserRecent) TableName() string { return "user_recents" }

// UserSeed 登录用户保存的常用种子 / 查询记录。
type UserSeed struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID    uuid.UUID `gorm:"type:uuid;not null" json:"-"`
	Name      string    `gorm:"size:120;not null" json:"name"`
	Seed      string    `gorm:"size:64;not null" json:"seed"`
	Note      string    `gorm:"size:500;not null;default:''" json:"note"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName 显式指定表名。
func (UserSeed) TableName() string { return "user_seeds" }
