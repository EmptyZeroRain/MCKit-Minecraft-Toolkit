package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"mckit/backend/internal/http/response"
)

// WithBodyID 兼容前端「动作子路径 + 全 POST」契约：
// 从 JSON 请求体读取 "id" 并注入 chi URL 参数，
// 从而复用按路径取 id 的既有处理器（detail/update/delete/check-link）。
// 读取后恢复 body，供后续处理器再次解码。
func WithBodyID(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Body == nil {
			next(w, r)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		_ = r.Body.Close()
		r.Body = io.NopCloser(bytes.NewReader(body))
		if err == nil && len(body) > 0 {
			var probe struct {
				ID string `json:"id"`
			}
			if json.Unmarshal(body, &probe) == nil && probe.ID != "" {
				if rctx := chi.RouteContext(r.Context()); rctx != nil {
					rctx.URLParams.Add("id", probe.ID)
				}
			}
		}
		next(w, r)
	}
}

// decodeID 从请求体读取 "id" 字段（用于动作子路径的 delete/update 等）。
func decodeID(w http.ResponseWriter, r *http.Request) (string, bool) {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return "", false
	}
	if req.ID == "" {
		response.Validation(w, r, []response.ErrorDetail{{Field: "id", Message: "id 必填"}})
		return "", false
	}
	return req.ID, true
}
