package growth

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"mckit/backend/internal/domain"
)

// 本文件提供后台「用户详情」视图所需的聚合查询：
//   - 用户列表：等级 / 经验 / 成就数 / 礼物币余额；
//   - 单用户详情：等级与进度、成就、经验账本、礼物账本、成长审计。
//
// 只读聚合，不改变任何数据。列表与详情覆盖全部账号（含后台管理员），
// 便于运营排查管理员账号是否也产生了成长数据；角色仅用于展示区分，
// 不扩大任何后台权限。明细默认返回最近 200 条，并附带总数，完整历史仍可
// 通过经验流水 / 审计日志 Tab 按用户继续筛选。

const detailEntryLimit = 200

// AdminGrowthUserSummary 后台成长用户摘要。
type AdminGrowthUserSummary struct {
	UserID             string     `json:"user_id"`
	UserNo             string     `json:"user_no"`
	Username           string     `json:"username"`
	Nickname           string     `json:"nickname"`
	Role               string     `json:"role"`
	IsActive           bool       `json:"is_active"`
	Level              int        `json:"level"`
	LevelTitle         string     `json:"level_title"`
	CurrentXP          int64      `json:"current_xp"`
	LifetimeEarnedXP   int64      `json:"lifetime_earned_xp"`
	LifetimeReversedXP int64      `json:"lifetime_reversed_xp"`
	AchievementCount   int64      `json:"achievement_count"`
	GiftBalance        int64      `json:"gift_balance"`
	GiftGeneralBalance int64      `json:"gift_general_balance"`
	GiftCheckinBalance int64      `json:"gift_checkin_balance"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          *time.Time `json:"updated_at"`
}

// AdminGrowthUserListFilter 后台成长用户列表筛选。
type AdminGrowthUserListFilter struct {
	Q        string
	Page     int
	PageSize int
}

// AdminGrowthAchievementItem 用户已解锁成就（后台详情）。
type AdminGrowthAchievementItem struct {
	ID           string    `json:"id"`
	Code         string    `json:"code"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	Category     string    `json:"category"`
	Rarity       string    `json:"rarity"`
	RewardXP     int       `json:"reward_xp"`
	UnlockedAt   time.Time `json:"unlocked_at"`
	GrantSource  string    `json:"grant_source"`
	IsPublic     bool      `json:"is_public"`
	DefinitionID string    `json:"definition_id"`
}

// AdminGiftLedgerItem 后台用户详情中的礼物币流水。
type AdminGiftLedgerItem struct {
	ID           string    `json:"id"`
	Amount       int64     `json:"amount"`
	CoinType     string    `json:"coin_type"`
	BalanceAfter int64     `json:"balance_after"`
	EventType    string    `json:"event_type"`
	SourceType   string    `json:"source_type"`
	SourceID     string    `json:"source_id"`
	Reason       string    `json:"reason"`
	CreatedAt    time.Time `json:"created_at"`
}

// AdminGrowthAuditItem 用户详情中的成长审计日志。
type AdminGrowthAuditItem struct {
	ID             string    `json:"id"`
	Action         string    `json:"action"`
	EntityType     string    `json:"entity_type"`
	EntityID       string    `json:"entity_id"`
	Reason         string    `json:"reason"`
	OperatorUserID string    `json:"operator_user_id"`
	CreatedAt      time.Time `json:"created_at"`
}

// AdminGrowthUserDetail 单用户成长详情聚合。
type AdminGrowthUserDetail struct {
	User                 AdminGrowthUserSummary       `json:"user"`
	BaseLevel            int                          `json:"base_level"`
	LevelOffset          int                          `json:"level_offset"`
	NextLevelXP          *int64                       `json:"next_level_xp"`
	XPToNextLevel        int64                        `json:"xp_to_next_level"`
	LevelProgressPercent int                          `json:"level_progress_percent"`
	IsMaxLevel           bool                         `json:"is_max_level"`
	AchievementTotal     int64                        `json:"achievement_total"`
	ExperienceTotal      int64                        `json:"experience_total"`
	GiftTotal            int64                        `json:"gift_total"`
	AuditTotal           int64                        `json:"audit_total"`
	Experience           []LedgerItem                 `json:"experience"`
	Achievements         []AdminGrowthAchievementItem `json:"achievements"`
	Gifts                []AdminGiftLedgerItem        `json:"gifts"`
	Audits               []AdminGrowthAuditItem       `json:"audits"`
}

