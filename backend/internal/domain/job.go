package domain

import (
	"time"

	"github.com/google/uuid"

	"mckit/backend/internal/repository/types"
)

// 后台任务状态。任务只会单向流转：
//
//	pending → running → succeeded
//	                  ↘ pending（自动重试，attempts < max_attempts）
//	                  ↘ failed（重试耗尽）
//	pending → cancelled（人工取消）
//
// 「running」不提供取消：执行中的副作用无法安全中断，取消只对排队中的任务生效。
const (
	JobStatusPending   = "pending"
	JobStatusRunning   = "running"
	JobStatusSucceeded = "succeeded"
	JobStatusFailed    = "failed"
	JobStatusCancelled = "cancelled"
)

// JobStatuses 全部合法状态（供校验与前端筛选）。
var JobStatuses = []string{
	JobStatusPending,
	JobStatusRunning,
	JobStatusSucceeded,
	JobStatusFailed,
	JobStatusCancelled,
}

// IsValidJobStatus 判断状态是否合法。
func IsValidJobStatus(s string) bool {
	for _, v := range JobStatuses {
		if v == s {
			return true
		}
	}
	return false
}

// AdminJob 一条后台运维任务。
//
// Payload / Result 都是 JSON：不同任务类型自行约定结构，
// 因此任务类型必须先注册处理器，未注册类型的入队会被拒绝。
type AdminJob struct {
	ID     uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Type   string    `gorm:"size:60;not null" json:"type"`
	Status string    `gorm:"size:20;not null;default:pending" json:"status"`
	// Payload 任务入参（由各任务类型定义）。
	Payload types.JSONB `gorm:"type:jsonb;default:'{}'" json:"payload"`
	// Result 任务产出摘要（面向运营展示，必须脱敏）。
	Result types.JSONB `gorm:"type:jsonb;default:'{}'" json:"result"`
	// Attempts 已执行次数（含自动重试）。
	Attempts int `gorm:"not null;default:0" json:"attempts"`
	// MaxAttempts 自动重试上限；达到后转为 failed，等待人工重试。
	MaxAttempts int `gorm:"not null;default:3" json:"max_attempts"`
	// LastError 最近一次失败原因（已脱敏，可直接展示）。
	LastError string `gorm:"type:text;not null;default:''" json:"last_error"`
	// RetryOf 人工重试来源任务 ID（首次执行为 nil）。
	RetryOf *uuid.UUID `gorm:"type:uuid" json:"retry_of,omitempty"`
	// CreatedBy 触发人（管理员昵称；系统触发为空）。
	CreatedBy string     `gorm:"size:80;not null;default:''" json:"created_by"`
	NextRunAt time.Time  `gorm:"not null;default:now()" json:"next_run_at"`
	StartedAt *time.Time `json:"started_at,omitempty"`
	// FinishedAt 进入 succeeded / failed / cancelled 的时间。
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	CreatedAt  time.Time  `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt  time.Time  `gorm:"not null;default:now()" json:"updated_at"`
}

// TableName 指定表名。
func (AdminJob) TableName() string { return "admin_jobs" }
