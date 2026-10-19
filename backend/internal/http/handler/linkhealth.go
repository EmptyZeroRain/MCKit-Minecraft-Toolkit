package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/time/rate"

	"mckit/backend/internal/audit"
	"mckit/backend/internal/domain"
	"mckit/backend/internal/http/response"
	"mckit/backend/internal/service/linkhealth"
)

// LinkHealthHandler 链接健康与人工确认处理器（TODO §6，admin 专属）。
type LinkHealthHandler struct {
	svc      *linkhealth.Service
	auditSvc *audit.Service
	// runLimiter 批量复查限流：每个操作者每 30 秒最多 1 次（突发 2 次）。
	// 批量复查是出网任务，必须限流，避免被当作批量探测工具。
	runLimiter *keyedLimiter
}

// NewLinkHealthHandler 创建处理器。
func NewLinkHealthHandler(svc *linkhealth.Service, auditSvc *audit.Service) *LinkHealthHandler {
	return &LinkHealthHandler{
		svc:        svc,
		auditSvc:   auditSvc,
		runLimiter: newKeyedLimiter(rate.Every(30*time.Second), 2, 30*time.Minute),
	}
}

type linkHealthRunRequest struct {
	ResourceID    string `json:"resource_id"`
	OnlyPending   *bool  `json:"only_pending"`
	Limit         int    `json:"limit"`
	TriggerSource string `json:"trigger_source"`
}

// Run POST /api/v1/admin/link-health/run
// 后台批量复查：受限并发检查资源来源链接，落库并更新链接状态。
func (h *LinkHealthHandler) Run(w http.ResponseWriter, r *http.Request) {
	if !h.runLimiter.allow(actorID(r)) {
		response.TooManyRequests(w, r)
		return
	}
	var req linkHealthRunRequest
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&req)
	}
	onlyPending := true
	if req.OnlyPending != nil {
		onlyPending = *req.OnlyPending
	}

	summary, err := h.svc.RunCheck(r.Context(), linkhealth.RunOptions{
		ResourceID:    req.ResourceID,
		OnlyPending:   onlyPending,
		Limit:         req.Limit,
		TriggerSource: domain.LinkTriggerManual,
	})
	if err != nil {
		response.HandleError(w, r, err)
		return
	}
	h.audit(r, "link_health.run", "", map[string]any{
		"checked":     summary.Checked,
		"broken":      summary.Broken,
		"blocked":     summary.Blocked,
		"error":       summary.Error,
		"concurrency": summary.Concurrency,
	})
	response.OK(w, summary)
}

type linkHealthListRequest struct {
	ReviewState string `json:"review_state"`
	Status      string `json:"status"`
	FailedOnly  *bool  `json:"failed_only"`
	Page        int    `json:"page"`
	PageSize    int    `json:"page_size"`
}

// List POST /api/v1/admin/link-health/list
// 查询复核列表（默认只看失败项）。
func (h *LinkHealthHandler) List(w http.ResponseWriter, r *http.Request) {
	var req linkHealthListRequest
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&req)
	}
	failedOnly := true
	if req.FailedOnly != nil {
		failedOnly = *req.FailedOnly
	}
	res, err := h.svc.List(r.Context(), linkhealth.ListFilter{
		ReviewState: req.ReviewState,
		Status:      req.Status,
		FailedOnly:  failedOnly,
		Page:        req.Page,
		PageSize:    req.PageSize,
	})
	if err != nil {
		response.HandleError(w, r, err)
		return
	}
	response.Page(w, r, res.Items, response.ResourceList{
		Total:      res.Total,
		Page:       res.Page,
		PageSize:   res.PageSize,
		TotalPages: totalPages(res.Total, res.PageSize),
	})
}

type linkHealthConfirmRequest struct {
	ID     string `json:"id"`
	Action string `json:"action"`
	Note   string `json:"note"`
}

// Confirm POST /api/v1/admin/link-health/{id}/confirm
// 人工确认链接结论：confirm_broken / confirm_ok / ignore。
func (h *LinkHealthHandler) Confirm(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req linkHealthConfirmRequest
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&req)
	}
	if id == "" {
		id = req.ID
	}
	if id == "" {
		response.BadRequest(w, r, "缺少链接检查记录 ID")
		return
	}

	rec, err := h.svc.Confirm(r.Context(), id, req.Action, actorID(r), req.Note)
	if err != nil {
		switch {
		case errors.Is(err, linkhealth.ErrNotFound):
			response.NotFound(w, r)
		case errors.Is(err, linkhealth.ErrBadAction):
			response.BadRequest(w, r, "无效的确认动作（confirm_broken / confirm_ok / ignore）")
		default:
			response.HandleError(w, r, err)
		}
		return
	}
	h.audit(r, "link_health.confirm", rec.ResourceID, map[string]any{
		"review_state": rec.ReviewState,
		"note":         rec.ReviewNote,
	})
	response.OK(w, rec)
}

// audit 写审计日志。
func (h *LinkHealthHandler) audit(r *http.Request, action, objID string, after map[string]any) {
	if h.auditSvc == nil {
		return
	}
	h.auditSvc.Write(r, audit.Entry{
		ActorAdminID: actorID(r),
		Action:       action,
		ObjectType:   "link_check",
		ObjectID:     objID,
		After:        after,
	})
}