type adminGrowthUserRow struct {
	UserID             uuid.UUID  `gorm:"column:user_id"`
	UserNo             string     `gorm:"column:user_no"`
	Username           string     `gorm:"column:username"`
	Nickname           string     `gorm:"column:nickname"`
	Role               string     `gorm:"column:role"`
	IsActive           bool       `gorm:"column:is_active"`
	Level              int        `gorm:"column:level"`
	CurrentXP          int64      `gorm:"column:current_xp"`
	LifetimeEarnedXP   int64      `gorm:"column:lifetime_earned_xp"`
	LifetimeReversedXP int64      `gorm:"column:lifetime_reversed_xp"`
	AchievementCount   int64      `gorm:"column:achievement_count"`
	GiftBalance        int64      `gorm:"column:gift_balance"`
	GiftGeneralBalance int64      `gorm:"column:gift_general_balance"`
	GiftCheckinBalance int64      `gorm:"column:gift_checkin_balance"`
	CreatedAt          time.Time  `gorm:"column:created_at"`
	UpdatedAt          *time.Time `gorm:"column:updated_at"`
}

const adminGrowthUserColumns = `a.id AS user_id, a.user_no, a.username, a.nickname, a.role, a.is_active,
	a.created_at,
	COALESCE(g.level, 1) AS level,
	COALESCE(g.current_xp, 0) AS current_xp,
	COALESCE(g.lifetime_earned_xp, 0) AS lifetime_earned_xp,
	COALESCE(g.lifetime_reversed_xp, 0) AS lifetime_reversed_xp,
	COALESCE(w.general_balance, 0) AS gift_general_balance,
	COALESCE(w.checkin_balance, 0) AS gift_checkin_balance,
	COALESCE(w.general_balance, 0) + COALESCE(w.checkin_balance, 0) AS gift_balance,
	COALESCE(g.updated_at, a.updated_at) AS updated_at,
	(SELECT count(*) FROM user_achievements ua
		WHERE ua.user_id = a.id AND ua.revoked_at IS NULL) AS achievement_count`

// ListAdminGrowthUsers 分页查询全部账号（含管理员）的成长摘要。
func (s *Service) ListAdminGrowthUsers(ctx context.Context, f AdminGrowthUserListFilter) ([]AdminGrowthUserSummary, int64, error) {
	page, pageSize := normalizeAdminPage(f.Page, f.PageSize)
	base := s.db.WithContext(ctx).Table("accounts a").
		Joins("LEFT JOIN user_growth g ON g.user_id = a.id").
		Joins("LEFT JOIN gift_wallets w ON w.user_id = a.id")
	if q := strings.TrimSpace(f.Q); q != "" {
		like := "%" + q + "%"
		base = base.Where(
			"a.username ILIKE ? OR a.nickname ILIKE ? OR a.user_no ILIKE ?",
			like, like, like,
		)
	}
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []adminGrowthUserRow
	if err := base.Select(adminGrowthUserColumns).
		Order("COALESCE(g.updated_at, a.created_at) DESC, a.created_at DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	levels, err := s.repo.ListLevels(ctx)
	if err != nil {
		return nil, 0, err
	}
	out := make([]AdminGrowthUserSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, toAdminGrowthUserSummary(row, levels))
	}
	return out, total, nil
}

