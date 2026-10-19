// Package handler 提供 HTTP 处理器。
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"mckit/backend/internal/audit"
	"mckit/backend/internal/auth"
	"mckit/backend/internal/config"
	"mckit/backend/internal/domain"
	"mckit/backend/internal/http/middleware"
	"mckit/backend/internal/http/response"
)

// AuthHandler 管理员认证与账号管理处理器（需求 §8.3）。
type AuthHandler struct {
	authService  *auth.Service
	sessionStore auth.SessionStore
	cfg          *config.Config
	secure       bool
	audit        *audit.Service
	perms        middleware.PermissionResolver
}

// NewAuthHandler 创建认证处理器。
func NewAuthHandler(svc *auth.Service, sessions auth.SessionStore, cfg *config.Config) *AuthHandler {
	return &AuthHandler{
		authService:  svc,
		sessionStore: sessions,
		cfg:          cfg,
		secure:       cfg.SessionSecure,
	}
}

// WithAudit 注入审计服务（默认无审计记录）。
func (h *AuthHandler) WithAudit(a *audit.Service) *AuthHandler {
	h.audit = a
	return h
}

// WithPermissions 注入角色权限解析器，使登录 / 会话接口返回权限点集合。
// 前端据此显示后台入口；未注入时返回空集合，后端路由仍按权限兜底。
func (h *AuthHandler) WithPermissions(resolver middleware.PermissionResolver) *AuthHandler {
	h.perms = resolver
	return h
}

// permissionsFor 解析角色权限；解析器缺失或出错时返回空集合（不阻断登录）。
func (h *AuthHandler) permissionsFor(ctx context.Context, role string) []string {
	if h.perms == nil {
		return []string{}
	}
	perms, err := h.perms.PermissionsForRole(ctx, role)
	if err != nil || perms == nil {
		return []string{}
	}
	return perms
}

// isAdminSide 判断会话是否属于后台账号（统一账号后 role = 'user' 为普通用户）。
// 后台登录入口与后台自助接口据此拒绝普通用户会话，避免普通用户误入后台。
func isAdminSide(session *auth.Session) bool {
	return session != nil && session.Role != domain.RoleUser
}

// loginRequest 登录请求体。
type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Login POST /api/v1/admin/auth/login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		response.Validation(w, r, []response.ErrorDetail{
			{Field: "username", Message: "用户名必填"},
			{Field: "password", Message: "密码必填"},
		})
		return
	}

	ip := clientIP(r)
	result, err := h.authService.Login(r.Context(), req.Username, req.Password, ip)
	if err != nil {
		// 记录失败审计（不记录密码）。
		if h.audit != nil {
			h.audit.Write(r, audit.Entry{
				Action:     "admin.login",
				ObjectType: "admin",
				SourceIP:   ip,
				Result:     domain.AuditFailure,
				After:      map[string]any{"username": req.Username},
			})
		}
		if errors.Is(err, auth.ErrLocked) {
			response.Error(w, r, http.StatusTooManyRequests, "LOGIN_LOCKED",
				"登录尝试次数过多，请稍后再试", nil)
			return
		}
		if errors.Is(err, auth.ErrCredentials) {
			response.Unauthorized(w, r)
			return
		}
		response.HandleError(w, r, err)
		return
	}

	if h.audit != nil {
		h.audit.Write(r, audit.Entry{
			ActorAdminID: result.Admin.ID.String(),
			Action:       "admin.login",
			ObjectType:   "admin",
			ObjectID:     result.Admin.ID.String(),
			SourceIP:     ip,
			Result:       domain.AuditSuccess,
		})
	}

	http.SetCookie(w, &http.Cookie{
		Name:     middleware.SessionCookieName(),
		Value:    result.SessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  result.ExpiresAt,
	})

	response.OK(w, map[string]any{
		"id":          result.Admin.ID,
		"username":    result.Admin.Username,
		"role":        result.Admin.Role,
		"permissions": h.permissionsFor(r.Context(), result.Admin.Role),
		"csrf_token":  result.CSRFToken,
		"expires_at":  result.ExpiresAt.Format(time.RFC3339),
	})
}

// Logout POST /api/v1/admin/auth/logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(middleware.SessionCookieName()); err == nil && cookie.Value != "" {
		_ = h.authService.Logout(r.Context(), cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.SessionCookieName(),
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   h.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Expires:  time.Unix(1, 0),
	})
	response.NoContent(w)
}

// Me POST /api/v1/admin/auth/me
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	session := middleware.SessionFrom(r.Context())
	// 普通用户会话（role = 'user'）不属后台，返回 401 让前端视为未登录后台。
	if !isAdminSide(session) {
		response.Unauthorized(w, r)
		return
	}
	response.OK(w, map[string]any{
		"id":          session.AccountID,
		"username":    session.Username,
		"role":        session.Role,
		"permissions": h.permissionsFor(r.Context(), session.Role),
		"csrf_token":  session.CSRFToken,
	})
}

// changePasswordRequest 修改密码请求体。
type changePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

// ChangePassword POST /api/v1/admin/auth/change-password
func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	session := middleware.SessionFrom(r.Context())
	// 后台自助改密仅对后台账号开放（普通用户走 /user/change-password）。
	if !isAdminSide(session) {
		response.Unauthorized(w, r)
		return
	}

	var req changePasswordRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	if req.OldPassword == "" || req.NewPassword == "" {
		response.Validation(w, r, []response.ErrorDetail{
			{Field: "old_password", Message: "旧密码必填"},
			{Field: "new_password", Message: "新密码必填"},
		})
		return
	}

	if err := h.authService.ChangePassword(r.Context(), session.AccountID, req.OldPassword, req.NewPassword); err != nil {
		if errors.Is(err, auth.ErrCredentials) {
			response.Validation(w, r, []response.ErrorDetail{
				{Field: "old_password", Message: "旧密码不正确"},
			})
			return
		}
		if strings.Contains(err.Error(), "至少 8 位") {
			response.Validation(w, r, []response.ErrorDetail{
				{Field: "new_password", Message: err.Error()},
			})
			return
		}
		response.HandleError(w, r, err)
		return
	}

	// 修改成功后注销当前会话，强制重新登录。
	if cookie, err := r.Cookie(middleware.SessionCookieName()); err == nil && cookie.Value != "" {
		_ = h.authService.Logout(r.Context(), cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.SessionCookieName(),
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   h.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Expires:  time.Unix(1, 0),
	})
	response.NoContent(w)
}

// clientIP 提取客户端 IP（限流与审计用）。
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return host
}
