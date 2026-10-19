// 资源链接健康与人工确认（TODO §6）。
package domain

import "time"

// 链接检查触发来源。
const (
	LinkTriggerAuto   = "auto"
	LinkTriggerManual = "manual"
)

// 链接复核状态（人工确认流程）。
const (
	ReviewPending         = "pending"
	ReviewConfirmedBroken = "confirmed_broken"
	ReviewConfirmedOK     = "confirmed_ok"
	ReviewIgnored         = "ignored"
)

// ReviewStates 全部合法复核状态。
var ReviewStates = []string{ReviewPending, ReviewConfirmedBroken, ReviewConfirmedOK, ReviewIgnored}

// IsValidReviewState 判断复核状态是否合法。
func IsValidReviewState(s string) bool {
	for _, v := range ReviewStates {
		if v == s {
			return true
		}
	}
	return false
}

// IsValidLinkTrigger 判断触发来源是否合法。
func IsValidLinkTrigger(s string) bool {
	return s == LinkTriggerAuto || s == LinkTriggerManual
}

// LinkCheck 资源链接的最新一次检查结果与复核状态。
type LinkCheck struct {
	ID            string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ResourceID    string     `gorm:"type:uuid;not null;uniqueIndex" json:"resource_id"`
	URL           string     `gorm:"type:text;not null;default:''" json:"url"`
	Status        string     `gorm:"size:20;not null;default:unchecked" json:"status"`
	StatusCode    int        `gorm:"not null;default:0" json:"status_code"`
	FinalURL      string     `gorm:"type:text;not null;default:''" json:"final_url"`
	TriggerSource string     `gorm:"size:10;not null;default:auto" json:"trigger_source"`
	ReviewState   string     `gorm:"size:20;not null;default:pending" json:"review_state"`
	ReviewedBy    string     `gorm:"size:120;not null;default:''" json:"reviewed_by"`
	ReviewedAt    *time.Time `json:"reviewed_at"`
	ReviewNote    string     `gorm:"type:text;not null;default:''" json:"review_note"`
	CheckedAt     time.Time  `gorm:"not null;default:now()" json:"checked_at"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// TableName 指定表名。
func (LinkCheck) TableName() string { return "resource_link_checks" }
