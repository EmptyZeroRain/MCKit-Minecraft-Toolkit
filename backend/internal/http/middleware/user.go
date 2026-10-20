package middleware

import (
	"net/http"

	"mckit/backend/internal/http/response"
)

// RequireAccount 在共享会话之上要求当前请求已登录（任意账号角色）。
//
// 统一账号（迁移 0022）后管理员与普通用户共用同一个会话 Cookie，且管理员
// 同样拥有前台能力（收藏、最近使用、常用种子、社区发帖、成长中心）。
// 因此前台接口不再限制 role = 'user'，只要求存在有效会话：
//   - 未登录 → 401（前端据此视为未登录）；
//   - 普通用户 / 后台账号 → 放行，个人数据一律按会话账号 ID 过滤。
//
// 后台接口仍由 RequirePermission 按权限点判定，普通用户不含任何权限点，自然 403。
//
// 必须放在 RequireAuth 之后（依赖会话已写入 context）。
func RequireAccount() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if SessionFrom(r.Context()) == nil {
				response.Unauthorized(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
