// Package handler 提供 HTTP 处理器。
package handler

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"mckit/backend/internal/audit"
	"mckit/backend/internal/http/response"
	"mckit/backend/internal/service/useradmin"
)

// UserAdminHandler 普通用户管理处理器（后台「普通用户管理」）。
// 仅具备 accounts.users 权限的账号可访问（路由层 RequireAuth + RequirePermission + CSRF 限制）。
type UserAdminHandler struct {
	svc      *useradmin.Service
	sessions adminSessionInvalidator
	auditSvc *audit.Service
}

// NewUserAdminHandler 创建处理器。
func NewUserAdminHandler(svc *useradmin.Service, sessions adminSessionInvalidator, auditSvc *audit.Service) *UserAdminHandler {
	return &UserAdminHandler{svc: svc, sessions: sessions, auditSvc: auditSvc}
}

// userListRequest 列表 / 导出共用入参。
type userListRequest struct {
	Q        string `json:"q"`
	Status   string `json:"status"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
}

// List POST /api/v1/admin/users/list
// 参数：q（用户名模糊）、status（active / disabled / 空）、page、page_size。
func (h *UserAdminHandler) List(w http.ResponseWriter, r *http.Request) {
	var req userListRequest
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}

	items, total, err := h.svc.List(r.Context(), useradmin.ListFilter{
		Q:        req.Q,
		Status:   req.Status,
		Page:     req.Page,
		PageSize: req.PageSize,
	})
	if err != nil {
		response.HandleError(w, r, err)
		return
	}

	page, pageSize := normalizePage(req.Page, req.PageSize)
	response.Page(w, r, items, response.ResourceList{
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: int((total + int64(pageSize) - 1) / int64(pageSize)),
	})
}

// Create POST /api/v1/admin/users/create
// 参数：username、password。角色固定为普通用户（user），不接受调用方指定。
func (h *UserAdminHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}

	item, err := h.svc.Create(r.Context(), useradmin.CreateInput{
		Username: req.Username,
		Password: req.Password,
	})
	if err != nil {
		writeUserAdminError(w, r, err)
		return
	}
	// 审计只记录用户名，绝不记录密码。
	h.auditLog(r, "user.create", item.ID, nil, map[string]any{"username": item.Username})
	response.Created(w, item)
}

// Detail POST /api/v1/admin/users/detail
// 参数：id。返回账号信息 + 收藏 / 最近使用 / 常用种子明细。
func (h *UserAdminHandler) Detail(w http.ResponseWriter, r *http.Request) {
	id, ok := decodeID(w, r)
	if !ok {
		return
	}
	detail, err := h.svc.Detail(r.Context(), id)
	if err != nil {
		writeUserAdminError(w, r, err)
		return
	}
	response.OK(w, detail)
}

// Update POST /api/v1/admin/users/update
// 白名单字段：is_active。停用后该用户全部会话立即失效。
func (h *UserAdminHandler) Update(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID       string `json:"id"`
		IsActive *bool  `json:"is_active"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	if strings.TrimSpace(req.ID) == "" {
		response.Validation(w, r, []response.ErrorDetail{{Field: "id", Message: "id 必填"}})
		return
	}
	if req.IsActive == nil {
		response.Validation(w, r, []response.ErrorDetail{{Field: "is_active", Message: "is_active 必填"}})
		return
	}

	item, err := h.svc.Update(r.Context(), req.ID, *req.IsActive)
	if err != nil {
		writeUserAdminError(w, r, err)
		return
	}
	// 停用后立即使其现有会话失效，避免已登录设备继续使用。
	if !item.IsActive {
		h.invalidate(r, req.ID)
	}
	h.auditLog(r, "user.update", req.ID, nil, map[string]any{
		"username":  item.Username,
		"is_active": item.IsActive,
	})
	response.OK(w, item)
}

