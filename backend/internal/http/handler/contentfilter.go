package handler

import (
	"errors"
	"net/http"
	"strings"

	"mckit/backend/internal/audit"
	"mckit/backend/internal/http/response"
	"mckit/backend/internal/service/contentfilter"
)

// ContentFilterHandler 内容关键字屏蔽管理处理器。
//
// 仅后台 ops.dataquality 权限可访问；所有变更写审计日志。
type ContentFilterHandler struct {
	svc   *contentfilter.Service
	audit *audit.Service
}

// NewContentFilterHandler 创建处理器。
func NewContentFilterHandler(svc *contentfilter.Service) *ContentFilterHandler {
	return &ContentFilterHandler{svc: svc}
}

// WithAudit 注入审计服务。
func (h *ContentFilterHandler) WithAudit(svc *audit.Service) *ContentFilterHandler {
	h.audit = svc
	return h
}

type contentKeywordCreateRequest struct {
	Keyword string `json:"keyword"`
	Note    string `json:"note"`
}

type contentKeywordUpdateRequest struct {
	ID      string `json:"id"`
	Keyword string `json:"keyword"`
	Note    string `json:"note"`
	Enabled bool   `json:"enabled"`
}

type contentKeywordDeleteRequest struct {
	ID string `json:"id"`
}

// List POST /api/v1/admin/content-health/keywords/list
func (h *ContentFilterHandler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.List(r.Context())
	if err != nil {
		writeContentFilterError(w, r, err)
		return
	}
	response.OK(w, map[string]any{"items": items, "total": len(items)})
}

// Create POST /api/v1/admin/content-health/keywords/create
func (h *ContentFilterHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req contentKeywordCreateRequest
	if !decodeBody(w, r, &req) {
		return
	}
	row, err := h.svc.Create(r.Context(), req.Keyword, req.Note, actorID(r))
	if err != nil {
		writeContentFilterError(w, r, err)
		return
	}
	h.auditLog(r, "content.keyword.create", row.ID.String(), map[string]any{
		"keyword": row.Keyword,
		"note":    row.Note,
	})
	response.Created(w, row)
}

// Update POST /api/v1/admin/content-health/keywords/update
func (h *ContentFilterHandler) Update(w http.ResponseWriter, r *http.Request) {
	var req contentKeywordUpdateRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.ID) == "" {
		response.Validation(w, r, []response.ErrorDetail{{Field: "id", Message: "id 必填"}})
		return
	}
	row, err := h.svc.Update(r.Context(), req.ID, req.Keyword, req.Note, req.Enabled)
	if err != nil {
		writeContentFilterError(w, r, err)
		return
	}
	h.auditLog(r, "content.keyword.update", row.ID.String(), map[string]any{
		"keyword": row.Keyword,
		"note":    row.Note,
		"enabled": row.Enabled,
	})
	response.OK(w, row)
}

// Delete POST /api/v1/admin/content-health/keywords/delete
func (h *ContentFilterHandler) Delete(w http.ResponseWriter, r *http.Request) {
	var req contentKeywordDeleteRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if err := h.svc.Delete(r.Context(), req.ID); err != nil {
		writeContentFilterError(w, r, err)
		return
	}
	h.auditLog(r, "content.keyword.delete", req.ID, nil)
	response.NoContent(w)
}

func (h *ContentFilterHandler) auditLog(r *http.Request, action, objectID string, after map[string]any) {
	if h.audit == nil {
		return
	}
	h.audit.Write(r, audit.Entry{
		ActorAdminID: actorID(r),
		Action:       action,
		ObjectType:   "content_keyword",
		ObjectID:     objectID,
		After:        after,
	})
}

func writeContentFilterError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, contentfilter.ErrNotFound):
		response.NotFound(w, r)
	case errors.Is(err, contentfilter.ErrBadInput),
		errors.Is(err, contentfilter.ErrBadNote),
		errors.Is(err, contentfilter.ErrDuplicate):
		response.Validation(w, r, []response.ErrorDetail{{Message: err.Error()}})
	default:
		response.HandleError(w, r, err)
	}
}
