package middleware

import (
	"net/http"

	"mckit/backend/internal/domain"
	"mckit/backend/internal/http/response"
)

// RequireUserRole 在共享会话之上要求当前会话为普通用户（role = 'user'）。
//
// 统一账号（迁移 0022）后管理员与普通用户共用同一个会话 Cookie，
// 因此这里不再校验独立 Cookie，而是校验会话角色：
//   - 普通用户接口只接受 role = 'user'，管理员会话访问返回 401（前端据此视为未登录）；
//   - 后台接口则由 RequirePermission 按权限点判定，role = 'user' 不含任何权限点，自然 403。
//
// 必须放在 RequireAuth 之后（依赖会话已写入 context）。
func RequireUserRole() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			session := SessionFrom(r.Context())
			if session == nil || session.Role != domain.RoleUser {
				response.Unauthorized(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