// GetAdminGrowthUserDetail 汇总单个账号（含管理员）的成长详情。
// 支持内部 UUID、对外 user_no 或用户名；后台账号即使没有成长投影也会返回零值详情。
func (s *Service) GetAdminGrowthUserDetail(ctx context.Context, rawUserID string) (*AdminGrowthUserDetail, error) {
	id, err := s.resolveGrowthUserID(ctx, rawUserID)
	if err != nil {
		return nil, err
	}
	var rows []adminGrowthUserRow
	if err := s.db.WithContext(ctx).Table("accounts a").
		Joins("LEFT JOIN user_growth g ON g.user_id = a.id").
		Joins("LEFT JOIN gift_wallets w ON w.user_id = a.id").
		Where("a.id = ?", id).
		Select(adminGrowthUserColumns).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrUnknownUser
	}
	levels, err := s.repo.ListLevels(ctx)
	if err != nil {
		return nil, err
	}
	summary := toAdminGrowthUserSummary(rows[0], levels)

	var growth domain.UserGrowth
	if err := s.db.WithContext(ctx).Where("user_id = ?", id).First(&growth).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		growth = domain.UserGrowth{UserID: id, Level: 1}
	}
	progress := computeLevelWithOffset(levels, growth.CurrentXP, growth.LevelOffset)
	base := computeLevel(levels, growth.CurrentXP)

	var achievementTotal int64
	if err := s.db.WithContext(ctx).Model(&domain.AchievementDefinition{}).
		Where("enabled = true").Count(&achievementTotal).Error; err != nil {
		return nil, err
	}
	var achievements []domain.UserAchievement
	if err := s.db.WithContext(ctx).
		Where("user_id = ? AND revoked_at IS NULL", id).
		Order("unlocked_at DESC").
		Find(&achievements).Error; err != nil {
		return nil, err
	}
	achievementRows := make([]AdminGrowthAchievementItem, 0, len(achievements))
	for _, ua := range achievements {
		achievementRows = append(achievementRows, AdminGrowthAchievementItem{
			ID:           ua.ID.String(),
			Code:         ua.AchievementCodeSnapshot,
			Name:         ua.NameSnapshot,
			Description:  ua.DescriptionSnapshot,
			Category:     "",
			Rarity:       "",
			RewardXP:     0,
			UnlockedAt:   ua.UnlockedAt,
			GrantSource:  ua.GrantSource,
			IsPublic:     ua.IsPublic,
			DefinitionID: ua.AchievementID.String(),
		})
	}
	// 补全成就分类 / 稀有度 / 奖励（定义可能已被停用，但历史解锁快照仍需展示）。
	if len(achievements) > 0 {
		ids := make([]uuid.UUID, 0, len(achievements))
		for _, ua := range achievements {
			ids = append(ids, ua.AchievementID)
		}
		var defs []domain.AchievementDefinition
		if err := s.db.WithContext(ctx).Where("id IN ?", ids).Find(&defs).Error; err != nil {
			return nil, err
		}
		defMap := make(map[uuid.UUID]domain.AchievementDefinition, len(defs))
		for _, def := range defs {
			defMap[def.ID] = def
		}
		for i := range achievementRows {
			if def, ok := defMap[achievements[i].AchievementID]; ok {
				achievementRows[i].Category = def.Category
				achievementRows[i].Rarity = def.Rarity
				achievementRows[i].RewardXP = def.RewardXP
			}
		}
	}

	ledgerRows, experienceTotal, err := s.repo.ListLedger(ctx, LedgerFilter{
		UserID: id.String(), Page: 1, PageSize: detailEntryLimit,
	})
	if err != nil {
		return nil, err
	}
	var giftRows []domain.GiftLedger
	if err := s.db.WithContext(ctx).Where("user_id = ?", id).
		Order("created_at DESC").Limit(detailEntryLimit).Find(&giftRows).Error; err != nil {
		return nil, err
	}
	var giftTotal int64
	if err := s.db.WithContext(ctx).Model(&domain.GiftLedger{}).
		Where("user_id = ?", id).Count(&giftTotal).Error; err != nil {
		return nil, err
	}
	gifts := make([]AdminGiftLedgerItem, 0, len(giftRows))
	for _, row := range giftRows {
		gifts = append(gifts, AdminGiftLedgerItem{
			ID:           row.ID.String(),
			Amount:       row.Amount,
			CoinType:     row.CoinType,
			BalanceAfter: row.CoinBalanceAfter,
			EventType:    row.EventType,
			SourceType:   row.SourceType,
			SourceID:     row.SourceID,
			Reason:       row.Reason,
			CreatedAt:    row.CreatedAt,
		})
	}

	auditRows, auditTotal, err := s.repo.ListAudit(ctx, AuditFilter{
		TargetUser: id.String(), Page: 1, PageSize: detailEntryLimit,
	})
	if err != nil {
		return nil, err
	}
	audits := make([]AdminGrowthAuditItem, 0, len(auditRows))
	for _, row := range auditRows {
		operator := ""
		if row.OperatorUserID != nil {
			operator = row.OperatorUserID.String()
		}
		audits = append(audits, AdminGrowthAuditItem{
			ID:             row.ID.String(),
			Action:         row.Action,
			EntityType:     row.EntityType,
			EntityID:       row.EntityID,
			Reason:         row.Reason,
			OperatorUserID: operator,
			CreatedAt:      row.CreatedAt,
		})
	}

	return &AdminGrowthUserDetail{
		User:                 summary,
		BaseLevel:            base.Level,
		LevelOffset:          growth.LevelOffset,
		NextLevelXP:          progress.NextThreshold,
		XPToNextLevel:        progress.XPToNext,
		LevelProgressPercent: progress.ProgressPercent,
		IsMaxLevel:           progress.IsMax,
		AchievementTotal:     achievementTotal,
		ExperienceTotal:      experienceTotal,
		GiftTotal:            giftTotal,
		AuditTotal:           auditTotal,
		Experience:           toLedgerItems(ledgerRows),
		Achievements:         achievementRows,
		Gifts:                gifts,
		Audits:               audits,
	}, nil
}

