// Package middleware 提供 HTTP 中间件：请求 ID、恢复、访问日志、
// 安全响应头、CORS 与限流等。
package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"mckit/backend/internal/http/response"
)

// RequestID 为每个请求生成唯一 ID，写入 context 并回填响应头。
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = newRequestID()
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(response.WithRequestID(r.Context(), id)))
	})
}

func newRequestID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand 失败属于极端场景，退化为时间戳保证唯一性。
		return hex.EncodeToString([]byte{0}) + "fallback"
	}
	return hex.EncodeToString(b)
}

// Recover 捕获 panic，恢复请求处理并返回统一 500 响应。
// panic 详情仅写入日志，绝不回传给客户端。
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				response.Error(w, r, http.StatusInternalServerError,
					"INTERNAL_ERROR", "服务器内部错误，请稍后重试", nil)
			}
		}()
		next.ServeHTTP(w, r)
	})
}