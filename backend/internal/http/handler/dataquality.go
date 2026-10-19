package handler

import (
	"encoding/json"
	"net/http"

	"mckit/backend/internal/http/response"
	"mckit/backend/internal/service/dataquality"
)

// DataQualityHandler 数据质量与导入任务处理器（admin 专属，TODO §4）。
type DataQualityHandler struct {
	svc *dataquality.Service
}

// NewDataQualityHandler 创建处理器。
func NewDataQualityHandler(svc *dataquality.Service) *DataQualityHandler {
	return &DataQualityHandler{svc: svc}
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
