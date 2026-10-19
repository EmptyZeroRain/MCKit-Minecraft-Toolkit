package middleware

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"

	"mckit/backend/internal/auth"
	"mckit/backend/internal/http/response"
)

// RequestIDFromCtx 从 context 读取请求 ID。
func RequestIDFromCtx(ctx context.Context) string {
	return response.RequestIDFrom(ctx)
}

// ClientIP 提取客户端 IP（去除端口）。
func ClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return host
}

// sessionContextKey 保存当前会话。
type sessionContextKey struct{}

// WithSession 将会话写入 context。
func WithSession(ctx context.Context, s *auth.Session) context.Context {
	return context.WithValue(ctx, sessionContextKey{}, s)
}

// SessionFrom 从 context 读取会话，可能为 nil。
func SessionFrom(ctx context.Context) *auth.Session {
	if s, ok := ctx.Value(sessionContextKey{}).(*auth.Session); ok {
		return s
	}
	return nil
}

// cookieName 会话 Cookie 名，由 config 注入。
var cookieName = "mc_admin_session"

// SetCookieName 设置会话 Cookie 名称（在路由初始化时调用）。
func SetCookieName(name string) {
	if name != "" {
		cookieName = name
	}
}

// SessionCookieName 返回当前会话 Cookie 名。
// 登录 / 登出处理器用它种 Cookie，保证与鉴权中间件读取的名字一致
// （统一账号后管理员与普通用户共用这一个 Cookie）。
func SessionCookieName() string { return cookieName }

// AuthService 认证服务接口（供中间件依赖注入，避免具体类型耦合）。
type AuthService interface {
	GetSession(ctx context.Context, sessionID string) (*auth.Session, error)
}

// RequireAuth 校验会话 Cookie，未登录返回 401。
func RequireAuth(svc AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(cookieName)
			if err != nil || cookie.Value == "" {
				response.Unauthorized(w, r)
				return
			}
			session, err := svc.GetSession(r.Context(), cookie.Value)
			if err != nil {
				// 共享会话存储不可用：安全失败（503），绝不跳过认证（gj.md §3.4）。
				if errors.Is(err, auth.ErrSessionStoreUnavailable) {
					response.ServiceUnavailable(w, r, "会话服务暂时不可用，请稍后重试")
					return
				}
				response.Unauthorized(w, r)
				return
			}
			if session == nil {
				response.Unauthorized(w, r)
				return
			}
			next.ServeHTTP(w, r.WithContext(WithSession(r.Context(), session)))
		})
	}
}

// OptionalAuth 尽力解析会话：Cookie 有效则写入 context，缺失 / 失效则按匿名继续。
//
// 用于「匿名可用、登录后能力更强」的接口，例如 mcsrc 的自动准备：
// 任何访客都能触发下载，但具备 ops.mcsrc 的管理员不受自动准备额度限制。
// 与 RequireAuth 的差别是**绝不**返回 401。
func OptionalAuth(svc AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if svc == nil {
				next.ServeHTTP(w, r)
				return
			}
			cookie, err := r.Cookie(cookieName)
			if err != nil || cookie.Value == "" {
				next.ServeHTTP(w, r)
				return
			}
			session, err := svc.GetSession(r.Context(), cookie.Value)
			if err != nil || session == nil {
				next.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r.WithContext(WithSession(r.Context(), session)))
		})
	}
}

// RequireRole 校验当前会话角色在允许集合内。
// 必须放在 RequireAuth 之后。
func RequireRole(allowed ...string) func(http.Handler) http.Handler {
	allowedSet := make(map[string]bool, len(allowed))
	for _, r := range allowed {
		allowedSet[r] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			session := SessionFrom(r.Context())
			if session == nil {
				response.Unauthorized(w, r)
				return
			}
			if !allowedSet[session.Role] {
				response.Forbidden(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CSRF 校验写请求携带的 CSRF token（双重提交：Cookie 与请求头）。
// 需要 RequireAuth 提供 session.CSRFToken。
// 仅对非 GET/HEAD/OPTIONS 方法生效（本 API 全 POST，因此覆盖写操作）。
func CSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		session := SessionFrom(r.Context())
		if session == nil {
			response.Unauthorized(w, r)
			return
		}
		headerToken := r.Header.Get("X-CSRF-Token")
		// 双重提交校验：请求头必须与会话绑定 token 一致。
		if headerToken == "" || headerToken != session.CSRFToken {
			response.Error(w, r, http.StatusForbidden, "CSRF_INVALID", "CSRF 校验失败", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}
