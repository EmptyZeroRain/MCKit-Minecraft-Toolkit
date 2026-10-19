package domain

import (
	"context"
	"time"

	"github.com/google/uuid"

	"mckit/backend/internal/repository/types"
)

// ---- 成长事件类型（jingyan.md §2.2）----
//
// 事件由内部业务服务在成功事务中产生；成长模块只消费已确认事实，
// 绝不接受前端直接提交 event_type / user_id / payload 发奖。
const (
	EventUserRegistered          = "user.registered"
	EventUserProfileCompleted    = "user.profile_completed"
	EventUserEmailVerified       = "user.email_verified"
	EventUserLoginSucceeded      = "user.login_succeeded"
	EventToolUsedValidly         = "tool.used_validly"
	EventResourceFavorited       = "resource.favorited"
	EventResourceSubmission      = "resource.submission_created"
	EventResourceApproved        = "resource.approved"
	EventResourceBrokenConfirmed = "resource.broken_report_confirmed"
	EventFeedbackCorrection      = "feedback.correction_accepted"
	EventPostLiked               = "post.liked"
	EventPostUnliked             = "post.unliked"
	EventPostCommentCreated      = "post.comment_created"
	EventPostCommentDeleted      = "post.comment_deleted"
	EventWikiContribution        = "wiki.contribution_approved"
)

// GrowthEventTypes 全部合法成长事件类型（规则 / 任务定义只允许引用此集合，
// 防止后台配置出现拼写错误的事件类型而静默不发奖）。
var GrowthEventTypes = []string{
	EventUserRegistered,
	EventUserProfileCompleted,
	EventUserEmailVerified,
	EventUserLoginSucceeded,
	EventToolUsedValidly,
	EventResourceFavorited,
	EventResourceSubmission,
	EventResourceApproved,
	EventResourceBrokenConfirmed,
	EventFeedbackCorrection,
	EventPostLiked,
	EventPostUnliked,
	EventPostCommentCreated,
	EventPostCommentDeleted,
	EventWikiContribution,
}

// IsValidGrowthEventType 判断事件类型是否合法。
func IsValidGrowthEventType(t string) bool {
	for _, v := range GrowthEventTypes {
		if v == t {
			return true
		}
	}
	return false
}

// GrowthEvent 成长事件：业务模块产生、可被成长系统消费的已确认事实。
type GrowthEvent struct {
	// EventID 全局唯一事件 ID；同一业务事实重复投递必须使用同一 EventID。
	EventID uuid.UUID
	// EventType 事件类型（见 Event* 常量）。
	EventType string
	// ActorUserID 行为发起者；匿名事件不奖励。
	ActorUserID uuid.UUID
	// SubjectType / SubjectID 目标对象类型与 ID（如 resource / post）。
	SubjectType string
	SubjectID   string
	// SourceModule 产生事件的业务模块（用于审计与排障）。
	SourceModule string
	// OccurredAt 业务事件发生时间（用于规则生效时间判定）。
	OccurredAt time.Time
	// Payload 经服务端验证的必要业务信息。
	Payload map[string]any
}

// GrowthEventSink 成长事件接收端。
//
// 业务模块（用户 / 内容 / 统计等）在完成自身业务后通过本接口投递事件；
// 由 main 装配为成长服务实现，避免业务模块直接依赖成长包内部实现。
type GrowthEventSink interface {
	EmitGrowthEvent(ctx context.Context, event GrowthEvent) error
}

// ---- 成长状态常量 ----

// 事件处理状态。
const (
	GrowthEventPending   = "pending"
	GrowthEventProcessed = "processed"
	GrowthEventFailed    = "failed"
)

// 每日任务状态。
const (
	DailyTaskInProgress = "in_progress"
	DailyTaskCompleted  = "completed"
	DailyTaskClaimed    = "claimed"
)

// 经验流水来源类型。
const (
	LedgerSourceRule        = "rule"        // 业务行为规则奖励
	LedgerSourceDailyTask   = "daily_task"  // 每日任务完成奖励
	LedgerSourceAchievement = "achievement" // 成就奖励（首期默认 0）
	LedgerSourceManual      = "manual"      // 人工调整
	LedgerSourceReversal    = "reversal"    // 冲正
)

