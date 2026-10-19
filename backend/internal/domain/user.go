package domain

import (
	"time"

	"github.com/google/uuid"
)

// RoleUser 普通用户角色常量（roles 表内置角色）。
// 统一账号后即 accounts.role = 'user'；该角色不含任何后台权限点。
const RoleUser = "user"

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
