package growth

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"mckit/backend/internal/domain"
)

// Repo 成长数据仓储（GORM 实现）。
type Repo struct {
	db       *gorm.DB
	timezone *time.Location
}

// NewRepo 创建仓储。loc 为站点业务时区，用于按业务日期统计（nil 时按 UTC）。
func NewRepo(db *gorm.DB, loc *time.Location) *Repo {
	if loc == nil {
		loc = time.UTC
	}
	return &Repo{db: db, timezone: loc}
}

// ---- 等级 ----

// ListLevels 返回全部等级（按等级升序）。
func (r *Repo) ListLevels(ctx context.Context) ([]domain.GrowthLevel, error) {
	var out []domain.GrowthLevel
	err := r.db.WithContext(ctx).Order("level ASC").Find(&out).Error
	return out, err
}

// ---- 规则 ----

// ListRules 返回全部经验规则（版本降序，编码升序）。
func (r *Repo) ListRules(ctx context.Context) ([]domain.GrowthRule, error) {
	var out []domain.GrowthRule
	err := r.db.WithContext(ctx).Order("code ASC, version DESC").Find(&out).Error
	return out, err
}

// GetRule 按 ID 查询规则。
func (r *Repo) GetRule(ctx context.Context, id string) (*domain.GrowthRule, error) {
	var rule domain.GrowthRule
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&rule).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrRuleNotFound
	}
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

// ---- 每日任务 ----

// ListTaskDefs 返回每日任务定义（enabledOnly 时只返回启用项）。
func (r *Repo) ListTaskDefs(ctx context.Context, enabledOnly bool) ([]domain.DailyTaskDefinition, error) {
	q := r.db.WithContext(ctx).Model(&domain.DailyTaskDefinition{})
	if enabledOnly {
		q = q.Where("enabled = true")
	}
	var out []domain.DailyTaskDefinition
	err := q.Order("sort_order ASC, code ASC").Find(&out).Error
	return out, err
}

// GetTaskDef 按 ID 查询任务定义。
func (r *Repo) GetTaskDef(ctx context.Context, id string) (*domain.DailyTaskDefinition, error) {
	var def domain.DailyTaskDefinition
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&def).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrTaskNotFound
	}
	if err != nil {
		return nil, err
	}
	return &def, nil
}

// ListUserDailyTasks 返回指定用户某业务日期的任务进度（按定义排序）。
func (r *Repo) ListUserDailyTasks(ctx context.Context, userID, date string) ([]domain.UserDailyTask, error) {
	var out []domain.UserDailyTask
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND task_date = ?", userID, date).
		Find(&out).Error
	return out, err
}

// ListUserDailyTaskHistory 分页查询用户历史每日任务。
func (r *Repo) ListUserDailyTaskHistory(ctx context.Context, userID string, page, pageSize int) ([]domain.UserDailyTask, int64, error) {
	base := r.db.WithContext(ctx).Model(&domain.UserDailyTask{}).Where("user_id = ?", userID)
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []domain.UserDailyTask
	err := base.Order("task_date DESC, task_code ASC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&out).Error
	return out, total, err
}

// ---- 成就 ----

// ListAchievementDefs 返回成就定义（enabledOnly 时只返回启用项）。
func (r *Repo) ListAchievementDefs(ctx context.Context, enabledOnly bool) ([]domain.AchievementDefinition, error) {
	q := r.db.WithContext(ctx).Model(&domain.AchievementDefinition{})
	if enabledOnly {
		q = q.Where("enabled = true")
	}
	var out []domain.AchievementDefinition
	err := q.Order("sort_order ASC, code ASC").Find(&out).Error
	return out, err
}

// GetAchievementDef 按 ID 查询成就定义。
func (r *Repo) GetAchievementDef(ctx context.Context, id string) (*domain.AchievementDefinition, error) {
	var def domain.AchievementDefinition
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&def).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAchievementNotFound
	}
	if err != nil {
		return nil, err
	}
	return &def, nil
}