// ResetPassword POST /api/v1/admin/users/reset-password
func (h *UserAdminHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID          string `json:"id"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	if strings.TrimSpace(req.ID) == "" {
		response.Validation(w, r, []response.ErrorDetail{{Field: "id", Message: "id 必填"}})
		return
	}
	if err := h.svc.ResetPassword(r.Context(), req.ID, req.NewPassword); err != nil {
		writeUserAdminError(w, r, err)
		return
	}
	h.invalidate(r, req.ID)
	// 审计只记录动作，绝不记录密码。
	h.auditLog(r, "user.reset_password", req.ID, nil, nil)
	response.NoContent(w)
}

// Delete POST /api/v1/admin/users/delete
// 物理删除账号，个人数据由外键 ON DELETE CASCADE 一并清除。
func (h *UserAdminHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := decodeID(w, r)
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		writeUserAdminError(w, r, err)
		return
	}
	h.invalidate(r, id)
	h.auditLog(r, "user.delete", id, nil, nil)
	response.NoContent(w)
}

// DeletePreview POST /api/v1/admin/users/delete-preview
// 返回删除该用户会一并清理的关联数据范围（只读，不执行删除、不写审计）。
func (h *UserAdminHandler) DeletePreview(w http.ResponseWriter, r *http.Request) {
	id, ok := decodeID(w, r)
	if !ok {
		return
	}
	preview, err := h.svc.DeletePreview(r.Context(), id)
	if err != nil {
		writeUserAdminError(w, r, err)
		return
	}
	response.OK(w, preview)
}

// Export POST /api/v1/admin/users/export
// 参数：q、status（与列表一致，但不分页）。返回 UTF-8 BOM + CSV 附件。
func (h *UserAdminHandler) Export(w http.ResponseWriter, r *http.Request) {
	var req userListRequest
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}

	items, truncated, err := h.svc.Export(r.Context(), useradmin.ListFilter{
		Q:      req.Q,
		Status: req.Status,
	})
	if err != nil {
		response.HandleError(w, r, err)
		return
	}

	filename := "users-" + time.Now().UTC().Format("20060102-150405") + ".csv"
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("X-Export-Truncated", strconv.FormatBool(truncated))
	w.WriteHeader(http.StatusOK)

	// 前置 UTF-8 BOM，保证 Excel 正确识别中文。
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"用户ID", "用户名", "状态", "注册时间", "最后登录", "收藏数", "最近使用数", "常用种子数"})
	for _, it := range items {
		last := ""
		if it.LastLoginAt != nil {
			last = it.LastLoginAt.UTC().Format(time.RFC3339)
		}
		_ = cw.Write([]string{
			csvSafe(it.UserNo),
			csvSafe(it.Username),
			statusLabel(it.IsActive),
			it.CreatedAt.UTC().Format(time.RFC3339),
			last,
			strconv.FormatInt(it.FavoriteCount, 10),
			strconv.FormatInt(it.RecentCount, 10),
			strconv.FormatInt(it.SeedCount, 10),
		})
	}
	cw.Flush()

	h.auditLog(r, "user.export", "", nil, map[string]any{
		"count":     len(items),
		"truncated": truncated,
		"q":         req.Q,
		"status":    req.Status,
	})
}

// invalidate 使指定用户的所有会话失效；失败仅记录，不阻断主流程。
func (h *UserAdminHandler) invalidate(r *http.Request, userID string) {
	if h.sessions == nil {
		return
	}
	if err := h.sessions.DeleteSessionsForAdmin(r.Context(), userID); err != nil {
		h.auditLog(r, "user.session_revoke_failed", userID, nil, map[string]any{"err": err.Error()})
	}
}

// auditLog 写审计日志。
func (h *UserAdminHandler) auditLog(r *http.Request, action, objID string, before, after map[string]any) {
	if h.auditSvc == nil {
		return
	}
	h.auditSvc.Write(r, audit.Entry{
		ActorAdminID: actorID(r),
		Action:       action,
		ObjectType:   "user",
		ObjectID:     objID,
		Before:       before,
		After:        after,
	})
}

// statusLabel 状态中文标签（用于 CSV 导出）。
func statusLabel(active bool) string {
	if active {
		return "启用"
	}
	return "禁用"
}

// csvSafe 防止 CSV 公式注入（CSV injection / formula injection）。
//
// 当单元格以 =、+、-、@ 或制表符 / 回车开头时，Excel / WPS 等会将其当作公式执行；
// 这里统一前置一个单引号使其被识别为文本。当前导出字段中只有用户名为用户可控，
// 但仍对全部字符串字段统一处理，避免后续新增字段时遗漏。
func csvSafe(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + s
	default:
		return s
	}
}

// writeUserAdminError 将服务层错误映射为 HTTP 响应。
func writeUserAdminError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, useradmin.ErrNotFound):
		response.NotFound(w, r)
	case errors.Is(err, useradmin.ErrUsernameTaken):
		response.Conflict(w, r, err.Error())
	case errors.Is(err, useradmin.ErrBadUsername), errors.Is(err, useradmin.ErrWeakPassword):
		response.Validation(w, r, []response.ErrorDetail{{Message: err.Error()}})
	default:
		response.HandleError(w, r, err)
	}
}
