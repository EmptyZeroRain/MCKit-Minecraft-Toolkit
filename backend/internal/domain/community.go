package domain

import (
	"time"

	"github.com/google/uuid"
)

// 社区帖子分类（与前端标签保持一致）。
const (
	CommunityCategoryChat     = "chat"     // 综合闲聊
	CommunityCategoryHelp     = "help"     // 求助
	CommunityCategoryShowcase = "showcase" // 作品展示
	CommunityCategoryTutorial = "tutorial" // 教程 / 心得
)

// CommunityCategories 全部合法分类（顺序即前端展示顺序）。
var CommunityCategories = []string{
	CommunityCategoryChat,
	CommunityCategoryHelp,
	CommunityCategoryShowcase,
	CommunityCategoryTutorial,
}

// IsValidCommunityCategory 判断帖子分类是否合法。
func IsValidCommunityCategory(category string) bool {
	for _, c := range CommunityCategories {
		if c == category {
			return true
		}
	}
	return false
}

// 社区内容状态：公开可见 / 已隐藏（审核下线，不物理删除以便追溯）。
const (
	CommunityStatusPublished = "published"
	CommunityStatusHidden    = "hidden"
)

// IsValidCommunityStatus 判断社区内容状态是否合法。
func IsValidCommunityStatus(status string) bool {
	return status == CommunityStatusPublished || status == CommunityStatusHidden
}

// CommunityPost 社区帖子。
type CommunityPost struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	AuthorID  uuid.UUID `gorm:"column:author_id;type:uuid;not null" json:"-"`
	Category  string    `gorm:"size:20;not null;default:chat" json:"category"`
	Title     string    `gorm:"size:120;not null" json:"title"`
	Body      string    `gorm:"type:text;not null" json:"body"`
	Status    string    `gorm:"size:20;not null;default:published" json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName 显式指定表名。
func (CommunityPost) TableName() string { return "community_posts" }

// CommunityComment 帖子留言。
type CommunityComment struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	PostID    uuid.UUID `gorm:"column:post_id;type:uuid;not null" json:"post_id"`
	AuthorID  uuid.UUID `gorm:"column:author_id;type:uuid;not null" json:"-"`
	Body      string    `gorm:"type:text;not null" json:"body"`
	Status    string    `gorm:"size:20;not null;default:published" json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName 显式指定表名。
func (CommunityComment) TableName() string { return "community_comments" }

// CommunityLike 帖子点赞（post_id + account_id 复合主键）。
type CommunityLike struct {
	PostID    uuid.UUID `gorm:"column:post_id;type:uuid;primaryKey" json:"post_id"`
	AccountID uuid.UUID `gorm:"column:account_id;type:uuid;primaryKey" json:"account_id"`
	CreatedAt time.Time `json:"created_at"`
}

// TableName 显式指定表名。
func (CommunityLike) TableName() string { return "community_likes" }
