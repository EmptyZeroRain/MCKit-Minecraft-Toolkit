package domain

import (
	"time"

	"github.com/google/uuid"
)

// 数据质量问题处理状态。
//
// 状态机刻意保持简单：open → assigned → resolved / ignored，
// 任何状态都可以回到 open（问题复发时重新打开）。
const (
	QualityIssueOpen     = "open"
	QualityIssueAssigned = "assigned"
	QualityIssueResolved = "resolved"
	QualityIssueIgnored  = "ignored"
)

// QualityIssueStatuses 全部合法状态。
var QualityIssueStatuses = []string{
	QualityIssueOpen,
	QualityIssueAssigned,
	QualityIssueResolved,
	QualityIssueIgnored,
}

// IsValidQualityIssueStatus 判断状态是否合法。
func IsValidQualityIssueStatus(s string) bool {
	for _, v := range QualityIssueStatuses {
		if v == s {
			return true
		}
	}
	return false
}

// DataQualityIssue 数据质量问题的处理工单。
//
// 检查结果本身是实时计算、不落库的；这张表只保存「谁在跟进、处理到哪一步」，
// 以 check_key 为业务唯一键，一个检查项同时只会有一条工单。
type DataQualityIssue struct {
	ID      uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CheckKey string   `gorm:"size:80;not null;uniqueIndex" json:"check_key"`
	Status  string    `gorm:"size:20;not null;default:open" json:"status"`
	// AssigneeID 负责人账号；为空表示未指派（同时清空 AssigneeName）。
	AssigneeID *uuid.UUID `gorm:"type:uuid" json:"assignee_id,omitempty"`
	// AssigneeName 负责人昵称快照：账号删除后仍能看到「当时指派给谁」。
	AssigneeName string `gorm:"size:80;not null;default:''" json:"assignee_name"`
	Note         string `gorm:"type:text;not null;default:''" json:"note"`
	// SnapshotCount 最后一次处理时的问题条数，用于和实时值对比（判断是否恶化）。
	SnapshotCount int64      `gorm:"not null;default:0" json:"snapshot_count"`
	ResolvedAt    *time.Time `json:"resolved_at,omitempty"`
	UpdatedBy     string     `gorm:"size:80;not null;default:''" json:"updated_by"`
	CreatedAt     time.Time  `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt     time.Time  `gorm:"not null;default:now()" json:"updated_at"`
}

// TableName 指定表名。
func (DataQualityIssue) TableName() string { return "data_quality_issues" }
