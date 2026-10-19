package middleware

import (
	"context"
	"net/http"

	"mckit/backend/internal/auth"
	"mckit/backend/internal/domain"
	"mckit/backend/internal/http/response"
)

// userCookieName 普通用户会话 Cookie 名。
// 与管理员的 sessionCookieName 分离：同一浏览器可同时保留管理会话与用户会话，
// 且用户会话绝不会被管理端鉴权中间件识别，避免权限混淆。
var userCookieName = "mc_user_session"

// SetUserCookieName 设置用户会话 Cookie 名称（路由初始化时调用）。
func SetUserCookieName(name string) {
	if name != "" {
		userCookieName = name
	}
}

// UserCookieName 返回当前用户会话 Cookie 名（供处理层种 Cookie 使用）。
func UserCookieName() string { return userCookieName }

// userSessionContextKey 保存当前用户会话（与管理员会话使用不同的 context key）。
type userSessionContextKey struct{}

// WithUserSession 将用户会话写入 context。
func WithUserSession(ctx context.Context, s *auth.Session) context.Context {
	return context.WithValue(ctx, userSessionContextKey{}, s)
}

// UserSessionFrom 从 context 读取用户会话，可能为 nil。
func UserSessionFrom(ctx context.Context) *auth.Session {
	if s, ok := ctx.Value(userSessionContextKey{}).(*auth.Session); ok {
		return s
	}
	return nil
}

// RequireUserAuth 校验用户会话 Cookie，未登录返回 401。
func RequireUserAuth(svc AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(userCookieName)
			if err != nil || cookie.Value == "" {
				response.Unauthorized(w, r)
				return
			}
			session, err := svc.GetSession(r.Context(), cookie.Value)
			if err != nil || session == nil {
				response.Unauthorized(w, r)
				return
			}
			// 双重保险：只接受普通用户角色，防止管理会话被误用于用户接口。
			if session.Role != domain.RoleUser {
				response.Unauthorized(w, r)
				return
			}
			next.ServeHTTP(w, r.WithContext(WithUserSession(r.Context(), session)))
		})
	}
}

// UserCSRF 校验用户写请求的 CSRF token（请求头必须与会话绑定 token 一致）。
func UserCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		session := UserSessionFrom(r.Context())
		if session == nil {
			response.Unauthorized(w, r)
			return
		}
		headerToken := r.Header.Get("X-CSRF-Token")
		if headerToken == "" || headerToken != session.CSRFToken {
			response.Error(w, r, http.StatusForbidden, "CSRF_INVALID", "CSRF 校验失败", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}