// 成就条件类型（后端允许列表，禁止任意代码执行条件）。
const (
	AchievementCondEventCount          = "event_count"           // 事件累计次数
	AchievementCondDistinctSubjectCount = "distinct_subject_count" // 去重目标数
	AchievementCondLevelReached        = "level_reached"         // 达到等级
	AchievementCondDailyTaskDays       = "daily_task_days"       // 完成任务的累计天数
)

// AchievementConditionTypes 全部合法成就条件类型。
var AchievementConditionTypes = []string{
	AchievementCondEventCount,
	AchievementCondDistinctSubjectCount,
	AchievementCondLevelReached,
	AchievementCondDailyTaskDays,
}

// IsValidAchievementCondition 判断成就条件类型是否合法。
func IsValidAchievementCondition(t string) bool {
	for _, v := range AchievementConditionTypes {
		if v == t {
			return true
		}
	}
	return false
}

// ---- 数据模型（对应迁移 0028）----

// UserGrowth 用户当前成长投影（可依据经验账本重建）。
type UserGrowth struct {
	UserID             uuid.UUID `gorm:"column:user_id;type:uuid;primaryKey" json:"user_id"`
	CurrentXP          int64     `gorm:"column:current_xp;not null;default:0" json:"current_xp"`
	LifetimeEarnedXP   int64     `gorm:"column:lifetime_earned_xp;not null;default:0" json:"lifetime_earned_xp"`
	LifetimeReversedXP int64     `gorm:"column:lifetime_reversed_xp;not null;default:0" json:"lifetime_reversed_xp"`
	Level              int       `gorm:"column:level;not null;default:1" json:"level"`
	Version            int64     `gorm:"column:version;not null;default:0" json:"version"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// TableName 指定表名。
func (UserGrowth) TableName() string { return "user_growth" }

// ExperienceLedger 经验流水（只增不删；冲正以负向流水表达）。
type ExperienceLedger struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID         uuid.UUID  `gorm:"column:user_id;type:uuid;not null" json:"user_id"`
	DeltaXP        int64      `gorm:"column:delta_xp;not null" json:"delta_xp"`
	BalanceAfter   int64      `gorm:"column:balance_after;not null" json:"balance_after"`
	SourceType     string     `gorm:"column:source_type;size:64;not null" json:"source_type"`
	SourceID       string     `gorm:"column:source_id;size:128;not null;default:''" json:"source_id"`
	EventID        *uuid.UUID `gorm:"column:event_id;type:uuid" json:"event_id"`
	IdempotencyKey string     `gorm:"column:idempotency_key;size:255;not null;uniqueIndex" json:"idempotency_key"`
	RuleCode       string     `gorm:"column:rule_code;size:100;not null;default:''" json:"rule_code"`
	RuleVersion    int        `gorm:"column:rule_version;not null;default:0" json:"rule_version"`
	Reason         string     `gorm:"column:reason;type:text;not null;default:''" json:"reason"`
	OperatorUserID *uuid.UUID `gorm:"column:operator_user_id;type:uuid" json:"operator_user_id"`
	CreatedAt      time.Time  `json:"created_at"`
}

// TableName 指定表名。
func (ExperienceLedger) TableName() string { return "experience_ledger" }

// GrowthEventRecord 成长事件接收记录（幂等去重）。
type GrowthEventRecord struct {
	EventID      uuid.UUID  `gorm:"column:event_id;type:uuid;primaryKey" json:"event_id"`
	EventType    string     `gorm:"column:event_type;size:100;not null" json:"event_type"`
	ActorUserID  *uuid.UUID `gorm:"column:actor_user_id;type:uuid" json:"actor_user_id"`
	SubjectType  string     `gorm:"column:subject_type;size:64;not null;default:''" json:"subject_type"`
	SubjectID    string     `gorm:"column:subject_id;size:128;not null;default:''" json:"subject_id"`
	SourceModule string     `gorm:"column:source_module;size:100;not null" json:"source_module"`
	OccurredAt   time.Time  `gorm:"column:occurred_at;not null" json:"occurred_at"`
	ProcessedAt  *time.Time `gorm:"column:processed_at" json:"processed_at"`
	Status       string     `gorm:"column:status;size:20;not null;default:pending" json:"status"`
	Payload      types.JSONB `gorm:"type:jsonb;not null;default:'{}'" json:"payload"`
	ErrorMessage string     `gorm:"column:error_message;type:text;not null;default:''" json:"error_message"`
	CreatedAt    time.Time  `json:"created_at"`
}

// TableName 指定表名。
func (GrowthEventRecord) TableName() string { return "growth_events" }

// GrowthRule 经验规则（版本化；修改创建新版本，不原地改写历史含义）。
type GrowthRule struct {
	ID              uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Code            string     `gorm:"size:100;not null" json:"code"`
	Version         int        `gorm:"not null;default:1" json:"version"`
	Name            string     `gorm:"size:150;not null" json:"name"`
	EventType       string     `gorm:"column:event_type;size:100;not null" json:"event_type"`
	XPAmount        int        `gorm:"column:xp_amount;not null;default:0" json:"xp_amount"`
	DailyLimit      *int       `gorm:"column:daily_limit" json:"daily_limit"`
	PerSubjectLimit *int       `gorm:"column:per_subject_limit" json:"per_subject_limit"`
	Enabled         bool       `gorm:"not null;default:true" json:"enabled"`
	EffectiveFrom   *time.Time `gorm:"column:effective_from" json:"effective_from"`
	EffectiveUntil  *time.Time `gorm:"column:effective_until" json:"effective_until"`
	CreatedBy       *uuid.UUID `gorm:"column:created_by;type:uuid" json:"created_by"`
	UpdatedBy       *uuid.UUID `gorm:"column:updated_by;type:uuid" json:"updated_by"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// TableName 指定表名。
func (GrowthRule) TableName() string { return "growth_rules" }

// GrowthLevel 等级阈值。
type GrowthLevel struct {
	Level       int       `gorm:"primaryKey" json:"level"`
	ThresholdXP int64     `gorm:"column:threshold_xp;not null" json:"threshold_xp"`
	Title       string    `gorm:"size:100;not null" json:"title"`
	Enabled     bool      `gorm:"not null;default:true" json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TableName 指定表名。
func (GrowthLevel) TableName() string { return "growth_levels" }

// DailyTaskDefinition 每日任务定义（后台可配置，版本化）。
type DailyTaskDefinition struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Code        string    `gorm:"size:100;not null;uniqueIndex" json:"code"`
	Name        string    `gorm:"size:150;not null" json:"name"`
	Description string    `gorm:"type:text;not null;default:''" json:"description"`
	EventType   string    `gorm:"column:event_type;size:100;not null" json:"event_type"`
	TargetCount int       `gorm:"column:target_count;not null" json:"target_count"`
	RewardXP    int       `gorm:"column:reward_xp;not null;default:0" json:"reward_xp"`
	Enabled     bool      `gorm:"not null;default:true" json:"enabled"`
	SortOrder   int       `gorm:"column:sort_order;not null;default:0" json:"sort_order"`
	Version     int       `gorm:"not null;default:1" json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TableName 指定表名。
func (DailyTaskDefinition) TableName() string { return "daily_task_definitions" }

// UserDailyTask 用户每日任务进度（每日独立记录，无需定时清空）。
type UserDailyTask struct {
	ID                  uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID              uuid.UUID  `gorm:"column:user_id;type:uuid;not null" json:"user_id"`
	TaskCode            string     `gorm:"column:task_code;size:100;not null" json:"task_code"`
	TaskDate            string     `gorm:"column:task_date;type:date;not null" json:"task_date"`
	ProgressCount       int        `gorm:"column:progress_count;not null;default:0" json:"progress_count"`
	TargetCountSnapshot int        `gorm:"column:target_count_snapshot;not null" json:"target_count"`
	RewardXPSnapshot    int        `gorm:"column:reward_xp_snapshot;not null;default:0" json:"reward_xp"`
	Status              string     `gorm:"size:20;not null;default:in_progress" json:"status"`
	CompletedAt         *time.Time `gorm:"column:completed_at" json:"completed_at"`
	ClaimedAt           *time.Time `gorm:"column:claimed_at" json:"claimed_at"`
	DefinitionVersion   int        `gorm:"column:definition_version;not null;default:1" json:"definition_version"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// TableName 指定表名。
func (UserDailyTask) TableName() string { return "user_daily_tasks" }

// AchievementDefinition 成就定义（版本化）。
type AchievementDefinition struct {
	ID                  uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Code                string    `gorm:"size:100;not null;uniqueIndex" json:"code"`
	Name                string    `gorm:"size:150;not null" json:"name"`
	Description         string    `gorm:"type:text;not null;default:''" json:"description"`
	Category            string    `gorm:"size:50;not null" json:"category"`
	Rarity              string    `gorm:"size:30;not null;default:common" json:"rarity"`
	IconURL             string    `gorm:"column:icon_url;type:text;not null;default:''" json:"icon_url"`
	ConditionType       string    `gorm:"column:condition_type;size:100;not null" json:"condition_type"`
	ConditionEventType  string    `gorm:"column:condition_event_type;size:100;not null;default:''" json:"condition_event_type"`
	ConditionSubjectType string   `gorm:"column:condition_subject_type;size:64;not null;default:''" json:"condition_subject_type"`
	TargetValue         int64     `gorm:"column:target_value;not null;default:1" json:"target_value"`
	RewardXP            int       `gorm:"column:reward_xp;not null;default:0" json:"reward_xp"`
	IsHidden            bool      `gorm:"column:is_hidden;not null;default:false" json:"is_hidden"`
	Enabled             bool      `gorm:"not null;default:true" json:"enabled"`
	Version             int       `gorm:"not null;default:1" json:"version"`
	SortOrder           int       `gorm:"column:sort_order;not null;default:0" json:"sort_order"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// TableName 指定表名。
func (AchievementDefinition) TableName() string { return "achievement_definitions" }

// UserAchievement 用户已解锁成就（默认永久保留）。
type UserAchievement struct {
	ID                     uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID                 uuid.UUID  `gorm:"column:user_id;type:uuid;not null" json:"user_id"`
	AchievementID          uuid.UUID  `gorm:"column:achievement_id;type:uuid;not null" json:"achievement_id"`
	AchievementCodeSnapshot string    `gorm:"column:achievement_code_snapshot;size:100;not null" json:"code"`
	NameSnapshot           string     `gorm:"column:name_snapshot;size:150;not null" json:"name"`
	DescriptionSnapshot    string     `gorm:"column:description_snapshot;type:text;not null;default:''" json:"description"`
	DefinitionVersion      int        `gorm:"column:definition_version;not null;default:1" json:"definition_version"`
	UnlockedAt             time.Time  `gorm:"column:unlocked_at;not null;default:now()" json:"unlocked_at"`
	SourceEventID          *uuid.UUID `gorm:"column:source_event_id;type:uuid" json:"source_event_id"`
	IsPublic               bool       `gorm:"column:is_public;not null;default:true" json:"is_public"`
}

// TableName 指定表名。
func (UserAchievement) TableName() string { return "user_achievements" }

// AchievementProgress 成就进度缓存（可从业务表重建，不作为唯一审计依据）。
type AchievementProgress struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID        uuid.UUID `gorm:"column:user_id;type:uuid;not null" json:"user_id"`
	AchievementID uuid.UUID `gorm:"column:achievement_id;type:uuid;not null" json:"achievement_id"`
	ProgressCount int64     `gorm:"column:progress_count;not null;default:0" json:"progress_count"`
	TargetCount   int64     `gorm:"column:target_count;not null" json:"target_count"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// TableName 指定表名。
func (AchievementProgress) TableName() string { return "achievement_progress" }

// GrowthAuditLog 成长审计日志（人工调整 / 冲正 / 规则修改 / 历史补算等）。
type GrowthAuditLog struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	OperatorUserID *uuid.UUID `gorm:"column:operator_user_id;type:uuid" json:"operator_user_id"`
	Action         string     `gorm:"size:100;not null" json:"action"`
	TargetUserID   *uuid.UUID `gorm:"column:target_user_id;type:uuid" json:"target_user_id"`
	EntityType     string     `gorm:"column:entity_type;size:100;not null" json:"entity_type"`
	EntityID       string     `gorm:"column:entity_id;size:128;not null;default:''" json:"entity_id"`
	Reason         string     `gorm:"type:text;not null" json:"reason"`
	BeforeData     types.JSONB `gorm:"column:before_data;type:jsonb" json:"before_data,omitempty"`
	AfterData      types.JSONB `gorm:"column:after_data;type:jsonb" json:"after_data,omitempty"`
	RequestID      string     `gorm:"column:request_id;size:128;not null;default:''" json:"request_id"`
	CreatedAt      time.Time  `json:"created_at"`
}

// TableName 指定表名。
func (GrowthAuditLog) TableName() string { return "growth_audit_logs" }
