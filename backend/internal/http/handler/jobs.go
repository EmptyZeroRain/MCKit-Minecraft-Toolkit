package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"mckit/backend/internal/audit"
	"mckit/backend/internal/http/response"
	"mckit/backend/internal/repository/types"
	"mckit/backend/internal/service/jobs"
)

// JobsHandler 后台运维任务处理器（admin 专属）。
//
// 接口一律为「动作 + POST」，与后台契约一致；任务执行与接口解耦，
// 入队后由进程内调度协程执行，前端轮询列表获取状态与失败详情。
type JobsHandler struct {
	svc      *jobs.Service
	auditSvc *audit.Service
}

// NewJobsHandler 创建处理器。
func NewJobsHandler(svc *jobs.Service, auditSvc *audit.Service) *JobsHandler {
	return &JobsHandler{svc: svc, auditSvc: auditSvc}
}

// Types POST /api/v1/admin/jobs/types
// 返回已注册的任务类型（前端据此渲染「新建任务」）。
func (h *JobsHandler) Types(w http.ResponseWriter, r *http.Request) {
	response.OK(w, h.svc.Types())
}

// List POST /api/v1/admin/jobs/list
// 分页查询任务，可按状态 / 类型过滤。
func (h *JobsHandler) List(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Status   string `json:"status"`
		Type     string `json:"type"`
		Page     int    `json:"page"`
		PageSize int    `json:"page_size"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}

	rows, total, err := h.svc.List(r.Context(), jobs.ListFilter{
		Status:   req.Status,
		Type:     req.Type,
		Page:     req.Page,
		PageSize: req.PageSize,
	})
	if err != nil {
		if strings.Contains(err.Error(), "非法的任务状态") {
			response.Validation(w, r, []response.ErrorDetail{{Field: "status", Message: err.Error()}})
			return
		}
		response.HandleError(w, r, err)
		return
	}

	page, pageSize := normalizePage(req.Page, req.PageSize)
	response.Page(w, r, rows, response.ResourceList{
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages(total, pageSize),
	})
}

// Detail POST /api/v1/admin/jobs/detail
// 返回单个任务（含 payload / result / last_error）。
func (h *JobsHandler) Detail(w http.ResponseWriter, r *http.Request) {
	id, ok := decodeJobID(w, r)
	if !ok {
		return
	}
	job, err := h.svc.Get(r.Context(), id)
	if err != nil {
		writeJobError(w, r, err)
		return
	}
	response.OK(w, job)
}

// Enqueue POST /api/v1/admin/jobs/enqueue
// body: {type, payload?}；未知类型返回 422。
func (h *JobsHandler) Enqueue(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}
	if r.Body == nil {
		response.BadRequest(w, r, "缺少请求体")
		return
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	if strings.TrimSpace(req.Type) == "" {
		response.Validation(w, r, []response.ErrorDetail{{Field: "type", Message: "任务类型必填"}})
		return
	}

	job, err := h.svc.Enqueue(r.Context(), jobs.EnqueueInput{
		Type:      strings.TrimSpace(req.Type),
		Payload:   types.JSONB(req.Payload),
		CreatedBy: actorName(r),
	})
	if err != nil {
		if errors.Is(err, jobs.ErrUnknownType) {
			response.Validation(w, r, []response.ErrorDetail{{Field: "type", Message: "未知的任务类型"}})
			return
		}
		response.HandleError(w, r, err)
		return
	}
	h.audit(r, "job.enqueue", job.ID.String(), map[string]any{
		"type": job.Type,
	})
	response.Created(w, job)
}

// Retry POST /api/v1/admin/jobs/retry
// 基于失败 / 已取消的任务新建一条任务（保留原始失败现场）。
func (h *JobsHandler) Retry(w http.ResponseWriter, r *http.Request) {
	id, ok := decodeJobID(w, r)
	if !ok {
		return
	}
	job, err := h.svc.Retry(r.Context(), id, actorName(r))
	if err != nil {
		writeJobError(w, r, err)
		return
	}
	h.audit(r, "job.retry", job.ID.String(), map[string]any{
		"type":     job.Type,
		"retry_of": id.String(),
	})
	response.Created(w, job)
}

// Cancel POST /api/v1/admin/jobs/cancel
// 仅能取消排队中的任务；执行中的任务返回 409。
func (h *JobsHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	id, ok := decodeJobID(w, r)
	if !ok {
		return
	}
	job, err := h.svc.Cancel(r.Context(), id, actorName(r))
	if err != nil {
		writeJobError(w, r, err)
		return
	}
	h.audit(r, "job.cancel", job.ID.String(), map[string]any{
		"type": job.Type,
	})
	response.OK(w, job)
}

// decodeJobID 解析 body 中的 id。
func decodeJobID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	var req struct {
		ID string `json:"id"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}
	id, err := uuid.Parse(strings.TrimSpace(req.ID))
	if err != nil {
		response.Validation(w, r, []response.ErrorDetail{{Field: "id", Message: "任务 ID 非法"}})
		return uuid.Nil, false
	}
	return id, true
}

// writeJobError 统一映射任务错误。
func writeJobError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, jobs.ErrNotFound):
		response.NotFound(w, r)
	case errors.Is(err, jobs.ErrNotRetryable):
		response.Error(w, r, http.StatusConflict, "JOB_NOT_RETRYABLE", err.Error(), nil)
	case errors.Is(err, jobs.ErrNotCancellable):
		response.Error(w, r, http.StatusConflict, "JOB_NOT_CANCELLABLE", err.Error(), nil)
	case errors.Is(err, jobs.ErrUnknownType):
		response.Validation(w, r, []response.ErrorDetail{{Field: "type", Message: "未知的任务类型"}})
	default:
		response.HandleError(w, r, err)
	}
}

// audit 写任务操作审计（入队 / 重试 / 取消）。
func (h *JobsHandler) audit(r *http.Request, action, objectID string, after map[string]any) {
	if h.auditSvc == nil {
		return
	}
	h.auditSvc.Write(r, audit.Entry{
		ActorAdminID: actorID(r),
		Action:       action,
		ObjectType:   "admin_job",
		ObjectID:     objectID,
		After:        after,
	})
}
