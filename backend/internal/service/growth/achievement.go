package growth

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"mckit/backend/internal/domain"
)

// applyAchievements 评估并解锁成就（jingyan.md §6）。
//
// 进度从业务事实（growth_events / user_daily_tasks / user_growth）实时聚合，
// achievement_progress 仅作缓存；解锁以 (user_id, achievement_id) 唯一约束兜底。
func (s *Service) applyAchievements(tx *gorm.DB, ev domain.GrowthEvent) error {
	_, err := s.evaluateAchievements(tx, ev.ActorUserID, &ev.EventID)
	return err
}

// evaluateAchievements 对单个用户重算成就进度并解锁满足条件的成就，返回本次新解锁数量。
//
// sourceEventID 为触发本次评估的事件（历史补算时可为 nil）。解锁与发奖均以
// (user_id, achievement_id) 唯一约束与账本幂等键兜底，可安全重复执行。
func (s *Service) evaluateAchievements(tx *gorm.DB, userID uuid.UUID, sourceEventID *uuid.UUID) (int, error) {
	var defs []domain.AchievementDefinition
	if err := tx.Where("enabled = true").Order("sort_order ASC, code ASC").Find(&defs).Error; err != nil {
		return 0, err
	}
	unlockedCount := 0
	for _, def := range defs {
		progress, err := computeAchievementProgress(tx, userID, def)
		if err != nil {
			return unlockedCount, err
		}
		if err := upsertAchievementProgress(tx, userID, def, progress); err != nil {
			return unlockedCount, err
		}
		if progress < def.TargetValue {
			continue
		}
		ua := domain.UserAchievement{
			UserID:                  userID,
			AchievementID:           def.ID,
			AchievementCodeSnapshot: def.Code,
			NameSnapshot:            def.Name,
			DescriptionSnapshot:     def.Description,
			DefinitionVersion:       def.Version,
			SourceEventID:           sourceEventID,
			IsPublic:                true,
		}
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&ua)
		if res.Error != nil {
			return unlockedCount, res.Error
		}
		if res.RowsAffected == 0 {
			continue // 已解锁
		}
		unlockedCount++
		if def.RewardXP <= 0 {
			continue
		}
		key := achievementRewardKey(def.Code, userID.String())
		absent, err := ledgerAbsent(tx, key)
		if err != nil {
			return unlockedCount, err
		}
		if !absent {
			continue
		}
		if _, err := s.applyLedger(tx, ledgerInput{
			UserID:      userID,
			DeltaXP:     int64(def.RewardXP),
			SourceType:  domain.LedgerSourceAchievement,
			SourceID:    def.Code,
			EventID:     sourceEventID,
			Key:         key,
			RuleCode:    def.Code,
			RuleVersion: def.Version,
			Reason:      "解锁成就：" + def.Name,
		}); err != nil {
			return unlockedCount, err
		}
	}
	return unlockedCount, nil
}

// computeAchievementProgress 依据条件类型从业务事实聚合进度。
func computeAchievementProgress(tx *gorm.DB, userID uuid.UUID, def domain.AchievementDefinition) (int64, error) {
	switch def.ConditionType {
	case domain.AchievementCondEventCount:
		var cnt int64
		err := tx.Model(&domain.GrowthEventRecord{}).
			Where("actor_user_id = ? AND event_type = ? AND status = ?",
				userID, def.ConditionEventType, domain.GrowthEventProcessed).
			Count(&cnt).Error
		return cnt, err
	case domain.AchievementCondDistinctSubjectCount:
		sql := `SELECT COUNT(DISTINCT subject_id) FROM growth_events
			WHERE actor_user_id = ? AND event_type = ? AND status = ? AND subject_id <> ''`
		args := []any{userID, def.ConditionEventType, domain.GrowthEventProcessed}
		if def.ConditionSubjectType != "" {
			sql += " AND subject_type = ?"
			args = append(args, def.ConditionSubjectType)
		}
		var cnt int64
		if err := tx.Raw(sql, args...).Scan(&cnt).Error; err != nil {
			return 0, err
		}
		return cnt, nil
	case domain.AchievementCondDailyTaskDays:
		var cnt int64
		err := tx.Raw(`SELECT COUNT(DISTINCT task_date) FROM user_daily_tasks
			WHERE user_id = ? AND status IN (?, ?)`,
			userID, domain.DailyTaskCompleted, domain.DailyTaskClaimed).Scan(&cnt).Error
		return cnt, err
	case domain.AchievementCondLevelReached:
		var lv int
		if err := tx.Model(&domain.UserGrowth{}).Where("user_id = ?", userID).
			Select("level").Scan(&lv).Error; err != nil {
			return 0, err
		}
		return int64(lv), nil
	default:
		return 0, nil
	}
}

// upsertAchievementProgress 写入 / 更新进度缓存。
func upsertAchievementProgress(tx *gorm.DB, userID uuid.UUID, def domain.AchievementDefinition, progress int64) error {
	return tx.Exec(`INSERT INTO achievement_progress
			(user_id, achievement_id, progress_count, target_count, updated_at)
		VALUES (?, ?, ?, ?, now())
		ON CONFLICT (user_id, achievement_id) DO UPDATE
			SET progress_count = EXCLUDED.progress_count,
			    target_count = EXCLUDED.target_count,
			    updated_at = now()`,
		userID, def.ID, progress, def.TargetValue).Error
}
