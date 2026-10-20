package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"mckit/backend/internal/domain"
	"mckit/backend/internal/http/response"
	"mckit/backend/internal/service/growth"
)

// GrowthHandler 用户成长处理器：用户端成长中心 + 公开主页 + 后台管理端。
//
// 依据 jingyan.md：
//   - 用户端只读本人成长数据（等级仅展示用），不做任何判定；
//   - 管理端需要 ops.growth 权限（路由层 RequireAuth + RequirePermission + CSRF）；
//   - 人工调整 / 冲正 / 定义维护均在服务层写账本与审计。
type GrowthHandler struct {
	svc *growth.Service
}

// NewGrowthHandler 创建成长处理器。
func NewGrowthHandler(svc *growth.Service) *GrowthHandler {
	return &GrowthHandler{svc: svc}
}

// ---- 用户端 ----

// Overview POST /api/v1/growth/overview
func (h *GrowthHandler) Overview(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUserID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.GetOverview(r.Context(), userID)
	if err != nil {
		writeGrowthError(w, r, err)
		return
	}
	response.OK(w, out)
}

// Ledger POST /api/v1/growth/ledger
// 参数：page、page_size。
func (h *GrowthHandler) Ledger(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUserID(w, r)
	if !ok {
		return
	}
	var req struct {
		Page     int `json:"page"`
		PageSize int `json:"page_size"`
	}
	_ = decodeOptionalBody(w, r, &req)
	page, pageSize := normalizePage(req.Page, req.PageSize)
	items, total, err := h.svc.ListLedger(r.Context(), userID, page, pageSize)
	if err != nil {
		writeGrowthError(w, r, err)
		return
	}
	writeGrowthPage(w, r, items, total, page, pageSize)
}

// DailyTasks POST /api/v1/growth/daily-tasks
func (h *GrowthHandler) DailyTasks(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUserID(w, r)
	if !ok {
		return
	}
	tasks, date, err := h.svc.DailyTasks(r.Context(), userID)
	if err != nil {
		writeGrowthError(w, r, err)
		return
	}
	response.OK(w, map[string]any{
		"date":     date,
		"timezone": h.svc.Timezone(),
		"tasks":    tasks,
	})
}

// DailyTaskHistory POST /api/v1/growth/daily-tasks/history
func (h *GrowthHandler) DailyTaskHistory(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUserID(w, r)
	if !ok {
		return
	}
	var req struct {
		Page     int `json:"page"`
		PageSize int `json:"page_size"`
	}
	_ = decodeOptionalBody(w, r, &req)
	page, pageSize := normalizePage(req.Page, req.PageSize)
	items, total, err := h.svc.DailyTaskHistory(r.Context(), userID, page, pageSize)
	if err != nil {
		writeGrowthError(w, r, err)
		return
	}
	writeGrowthPage(w, r, items, total, page, pageSize)
}

// Achievements POST /api/v1/growth/achievements
func (h *GrowthHandler) Achievements(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUserID(w, r)
	if !ok {
		return
	}
	items, err := h.svc.AchievementCatalog(r.Context(), userID)
	if err != nil {
		writeGrowthError(w, r, err)
		return
	}
	response.OK(w, map[string]any{"items": items, "total": len(items)})
}

// PublicProfile POST /api/v1/public/users/detail
// 参数：id。返回公开等级与展示资料（不含经验流水与登录信息）。
func (h *GrowthHandler) PublicProfile(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	out, err := h.svc.GetPublicProfile(r.Context(), strings.TrimSpace(req.ID))
	if err != nil {
		writeGrowthError(w, r, err)
		return
	}
	response.OK(w, out)
}

// PublicAchievements POST /api/v1/public/users/achievements
// 参数：id。返回用户公开展示的成就。
func (h *GrowthHandler) PublicAchievements(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	items, err := h.svc.PublicAchievements(r.Context(), strings.TrimSpace(req.ID))
	if err != nil {
		writeGrowthError(w, r, err)
		return
	}
	response.OK(w, map[string]any{"items": items, "total": len(items)})
}

