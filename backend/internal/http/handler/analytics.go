package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"mckit/backend/internal/http/response"
	"mckit/backend/internal/service/analytics"
)

// AnalyticsHandler 运营看板处理器（admin 专属，TODO §13）。
//
// 只读接口；数据来源为按天聚合的使用统计与公开内容计数，不含任何个人身份信息。
type AnalyticsHandler struct {
	svc *analytics.Service
}

// NewAnalyticsHandler 创建处理器。
func NewAnalyticsHandler(svc *analytics.Service) *AnalyticsHandler {
	return &AnalyticsHandler{svc: svc}
}

// analyticsRangeRequest 时间范围入参。
//   - 只给 days 时表示最近 N 天（含当天），默认 30；
//   - from/to 均按 YYYY-MM-DD 解析，同时给出时以 from/to 为准。
type analyticsRangeRequest struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Days  int    `json:"days"`
	Limit int    `json:"limit"`
}

const (
	defaultAnalyticsDays = 30
	maxAnalyticsDays     = 366
)

// decodeRange 解析请求体中的时间范围。
func decodeRange(w http.ResponseWriter, r *http.Request, dst *analyticsRangeRequest) (time.Time, time.Time, bool) {
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(dst)
	}
	from, to, err := resolveRange(*dst)
	if err != nil {
		response.Validation(w, r, []response.ErrorDetail{{Field: "range", Message: err.Error()}})
		return time.Time{}, time.Time{}, false
	}
	return from, to, true
}

// resolveRange 将入参归一为 UTC 日期区间（含首尾），并限制最大跨度。
// 「今天」按服务器本地日期计算，与采集端 metrics.dayStart 的口径保持一致。
func resolveRange(req analyticsRangeRequest) (time.Time, time.Time, error) {
	to := localDayUTC(time.Now())
	if req.To != "" {
		t, err := time.ParseInLocation("2006-01-02", req.To, time.UTC)
		if err != nil {
			return time.Time{}, time.Time{}, errInvalidDate
		}
		to = t
	}
	if req.From != "" {
		f, err := time.ParseInLocation("2006-01-02", req.From, time.UTC)
		if err != nil {
			return time.Time{}, time.Time{}, errInvalidDate
		}
		if f.After(to) {
			return time.Time{}, time.Time{}, errRangeOrder
		}
		if days := int(to.Sub(f).Hours()/24) + 1; days > maxAnalyticsDays {
			f = to.AddDate(0, 0, -(maxAnalyticsDays - 1))
		}
		return f, to, nil
	}
	days := req.Days
	if days <= 0 {
		days = defaultAnalyticsDays
	}
	if days > maxAnalyticsDays {
		days = maxAnalyticsDays
	}
	return to.AddDate(0, 0, -(days - 1)), to, nil
}

// localDayUTC 将时间归一到「服务器本地日期」的 UTC 零点（与历史聚合口径一致）。
func localDayUTC(t time.Time) time.Time {
	y, m, d := t.In(time.Local).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// rangeError 时间范围校验错误。
type rangeError string

func (e rangeError) Error() string { return string(e) }

const (
	errInvalidDate = rangeError("from/to 需为 YYYY-MM-DD 格式")
	errRangeOrder  = rangeError("from 不能晚于 to")
)

// Overview POST /api/v1/admin/analytics/overview
func (h *AnalyticsHandler) Overview(w http.ResponseWriter, r *http.Request) {
	var req analyticsRangeRequest
	from, to, ok := decodeRange(w, r, &req)
	if !ok {
		return
	}
	overview, err := h.svc.Overview(r.Context(), from, to)
	if err != nil {
		response.HandleError(w, r, err)
		return
	}
	response.OK(w, overview)
}

// Tools POST /api/v1/admin/analytics/tools
func (h *AnalyticsHandler) Tools(w http.ResponseWriter, r *http.Request) {
	var req analyticsRangeRequest
	from, to, ok := decodeRange(w, r, &req)
	if !ok {
		return
	}
	rows, err := h.svc.ToolStats(r.Context(), from, to, req.Limit)
	if err != nil {
		response.HandleError(w, r, err)
		return
	}
	response.OK(w, rows)
}

// Resources POST /api/v1/admin/analytics/resources
func (h *AnalyticsHandler) Resources(w http.ResponseWriter, r *http.Request) {
	var req analyticsRangeRequest
	from, to, ok := decodeRange(w, r, &req)
	if !ok {
		return
	}
	rows, err := h.svc.ResourceStats(r.Context(), from, to, req.Limit)
	if err != nil {
		response.HandleError(w, r, err)
		return
	}
	response.OK(w, rows)
}

// API POST /api/v1/admin/analytics/api
func (h *AnalyticsHandler) API(w http.ResponseWriter, r *http.Request) {
	var req analyticsRangeRequest
	from, to, ok := decodeRange(w, r, &req)
	if !ok {
		return
	}
	rows, err := h.svc.APIStats(r.Context(), from, to, req.Limit)
	if err != nil {
		response.HandleError(w, r, err)
		return
	}
	response.OK(w, rows)
}

// Trend POST /api/v1/admin/analytics/trend
func (h *AnalyticsHandler) Trend(w http.ResponseWriter, r *http.Request) {
	var req analyticsRangeRequest
	from, to, ok := decodeRange(w, r, &req)
	if !ok {
		return
	}
	rows, err := h.svc.Trend(r.Context(), from, to)
	if err != nil {
		response.HandleError(w, r, err)
		return
	}
	response.OK(w, rows)
}
