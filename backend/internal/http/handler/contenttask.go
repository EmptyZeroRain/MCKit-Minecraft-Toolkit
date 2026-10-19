package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"mckit/backend/internal/audit"
	"mckit/backend/internal/http/response"
	"mckit/backend/internal/service/contenttask"
)

// ContentTaskHandler 内容建设任务处理器（gj.md §8.3 搜索词闭环）。
//
// 全部接口要求 ops.contenttasks 权限；任务只涉及聚合关键词与账号指派，
// 不含任何用户标识或敏感数据。
type ContentTaskHandler struct {
	svc      *contenttask.Service
	auditSvc *audit.Service
}

// NewContentTaskHandler 创建处理器。
func NewContentTaskHandler(svc *contenttask.Service, auditSvc *audit.Service) *ContentTaskHandler {
	return &ContentTaskHandler{svc: svc, auditSvc: auditSvc}
}

// List POST /api/v1/admin/content-tasks/list
// 入参：status（open / in_progress / done / ignored）、q（关键词模糊）、分页。
func (h *ContentTaskHandler) List(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Status   string `json:"status"`
		Q        string `json:"q"`
		Page     int    `json:"page"`
		PageSize int    `json:"page_size"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}

	items, total, err := h.svc.List(r.Context(), contenttask.ListFilter{
		Status:   req.Status,
		Q:        req.Q,
		Page:     req.Page,
		PageSize: req.PageSize,
	})
	if err != nil {
		writeContentTaskError(w, r, err)
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

// Candidates POST /api/v1/admin/content-tasks/candidates
// 返回达到阈值且尚无未完结任务的无结果搜索词。
func (h *ContentTaskHandler) Candidates(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Limit int `json:"limit"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}
	items, err := h.svc.Candidates(r.Context(), req.Limit)
	if err != nil {
		writeContentTaskError(w, r, err)
		return
	}
	response.OK(w, items)
}

// Create POST /api/v1/admin/content-tasks/create
func (h *ContentTaskHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Keyword    string `json:"keyword"`
		Note       string `json:"note"`
		AssigneeID string `json:"assignee_id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}

	task, err := h.svc.Create(r.Context(), &contenttask.CreateInput{
		Keyword:    req.Keyword,
		Note:       req.Note,
		AssigneeID: req.AssigneeID,
		CreatedBy:  actorID(r),
	})
	if err != nil {
		writeContentTaskError(w, r, err)
		return
	}
	h.writeAudit(r, "content_task.create", task.ID.String(), map[string]any{
		"keyword": task.Keyword,
		"status":  task.Status,
	})
	response.Created(w, task)
}

// Update POST /api/v1/admin/content-tasks/update
// 仅传入的字段生效；assignee_id / related_content_id 传空串表示清空。
func (h *ContentTaskHandler) Update(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID                 string  `json:"id"`
		Status             *string `json:"status"`
		AssigneeID         *string `json:"assignee_id"`
		RelatedContentType *string `json:"related_content_type"`
		RelatedContentID   *string `json:"related_content_id"`
		Note               *string `json:"note"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	if req.ID == "" {
		response.Validation(w, r, []response.ErrorDetail{{Field: "id", Message: "id 必填"}})
		return
	}

	task, err := h.svc.Update(r.Context(), req.ID, &contenttask.UpdateInput{
		Status:             req.Status,
		AssigneeID:         req.AssigneeID,
		RelatedContentType: req.RelatedContentType,
		RelatedContentID:   req.RelatedContentID,
		Note:               req.Note,
	})
	if err != nil {
		writeContentTaskError(w, r, err)
		return
	}
	h.writeAudit(r, "content_task.update", task.ID.String(), map[string]any{
		"keyword": task.Keyword,
		"status":  task.Status,
	})
	response.OK(w, task)
}

// Recheck POST /api/v1/admin/content-tasks/recheck
// 重新检测关键词的搜索结果；不自动改变任务状态。
func (h *ContentTaskHandler) Recheck(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	if req.ID == "" {
		response.Validation(w, r, []response.ErrorDetail{{Field: "id", Message: "id 必填"}})
		return
	}
	result, err := h.svc.Recheck(r.Context(), req.ID)
	if err != nil {
		writeContentTaskError(w, r, err)
		return
	}
	response.OK(w, result)
}

// Delete POST /api/v1/admin/content-tasks/delete
func (h *ContentTaskHandler) Delete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	if req.ID == "" {
		response.Validation(w, r, []response.ErrorDetail{{Field: "id", Message: "id 必填"}})
		return
	}
	if err := h.svc.Delete(r.Context(), req.ID); err != nil {
		writeContentTaskError(w, r, err)
		return
	}
	h.writeAudit(r, "content_task.delete", req.ID, nil)
	response.NoContent(w)
}

func (h *ContentTaskHandler) writeAudit(r *http.Request, action, objectID string, after map[string]any) {
	if h.auditSvc == nil {
		return
	}
	h.auditSvc.Write(r, audit.Entry{
		ActorAdminID: actorID(r),
		Action:       action,
		ObjectType:   "content_task",
		ObjectID:     objectID,
		After:        after,
	})
}

func writeContentTaskError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, contenttask.ErrNotFound):
		response.NotFound(w, r)
	case errors.Is(err, contenttask.ErrDuplicate):
		response.Conflict(w, r, "该关键词已存在未完结任务")
	case errors.Is(err, contenttask.ErrBadKeyword),
		errors.Is(err, contenttask.ErrBadStatus),
		errors.Is(err, contenttask.ErrBadContentType),
		errors.Is(err, contenttask.ErrBadRelatedContent),
		errors.Is(err, contenttask.ErrBadAssignee):
		response.Validation(w, r, []response.ErrorDetail{{Message: err.Error()}})
	default:
		// 未知错误（检索服务不可用、数据库异常等）统一走通用处理：记录服务端日志，
		// 响应体只返回通用提示，不向客户端泄漏内部细节。
		response.HandleError(w, r, err)
	}
}
