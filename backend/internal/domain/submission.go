package domain

import (
	"time"

	"github.com/google/uuid"

	"mckit/backend/internal/repository/types"
)

// 资源投稿审核状态（与 resource_submissions.status 的 CHECK 约束一致）。
//
// 与 redstone / mods 的 review_status 语义一致，但独立成表：
// 资源投稿是「用户提交的新资源」，审核通过后会真正生成 resources 记录。
const (
	SubmissionStatusPending  = "pending"
	SubmissionStatusApproved = "approved"
	SubmissionStatusRejected = "rejected"
)

// SubmissionStatuses 全部合法投稿状态，顺序即前端筛选与展示顺序。
var SubmissionStatuses = []string{
	SubmissionStatusPending,
	SubmissionStatusApproved,
	SubmissionStatusRejected,
}

// SubmissionLicenseVersion 资源投稿授权条款版本（TODO §17）。
//
// 投稿属于「向本站贡献内容」，须先接受授权条款；版本号变化时需重新确认，
// 历史投稿保留其提交时的版本，便于日后追溯授权范围。
const SubmissionLicenseVersion = "2026-10-19"

// IsValidSubmissionStatus 判断投稿状态是否合法。
func IsValidSubmissionStatus(s string) bool {
	for _, v := range SubmissionStatuses {
		if v == s {
			return true
		}
	}
	return false
}

// ResourceSubmission 用户提交的资源投稿（结构化字段，与 resources 对齐）。
//
// 生命周期：pending（待审核）→ approved（已通过，ResourceID 指向生成的资源）
// 或 rejected（已驳回）。字段选择以「审核通过后无需二次录入即可创建资源」为准。
type ResourceSubmission struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	AccountID      uuid.UUID `gorm:"type:uuid;not null" json:"account_id"`
	SubmitterName  string    `gorm:"size:40;not null;default:''" json:"submitter_name"`
	Name           string    `gorm:"size:200;not null" json:"name"`
	Summary        string    `gorm:"size:500;not null" json:"summary"`
	Description    string    `gorm:"type:text;not null;default:''" json:"description"`
	ResourceType   string    `gorm:"size:40;not null" json:"resource_type"`
	AuthorName     string    `gorm:"size:160;not null;default:''" json:"author_name"`
	SourcePlatform string    `gorm:"size:100;not null;default:''" json:"source_platform"`
	SourceURL      string    `gorm:"type:text;not null" json:"source_url"`
	License        string    `gorm:"size:160;not null;default:''" json:"license"`
	GameEdition    string    `gorm:"size:20;not null;default:'java'" json:"game_edition"`
	// SupportedVersions / LoaderTypes 以 JSONB 数组存储，与 resources 同构。
	SupportedVersions types.JSONSlice `gorm:"type:jsonb;not null;default:'[]'" json:"supported_versions"`
	LoaderTypes       types.JSONSlice `gorm:"type:jsonb;not null;default:'[]'" json:"loader_types"`
	InstallGuide      string          `gorm:"type:text;not null;default:''" json:"install_guide"`
	// Note 提交者给审核者的说明（不写入 resources）。
	Note string `gorm:"type:text;not null;default:''" json:"note"`
	// LicenseVersion / LicenseAcceptedAt 投稿时接受的授权条款版本与时间（TODO §17）。
	LicenseVersion    string     `gorm:"size:40;not null;default:''" json:"license_version"`
	LicenseAcceptedAt *time.Time `json:"license_accepted_at"`
	// Status 审核状态；resource_id 审核通过后生成的资源。
	Status     string     `gorm:"size:20;not null;default:'pending'" json:"status"`
	Reviewer   string     `gorm:"size:40;not null;default:''" json:"reviewer"`
	ReviewedAt *time.Time `json:"reviewed_at"`
	ReviewNote string     `gorm:"type:text;not null;default:''" json:"review_note"`
	ResourceID *uuid.UUID `gorm:"type:uuid" json:"resource_id"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// TableName 指定表名。
func (ResourceSubmission) TableName() string { return "resource_submissions" }
