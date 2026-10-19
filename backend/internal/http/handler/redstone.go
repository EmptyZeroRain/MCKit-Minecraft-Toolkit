package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"mckit/backend/internal/audit"
	"mckit/backend/internal/http/response"
	"mckit/backend/internal/service/catalog"
)

// RedstoneAdminHandler 红石机器设计的审核处理器（TODO §15，admin 专属）。
// 内容审核流程：设计默认 pending，仅 published 对公开接口可见；
// 审核人取自登录会话（middleware.SessionFrom），并写入审计日志。
type RedstoneAdminHandler struct {
	catalog  *catalog.Service
	auditSvc *audit.Service
}

// NewRedstoneAdminHandler 创建审核处理器。auditSvc 可为 nil（不写审计）。
func NewRedstoneAdminHandler(svc *catalog.Service, auditSvc *audit.Service) *RedstoneAdminHandler {
	return &RedstoneAdminHandler{catalog: svc, auditSvc: auditSvc}
}

type redstoneDesignListRequest struct {
	Q            string `json:"q"`
	ReviewStatus string `json:"review_status"`
	Purpose      string `json:"purpose"`
	Page         int    `json:"page"`
	PageSize     int    `json:"page_size"`
}

// ListDesigns POST /api/v1/admin/redstone/designs/list
// 按审核状态筛选机器设计（不过滤发布态，供审核使用）。
func (h *RedstoneAdminHandler) ListDesigns(w http.ResponseWriter, r *http.Request) {
	var req redstoneDesignListRequest
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}
	items, total, err := h.catalog.ListRedstoneDesignsForAdmin(r.Context(), catalog.RedstoneDesignAdminFilter{
		Query:        req.Q,
		ReviewStatus: req.ReviewStatus,
		Purpose:      req.Purpose,
		Page:         req.Page,
		PageSize:     req.PageSize,
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

type redstoneDesignReviewRequest struct {
	ID           string `json:"id"`
	ReviewStatus string `json:"review_status"`
	Note         string `json:"note"`
}

// ReviewDesign POST /api/v1/admin/redstone/designs/review
// 审核机器设计：更新审核状态并留痕（审核人取自登录会话）。
func (h *RedstoneAdminHandler) ReviewDesign(w http.ResponseWriter, r *http.Request) {
	var req redstoneDesignReviewRequest
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}
	if req.ID == "" {
		response.BadRequest(w, r, "缺少设计 ID")
		return
	}

	reviewer := actorName(r)
	design, err := h.catalog.ReviewRedstoneDesign(r.Context(), req.ID, req.ReviewStatus, reviewer, req.Note)
	if err != nil {
		switch {
		case errors.Is(err, catalog.ErrNotFound):
			response.NotFound(w, r)
		case errors.Is(err, catalog.ErrInvalidReviewStatus):
			response.BadRequest(w, r, "无效的审核状态（pending / published / rejected）")
		default:
			response.HandleError(w, r, err)
		}
		return
	}

	if h.auditSvc != nil {
		h.auditSvc.Write(r, audit.Entry{
			ActorAdminID: actorID(r),
			Action:       "redstone_design.review",
			ObjectType:   "redstone_design",
			ObjectID:     design.ID.String(),
			After: map[string]any{
				"review_status": design.ReviewStatus,
				"reviewer":      design.Reviewer,
				"note":          design.ReviewNote,
			},
		})
	}
	response.OK(w, design)
}
