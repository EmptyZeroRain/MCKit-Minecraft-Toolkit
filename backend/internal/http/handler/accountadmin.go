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
	"mckit/backend/internal/domain"
	"mckit/backend/internal/http/middleware"
	"mckit/backend/internal/http/response"
	"mckit/backend/internal/service/accountadmin"
)

// AccountAdminHandler 统一账号管理处理器（后台「账号管理」单列表）。
//
// 路由层只要求 accounts.admins 或 accounts.users 之一；本处理器按
// 「目标账号角色」做二次权限判定：
//   - 普通用户（role = 'user'）→ accounts.users；
//   - 后台账号（role <> 'user'）→ accounts.admins；
//   - 分配后台角色、解绑 MFA → 一律要求 accounts.admins。
//
// 这样仅持有 accounts.users 的运营只能看到并管理普通用户，无法通过对象 ID
// 越过账号类型边界。
type AccountAdminHandler struct {
	svc      *accountadmin.Service
	sessions adminSessionInvalidator
	auditSvc *audit.Service
	perms    middleware.PermissionResolver
}

// NewAccountAdminHandler 创建统一账号管理处理器。
func NewAccountAdminHandler(svc *accountadmin.Service, sessions adminSessionInvalidator, auditSvc *audit.Service) *AccountAdminHandler {
	return &AccountAdminHandler{svc: svc, sessions: sessions, auditSvc: auditSvc}
}

// WithPermissions 注入权限解析器（处理器按目标账号做二次判定）。
func (h *AccountAdminHandler) WithPermissions(resolver middleware.PermissionResolver) *AccountAdminHandler {
	h.perms = resolver
	return h
}

// accountListRequest 列表 / 导出共用入参。
type accountListRequest struct {
	Q        string `json:"q"`
	Status   string `json:"status"`
	Scope    string `json:"scope"` // "" 全部 / admin 后台账号 / user 普通用户
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
}

