package handler

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"mckit/backend/internal/audit"
	"mckit/backend/internal/domain"
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
	ActorID    string `json:"actor_id"`
	Action     string `json:"action"`
	ObjectType string `json:"object_type"`
	Result     string `json:"result"`
	SourceIP   string `json:"source_ip"`
	// Security 为 true 时只看账号安全事件（登录 / 改密 / MFA / 会话撤销）。
	Security bool `json:"security"`
	Page     int  `json:"page"`
	PageSize int  `json:"page_size"`
}

// List POST /api/v1/admin/audit-logs
func (h *AuditHandler) List(w http.ResponseWriter, r *http.Request) {
	var req listRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	if !validAuditResult(req.Result) {
		response.Validation(w, r, []response.ErrorDetail{{Field: "result", Message: "结果只能是 success 或 failure"}})
		return
	}

	items, total, err := h.svc.List(audit.ListFilter{
		ActorID:        req.ActorID,
		Action:         req.Action,
		ObjectType:     req.ObjectType,
		Result:         req.Result,
		SourceIP:       req.SourceIP,
		ActionPrefixes: securityPrefixes(req.Security),
		Page:           req.Page,
		PageSize:       req.PageSize,
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

// Export POST /api/v1/admin/audit-logs/export
// 按当前筛选条件导出 CSV（UTF-8 BOM，最多 audit.ExportMax 条）。
func (h *AuditHandler) Export(w http.ResponseWriter, r *http.Request) {
	var req listRequest
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&req)
	}
	if !validAuditResult(req.Result) {
		response.Validation(w, r, []response.ErrorDetail{{Field: "result", Message: "结果只能是 success 或 failure"}})
		return
	}

	items, truncated, err := h.svc.Export(r.Context(), audit.ListFilter{
		ActorID:        req.ActorID,
		Action:         req.Action,
		ObjectType:     req.ObjectType,
		Result:         req.Result,
		SourceIP:       req.SourceIP,
		ActionPrefixes: securityPrefixes(req.Security),
	})
	if err != nil {
		response.HandleError(w, r, err)
		return
	}

	filename := "audit-logs-" + time.Now().UTC().Format("20060102-150405") + ".csv"
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("X-Export-Truncated", strconv.FormatBool(truncated))
	w.WriteHeader(http.StatusOK)

	// UTF-8 BOM：Excel 打开中文 CSV 不乱码。
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"时间", "操作者", "动作", "对象类型", "对象ID", "结果", "来源IP", "请求ID"})
	for _, it := range items {
		actor := ""
		if it.ActorAdminID != nil {
			actor = *it.ActorAdminID
		}
		objID := ""
		if it.ObjectID != nil {
			objID = *it.ObjectID
		}
		_ = cw.Write([]string{
			it.CreatedAt,
			csvSafe(actor),
			csvSafe(it.Action),
			csvSafe(it.ObjectType),
			csvSafe(objID),
			csvSafe(it.Result),
			csvSafe(it.SourceIP),
			csvSafe(it.RequestID),
		})
	}
	cw.Flush()

	// 导出本身也是敏感操作：记录条数与筛选条件，便于事后追溯谁导出了什么。
	actorIDValue := actorID(r)
	h.svc.Write(r, audit.Entry{
		ActorAdminID: actorIDValue,
		Action:       "audit.export",
		ObjectType:   "audit_log",
		After: map[string]any{
			"count":       len(items),
			"truncated":   truncated,
			"object_type": req.ObjectType,
			"result":      req.Result,
			"security":    req.Security,
		},
	})
}

// validAuditResult 校验结果筛选值。
func validAuditResult(result string) bool {
	return result == "" || result == domain.AuditSuccess || result == domain.AuditFailure
}

// securityPrefixes 安全事件视图的动作前缀。
func securityPrefixes(security bool) []string {
	if security {
		return audit.SecurityActionPrefixes
	}
	return nil
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
