package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"mckit/backend/internal/service/metrics"
)

// Metrics 采集关键 API 指标（路由模板 / 状态码 / 耗时）用于后台运营看板（TODO §13）。
//
// 隐私设计：只读取 chi 路由模板与状态码、耗时，**不读取** query、请求体、
// 客户端 IP、User-Agent 或会话；路由模板不含具体参数值（如 /api/v1/tools/{slug}）。
//
// 读点在 next 返回之后：此时 chi 已填充 route context（含挂载前缀）。
// 未匹配到路由的请求（含被全局限流提前拒绝的请求）归并为 metrics.UnmatchedRoute。
func Metrics(c *metrics.Collector) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if c == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			c.Observe(routePattern(r), r.Method, rec.status, time.Since(start))
		})
	}
}

// routePattern 返回当前请求匹配到的路由模板。
// chi 对 Mount 的子路由会拼接出形如 /api/v1/*/tools/{slug} 的模式，
// 这里把挂载分隔符 "/*/" 归一为 "/"，得到稳定的 /api/v1/tools/{slug}。
func routePattern(r *http.Request) string {
	rc := chi.RouteContext(r.Context())
	if rc == nil {
		return metrics.UnmatchedRoute
	}
	p := rc.RoutePattern()
	if p == "" {
		return metrics.UnmatchedRoute
	}
	return strings.ReplaceAll(p, "/*/", "/")
}