// List POST /api/v1/admin/accounts/list
func (h *AccountAdminHandler) List(w http.ResponseWriter, r *http.Request) {
	var req accountListRequest
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}
	scope, ok := h.effectiveScope(w, r, req.Scope)
	if !ok {
		return
	}
	items, total, err := h.svc.List(r.Context(), accountadmin.ListFilter{
		Q:        req.Q,
		Status:   req.Status,
		Scope:    scope,
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

// Create POST /api/v1/admin/accounts/create
// 参数：username、password、role（省略时按普通用户创建）。
func (h *AccountAdminHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	role := strings.TrimSpace(req.Role)
	if role == "" {
		role = domain.RoleUser
	}
	if !h.requireTargetPermission(w, r, role) {
		return
	}

	item, err := h.svc.Create(r.Context(), accountadmin.CreateInput{
		Username: req.Username,
		Password: req.Password,
		Role:     role,
	})
	if err != nil {
		writeAccountAdminError(w, r, err)
		return
	}
	h.auditLog(r, "account.create", item.ID, nil, map[string]any{
		"username": item.Username,
		"role":     item.Role,
	})
	response.Created(w, item)
}

// Detail POST /api/v1/admin/accounts/detail
func (h *AccountAdminHandler) Detail(w http.ResponseWriter, r *http.Request) {
	acc, ok := h.loadTarget(w, r)
	if !ok {
		return
	}
	detail, err := h.svc.Detail(r.Context(), acc.ID.String())
	if err != nil {
		writeAccountAdminError(w, r, err)
		return
	}
	response.OK(w, detail)
}

// Update POST /api/v1/admin/accounts/update
// 白名单字段：role、is_active。改角色 / 停用后该账号会话立即失效。
func (h *AccountAdminHandler) Update(w http.ResponseWriter, r *http.Request) {
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

	acc, ok := h.loadTargetByID(w, r, req.ID)
	if !ok {
		return
	}

	newRole := ""
	if req.Role != nil {
		newRole = strings.TrimSpace(*req.Role)
	}
	// 目标账号是后台账号，或要把账号改成后台角色 → 必须持有 accounts.admins。
	needsAdminPerm := acc.Role != domain.RoleUser || (newRole != "" && newRole != domain.RoleUser)
	need := domain.PermAccountsUsers
	if needsAdminPerm {
		need = domain.PermAccountsAdmins
	}
	if !h.hasPermission(r, need) {
		response.Forbidden(w, r)
		return
	}

	if req.ID == actorID(r) && req.IsActive != nil && !*req.IsActive {
		response.Validation(w, r, []response.ErrorDetail{{Field: "is_active", Message: "不能停用当前登录账号"}})
		return
	}

	item, err := h.svc.Update(r.Context(), req.ID, accountadmin.UpdateInput{
		Role:     req.Role,
		IsActive: req.IsActive,
	})
	if err != nil {
		writeAccountAdminError(w, r, err)
		return
	}
	h.invalidate(r, req.ID)
	h.auditLog(r, "account.update", req.ID, nil, map[string]any{
		"username":  item.Username,
		"role":      item.Role,
		"is_active": item.IsActive,
	})
	response.OK(w, item)
}

// ResetPassword POST /api/v1/admin/accounts/reset-password
func (h *AccountAdminHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
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
	if _, ok := h.loadTargetByID(w, r, req.ID); !ok {
		return
	}
	if err := h.svc.ResetPassword(r.Context(), req.ID, req.NewPassword); err != nil {
		writeAccountAdminError(w, r, err)
		return
	}
	h.invalidate(r, req.ID)
	h.auditLog(r, "account.reset_password", req.ID, nil, nil)
	response.NoContent(w)
}

// Delete POST /api/v1/admin/accounts/delete
func (h *AccountAdminHandler) Delete(w http.ResponseWriter, r *http.Request) {
	acc, ok := h.loadTarget(w, r)
	if !ok {
		return
	}
	if acc.ID.String() == actorID(r) {
		response.Validation(w, r, []response.ErrorDetail{{Field: "id", Message: "不能删除当前登录账号"}})
		return
	}
	if err := h.svc.Delete(r.Context(), acc.ID.String()); err != nil {
		writeAccountAdminError(w, r, err)
		return
	}
	h.invalidate(r, acc.ID.String())
	h.auditLog(r, "account.delete", acc.ID.String(), nil, nil)
	response.NoContent(w)
}

// DeletePreview POST /api/v1/admin/accounts/delete-preview
func (h *AccountAdminHandler) DeletePreview(w http.ResponseWriter, r *http.Request) {
	acc, ok := h.loadTarget(w, r)
	if !ok {
		return
	}
	preview, err := h.svc.DeletePreview(r.Context(), acc.ID.String(), actorID(r))
	if err != nil {
		writeAccountAdminError(w, r, err)
		return
	}
	response.OK(w, preview)
}

// ResetMFA POST /api/v1/admin/accounts/mfa/reset
// 解绑双因子认证属于高权限操作，无论目标账号角色都要求 accounts.admins。
func (h *AccountAdminHandler) ResetMFA(w http.ResponseWriter, r *http.Request) {
	if !h.hasPermission(r, domain.PermAccountsAdmins) {
		response.Forbidden(w, r)
		return
	}
	acc, ok := h.loadTarget(w, r)
	if !ok {
		return
	}
	if err := h.svc.ResetMFA(r.Context(), acc.ID.String()); err != nil {
		writeAccountAdminError(w, r, err)
		return
	}
	h.auditLog(r, "account.mfa.reset", acc.ID.String(), nil, nil)
	response.NoContent(w)
}

// Export POST /api/v1/admin/accounts/export
// 参数与列表一致（q / status / scope），但不分页；返回 UTF-8 BOM + CSV。
func (h *AccountAdminHandler) Export(w http.ResponseWriter, r *http.Request) {
	var req accountListRequest
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}
	scope, ok := h.effectiveScope(w, r, req.Scope)
	if !ok {
		return
	}
	items, truncated, err := h.svc.Export(r.Context(), accountadmin.ListFilter{
		Q:      req.Q,
		Status: req.Status,
		Scope:  scope,
	})
	if err != nil {
		response.HandleError(w, r, err)
		return
	}

	filename := "accounts-" + time.Now().UTC().Format("20060102-150405") + ".csv"
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("X-Export-Truncated", strconv.FormatBool(truncated))
	w.WriteHeader(http.StatusOK)

	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"用户ID", "用户名", "角色", "状态", "注册时间", "最后登录", "邮箱", "礼物币", "等级"})
	for _, it := range items {
		last := ""
		if it.LastLoginAt != nil {
			last = it.LastLoginAt.UTC().Format(time.RFC3339)
		}
		_ = cw.Write([]string{
			csvSafe(it.UserNo),
			csvSafe(it.Username),
			csvSafe(it.Role),
			statusLabel(it.IsActive),
			it.CreatedAt.UTC().Format(time.RFC3339),
			last,
			csvSafe(it.Email),
			strconv.FormatInt(it.GiftCoins, 10),
			strconv.Itoa(it.Level),
		})
	}
	cw.Flush()

	h.auditLog(r, "account.export", "", nil, map[string]any{
		"count":     len(items),
		"truncated": truncated,
		"q":         req.Q,
		"status":    req.Status,
		"scope":     scope,
	})
}

