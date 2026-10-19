// Package handler 提供 HTTP 处理器。
package handler

import (
	"encoding/json"
	"net/http"

	"mckit/backend/internal/http/response"
	"mckit/backend/internal/service/useradmin"
)

// UserAdminHandler 普通用户管理处理器（后台「普通用户管理」）。
// 仅 admin 角色可访问（路由层 RequireAuth + RequireRole(admin) + CSRF 限制）。
type UserAdminHandler struct {
	svc *useradmin.Service
}

// NewUserAdminHandler 创建处理器。
func NewUserAdminHandler(svc *useradmin.Service) *UserAdminHandler {
	return &UserAdminHandler{svc: svc}
}

// List POST /api/v1/admin/users/list
// 参数：q（用户名模糊）、status（active / disabled / 空）、page、page_size。
func (h *UserAdminHandler) List(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Q        string `json:"q"`
		Status   string `json:"status"`
		Page     int    `json:"page"`
		PageSize int    `json:"page_size"`
	}
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
