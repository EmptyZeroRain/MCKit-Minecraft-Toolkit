package domain

import (
	"time"

	"github.com/google/uuid"
)

// AuditLog 审计日志（需求 §6.4）。
// 不得记录密码、会话 Cookie、Authorization 头、数据库连接串或完整敏感请求体。
type AuditLog struct {
	ID            uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ActorAdminID  *uuid.UUID `gorm:"type:uuid" json:"actor_admin_id"`
	Action        string     `gorm:"size:80;not null" json:"action"`
	ObjectType    string     `gorm:"size:80;not null" json:"object_type"`
	ObjectID      *uuid.UUID `gorm:"type:uuid" json:"object_id"`
	BeforeSummary string     `gorm:"type:jsonb" json:"before_summary,omitempty"`
	AfterSummary  string     `gorm:"type:jsonb" json:"after_summary,omitempty"`
	RequestID     string     `gorm:"size:100" json:"request_id"`
	SourceIP      string     `gorm:"type:inet" json:"source_ip,omitempty"`
	Result        string     `gorm:"size:20;not null" json:"result"` // success | failure
	CreatedAt     time.Time  `json:"created_at"`
}

// 审计结果常量。
const (
	AuditSuccess = "success"
	AuditFailure = "failure"
)

// TableName 指定表名。
func (AuditLog) TableName() string { return "audit_logs" }