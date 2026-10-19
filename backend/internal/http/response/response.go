// Package response 提供统一的 API 成功/错误响应结构，
// 以及请求 ID 的存取能力。
package response

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
)

// requestIDKey 用于在 context 中传递 Request ID。
type requestIDKey struct{}

// WithRequestID 将请求 ID 写入 context。
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestIDFrom 从 context 读取请求 ID，读取不到时返回空串。
func RequestIDFrom(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey{}).(string); ok {
		return v
	}
	return ""
}

// Data 统一成功响应体：data 为业务数据，meta 为分页等元信息。
type Data struct {
	Data any `json:"data"`
	Meta any `json:"meta,omitempty"`
}

// ErrorDetail 单条字段错误。
type ErrorDetail struct {
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

// ErrorInfo 错误详情。
type ErrorInfo struct {
	Code    string        `json:"code"`
	Message string        `json:"message"`
	Details []ErrorDetail `json:"details,omitempty"`
}

// ErrorBody 统一错误响应体，包含 request_id 便于排障。
type ErrorBody struct {
	Error     ErrorInfo `json:"error"`
	RequestID string    `json:"request_id"`
}

// writeJSON 以 JSON 写出响应。
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		return
	}
}

// OK 返回 200 成功响应。
func OK(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, Data{Data: data})
}

// Created 返回 201 创建成功响应。
func Created(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusCreated, Data{Data: data})
}

// NoContent 返回 204 无内容响应。
func NoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// ResourceList 封装分页列表数据，方便 meta 中携带分页信息。
type ResourceList struct {
	Total      int64 `json:"total"`
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	TotalPages int   `json:"total_pages"`
	// Suggestions 无结果搜索时可尝试的关键词（TODO §5），仅搜索接口在无结果时填充；
	// 其余列表接口保持省略，不影响既有契约。
	Suggestions []string `json:"suggestions,omitempty"`
}

// Page 返回包含分页信息的列表成功响应。
// data 为列表本身，meta 携带 ResourceList 分页信息。
func Page(w http.ResponseWriter, r *http.Request, data any, p ResourceList) {
	writeJSON(w, http.StatusOK, Data{Data: data, Meta: p})
}

// Error 写出带请求 ID 的统一错误响应。
func Error(w http.ResponseWriter, r *http.Request, status int, code, message string, details []ErrorDetail) {
	writeJSON(w, status, ErrorBody{
		Error: ErrorInfo{
			Code:    code,
			Message: message,
			Details: details,
		},
		RequestID: RequestIDFrom(r.Context()),
	})
}

// HandleError 内部错误统一转换为 500，并抑制敏感细节。
//
// 关键可观察性（TODO §19）：数据库 / 缓存等底层异常在此**记录服务端日志**
// （含请求 ID、方法与路径），便于排查；响应体仍只返回通用提示，不向客户端泄漏内部细节。
func HandleError(w http.ResponseWriter, r *http.Request, err error) {
	if err != nil {
		attrs := []any{"err", err}
		if r != nil {
			attrs = append(attrs,
				"request_id", RequestIDFrom(r.Context()),
				"method", r.Method,
				"path", r.URL.Path,
			)
		}
		slog.Error("请求处理失败", attrs...)
	}
	Error(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "服务器内部错误，请稍后重试", nil)
}

// Validation 输出参数校验失败（422）。
func Validation(w http.ResponseWriter, r *http.Request, details []ErrorDetail) {
	Error(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "请求参数不合法", details)
}

// BadRequest 输出参数格式错误（400）。
func BadRequest(w http.ResponseWriter, r *http.Request, message string) {
	Error(w, r, http.StatusBadRequest, "BAD_REQUEST", message, nil)
}

// NotFound 输出资源不存在。
func NotFound(w http.ResponseWriter, r *http.Request) {
	Error(w, r, http.StatusNotFound, "NOT_FOUND", "请求的资源不存在", nil)
}

// Conflict 输出唯一性冲突。
func Conflict(w http.ResponseWriter, r *http.Request, message string) {
	Error(w, r, http.StatusConflict, "CONFLICT", message, nil)
}

// Unauthorized 输出未登录/凭证无效。
func Unauthorized(w http.ResponseWriter, r *http.Request) {
	Error(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "请先登录", nil)
}

// Forbidden 输出无权限。
func Forbidden(w http.ResponseWriter, r *http.Request) {
	Error(w, r, http.StatusForbidden, "FORBIDDEN", "无权执行该操作", nil)
}

// TooManyRequests 输出限流。
func TooManyRequests(w http.ResponseWriter, r *http.Request) {
	Error(w, r, http.StatusTooManyRequests, "RATE_LIMITED", "请求过于频繁，请稍后重试", nil)
}

// ServiceUnavailable 输出依赖服务不可用（503）。
//
// 用于共享会话存储等关键依赖故障：必须安全失败（拒绝请求），不得降级为「跳过认证」。
func ServiceUnavailable(w http.ResponseWriter, r *http.Request, message string) {
	if message == "" {
		message = "服务暂时不可用，请稍后重试"
	}
	Error(w, r, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", message, nil)
}