// ---- 管理端：概览 / 审计 ----

// AdminOverview POST /api/v1/admin/growth/overview
func (h *GrowthHandler) AdminOverview(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.GetAdminOverview(r.Context())
	if err != nil {
		writeGrowthError(w, r, err)
		return
	}
	response.OK(w, out)
}

// AdminUsersList POST /api/v1/admin/growth/users/list
// 参数：q（用户名 / 昵称 / user_no）、page、page_size。
func (h *GrowthHandler) AdminUsersList(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Q        string `json:"q"`
		Page     int    `json:"page"`
		PageSize int    `json:"page_size"`
	}
	_ = decodeOptionalBody(w, r, &req)
	page, pageSize := normalizePage(req.Page, req.PageSize)
	items, total, err := h.svc.ListAdminGrowthUsers(r.Context(), growth.AdminGrowthUserListFilter{
		Q:        strings.TrimSpace(req.Q),
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		writeGrowthError(w, r, err)
		return
	}
	writeGrowthPage(w, r, items, total, page, pageSize)
}

// AdminUserDetail POST /api/v1/admin/growth/users/detail
// 参数：user_id（内部 UUID 或对外 user_no）。
func (h *GrowthHandler) AdminUserDetail(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID string `json:"user_id"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	out, err := h.svc.GetAdminGrowthUserDetail(r.Context(), strings.TrimSpace(req.UserID))
	if err != nil {
		writeGrowthError(w, r, err)
		return
	}
	response.OK(w, out)
}

// AdminLedger POST /api/v1/admin/growth/ledger/list
func (h *GrowthHandler) AdminLedger(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID     string `json:"user_id"`
		RuleCode   string `json:"rule_code"`
		SourceType string `json:"source_type"`
		Page       int    `json:"page"`
		PageSize   int    `json:"page_size"`
	}
	_ = decodeOptionalBody(w, r, &req)
	page, pageSize := normalizePage(req.Page, req.PageSize)
	items, total, err := h.svc.AdminListLedger(r.Context(), growth.LedgerFilter{
		UserID:     strings.TrimSpace(req.UserID),
		RuleCode:   strings.TrimSpace(req.RuleCode),
		SourceType: strings.TrimSpace(req.SourceType),
		Page:       page,
		PageSize:   pageSize,
	})
	if err != nil {
		writeGrowthError(w, r, err)
		return
	}
	writeGrowthPage(w, r, items, total, page, pageSize)
}

// Adjust POST /api/v1/admin/growth/adjust
// 参数：target_user_id、delta_xp、reason、request_key（幂等，可选）。
func (h *GrowthHandler) Adjust(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TargetUserID string `json:"target_user_id"`
		DeltaXP      int64  `json:"delta_xp"`
		Reason       string `json:"reason"`
		RequestKey   string `json:"request_key"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	entry, err := h.svc.Adjust(r.Context(), growth.AdjustInput{
		TargetUserID: req.TargetUserID,
		DeltaXP:      req.DeltaXP,
		Reason:       req.Reason,
		RequestKey:   req.RequestKey,
		OperatorID:   actorID(r),
		RequestID:    response.RequestIDFrom(r.Context()),
	})
	if err != nil {
		writeGrowthError(w, r, err)
		return
	}
	response.OK(w, entry)
}

