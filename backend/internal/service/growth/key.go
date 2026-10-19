package growth

import (
	"strconv"
	"strings"
)

// 规则奖励幂等键生成策略（jingyan.md §7.1）：
//  1. 有单对象上限（per_subject_limit）→ 以「账号 + 目标对象」为键，历史只奖励一次；
//     典型如点赞：取消后重赞不再获得同一帖子的点赞经验。
//  2. 有每日上限（daily_limit）→ 以「账号 + 业务日期」为键，每天只奖励一次；
//     典型如每日登录、每日工具使用。
//  3. 其余 → 以「账号 + 事件 ID」为键，同一次业务事实重复处理不重复发奖。

// ruleKeyForSubject 以账号 + 目标对象为键（跨天长期唯一）。
func ruleKeyForSubject(ruleCode, actorID, subjectID string) string {
	return "rule:" + ruleCode + ":" + actorID + ":subject:" + subjectID
}

// ruleKeyForDay 以账号 + 业务日期为键（每天唯一）。
func ruleKeyForDay(ruleCode, actorID, date string) string {
	return "rule:" + ruleCode + ":" + actorID + ":day:" + date
}

// ruleKeyForEvent 以账号 + 事件 ID 为键（同一事件唯一）。
func ruleKeyForEvent(ruleCode, actorID, eventID string) string {
	return "rule:" + ruleCode + ":" + actorID + ":event:" + eventID
}

// taskRewardKey 每日任务完成奖励键：账号 + 任务 + 业务日期，每天每任务一次。
func taskRewardKey(taskCode, actorID, date string) string {
	return "task:" + taskCode + ":" + actorID + ":" + date
}

// achievementRewardKey 成就奖励键：账号 + 成就编码，终身一次。
func achievementRewardKey(code, actorID string) string {
	return "achievement:" + code + ":" + actorID
}

// manualAdjustKey 人工调整幂等键：账号 + 客户端请求唯一 ID。
func manualAdjustKey(actorID, requestKey string) string {
	return "manual:" + actorID + ":" + requestKey
}

// reversalKey 冲正幂等键：原流水 ID，保证同一流水不被重复冲正。
func reversalKey(ledgerID string) string {
	return "reversal:" + ledgerID
}

// parsePositiveInt 解析正整数；空串或非法返回 nil（表示不限制）。
func parsePositiveInt(s string) *int {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	v, err := strconv.Atoi(s)
	if err != nil || v <= 0 {
		return nil
	}
	return &v
}
