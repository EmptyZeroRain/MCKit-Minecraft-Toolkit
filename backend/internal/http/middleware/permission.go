package middleware

import (
	"context"
	"net/http"

	"mckit/backend/internal/http/response"
)

// PermissionResolver 解析角色对应的权限点集合（由 role.Service 实现）。
// 定义在中间件侧，避免中间件反向依赖具体服务包。
type PermissionResolver interface {
	PermissionsForRole(ctx context.Context, role string) ([]string, error)
}

// RequirePermission 校验当前会话角色是否具备全部指定权限点。
// 必须放在 RequireAuth 之后；角色权限解析失败按 500 处理（不静默降级为放行或 403）。
func RequirePermission(resolver PermissionResolver, points ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			session := SessionFrom(r.Context())
			if session == nil {
				response.Unauthorized(w, r)
				return
			}
			if resolver == nil {
				response.Forbidden(w, r)
				return
			}
			perms, err := resolver.PermissionsForRole(r.Context(), session.Role)
			if err != nil {
				response.HandleError(w, r, err)
				return
			}
			have := make(map[string]struct{}, len(perms))
			for _, p := range perms {
				have[p] = struct{}{}
			}
			for _, need := range points {
				if _, ok := have[need]; !ok {
					response.Forbidden(w, r)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
