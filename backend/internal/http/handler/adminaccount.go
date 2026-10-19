// Package handler 提供 HTTP 处理器。
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"mckit/backend/internal/audit"
	"mckit/backend/internal/http/response"
	"mckit/backend/internal/service/adminaccount"
)

// adminSessionInvalidator 提供使管理员会话失效的能力（由 auth.Service 实现）。
// 停用、改角色、重置密码、删除账号后必须让其已登录会话立即失效。
type adminSessionInvalidator interface {
	DeleteSessionsForAdmin(ctx context.Context, adminID string) error
}

// AdminAccountHandler 管理员账号管理处理器（需求 §6.1 / §6.2）。
// 仅 admin 角色可访问（路由层 RequireRole 限制）。
type AdminAccountHandler struct {
	svc      *adminaccount.Service
	sessions adminSessionInvalidator
	auditSvc *audit.Service
}

// NewAdminAccountHandler 创建账号管理处理器。
func NewAdminAccountHandler(svc *adminaccount.Service, sessions adminSessionInvalidator, auditSvc *audit.Service) *AdminAccountHandler {
	return &AdminAccountHandler{svc: svc, sessions: sessions, auditSvc: auditSvc}
}

// List POST /api/v1/admin/admins/list
func (h *AdminAccountHandler) List(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Q        string `json:"q"`
		Page     int    `json:"page"`
		PageSize int    `json:"page_size"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}
	items, total, err := h.svc.List(r.Context(), req.Q, req.Page, req.PageSize)
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

// Create POST /api/v1/admin/admins/create
func (h *AdminAccountHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	admin, err := h.svc.Create(r.Context(), adminaccount.CreateInput{
		Username: req.Username,
		Password: req.Password,
		Role:     req.Role,
	})
	if err != nil {
		writeAdminAccountError(w, r, err)
		return
	}
	// 审计只记录用户名与角色，绝不记录密码。
	h.auditLog(r, "admin.create", admin.ID.String(), nil, map[string]any{
		"username": admin.Username,
		"role":     admin.Role,
	})
	response.Created(w, admin)
}

// Update POST /api/v1/admin/admins/update
// 白名单字段：role、is_active。停用或降级自己会被拒绝，避免自锁。
func (h *AdminAccountHandler) Update(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID       string  `json:"id"`
		Role     *string `json:"role"`
		IsActive *bool   `json:"is_active"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	if strings.TrimSpace(req.ID) == "" {
		response.Validation(w, r, []response.ErrorDetail{{Field: "id", Message: "id 必填"}})
		return
	}
	self := req.ID == actorID(r)
	if self && req.IsActive != nil && !*req.IsActive {
		response.Validation(w, r, []response.ErrorDetail{{Field: "is_active", Message: "不能停用当前登录账号"}})
		return
	}

	admin, err := h.svc.Update(r.Context(), req.ID, adminaccount.UpdateInput{
		Role:     req.Role,
		IsActive: req.IsActive,
	})
	if err != nil {
		writeAdminAccountError(w, r, err)
		return
	}
	// 角色或状态变更后，立即使其现有会话失效，避免旧角色会话继续生效。
	h.invalidate(r, req.ID)
	h.auditLog(r, "admin.update", req.ID, nil, map[string]any{
		"username":  admin.Username,
		"role":      admin.Role,
		"is_active": admin.IsActive,
	})
	response.OK(w, admin)
}

// ResetPassword POST /api/v1/admin/admins/reset-password
func (h *AdminAccountHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
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
		writeAdminAccountError(w, r, err)
		return
	}
	h.invalidate(r, req.ID)
	h.auditLog(r, "admin.reset_password", req.ID, nil, nil)
	response.NoContent(w)
}

// Delete POST /api/v1/admin/admins/delete
func (h *AdminAccountHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := decodeID(w, r)
	if !ok {
		return
	}
	if id == actorID(r) {
		response.Validation(w, r, []response.ErrorDetail{{Field: "id", Message: "不能删除当前登录账号"}})
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		writeAdminAccountError(w, r, err)
		return
	}
	h.invalidate(r, id)
	h.auditLog(r, "admin.delete", id, nil, nil)
	response.NoContent(w)
}

// invalidate 使指定管理员的所有会话失效；失败仅记录，不阻断主流程。
func (h *AdminAccountHandler) invalidate(r *http.Request, adminID string) {
	if h.sessions == nil {
		return
	}
	if err := h.sessions.DeleteSessionsForAdmin(r.Context(), adminID); err != nil {
		h.auditLog(r, "admin.session_revoke_failed", adminID, nil, map[string]any{"err": err.Error()})
	}
}

// auditLog 写审计日志。
func (h *AdminAccountHandler) auditLog(r *http.Request, action, objID string, before, after map[string]any) {
	if h.auditSvc == nil {
		return
	}
	h.auditSvc.Write(r, audit.Entry{
		ActorAdminID: actorID(r),
		Action:       action,
		ObjectType:   "admin",
		ObjectID:     objID,
		Before:       before,
		After:        after,
	})
}

// writeAdminAccountError 将服务层错误映射为 HTTP 响应。
func writeAdminAccountError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, adminaccount.ErrNotFound):
		response.NotFound(w, r)
	case errors.Is(err, adminaccount.ErrUsernameTaken):
		response.Conflict(w, r, err.Error())
	case errors.Is(err, adminaccount.ErrBadRole),
		errors.Is(err, adminaccount.ErrReservedRole),
		errors.Is(err, adminaccount.ErrBadUsername),
		errors.Is(err, adminaccount.ErrWeakPassword),
		errors.Is(err, adminaccount.ErrLastAdmin):
		response.Validation(w, r, []response.ErrorDetail{{Message: err.Error()}})
	default:
		response.HandleError(w, r, err)
	}
}
