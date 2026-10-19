package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"mckit/backend/internal/audit"
	"mckit/backend/internal/http/response"
	"mckit/backend/internal/service/dataquality"
)

// DataQualityHandler 数据质量与导入任务处理器（admin 专属，TODO §4）。
type DataQualityHandler struct {
	svc      *dataquality.Service
	auditSvc *audit.Service
}

// NewDataQualityHandler 创建处理器。
func NewDataQualityHandler(svc *dataquality.Service, auditSvc *audit.Service) *DataQualityHandler {
	return &DataQualityHandler{svc: svc, auditSvc: auditSvc}
}

// Overview POST /api/v1/admin/data-quality/overview
// 返回客观的数据质量检查项（缺失 / 非法格式 / 悬空引用）与导入任务概况。
func (h *DataQualityHandler) Overview(w http.ResponseWriter, r *http.Request) {
	ov, err := h.svc.Overview(r.Context())
	if err != nil {
		response.HandleError(w, r, err)
		return
	}
	response.OK(w, ov)
}

// Assignees POST /api/v1/admin/data-quality/assignees
// 可指派的数据质量负责人（启用中的管理员，昵称优先）。
func (h *DataQualityHandler) Assignees(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.Assignees(r.Context())
	if err != nil {
		response.HandleError(w, r, err)
		return
	}
	response.OK(w, rows)
}

// UpdateIssue POST /api/v1/admin/data-quality/issues/update
// 新建 / 更新数据质量问题工单（分派、解决、忽略、重新打开）。
func (h *DataQualityHandler) UpdateIssue(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CheckKey      string `json:"check_key"`
		Status        string `json:"status"`
		AssigneeID    string `json:"assignee_id"`
		Note          string `json:"note"`
		SnapshotCount int64  `json:"snapshot_count"`
	}
	if r.Body == nil {
		response.BadRequest(w, r, "缺少请求体")
		return
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}

	var assigneeID *uuid.UUID
	if strings.TrimSpace(req.AssigneeID) != "" {
		id, err := uuid.Parse(strings.TrimSpace(req.AssigneeID))
		if err != nil {
			response.Validation(w, r, []response.ErrorDetail{{Field: "assignee_id", Message: "负责人 ID 非法"}})
			return
		}
		assigneeID = &id
	}

	issue, err := h.svc.UpdateIssue(r.Context(), dataquality.UpdateIssueInput{
		CheckKey:      req.CheckKey,
		Status:        req.Status,
		AssigneeID:    assigneeID,
		Note:          req.Note,
		SnapshotCount: req.SnapshotCount,
		Operator:      actorName(r),
	})
	if err != nil {
		if errors.Is(err, dataquality.ErrBadIssue) {
			response.Validation(w, r, []response.ErrorDetail{{Field: "status", Message: err.Error()}})
			return
		}
		response.HandleError(w, r, err)
		return
	}
	if h.auditSvc != nil {
		h.auditSvc.Write(r, audit.Entry{
			ActorAdminID: actorID(r),
			Action:       "data_quality.issue.update",
			ObjectType:   "data_quality_issue",
			ObjectID:     issue.CheckKey,
			After: map[string]any{
				"status":     issue.Status,
				"assignee":   issue.AssigneeName,
				"note":       issue.Note,
				"check_key":  issue.CheckKey,
				"snapshot_n": issue.SnapshotCount,
			},
		})
	}
	response.OK(w, issue)
}

// ListJobs POST /api/v1/admin/data-import-jobs/list
// 分页查询数据导入任务，可按 data_type 过滤。
func (h *DataQualityHandler) ListJobs(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DataType string `json:"data_type"`
		Page     int    `json:"page"`
		PageSize int    `json:"page_size"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}

	jobs, total, err := h.svc.ListJobs(r.Context(), req.DataType, req.Page, req.PageSize)
	if err != nil {
		response.HandleError(w, r, err)
		return
	}

	page, pageSize := normalizePage(req.Page, req.PageSize)
	response.Page(w, r, jobs, response.ResourceList{
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages(total, pageSize),
	})
}
