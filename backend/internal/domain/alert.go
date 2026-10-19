package domain

import (
	"time"

	"github.com/google/uuid"

	"mckit/backend/internal/repository/types"
)

// 告警状态。
//
//	open → acked（运维已确认，处理中）
//	open/acked → resolved（手动关闭或指标自动恢复）
const (
	AlertStatusOpen     = "open"
	AlertStatusAcked    = "acked"
	AlertStatusResolved = "resolved"
)

// 告警严重级别。
const (
	AlertSeverityWarning = "warning"
	AlertSeverityError   = "error"
)

// AlertStatuses 全部合法状态（用于筛选校验）。
var AlertStatuses = []string{AlertStatusOpen, AlertStatusAcked, AlertStatusResolved}

// IsValidAlertStatus 判断状态是否合法。
func IsValidAlertStatus(s string) bool {
	for _, v := range AlertStatuses {
		if v == s {
			return true
		}
	}
	return false
}

// OpsAlert 一条运营告警。
//
// 同一 rule_key 在未关闭期间只允许一条（部分唯一索引兜底），
// 指标持续异常时只刷新 last_seen_at / detail，不重复刷屏。
type OpsAlert struct {
	ID       uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	RuleKey  string    `gorm:"size:60;not null" json:"rule_key"`
	Severity string    `gorm:"size:10;not null;default:warning" json:"severity"`
	Status   string    `gorm:"size:20;not null;default:open" json:"status"`
	Title    string    `gorm:"size:200;not null" json:"title"`
	Detail   string    `gorm:"type:text;not null;default:''" json:"detail"`
	// Metric 触发时的指标快照（便于判断严重程度与趋势）。
	Metric types.JSONB `gorm:"type:jsonb;default:'{}'" json:"metric"`
	// DedupeKey 去重键（当前等于 rule_key；预留按对象拆分的空间）。
	DedupeKey   string     `gorm:"size:80;not null" json:"dedupe_key"`
	TriggeredAt time.Time  `gorm:"not null;default:now()" json:"triggered_at"`
	LastSeenAt  time.Time  `gorm:"not null;default:now()" json:"last_seen_at"`
	AckedBy     string     `gorm:"size:80;not null;default:''" json:"acked_by"`
	AckedAt     *time.Time `json:"acked_at,omitempty"`
	ResolvedBy  string     `gorm:"size:80;not null;default:''" json:"resolved_by"`
	ResolvedAt  *time.Time `json:"resolved_at,omitempty"`
	CreatedAt   time.Time  `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt   time.Time  `gorm:"not null;default:now()" json:"updated_at"`
}

// TableName 指定表名。
func (OpsAlert) TableName() string { return "ops_alerts" }