// Reverse POST /api/v1/admin/growth/reverse
// 参数：id（原流水 ID）、reason。
func (h *GrowthHandler) Reverse(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID     string `json:"id"`
		Reason string `json:"reason"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	entry, err := h.svc.Reverse(r.Context(), growth.ReverseInput{
		LedgerID:   req.ID,
		Reason:     req.Reason,
		OperatorID: actorID(r),
		RequestID:  response.RequestIDFrom(r.Context()),
	})
	if err != nil {
		writeGrowthError(w, r, err)
		return
	}
	response.OK(w, entry)
}

// AdminAudit POST /api/v1/admin/growth/audit/list
func (h *GrowthHandler) AdminAudit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OperatorID string `json:"operator_id"`
		TargetUser string `json:"target_user_id"`
		Action     string `json:"action"`
		Page       int    `json:"page"`
		PageSize   int    `json:"page_size"`
	}
	_ = decodeOptionalBody(w, r, &req)
	page, pageSize := normalizePage(req.Page, req.PageSize)
	items, total, err := h.svc.ListAudit(r.Context(), growth.AuditFilter{
		OperatorID: strings.TrimSpace(req.OperatorID),
		TargetUser: strings.TrimSpace(req.TargetUser),
		Action:     strings.TrimSpace(req.Action),
		Page:       page,
		PageSize:   pageSize,
	})
	if err != nil {
		writeGrowthError(w, r, err)
		return
	}
	writeGrowthPage(w, r, items, total, page, pageSize)
}

// ---- 管理端：经验规则 ----

// RulesList POST /api/v1/admin/growth/rules/list
func (h *GrowthHandler) RulesList(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.ListRules(r.Context())
	if err != nil {
		writeGrowthError(w, r, err)
		return
	}
	response.OK(w, map[string]any{"items": items, "total": len(items)})
}

// RulesCreate POST /api/v1/admin/growth/rules/create
func (h *GrowthHandler) RulesCreate(w http.ResponseWriter, r *http.Request) {
	var req growth.RuleInput
	if !decodeBody(w, r, &req) {
		return
	}
	out, err := h.svc.CreateRule(r.Context(), req, actorID(r))
	if err != nil {
		writeGrowthError(w, r, err)
		return
	}
	response.Created(w, out)
}

// RulesUpdate POST /api/v1/admin/growth/rules/update（版本化：新版本 +1）
func (h *GrowthHandler) RulesUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
		growth.RuleInput
	}
	if !decodeBody(w, r, &req) {
		return
	}
	out, err := h.svc.UpdateRule(r.Context(), strings.TrimSpace(req.ID), req.RuleInput, actorID(r))
	if err != nil {
		writeGrowthError(w, r, err)
		return
	}
	response.OK(w, out)
}

// RulesToggle POST /api/v1/admin/growth/rules/toggle
// 参数：id、enabled。
func (h *GrowthHandler) RulesToggle(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID      string `json:"id"`
		Enabled bool   `json:"enabled"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if err := h.svc.SetRuleEnabled(r.Context(), strings.TrimSpace(req.ID), req.Enabled, actorID(r)); err != nil {
		writeGrowthError(w, r, err)
		return
	}
	response.NoContent(w)
}

// RulesDelete POST /api/v1/admin/growth/rules/delete
func (h *GrowthHandler) RulesDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := decodeID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteRule(r.Context(), id, actorID(r)); err != nil {
		writeGrowthError(w, r, err)
		return
	}
	response.NoContent(w)
}

// ---- 管理端：等级 ----

// LevelsList POST /api/v1/admin/growth/levels/list
func (h *GrowthHandler) LevelsList(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.ListLevels(r.Context())
	if err != nil {
		writeGrowthError(w, r, err)
		return
	}
	response.OK(w, map[string]any{"items": items, "total": len(items)})
}

