package middleware

import (
	"net/http"

	"mckit/backend/internal/http/response"
)

// ConcurrencyLimit 用全局信号量限制同时执行的请求数。
//
// 场景：结构候选 / 史莱姆区块等遍历型接口单次可跑数千次噪声采样，
// 多个请求并发会叠加占用 CPU 与内存。限流（RateLimit）约束的是速率，
// 这里进一步约束**同时在跑**的请求数，超出即返回 429。
// max <= 0 表示不限制。
func ConcurrencyLimit(max int) func(http.Handler) http.Handler {
	if max <= 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	sem := make(chan struct{}, max)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
				next.ServeHTTP(w, r)
			default:
				// 已达并发上限：明确拒绝，避免排队拖垮 worker。
				response.TooManyRequests(w, r)
			}
		})
	}
}