// hasPermission 判断当前会话是否具备指定权限点。
func (h *AccountAdminHandler) hasPermission(r *http.Request, perm string) bool {
	session := middleware.SessionFrom(r.Context())
	if session == nil || h.perms == nil {
		return false
	}
	perms, err := h.perms.PermissionsForRole(r.Context(), session.Role)
	if err != nil {
		return false
	}
	for _, p := range perms {
		if p == perm {
			return true
		}
	}
	return false
}

// requireTargetPermission 按目标账号角色校验权限点。
func (h *AccountAdminHandler) requireTargetPermission(w http.ResponseWriter, r *http.Request, role string) bool {
	need := domain.PermAccountsUsers
	if strings.TrimSpace(role) != domain.RoleUser {
		need = domain.PermAccountsAdmins
	}
	if !h.hasPermission(r, need) {
		response.Forbidden(w, r)
		return false
	}
	return true
}

// effectiveScope 把请求范围收敛到调用方有权查看的范围。
// 只有单项权限时自动只看对应类型；显式请求无权限的范围返回 403。
func (h *AccountAdminHandler) effectiveScope(w http.ResponseWriter, r *http.Request, requested string) (string, bool) {
	canAdmins := h.hasPermission(r, domain.PermAccountsAdmins)
	canUsers := h.hasPermission(r, domain.PermAccountsUsers)
	switch strings.TrimSpace(requested) {
	case accountadmin.ScopeAdmin:
		if !canAdmins {
			response.Forbidden(w, r)
			return "", false
		}
		return accountadmin.ScopeAdmin, true
	case accountadmin.ScopeUser:
		if !canUsers {
			response.Forbidden(w, r)
			return "", false
		}
		return accountadmin.ScopeUser, true
	default:
		switch {
		case canAdmins && canUsers:
			return accountadmin.ScopeAll, true
		case canAdmins:
			return accountadmin.ScopeAdmin, true
		case canUsers:
			return accountadmin.ScopeUser, true
		default:
			response.Forbidden(w, r)
			return "", false
		}
	}
}

// loadTarget 从请求体读取 id，加载目标账号并做权限判定。
func (h *AccountAdminHandler) loadTarget(w http.ResponseWriter, r *http.Request) (*domain.Account, bool) {
	id, ok := decodeID(w, r)
	if !ok {
		return nil, false
	}
	return h.loadTargetByID(w, r, id)
}

// loadTargetByID 加载目标账号、校验存在性与目标权限。
func (h *AccountAdminHandler) loadTargetByID(w http.ResponseWriter, r *http.Request, id string) (*domain.Account, bool) {
	acc, err := h.svc.Get(r.Context(), id)
	if err != nil {
		response.HandleError(w, r, err)
		return nil, false
	}
	if acc == nil {
		response.NotFound(w, r)
		return nil, false
	}
	if !h.requireTargetPermission(w, r, acc.Role) {
		return nil, false
	}
	return acc, true
}

// invalidate 使指定账号所有会话失效；失败仅记录，不阻断主流程。
func (h *AccountAdminHandler) invalidate(r *http.Request, accountID string) {
	if h.sessions == nil {
		return
	}
	if err := h.sessions.DeleteSessionsForAdmin(r.Context(), accountID); err != nil {
		h.auditLog(r, "account.session_revoke_failed", accountID, nil, map[string]any{"err": err.Error()})
	}
}

// auditLog 写审计日志（对象类型统一为 account）。
func (h *AccountAdminHandler) auditLog(r *http.Request, action, objID string, before, after map[string]any) {
	if h.auditSvc == nil {
		return
	}
	h.auditSvc.Write(r, audit.Entry{
		ActorAdminID: actorID(r),
		Action:       action,
		ObjectType:   "account",
		ObjectID:     objID,
		Before:       before,
		After:        after,
	})
}

// writeAccountAdminError 将服务层错误映射为 HTTP 响应。
func writeAccountAdminError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, accountadmin.ErrNotFound):
		response.NotFound(w, r)
	case errors.Is(err, accountadmin.ErrUsernameTaken):
		response.Conflict(w, r, err.Error())
	case errors.Is(err, accountadmin.ErrBadRole),
		errors.Is(err, accountadmin.ErrBadUsername),
		errors.Is(err, accountadmin.ErrBadUserUsername),
		errors.Is(err, accountadmin.ErrWeakPassword),
		errors.Is(err, accountadmin.ErrLastAdmin):
		response.Validation(w, r, []response.ErrorDetail{{Message: err.Error()}})
	default:
		response.HandleError(w, r, err)
	}
}