// LevelsUpdate POST /api/v1/admin/growth/levels/update
func (h *GrowthHandler) LevelsUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Level       int    `json:"level"`
		ThresholdXP int64  `json:"threshold_xp"`
		Title       string `json:"title"`
		Enabled     *bool  `json:"enabled"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	lvl := domain.GrowthLevel{Level: req.Level, ThresholdXP: req.ThresholdXP, Title: req.Title, Enabled: enabled}
	if err := h.svc.UpdateLevel(r.Context(), lvl, actorID(r)); err != nil {
		writeGrowthError(w, r, err)
		return
	}
	response.NoContent(w)
}

// LevelsCreate POST /api/v1/admin/growth/levels/create
// 参数：level（可选，省略时追加为最高级 +1）、threshold_xp、title、enabled。
func (h *GrowthHandler) LevelsCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Level       int    `json:"level"`
		ThresholdXP int64  `json:"threshold_xp"`
		Title       string `json:"title"`
		Enabled     *bool  `json:"enabled"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	out, err := h.svc.CreateLevel(r.Context(), growth.CreateLevelInput{
		Level:       req.Level,
		ThresholdXP: req.ThresholdXP,
		Title:       req.Title,
		Enabled:     enabled,
	}, actorID(r))
	if err != nil {
		writeGrowthError(w, r, err)
		return
	}
	response.Created(w, out)
}

// LevelsDelete POST /api/v1/admin/growth/levels/delete
// 参数：level。可删除任意 Lv.2 及以上等级；Lv.1 不可删除。
func (h *GrowthHandler) LevelsDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Level int `json:"level"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if err := h.svc.DeleteLevel(r.Context(), req.Level, actorID(r)); err != nil {
		writeGrowthError(w, r, err)
		return
	}
	response.NoContent(w)
}

// LevelsRecalculate POST /api/v1/admin/growth/levels/recalculate
func (h *GrowthHandler) LevelsRecalculate(w http.ResponseWriter, r *http.Request) {
	fixed, err := h.svc.RecalculateLevels(r.Context())
	if err != nil {
		writeGrowthError(w, r, err)
		return
	}
	response.OK(w, map[string]any{"fixed": fixed})
}

// LevelsAdjust POST /api/v1/admin/growth/levels/adjust
// 参数：target_user_id（仅对外用户 ID）、delta_level（可负）、reason、request_key（仅留痕）。
func (h *GrowthHandler) LevelsAdjust(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TargetUserID string `json:"target_user_id"`
		DeltaLevel   int    `json:"delta_level"`
		Reason       string `json:"reason"`
		RequestKey   string `json:"request_key"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	out, err := h.svc.AdjustLevel(r.Context(), growth.AdjustLevelInput{
		TargetUserID: req.TargetUserID,
		DeltaLevel:   req.DeltaLevel,
		Reason:       req.Reason,
		RequestKey:   req.RequestKey,
		OperatorID:   actorID(r),
		RequestID:    response.RequestIDFrom(r.Context()),
	})
	if err != nil {
		writeGrowthError(w, r, err)
		return
	}
	response.OK(w, out)
}

// ---- 管理端：每日任务定义 ----

// TasksList POST /api/v1/admin/daily-tasks/list
func (h *GrowthHandler) TasksList(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.ListTaskDefs(r.Context())
	if err != nil {
		writeGrowthError(w, r, err)
		return
	}
	response.OK(w, map[string]any{"items": items, "total": len(items)})
}

// TasksCreate POST /api/v1/admin/daily-tasks/create
func (h *GrowthHandler) TasksCreate(w http.ResponseWriter, r *http.Request) {
	var req growth.TaskDefInput
	if !decodeBody(w, r, &req) {
		return
	}
	out, err := h.svc.CreateTaskDef(r.Context(), req, actorID(r))
	if err != nil {
		writeGrowthError(w, r, err)
		return
	}
	response.Created(w, out)
}

// TasksUpdate POST /api/v1/admin/daily-tasks/update
func (h *GrowthHandler) TasksUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
		growth.TaskDefInput
	}
	if !decodeBody(w, r, &req) {
		return
	}
	out, err := h.svc.UpdateTaskDef(r.Context(), strings.TrimSpace(req.ID), req.TaskDefInput, actorID(r))
	if err != nil {
		writeGrowthError(w, r, err)
		return
	}
	response.OK(w, out)
}

