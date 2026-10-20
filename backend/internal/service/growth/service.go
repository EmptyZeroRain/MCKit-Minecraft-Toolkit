package growth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"mckit/backend/internal/domain"
)

// Service 用户成长服务。
type Service struct {
	db   *gorm.DB
	repo *Repo
	loc  *time.Location
	now  func() time.Time
}

// NewService 创建成长服务。timezone 为站点业务时区（空 / 非法时回退 Asia/Shanghai）。
func NewService(db *gorm.DB, timezone string) *Service {
	loc := normalizeTimezone(timezone)
	return &Service{
		db:   db,
		repo: NewRepo(db, loc),
		loc:  loc,
		now:  time.Now,
	}
}

// Timezone 返回站点业务时区名。
func (s *Service) Timezone() string { return s.loc.String() }

// Overview 成长概览：经验、等级、升级进度与成长统计（jingyan.md §9.4）。
type Overview struct {
	CurrentXP            int64  `json:"current_xp"`
	LifetimeEarnedXP     int64  `json:"lifetime_earned_xp"`
	LifetimeReversedXP   int64  `json:"lifetime_reversed_xp"`
	Level                int    `json:"level"`
	LevelTitle           string `json:"level_title"`
	NextLevelXP          *int64 `json:"next_level_xp"`
	XPToNextLevel        int64  `json:"xp_to_next_level"`
	LevelProgressPercent int    `json:"level_progress_percent"`
	IsMaxLevel           bool   `json:"is_max_level"`
	UnlockedAchievements int    `json:"unlocked_achievements"`
	AchievementTotal     int    `json:"achievement_total"`
	TodayTasksCompleted  int    `json:"today_tasks_completed"`
	TodayTasksTotal      int    `json:"today_tasks_total"`
	TodayXP              int64  `json:"today_xp"`
}

// GetOverview 汇总当前用户的成长概览。
func (s *Service) GetOverview(ctx context.Context, userID string) (*Overview, error) {
	levels, err := s.repo.ListLevels(ctx)
	if err != nil {
		return nil, err
	}
	g, err := s.repo.GetGrowth(ctx, userID)
	if err != nil {
		return nil, err
	}
	if g == nil {
		g = &domain.UserGrowth{Level: 1}
	}
	view := computeLevelWithOffset(levels, g.CurrentXP, g.LevelOffset)

	date := businessDate(s.now(), s.loc)
	defs, err := s.repo.ListTaskDefs(ctx, true)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.ListUserDailyTasks(ctx, userID, date)
	if err != nil {
		return nil, err
	}
	tasks := buildTaskViews(defs, rows)
	completed := 0
	for _, t := range tasks {
		if t.Status == domain.DailyTaskCompleted || t.Status == domain.DailyTaskClaimed {
			completed++
		}
	}

	unlocked, err := s.repo.ListUserAchievements(ctx, userID)
	if err != nil {
		return nil, err
	}
	allDefs, err := s.repo.ListAchievementDefs(ctx, true)
	if err != nil {
		return nil, err
	}
	todayXP, err := s.todayXP(ctx, userID, date)
	if err != nil {
		return nil, err
	}

	return &Overview{
		CurrentXP:            g.CurrentXP,
		LifetimeEarnedXP:     g.LifetimeEarnedXP,
		LifetimeReversedXP:   g.LifetimeReversedXP,
		Level:                view.Level,
		LevelTitle:           view.Title,
		NextLevelXP:          view.NextThreshold,
		XPToNextLevel:        view.XPToNext,
		LevelProgressPercent: view.ProgressPercent,
		IsMaxLevel:           view.IsMax,
		UnlockedAchievements: len(unlocked),
		AchievementTotal:     len(allDefs),
		TodayTasksCompleted:  completed,
		TodayTasksTotal:      len(tasks),
		TodayXP:              todayXP,
	}, nil
}

// todayXP 统计用户今日入账的有效经验（含正向与冲正，按业务日期）。
func (s *Service) todayXP(ctx context.Context, userID, date string) (int64, error) {
	start, end, err := dayRange(date, s.loc)
	if err != nil {
		return 0, err
	}
	var sum int64
	err = s.db.WithContext(ctx).Model(&domain.ExperienceLedger{}).
		Where("user_id = ? AND created_at >= ? AND created_at < ?", userID, start, end).
		Select("COALESCE(SUM(delta_xp),0)").Scan(&sum).Error
	return sum, err
}

