package handler

import (
	"encoding/json"
	"net/http"

	"mckit/backend/internal/audit"
	"mckit/backend/internal/http/response"
)

// AuditHandler 审计日志查询处理器（需求 §8.4）。
type AuditHandler struct {
	svc *audit.Service
}

// NewAuditHandler 创建审计处理器。
func NewAuditHandler(svc *audit.Service) *AuditHandler {
	return &AuditHandler{svc: svc}
}

// listRequest 审计日志筛选请求体（全 POST 决策：参数放 body）。
type listRequest struct {
	ActorID  string `json:"actor_id"`
	Action   string `json:"action"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
}

// List POST /api/v1/admin/audit-logs
func (h *AuditHandler) List(w http.ResponseWriter, r *http.Request) {
	var req listRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}

	items, total, err := h.svc.List(audit.ListFilter{
		ActorID:  req.ActorID,
		Action:   req.Action,
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
		TotalPages: totalPages(total, pageSize),
	})
}

func normalizePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func totalPages(total int64, pageSize int) int {
	if pageSize <= 0 {
		return 0
	}
	pages := int((total + int64(pageSize) - 1) / int64(pageSize))
	if pages < 1 {
		return 1
	}
	return pages
}