// TasksToggle POST /api/v1/admin/daily-tasks/toggle
func (h *GrowthHandler) TasksToggle(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID      string `json:"id"`
		Enabled bool   `json:"enabled"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if err := h.svc.SetTaskDefEnabled(r.Context(), strings.TrimSpace(req.ID), req.Enabled, actorID(r)); err != nil {
		writeGrowthError(w, r, err)
		return
	}
	response.NoContent(w)
}

// TasksDelete POST /api/v1/admin/daily-tasks/delete
func (h *GrowthHandler) TasksDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := decodeID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteTaskDef(r.Context(), id, actorID(r)); err != nil {
		writeGrowthError(w, r, err)
		return
	}
	response.NoContent(w)
}

// ---- 管理端：成就定义 ----

// AchievementsList POST /api/v1/admin/achievements/list
func (h *GrowthHandler) AchievementsList(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.ListAchievementDefs(r.Context())
	if err != nil {
		writeGrowthError(w, r, err)
		return
	}
	response.OK(w, map[string]any{"items": items, "total": len(items)})
}

// AchievementsCreate POST /api/v1/admin/achievements/create
func (h *GrowthHandler) AchievementsCreate(w http.ResponseWriter, r *http.Request) {
	var req growth.AchievementInput
	if !decodeBody(w, r, &req) {
		return
	}
	out, err := h.svc.CreateAchievement(r.Context(), req, actorID(r))
	if err != nil {
		writeGrowthError(w, r, err)
		return
	}
	response.Created(w, out)
}

// AchievementsUpdate POST /api/v1/admin/achievements/update
func (h *GrowthHandler) AchievementsUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
		growth.AchievementInput
	}
	if !decodeBody(w, r, &req) {
		return
	}
	out, err := h.svc.UpdateAchievement(r.Context(), strings.TrimSpace(req.ID), req.AchievementInput, actorID(r))
	if err != nil {
		writeGrowthError(w, r, err)
		return
	}
	response.OK(w, out)
}

// AchievementsToggle POST /api/v1/admin/achievements/toggle
func (h *GrowthHandler) AchievementsToggle(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID      string `json:"id"`
		Enabled bool   `json:"enabled"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if err := h.svc.SetAchievementEnabled(r.Context(), strings.TrimSpace(req.ID), req.Enabled, actorID(r)); err != nil {
		writeGrowthError(w, r, err)
		return
	}
	response.NoContent(w)
}

// AchievementsRecalculate POST /api/v1/admin/achievements/recalculate
// 参数：user_id（可选，为空时对全部用户补算）。
func (h *GrowthHandler) AchievementsRecalculate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID string `json:"user_id"`
	}
	_ = decodeOptionalBody(w, r, &req)
	total, err := h.svc.RecalculateAchievements(r.Context(), strings.TrimSpace(req.UserID), actorID(r))
	if err != nil {
		writeGrowthError(w, r, err)
		return
	}
	response.OK(w, map[string]any{"unlocked": total})
}

// AchievementsUserList POST /api/v1/admin/achievements/user/list
// 参数：target_user_id。返回该用户全部成就的持有 / 撤销状态。
func (h *GrowthHandler) AchievementsUserList(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TargetUserID string `json:"target_user_id"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	items, err := h.svc.ListAdminUserAchievements(r.Context(), req.TargetUserID)
	if err != nil {
		writeGrowthError(w, r, err)
		return
	}
	response.OK(w, map[string]any{"items": items, "total": len(items)})
}

// AchievementsGrant POST /api/v1/admin/achievements/grant
// 参数：target_user_id、achievement_id、reason。
func (h *GrowthHandler) AchievementsGrant(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TargetUserID  string `json:"target_user_id"`
		AchievementID string `json:"achievement_id"`
		Reason        string `json:"reason"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	out, err := h.svc.GrantAchievement(r.Context(), growth.AchievementGrantInput{
		TargetUserID:  req.TargetUserID,
		AchievementID: req.AchievementID,
		Reason:        req.Reason,
		OperatorID:    actorID(r),
		RequestID:     response.RequestIDFrom(r.Context()),
	})
	if err != nil {
		writeGrowthError(w, r, err)
		return
	}
	response.OK(w, out)
}

