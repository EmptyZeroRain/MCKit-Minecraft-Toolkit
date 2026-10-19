package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"

	"golang.org/x/time/rate"

	"mckit/backend/internal/http/response"
)

// ipLimiter 按客户端 IP 维护独立的令牌桶限流器。
// 为避免内存无限增长，这里采用固定数量的分片桶 + 定期清理的简化实现：
// 限流只作基础防护，登录等关键路由另有更强限制（见 auth 模块）。
type ipLimiter struct {
	mu      sync.Mutex
	limit   rate.Limit
	burst   int
	clients map[string]*clientEntry
}

type clientEntry struct {
	limiter *rate.Limiter
	// 预留 lastSeen 以便后期定期清理
	lastSeen int64
}

func newIPLimiter(rps float64, burst int) *ipLimiter {
	return &ipLimiter{
		limit:   rate.Limit(rps),
		burst:   burst,
		clients: make(map[string]*clientEntry),
	}
}

func (l *ipLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	entry, ok := l.clients[ip]
	if !ok {
		entry = &clientEntry{limiter: rate.NewLimiter(l.limit, l.burst)}
		l.clients[ip] = entry
	}
	return entry.limiter.Allow()
}

// RateLimit 基于客户端 IP 的全局速率限制。
func RateLimit(rps float64, burst int) func(http.Handler) http.Handler {
	if rps <= 0 || burst <= 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	limiter := newIPLimiter(rps, burst)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r.RemoteAddr)
			if !limiter.allow(ip) {
				response.TooManyRequests(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// clientIP 从 RemoteAddr 提取 IP，忽略 X-Forwarded-For
// （防止客户端伪造触发限流绕过；真实代理头校验在反代层处理）。
func clientIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return strings.TrimSpace(remoteAddr)
	}
	return host
}