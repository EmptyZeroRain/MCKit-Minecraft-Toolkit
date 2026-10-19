package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"mckit/backend/internal/audit"
	"mckit/backend/internal/http/response"
	"mckit/backend/internal/service/alert"
)

// AlertsHandler 运营告警处理器（admin 专属）。
//
// 告警由规则自动生成、由运营确认 / 关闭；手动评估用于「马上看一下现在有没有问题」。
type AlertsHandler struct {
	svc      *alert.Service
	auditSvc *audit.Service
}

// NewAlertsHandler 创建处理器。
func NewAlertsHandler(svc *alert.Service, auditSvc *audit.Service) *AlertsHandler {
	return &AlertsHandler{svc: svc, auditSvc: auditSvc}
}

// Summary POST /api/v1/admin/alerts/summary
// 未关闭告警计数（用于页面顶部与导航提示）。
func (h *AlertsHandler) Summary(w http.ResponseWriter, r *http.Request) {
	sum, err := h.svc.Summary(r.Context())
	if err != nil {
		response.HandleError(w, r, err)
		return
	}
	response.OK(w, sum)
}

// List POST /api/v1/admin/alerts/list
// 分页查询告警（含历史：resolved 记录不删除）。
func (h *AlertsHandler) List(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Status   string `json:"status"`
		Severity string `json:"severity"`
		RuleKey  string `json:"rule_key"`
		Page     int    `json:"page"`
		PageSize int    `json:"page_size"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}

	rows, total, err := h.svc.List(r.Context(), alert.ListFilter{
		Status:   req.Status,
		Severity: req.Severity,
		RuleKey:  req.RuleKey,
		Page:     req.Page,
		PageSize: req.PageSize,
	})
	if err != nil {
		if errors.Is(err, alert.ErrBadInput) {
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

// Evaluate POST /api/v1/admin/alerts/evaluate
// 立即评估全部规则（定时评估默认每 5 分钟一次）。
func (h *AlertsHandler) Evaluate(w http.ResponseWriter, r *http.Request) {
	sum, err := h.svc.Evaluate(r.Context())
	if err != nil {
		response.HandleError(w, r, err)
		return
	}
	response.OK(w, sum)
}

// Ack POST /api/v1/admin/alerts/ack
// 确认告警（open → acked），表示「已有人接手」。
func (h *AlertsHandler) Ack(w http.ResponseWriter, r *http.Request) {
	id, ok := decodeAlertID(w, r)
	if !ok {
		return
	}
	alertRow, err := h.svc.Ack(r.Context(), id, actorName(r))
	if err != nil {
		writeAlertError(w, r, err)
		return
	}
	h.audit(r, "alert.ack", alertRow.ID.String(), map[string]any{
		"rule_key": alertRow.RuleKey,
	})
	response.OK(w, alertRow)
}

// Resolve POST /api/v1/admin/alerts/resolve
// 关闭告警并记录原因（历史保留）。
func (h *AlertsHandler) Resolve(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID   string `json:"id"`
		Note string `json:"note"`
	}
	if r.Body == nil {
		response.BadRequest(w, r, "缺少请求体")
		return
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	id, err := uuid.Parse(strings.TrimSpace(req.ID))
	if err != nil {
		response.Validation(w, r, []response.ErrorDetail{{Field: "id", Message: "告警 ID 非法"}})
		return
	}

	alertRow, err := h.svc.Resolve(r.Context(), id, actorName(r), req.Note)
	if err != nil {
		writeAlertError(w, r, err)
		return
	}
	h.audit(r, "alert.resolve", alertRow.ID.String(), map[string]any{
		"rule_key": alertRow.RuleKey,
		"note":     req.Note,
	})
	response.OK(w, alertRow)
}

func decodeAlertID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	var req struct {
		ID string `json:"id"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}
	id, err := uuid.Parse(strings.TrimSpace(req.ID))
	if err != nil {
		response.Validation(w, r, []response.ErrorDetail{{Field: "id", Message: "告警 ID 非法"}})
		return uuid.Nil, false
	}
	return id, true
}

func writeAlertError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, alert.ErrNotFound):
		response.NotFound(w, r)
	case errors.Is(err, alert.ErrNotOpen):
		response.Error(w, r, http.StatusConflict, "ALERT_NOT_OPEN", err.Error(), nil)
	case errors.Is(err, alert.ErrBadInput):
		response.Validation(w, r, []response.ErrorDetail{{Field: "status", Message: err.Error()}})
	default:
		response.HandleError(w, r, err)
	}
}

func (h *AlertsHandler) audit(r *http.Request, action, objectID string, after map[string]any) {
	if h.auditSvc == nil {
		return
	}
	h.auditSvc.Write(r, audit.Entry{
		ActorAdminID: actorID(r),
		Action:       action,
		ObjectType:   "ops_alert",
		ObjectID:     objectID,
		After:        after,
	})
}