// AchievementsRevoke POST /api/v1/admin/achievements/revoke
// 参数：target_user_id、achievement_id、reason。
func (h *GrowthHandler) AchievementsRevoke(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TargetUserID  string `json:"target_user_id"`
		AchievementID string `json:"achievement_id"`
		Reason        string `json:"reason"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	out, err := h.svc.RevokeAchievement(r.Context(), growth.AchievementRevokeInput{
		TargetUserID:  req.TargetUserID,
		AchievementID: req.AchievementID,
		Reason:        req.Reason,
		OperatorID:    actorID(r),
		RequestID:     response.RequestIDFrom(r.Context()),
	})
	if err != nil {
		writeGrowthError(w, r, err)
		return
	}
	response.OK(w, out)
}

// ---- 辅助 ----

// writeGrowthPage 统一输出分页响应。
func writeGrowthPage(w http.ResponseWriter, r *http.Request, data any, total int64, page, pageSize int) {
	response.Page(w, r, data, response.ResourceList{
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages(total, pageSize),
	})
}

// writeGrowthError 将成长服务错误映射为 HTTP 响应。
func writeGrowthError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, growth.ErrRuleNotFound),
		errors.Is(err, growth.ErrTaskNotFound),
		errors.Is(err, growth.ErrAchievementNotFound),
		errors.Is(err, growth.ErrLevelNotFound),
		errors.Is(err, growth.ErrLedgerNotFound),
		errors.Is(err, growth.ErrUnknownUser):
		response.NotFound(w, r)
	case errors.Is(err, growth.ErrRuleCodeTaken),
		errors.Is(err, growth.ErrTaskCodeTaken),
		errors.Is(err, growth.ErrAchievementCodeTaken),
		errors.Is(err, growth.ErrAchievementAlreadyUnlocked),
		errors.Is(err, growth.ErrAchievementNotUnlocked),
		errors.Is(err, growth.ErrLevelExists):
		response.Conflict(w, r, err.Error())
	case errors.Is(err, growth.ErrAlreadyReversed),
		errors.Is(err, growth.ErrRuleInUse),
		errors.Is(err, growth.ErrTaskInUse):
		response.Conflict(w, r, err.Error())
	case errors.Is(err, growth.ErrBadInput),
		errors.Is(err, growth.ErrReasonRequired),
		errors.Is(err, growth.ErrLevelThreshold),
		errors.Is(err, growth.ErrLevelOrder),
		errors.Is(err, growth.ErrLevelDeleteLv1),
		errors.Is(err, growth.ErrLevelRange),
		errors.Is(err, growth.ErrBadCondition),
		errors.Is(err, growth.ErrBadIconURL),
		errors.Is(err, growth.ErrInvalidEvent),
		errors.Is(err, growth.ErrReverseExceeds),
		errors.Is(err, growth.ErrCannotReverseReversal):
		response.Validation(w, r, []response.ErrorDetail{{Message: err.Error()}})
	case errors.Is(err, growth.ErrLevelAdjustUserIDRequired),
		errors.Is(err, growth.ErrLevelAdjustUserNotFound):
		response.Validation(w, r, []response.ErrorDetail{{
			Field: "target_user_id", Message: err.Error(),
		}})
	default:
		response.HandleError(w, r, err)
	}
}

// decodeOptionalBody 解码可选的 JSON 请求体：空体或格式错误时按零值继续（列表接口默认分页）。
func decodeOptionalBody(w http.ResponseWriter, r *http.Request, dst any) error {
	if r.Body == nil {
		return nil
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(dst); err != nil {
		return err
	}
	return nil
}