func toAdminGrowthUserSummary(row adminGrowthUserRow, levels []domain.GrowthLevel) AdminGrowthUserSummary {
	return AdminGrowthUserSummary{
		UserID:             row.UserID.String(),
		UserNo:             row.UserNo,
		Username:           row.Username,
		Nickname:           row.Nickname,
		Role:               row.Role,
		IsActive:           row.IsActive,
		Level:              row.Level,
		LevelTitle:         levelTitle(levels, row.Level),
		CurrentXP:          row.CurrentXP,
		LifetimeEarnedXP:   row.LifetimeEarnedXP,
		LifetimeReversedXP: row.LifetimeReversedXP,
		AchievementCount:   row.AchievementCount,
		GiftBalance:        row.GiftBalance,
		GiftGeneralBalance: row.GiftGeneralBalance,
		GiftCheckinBalance: row.GiftCheckinBalance,
		CreatedAt:          row.CreatedAt,
		UpdatedAt:          row.UpdatedAt,
	}
}

// resolveGrowthUserID 将后台输入解析为账号 UUID，覆盖普通用户与后台管理员。
// 与成长业务写操作的 resolveUserID 不同：这里只做只读详情查询，允许管理员账号，
// 以便运营排查后台账号的成长数据；写操作仍只允许普通用户。
func (s *Service) resolveGrowthUserID(ctx context.Context, raw string) (uuid.UUID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return uuid.Nil, ErrUnknownUser
	}
	if id, err := uuid.Parse(raw); err == nil {
		var count int64
		if err := s.db.WithContext(ctx).Model(&domain.Account{}).
			Where("id = ?", id).Count(&count).Error; err != nil {
			return uuid.Nil, err
		}
		if count == 0 {
			return uuid.Nil, ErrUnknownUser
		}
		return id, nil
	}
	var account domain.Account
	err := s.db.WithContext(ctx).Where("user_no = ?", raw).First(&account).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = s.db.WithContext(ctx).Where("username = ?", raw).First(&account).Error
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return uuid.Nil, ErrUnknownUser
	}
	if err != nil {
		return uuid.Nil, err
	}
	return account.ID, nil
}

func normalizeAdminPage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}
