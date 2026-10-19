package domain

import (
	"time"

	"github.com/google/uuid"

	"mckit/backend/internal/repository/types"
)

// 可版本化的内容类型（与 content_revisions.content_type 的 CHECK 约束一致）。
const (
	RevisionContentResource = "resource"
	RevisionContentTool     = "tool"
	RevisionContentWiki     = "wiki"
)

// 版本动作：新建 / 更新 / 回滚。回滚同样追加新版本，不覆盖历史。
const (
	RevisionActionCreate   = "create"
	RevisionActionUpdate   = "update"
	RevisionActionRollback = "rollback"
)

// IsValidRevisionContentType 判断内容类型是否可版本化。
func IsValidRevisionContentType(t string) bool {
	switch t {
	case RevisionContentResource, RevisionContentTool, RevisionContentWiki:
		return true
	}
	return false
}

// ContentRevision 内容历史版本（整行快照）。
//
// Snapshot 保存该次写入后的完整内容 JSON，ChangeSummary 记录相对上一版发生
// 变化的顶层字段名（有序）。回滚通过追加一条 Action=rollback 的新版本完成，
// 因此历史只增不改（§16.2）。
type ContentRevision struct {
	ID            uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ContentType   string          `gorm:"size:20;not null" json:"content_type"`
	ContentID     uuid.UUID       `gorm:"type:uuid;not null" json:"content_id"`
	RevisionNo    int             `gorm:"not null" json:"revision_no"`
	Action        string          `gorm:"size:20;not null" json:"action"`
	Snapshot      types.JSONB     `gorm:"type:jsonb;not null;default:'{}'" json:"snapshot"`
	ChangeSummary types.JSONSlice `gorm:"type:jsonb;not null;default:'[]'" json:"change_summary"`
	ActorID       *uuid.UUID      `gorm:"type:uuid" json:"actor_id"`
	ActorName     string          `gorm:"size:120;not null;default:''" json:"actor_name"`
	Note          string          `gorm:"type:text;not null;default:''" json:"note"`
	CreatedAt     time.Time       `json:"created_at"`
}

// TableName 指定表名。
func (ContentRevision) TableName() string { return "content_revisions" }
