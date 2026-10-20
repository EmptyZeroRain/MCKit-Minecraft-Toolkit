package domain

import (
	"time"

	"github.com/google/uuid"
)

// ContentKeyword 内容关键字屏蔽规则。
//
// Normalized 用于大小写无关的唯一性与匹配；Keyword 保留后台输入时的原始展示形态。
type ContentKeyword struct {
	ID         uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Keyword    string     `gorm:"size:100;not null" json:"keyword"`
	Normalized string     `gorm:"size:100;not null;uniqueIndex" json:"-"`
	Enabled    bool       `gorm:"not null;default:true" json:"enabled"`
	Note       string     `gorm:"size:255;not null;default:''" json:"note"`
	CreatedBy  *uuid.UUID `gorm:"column:created_by;type:uuid" json:"created_by"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// TableName 指定表名。
func (ContentKeyword) TableName() string { return "content_keywords" }
