// Package handler 提供 HTTP 处理器。
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"mckit/backend/internal/audit"
	"mckit/backend/internal/domain"
	"mckit/backend/internal/http/response"
	"mckit/backend/internal/service/role"
)

// RoleHandler 角色管理处理器（后台「角色管理」）。
// 需要 roles.manage 权限点（内置 admin 具备，editor 不具备）。
type RoleHandler struct {
	svc      *role.Service
	auditSvc *audit.Service
}

// NewRoleHandler 创建角色处理器。
func NewRoleHandler(svc *role.Service, auditSvc *audit.Service) *RoleHandler {
	return &RoleHandler{svc: svc, auditSvc: auditSvc}
}

// List POST /api/v1/admin/roles/list
// 角色总量小（内置 3 + 自定义少量），一次性返回，不做分页。
func (h *RoleHandler) List(w http.ResponseWriter, r *http.Request) {
	roles, err := h.svc.List(r.Context())
	if err != nil {
		response.HandleError(w, r, err)
		return
	}
	response.OK(w, roles)
}

// Permissions POST /api/v1/admin/roles/permissions
// 返回权限点目录（分组 + 中文名），供前端按组勾选。
func (h *RoleHandler) Permissions(w http.ResponseWriter, r *http.Request) {
	response.OK(w, domain.PermissionGroups)
}

// Create POST /api/v1/admin/roles/create
func (h *RoleHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string   `json:"name"`
		Label       string   `json:"label"`
		Description string   `json:"description"`
		Permissions []string `json:"permissions"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}

	created, err := h.svc.Create(r.Context(), role.CreateInput{
		Name:        req.Name,
		Label:       req.Label,
		Description: req.Description,
		Permissions: req.Permissions,
	})
	if err != nil {
		writeRoleError(w, r, err)
		return
	}
	h.auditLog(r, "role.create", created.Name, nil, map[string]any{
		"label":       created.Label,
		"permissions": created.Permissions,
	})
	response.Created(w, created)
}

// Update POST /api/v1/admin/roles/update
// 白名单字段：label、description、permissions（name 为主键，不可改）。
func (h *RoleHandler) Update(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string    `json:"name"`
		Label       *string   `json:"label"`
		Description *string   `json:"description"`
		Permissions *[]string `json:"permissions"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		response.Validation(w, r, []response.ErrorDetail{{Field: "name", Message: "name 必填"}})
		return
	}

	updated, err := h.svc.Update(r.Context(), req.Name, role.UpdateInput{
		Label:       req.Label,
		Description: req.Description,
		Permissions: req.Permissions,
	})
	if err != nil {
		writeRoleError(w, r, err)
		return
	}
	h.auditLog(r, "role.update", updated.Name, nil, map[string]any{
		"label":       updated.Label,
		"permissions": updated.Permissions,
	})
	response.OK(w, updated)
}

// Delete POST /api/v1/admin/roles/delete
func (h *RoleHandler) Delete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		response.Validation(w, r, []response.ErrorDetail{{Field: "name", Message: "name 必填"}})
		return
	}

	if err := h.svc.Delete(r.Context(), req.Name); err != nil {
		writeRoleError(w, r, err)
		return
	}
	h.auditLog(r, "role.delete", req.Name, nil, nil)
	response.NoContent(w)
}

// auditLog 写审计日志。
func (h *RoleHandler) auditLog(r *http.Request, action, objID string, before, after map[string]any) {
	if h.auditSvc == nil {
		return
	}
	h.auditSvc.Write(r, audit.Entry{
		ActorAdminID: actorID(r),
		Action:       action,
		ObjectType:   "role",
		ObjectID:     objID,
		Before:       before,
		After:        after,
	})
}

// writeRoleError 将服务层错误映射为 HTTP 响应。
func writeRoleError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, role.ErrNotFound):
		response.NotFound(w, r)
	case errors.Is(err, role.ErrNameTaken), errors.Is(err, role.ErrRoleInUse):
		response.Conflict(w, r, err.Error())
	case errors.Is(err, role.ErrBadName),
		errors.Is(err, role.ErrBadLabel),
		errors.Is(err, role.ErrBadPermission),
		errors.Is(err, role.ErrSystemRole),
		errors.Is(err, role.ErrAdminMustKeepAll):
		response.Validation(w, r, []response.ErrorDetail{{Message: err.Error()}})
	default:
		response.HandleError(w, r, err)
	}
}