// ListUserAchievements 返回用户已解锁成就（按解锁时间倒序）。
func (r *Repo) ListUserAchievements(ctx context.Context, userID string) ([]domain.UserAchievement, error) {
	var out []domain.UserAchievement
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).
		Order("unlocked_at DESC").Find(&out).Error
	return out, err
}

// ListPublicUserAchievements 返回用户公开展示的成就。
func (r *Repo) ListPublicUserAchievements(ctx context.Context, userID string) ([]domain.UserAchievement, error) {
	var out []domain.UserAchievement
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND is_public = true", userID).
		Order("unlocked_at DESC").Find(&out).Error
	return out, err
}

// ListAchievementProgress 返回用户成就进度缓存。
func (r *Repo) ListAchievementProgress(ctx context.Context, userID string) ([]domain.AchievementProgress, error) {
	var out []domain.AchievementProgress
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Find(&out).Error
	return out, err
}

// ---- 成长投影 ----

// GetGrowth 读取用户成长投影；不存在返回 (nil, nil)。
func (r *Repo) GetGrowth(ctx context.Context, userID string) (*domain.UserGrowth, error) {
	var g domain.UserGrowth
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&g).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &g, nil
}

// ---- 经验流水 ----

// LedgerFilter 经验流水查询筛选。
type LedgerFilter struct {
	UserID     string
	RuleCode   string
	SourceType string
	SourceID   string
	OperatorID string
	EventID    string
	From       *time.Time
	To         *time.Time
	Page       int
	PageSize   int
}

