// Package handler 提供 HTTP 处理器。
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"mckit/backend/internal/audit"
	"mckit/backend/internal/http/response"
	"mckit/backend/internal/service/revision"
)

// RevisionHandler 内容历史版本处理器（gj.md §15：差异查看与回滚）。
//
// 权限按内容类型拆分（见 main.go 路由装配），保证「能改哪类内容」与
// 「能看/回滚哪类内容历史」的最小权限边界一致。
type RevisionHandler struct {
	svc      *revision.Service
	auditSvc *audit.Service
}

// NewRevisionHandler 创建历史版本处理器。
func NewRevisionHandler(svc *revision.Service, auditSvc *audit.Service) *RevisionHandler {
	return &RevisionHandler{svc: svc, auditSvc: auditSvc}
}

// revisionListItem 历史版本列表项（不含整行快照，避免列表响应过大）。
type revisionListItem struct {
	ID            string    `json:"id"`
	ContentType   string    `json:"content_type"`
	ContentID     string    `json:"content_id"`
	RevisionNo    int       `json:"revision_no"`
	Action        string    `json:"action"`
	ChangeSummary []string  `json:"change_summary"`
	ActorName     string    `json:"actor_name"`
	Note          string    `json:"note"`
	CreatedAt     time.Time `json:"created_at"`
}

// revisionTargetRequest 历史版本定位请求体。
type revisionTargetRequest struct {
	ContentID  string `json:"content_id"`
	RevisionNo int    `json:"revision_no"`
	Note       string `json:"note"`
}

// List 返回某内容的历史版本列表（新→旧）。
func (h *RevisionHandler) List(contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ContentID string `json:"content_id"`
			Page      int    `json:"page"`
			PageSize  int    `json:"page_size"`
		}
		if r.Body != nil {
			_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
		}
		if req.ContentID == "" {
			response.Validation(w, r, []response.ErrorDetail{{Field: "content_id", Message: "content_id 必填"}})
			return
		}
		items, total, err := h.svc.List(r.Context(), contentType, req.ContentID, req.Page, req.PageSize)
		if err != nil {
			writeRevisionError(w, r, err)
			return
		}
		out := make([]revisionListItem, 0, len(items))
		for _, it := range items {
			out = append(out, revisionListItem{
				ID:            it.ID.String(),
				ContentType:   it.ContentType,
				ContentID:     it.ContentID.String(),
				RevisionNo:    it.RevisionNo,
				Action:        it.Action,
				ChangeSummary: it.ChangeSummary,
				ActorName:     it.ActorName,
				Note:          it.Note,
				CreatedAt:     it.CreatedAt,
			})
		}
		page, pageSize := normalizePage(req.Page, req.PageSize)
		response.Page(w, r, out, response.ResourceList{
			Total:      total,
			Page:       page,
			PageSize:   pageSize,
			TotalPages: int((total + int64(pageSize) - 1) / int64(pageSize)),
		})
	}
}

// Diff 返回某版本相对上一版的差异（含该版本完整快照）。
func (h *RevisionHandler) Diff(contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req, ok := decodeRevisionTarget(w, r)
		if !ok {
			return
		}
		diff, err := h.svc.Diff(r.Context(), contentType, req.ContentID, req.RevisionNo)
		if err != nil {
			writeRevisionError(w, r, err)
			return
		}
		response.OK(w, diff)
	}
}

// Rollback 把内容回滚到指定历史版本（追加一条 action=rollback 的新版本）。
func (h *RevisionHandler) Rollback(contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req, ok := decodeRevisionTarget(w, r)
		if !ok {
			return
		}
		rec, err := h.svc.Rollback(r.Context(), contentType, req.ContentID, req.RevisionNo, actorID(r), actorName(r), req.Note)
		if err != nil {
			writeRevisionError(w, r, err)
			return
		}
		if h.auditSvc != nil {
			h.auditSvc.Write(r, audit.Entry{
				ActorAdminID: actorID(r),
				Action:       "content.rollback",
				ObjectType:   contentType,
				ObjectID:     req.ContentID,
				After: map[string]any{
					"target_revision_no": req.RevisionNo,
					"new_revision_no":    rec.RevisionNo,
					"note":               req.Note,
				},
			})
		}
		response.OK(w, rec)
	}
}

func decodeRevisionTarget(w http.ResponseWriter, r *http.Request) (revisionTargetRequest, bool) {
	var req revisionTargetRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return req, false
	}
	if req.ContentID == "" || req.RevisionNo <= 0 {
		response.Validation(w, r, []response.ErrorDetail{{Message: "content_id 与 revision_no 必填且 revision_no 需为正整数"}})
		return req, false
	}
	return req, true
}

func writeRevisionError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, revision.ErrNotFound):
		response.NotFound(w, r)
	case errors.Is(err, revision.ErrBadTarget):
		response.Validation(w, r, []response.ErrorDetail{{Message: "内容类型非法"}})
	case errors.Is(err, revision.ErrUnsupported):
		response.Validation(w, r, []response.ErrorDetail{{Message: "该内容类型暂不支持回滚"}})
	case err == nil:
		response.HandleError(w, r, err)
	default:
		response.Validation(w, r, []response.ErrorDetail{{Message: err.Error()}})
	}
}
