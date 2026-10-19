package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"mckit/backend/internal/audit"
	"mckit/backend/internal/domain"
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

// redstoneBatchReviewRequest 批量审核红石设计请求。
type redstoneBatchReviewRequest struct {
	IDs          []string `json:"ids"`
	ReviewStatus string   `json:"review_status"`
	Note         string   `json:"note"`
}

// BatchReviewDesigns POST /api/v1/admin/redstone/designs/batch-review
//
// 批量审核红石设计：逐条独立事务，返回逐条结果（部分成功是正常情况）。
func (h *RedstoneAdminHandler) BatchReviewDesigns(w http.ResponseWriter, r *http.Request) {
	var req redstoneBatchReviewRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	if len(req.IDs) == 0 {
		response.Validation(w, r, []response.ErrorDetail{{Field: "ids", Message: "请至少选择一条设计"}})
		return
	}
	if len(req.IDs) > domain.MaxBatchReview {
		response.Validation(w, r, []response.ErrorDetail{{
			Field:   "ids",
			Message: fmt.Sprintf("单次最多 %d 条，请分批提交", domain.MaxBatchReview),
		}})
		return
	}
	for _, id := range req.IDs {
		if _, err := uuid.Parse(strings.TrimSpace(id)); err != nil {
			response.Validation(w, r, []response.ErrorDetail{{Field: "ids", Message: "存在非法的设计 ID"}})
			return
		}
	}

	reviewer := actorName(r)
	result, err := h.catalog.BatchReviewRedstoneDesigns(r.Context(), req.IDs, req.ReviewStatus, reviewer, req.Note)
	if err != nil {
		switch {
		case errors.Is(err, catalog.ErrNoIDs), errors.Is(err, catalog.ErrTooManyIDs):
			response.Validation(w, r, []response.ErrorDetail{{Field: "ids", Message: err.Error()}})
		case errors.Is(err, catalog.ErrInvalidReviewStatus):
			response.BadRequest(w, r, "无效的审核状态（pending / published / rejected）")
		default:
			response.HandleError(w, r, err)
		}
		return
	}

	if h.auditSvc != nil {
		for _, item := range result.Items {
			if !item.OK {
				continue
			}
			h.auditSvc.Write(r, audit.Entry{
				ActorAdminID: actorID(r),
				Action:       "redstone_design.review",
				ObjectType:   "redstone_design",
				ObjectID:     item.ID,
				After: map[string]any{
					"review_status": req.ReviewStatus,
					"reviewer":      reviewer,
					"note":          req.Note,
					"batch":         true,
				},
			})
		}
	}
	response.OK(w, result)
}
