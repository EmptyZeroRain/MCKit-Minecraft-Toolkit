package growth

import (
	"gorm.io/gorm"

	"mckit/backend/internal/domain"
)

// applyDailyTasks 推进与事件类型匹配的每日任务（jingyan.md §4）。
//
// 仅在对应事件确实产生奖励（或该事件类型无经验规则）时推进，避免：
//   - 超出每日上限的事件继续推进进度；
//   - 同一目标对象被反复计数（如取消后重赞）。
func (s *Service) applyDailyTasks(tx *gorm.DB, ev domain.GrowthEvent, date string, hasRules bool, granted map[string]bool) error {
	if hasRules && !granted[ev.EventType] {
		return nil
	}
	var defs []domain.DailyTaskDefinition
	if err := tx.Where("enabled = true").Order("sort_order ASC, code ASC").Find(&defs).Error; err != nil {
		return err
	}
	for _, def := range defs {
		if def.EventType != ev.EventType {
			continue
		}
		if err := s.advanceTask(tx, ev, def, date); err != nil {
			return err
		}
	}
	return nil
}

// taskUpsert 任务进度原子自增的返回结果。
type taskUpsert struct {
	ProgressCount       int    `gorm:"column:progress_count"`
	TargetCountSnapshot int    `gorm:"column:target_count_snapshot"`
	RewardXPSnapshot    int    `gorm:"column:reward_xp_snapshot"`
	Status              string `gorm:"column:status"`
}

// advanceTask 原子推进单个任务进度；首次达到目标时自动发奖（jingyan.md §4.3）。
//
// 采用 INSERT ... ON CONFLICT DO UPDATE ... WHERE status='in_progress' 保证并发安全：
// 已完成的当天任务不再自增，也不会重复发奖。
func (s *Service) advanceTask(tx *gorm.DB, ev domain.GrowthEvent, def domain.DailyTaskDefinition, date string) error {
	const sql = `
INSERT INTO user_daily_tasks
    (user_id, task_code, task_date, progress_count, target_count_snapshot,
     reward_xp_snapshot, status, definition_version, created_at, updated_at)
VALUES (?, ?, ?, 1, ?, ?, 'in_progress', ?, now(), now())
ON CONFLICT (user_id, task_code, task_date) DO UPDATE
    SET progress_count = user_daily_tasks.progress_count + 1,
        updated_at = now()
    WHERE user_daily_tasks.status = 'in_progress'
RETURNING progress_count, target_count_snapshot, reward_xp_snapshot, status`

	var res taskUpsert
	if err := tx.Raw(sql, ev.ActorUserID, def.Code, date,
		def.TargetCount, def.RewardXP, def.Version).Scan(&res).Error; err != nil {
		return err
	}
	if res.Status == "" {
		return nil // 已完成 / 已发奖，无需再推进
	}
	if res.ProgressCount < res.TargetCountSnapshot {
		return nil
	}

	// 首次达到目标：标记完成并自动发奖（claimed）。
	now := s.now()
	if err := tx.Model(&domain.UserDailyTask{}).
		Where("user_id = ? AND task_code = ? AND task_date = ?", ev.ActorUserID, def.Code, date).
		Updates(map[string]any{
			"status":       domain.DailyTaskClaimed,
			"completed_at": now,
			"claimed_at":   now,
			"updated_at":   now,
		}).Error; err != nil {
		return err
	}
	if res.RewardXPSnapshot <= 0 {
		return nil
	}
	key := taskRewardKey(def.Code, ev.ActorUserID.String(), date)
	absent, err := ledgerAbsent(tx, key)
	if err != nil {
		return err
	}
	if !absent {
		return nil
	}
	_, err = s.applyLedger(tx, ledgerInput{
		UserID:      ev.ActorUserID,
		DeltaXP:     int64(res.RewardXPSnapshot),
		SourceType:  domain.LedgerSourceDailyTask,
		SourceID:    def.Code,
		EventID:     &ev.EventID,
		Key:         key,
		RuleCode:    def.Code,
		RuleVersion: def.Version,
		Reason:      "完成每日任务：" + def.Name,
	})
	return err
}

// TaskView 用户端每日任务视图（定义 + 当天进度合并）。
type TaskView struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Progress    int    `json:"progress"`
	Target      int    `json:"target"`
	RewardXP    int    `json:"reward_xp"`
	Status      string `json:"status"`
	CompletedAt string `json:"completed_at"`
}

// buildTaskViews 合并任务定义与当天进度。
func buildTaskViews(defs []domain.DailyTaskDefinition, rows []domain.UserDailyTask) []TaskView {
	byCode := make(map[string]domain.UserDailyTask, len(rows))
	for _, r := range rows {
		byCode[r.TaskCode] = r
	}
	out := make([]TaskView, 0, len(defs))
	for _, def := range defs {
		view := TaskView{
			Code: def.Code, Name: def.Name, Description: def.Description,
			Target: def.TargetCount, RewardXP: def.RewardXP, Status: domain.DailyTaskInProgress,
		}
		if r, ok := byCode[def.Code]; ok {
			view.Progress = r.ProgressCount
			view.Target = r.TargetCountSnapshot
			view.RewardXP = r.RewardXPSnapshot
			view.Status = r.Status
			if r.CompletedAt != nil {
				view.CompletedAt = r.CompletedAt.Format("2006-01-02T15:04:05Z07:00")
			}
		}
		if view.Progress >= view.Target && view.Status == domain.DailyTaskInProgress {
			view.Status = domain.DailyTaskCompleted
		}
		out = append(out, view)
	}
	return out
}
