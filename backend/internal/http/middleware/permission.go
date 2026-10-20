package middleware

import (
	"context"
	"net/http"

	"mckit/backend/internal/http/response"
)

// passwordChangeRequired 输出「必须先修改密码」（403 + 专用错误码）。
// 前端据此跳转到改密页，而不是把用户当作无权限处理。
func passwordChangeRequired(w http.ResponseWriter, r *http.Request) {
	response.Error(w, r, http.StatusForbidden, "PASSWORD_CHANGE_REQUIRED",
		"请先修改密码后再继续操作", nil)
}

// RequirePasswordChanged 拒绝处于「必须改密」状态的会话。
//
// 用于普通用户侧写操作：账号被管理员重置密码后，除自助改密与资料维护外
// 的写操作一律拒绝（gj.md §10.2）。
func RequirePasswordChanged() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			session := SessionFrom(r.Context())
			if session != nil && session.MustChangePassword {
				passwordChangeRequired(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// PermissionResolver 解析角色对应的权限点集合（由 role.Service 实现）。
// 定义在中间件侧，避免中间件反向依赖具体服务包。
type PermissionResolver interface {
	PermissionsForRole(ctx context.Context, role string) ([]string, error)
}

// RequirePermission 校验当前会话角色是否具备全部指定权限点。
// 必须放在 RequireAuth 之后；角色权限解析失败按 500 处理（不静默降级为放行或 403）。
//
// 同时强制「必须改密」账号先完成改密：账号被管理员重置密码后
// （accounts.must_change_password = true），在完成自助改密前不得使用任何
// 需要权限点的后台功能，只有 /admin/auth 下的自助接口（改密 / 登出 / 会话管理）可访问。
func RequirePermission(resolver PermissionResolver, points ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			session := SessionFrom(r.Context())
			if session == nil {
				response.Unauthorized(w, r)
				return
			}
			if session.MustChangePassword {
				passwordChangeRequired(w, r)
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

// RequireAnyPermission 校验当前会话角色具备指定权限点中的任意一个。
//
// 用于「合并页」：例如统一账号管理同时接受 accounts.admins 与 accounts.users，
// 具体能查看 / 操作哪些账号由处理器按目标账号角色二次判定。
// 其余行为（必须改密拦截、解析失败按 500）与 RequirePermission 保持一致。
func RequireAnyPermission(resolver PermissionResolver, points ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			session := SessionFrom(r.Context())
			if session == nil {
				response.Unauthorized(w, r)
				return
			}
			if session.MustChangePassword {
				passwordChangeRequired(w, r)
				return
			}
			if resolver == nil || len(points) == 0 {
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
				if _, ok := have[need]; ok {
					next.ServeHTTP(w, r)
					return
				}
			}
			response.Forbidden(w, r)
		})
	}
}
