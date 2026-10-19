package handler

import (
	"encoding/json"
	"net/http"

	"mckit/backend/internal/http/response"
	"mckit/backend/internal/service/searchstats"
)

// SearchTermsHandler 搜索词统计处理器（admin 专属，TODO §5）。
//
// 统计内容为匿名化聚合结果（关键词 + 次数），不含任何用户标识，
// 因此可安全地用于运营分析：热门搜索、高频无结果词（即资料缺口）。
type SearchTermsHandler struct {
	svc *searchstats.Service
}

// NewSearchTermsHandler 创建处理器。
func NewSearchTermsHandler(svc *searchstats.Service) *SearchTermsHandler {
	return &SearchTermsHandler{svc: svc}
}

// List POST /api/v1/admin/search-terms/list
// kind：hot（热门搜索，默认）/ no_result（高频无结果词）；q 为关键词模糊过滤。
func (h *SearchTermsHandler) List(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kind     string `json:"kind"`
		Q        string `json:"q"`
		Page     int    `json:"page"`
		PageSize int    `json:"page_size"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}

	terms, total, err := h.svc.List(r.Context(), req.Kind, req.Q, req.Page, req.PageSize)
	if err != nil {
		response.HandleError(w, r, err)
		return
	}

	page, pageSize := normalizePage(req.Page, req.PageSize)
	response.Page(w, r, terms, response.ResourceList{
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages(total, pageSize),
	})
}