// ListLedger 分页查询经验流水。
func (r *Repo) ListLedger(ctx context.Context, f LedgerFilter) ([]domain.ExperienceLedger, int64, error) {
	q := r.db.WithContext(ctx).Model(&domain.ExperienceLedger{})
	if f.UserID != "" {
		q = q.Where("user_id = ?", f.UserID)
	}
	if f.RuleCode != "" {
		q = q.Where("rule_code = ?", f.RuleCode)
	}
	if f.SourceType != "" {
		q = q.Where("source_type = ?", f.SourceType)
	}
	if f.SourceID != "" {
		q = q.Where("source_id = ?", f.SourceID)
	}
	if f.OperatorID != "" {
		q = q.Where("operator_user_id = ?", f.OperatorID)
	}
	if f.EventID != "" {
		q = q.Where("event_id = ?", f.EventID)
	}
	if f.From != nil {
		q = q.Where("created_at >= ?", *f.From)
	}
	if f.To != nil {
		q = q.Where("created_at < ?", *f.To)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []domain.ExperienceLedger
	err := q.Order("created_at DESC, id DESC").
		Offset((f.Page - 1) * f.PageSize).Limit(f.PageSize).Find(&out).Error
	return out, total, err
}

// GetLedger 按 ID 查询流水。
func (r *Repo) GetLedger(ctx context.Context, id string) (*domain.ExperienceLedger, error) {
	var l domain.ExperienceLedger
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&l).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrLedgerNotFound
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

// ---- 成长审计 ----

// AuditFilter 成长审计查询筛选。
type AuditFilter struct {
	OperatorID string
	TargetUser string
	Action     string
	Page       int
	PageSize   int
}

// ListAudit 分页查询成长审计日志。
func (r *Repo) ListAudit(ctx context.Context, f AuditFilter) ([]domain.GrowthAuditLog, int64, error) {
	q := r.db.WithContext(ctx).Model(&domain.GrowthAuditLog{})
	if f.OperatorID != "" {
		q = q.Where("operator_user_id = ?", f.OperatorID)
	}
	if f.TargetUser != "" {
		q = q.Where("target_user_id = ?", f.TargetUser)
	}
	if f.Action != "" {
		q = q.Where("action = ?", f.Action)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []domain.GrowthAuditLog
	err := q.Order("created_at DESC").
		Offset((f.Page - 1) * f.PageSize).Limit(f.PageSize).Find(&out).Error
	return out, total, err
}

// ---- 统计（后台概览） ----

// GrowthOverviewStats 后台成长概览统计（口径见注释）。
type GrowthOverviewStats struct {
	ActiveUsers        int64 `json:"active_users"`         // 有成长记录的用户数
	XPIssuedToday      int64 `json:"xp_issued_today"`      // 今日正向发放经验合计
	XPIssued7Days      int64 `json:"xp_issued_7days"`      // 近 7 日正向发放经验合计
	LedgerToday        int64 `json:"ledger_today"`         // 今日奖励笔数（正向流水）
	ReversedTotal      int64 `json:"reversed_total"`       // 累计冲正经验（绝对值）
	ManualAdjustCount  int64 `json:"manual_adjust_count"`  // 累计人工调整笔数
	UnlockedTotal      int64 `json:"unlocked_total"`       // 累计成就解锁数
	AchievementCount   int64 `json:"achievement_count"`    // 启用成就总数
	LevelDistribution  []LevelBucket `json:"level_distribution"`
	TasksCompletedToday int64 `json:"tasks_completed_today"` // 今日完成任务数
	TaskParticipantsToday int64 `json:"task_participants_today"` // 今日任务参与人数
}

// LevelBucket 单个等级的用户分布。
type LevelBucket struct {
	Level int   `json:"level"`
	Count int64 `json:"count"`
}

// Stats 汇总后台概览统计（date 为业务日期，dayStart/dayEnd/weekStart 为其对应的 UTC 区间）。
func (r *Repo) Stats(ctx context.Context, date string, dayStart, dayEnd, weekStart time.Time) (*GrowthOverviewStats, error) {
	db := r.db.WithContext(ctx)
	out := &GrowthOverviewStats{LevelDistribution: []LevelBucket{}}

	if err := db.Model(&domain.UserGrowth{}).Count(&out.ActiveUsers).Error; err != nil {
		return nil, err
	}
	if err := db.Model(&domain.ExperienceLedger{}).
		Where("delta_xp > 0 AND created_at >= ? AND created_at < ?", dayStart, dayEnd).
		Select("COALESCE(SUM(delta_xp),0)").Scan(&out.XPIssuedToday).Error; err != nil {
		return nil, err
	}
	if err := db.Model(&domain.ExperienceLedger{}).
		Where("delta_xp > 0 AND created_at >= ?", weekStart).
		Select("COALESCE(SUM(delta_xp),0)").Scan(&out.XPIssued7Days).Error; err != nil {
		return nil, err
	}
	if err := db.Model(&domain.ExperienceLedger{}).
		Where("delta_xp > 0 AND created_at >= ? AND created_at < ?", dayStart, dayEnd).
		Count(&out.LedgerToday).Error; err != nil {
		return nil, err
	}
	if err := db.Model(&domain.ExperienceLedger{}).
		Where("source_type = ?", domain.LedgerSourceReversal).
		Select("COALESCE(SUM(-delta_xp),0)").Scan(&out.ReversedTotal).Error; err != nil {
		return nil, err
	}
	if err := db.Model(&domain.ExperienceLedger{}).
		Where("source_type = ?", domain.LedgerSourceManual).
		Count(&out.ManualAdjustCount).Error; err != nil {
		return nil, err
	}
	if err := db.Model(&domain.UserAchievement{}).Count(&out.UnlockedTotal).Error; err != nil {
		return nil, err
	}
	if err := db.Model(&domain.AchievementDefinition{}).Where("enabled = true").Count(&out.AchievementCount).Error; err != nil {
		return nil, err
	}
	if err := db.Model(&domain.UserGrowth{}).
		Select("level, COUNT(*) AS count").Group("level").Order("level ASC").
		Scan(&out.LevelDistribution).Error; err != nil {
		return nil, err
	}
	if err := db.Model(&domain.UserDailyTask{}).
		Where("task_date = ? AND status IN ?", date, []string{domain.DailyTaskCompleted, domain.DailyTaskClaimed}).
		Count(&out.TasksCompletedToday).Error; err != nil {
		return nil, err
	}
	if err := db.Model(&domain.UserDailyTask{}).
		Where("task_date = ?", date).
		Distinct("user_id").Count(&out.TaskParticipantsToday).Error; err != nil {
		return nil, err
	}
	return out, nil
}