// LedgerItem 用户端 / 管理端经验流水条目。
type LedgerItem struct {
	ID           string `json:"id"`
	UserID       string `json:"user_id"`
	DeltaXP      int64  `json:"delta_xp"`
	BalanceAfter int64  `json:"balance_after"`
	SourceType   string `json:"source_type"`
	SourceID     string `json:"source_id"`
	RuleCode     string `json:"rule_code"`
	Reason       string `json:"reason"`
	CreatedAt    string `json:"created_at"`
}

func toLedgerItems(rows []domain.ExperienceLedger) []LedgerItem {
	out := make([]LedgerItem, 0, len(rows))
	for _, l := range rows {
		out = append(out, LedgerItem{
			ID:           l.ID.String(),
			UserID:       l.UserID.String(),
			DeltaXP:      l.DeltaXP,
			BalanceAfter: l.BalanceAfter,
			SourceType:   l.SourceType,
			SourceID:     l.SourceID,
			RuleCode:     l.RuleCode,
			Reason:       l.Reason,
			CreatedAt:    l.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	return out
}

// ListLedger 本人经验流水（分页）。
func (s *Service) ListLedger(ctx context.Context, userID string, page, pageSize int) ([]LedgerItem, int64, error) {
	rows, total, err := s.repo.ListLedger(ctx, LedgerFilter{UserID: userID, Page: page, PageSize: pageSize})
	if err != nil {
		return nil, 0, err
	}
	return toLedgerItems(rows), total, nil
}

// DailyTasks 返回当日任务列表与业务日期 / 时区。
func (s *Service) DailyTasks(ctx context.Context, userID string) ([]TaskView, string, error) {
	date := businessDate(s.now(), s.loc)
	defs, err := s.repo.ListTaskDefs(ctx, true)
	if err != nil {
		return nil, date, err
	}
	rows, err := s.repo.ListUserDailyTasks(ctx, userID, date)
	if err != nil {
		return nil, date, err
	}
	return buildTaskViews(defs, rows), date, nil
}

// DailyTaskHistory 历史每日任务（分页）。
func (s *Service) DailyTaskHistory(ctx context.Context, userID string, page, pageSize int) ([]domain.UserDailyTask, int64, error) {
	return s.repo.ListUserDailyTaskHistory(ctx, userID, page, pageSize)
}

// AchievementView 成就目录 / 进度视图。
type AchievementView struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Rarity      string `json:"rarity"`
	IconURL     string `json:"icon_url"`
	IsHidden    bool   `json:"is_hidden"`
	Unlocked    bool   `json:"unlocked"`
	UnlockedAt  string `json:"unlocked_at,omitempty"`
	Progress    int64  `json:"progress"`
	Target      int64  `json:"target"`
	RewardXP    int    `json:"reward_xp"`
}

// AchievementCatalog 成就目录（含本人解锁状态与进度）。
// 隐藏成就未解锁时只返回占位信息，不泄露名称、描述、条件与奖励细节（jingyan.md §6.2）。
func (s *Service) AchievementCatalog(ctx context.Context, userID string) ([]AchievementView, error) {
	defs, err := s.repo.ListAchievementDefs(ctx, true)
	if err != nil {
		return nil, err
	}
	unlocked, err := s.repo.ListUserAchievements(ctx, userID)
	if err != nil {
		return nil, err
	}
	progress, err := s.repo.ListAchievementProgress(ctx, userID)
	if err != nil {
		return nil, err
	}
	unlockedByID := map[uuid.UUID]domain.UserAchievement{}
	for _, u := range unlocked {
		unlockedByID[u.AchievementID] = u
	}
	progressByID := map[uuid.UUID]domain.AchievementProgress{}
	for _, p := range progress {
		progressByID[p.AchievementID] = p
	}

	out := make([]AchievementView, 0, len(defs))
	for _, def := range defs {
		view := AchievementView{
			Code: def.Code, Name: def.Name, Description: def.Description,
			Category: def.Category, Rarity: def.Rarity, IconURL: def.IconURL,
			IsHidden: def.IsHidden, Target: def.TargetValue, RewardXP: def.RewardXP,
		}
		if p, ok := progressByID[def.ID]; ok {
			view.Progress = p.ProgressCount
		}
		if u, ok := unlockedByID[def.ID]; ok {
			view.Unlocked = true
			view.Progress = def.TargetValue
			view.UnlockedAt = u.UnlockedAt.Format("2006-01-02T15:04:05Z07:00")
		}
		if def.IsHidden && !view.Unlocked {
			view.Name = "隐藏成就"
			view.Description = "达成特定条件后揭晓"
			view.Progress = 0
			view.Target = 0
			view.RewardXP = 0
		}
		out = append(out, view)
	}
	return out, nil
}

// MyAchievements 返回本人已解锁成就（快照）。
func (s *Service) MyAchievements(ctx context.Context, userID string) ([]domain.UserAchievement, error) {
	return s.repo.ListUserAchievements(ctx, userID)
}

// PublicProfile 公开个人主页成长摘要（不含经验流水、登录时间、风险标记等）。
type PublicProfile struct {
	UserID               string `json:"user_id"`
	UserNo               string `json:"user_no"`
	Username             string `json:"username"`
	Nickname             string `json:"nickname"`
	AvatarURL            string `json:"avatar_url"`
	Level                int    `json:"level"`
	LevelTitle           string `json:"level_title"`
	CurrentXP            int64  `json:"current_xp"`
	UnlockedAchievements int    `json:"unlocked_achievements"`
}

// GetPublicProfile 返回用户公开等级与展示资料。
func (s *Service) GetPublicProfile(ctx context.Context, userID string) (*PublicProfile, error) {
	id, err := uuid.Parse(userID)
	if err != nil {
		return nil, ErrBadInput
	}
	var acc struct {
		UserNo    string
		Username  string
		Nickname  string
		AvatarURL string
	}
	err = s.db.WithContext(ctx).Table("accounts").
		Select("user_no, username, nickname, avatar_url").
		Where("id = ? AND role = ?", id, domain.RoleUser).Scan(&acc).Error
	if err != nil {
		return nil, err
	}
	if acc.Username == "" {
		return nil, ErrUnknownUser
	}
	levels, err := s.repo.ListLevels(ctx)
	if err != nil {
		return nil, err
	}
	g, err := s.repo.GetGrowth(ctx, userID)
	if err != nil {
		return nil, err
	}
	if g == nil {
		g = &domain.UserGrowth{Level: 1}
	}
	view := computeLevelWithOffset(levels, g.CurrentXP, g.LevelOffset)
	unlocked, err := s.repo.ListUserAchievements(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &PublicProfile{
		UserID:               userID,
		UserNo:               acc.UserNo,
		Username:             acc.Username,
		Nickname:             acc.Nickname,
		AvatarURL:            acc.AvatarURL,
		Level:                view.Level,
		LevelTitle:           view.Title,
		CurrentXP:            g.CurrentXP,
		UnlockedAchievements: len(unlocked),
	}, nil
}

// PublicAchievements 返回用户公开展示的成就。
func (s *Service) PublicAchievements(ctx context.Context, userID string) ([]domain.UserAchievement, error) {
	if _, err := uuid.Parse(userID); err != nil {
		return nil, ErrBadInput
	}
	return s.repo.ListPublicUserAchievements(ctx, userID)
}

// auditEntry 成长审计写入入参。
type auditEntry struct {
	OperatorID string
	Action     string
	TargetUser string
	EntityType string
	EntityID   string
	Reason     string
	Before     map[string]any
	After      map[string]any
	RequestID  string
}

// writeAudit 写入成长审计日志（失败不阻断业务，仅记日志由调用方决定）。
func writeAudit(tx *gorm.DB, e auditEntry) error {
	log := domain.GrowthAuditLog{
		OperatorUserID: parseUUIDPtr(e.OperatorID),
		Action:         e.Action,
		TargetUserID:   parseUUIDPtr(e.TargetUser),
		EntityType:     e.EntityType,
		EntityID:       e.EntityID,
		Reason:         e.Reason,
		BeforeData:     mustJSONB(e.Before),
		AfterData:      mustJSONB(e.After),
		RequestID:      e.RequestID,
	}
	return tx.Create(&log).Error
}

// parseUUIDPtr 解析可选 UUID 字符串；空 / 非法返回 nil。
func parseUUIDPtr(s string) *uuid.UUID {
	if s == "" {
		return nil
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return nil
	}
	return &id
}

// ErrServiceUnavailable 依赖缺失时的统一错误。
var ErrServiceUnavailable = errors.New("成长服务不可用")
