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
	mfasvc "mckit/backend/internal/service/mfa"
)

// AuthHandler 管理员认证与账号管理处理器（需求 §8.3）。
type AuthHandler struct {
	authService  *auth.Service
	sessionStore auth.SessionStore
	cfg          *config.Config
	secure       bool
	audit        *audit.Service
	perms        middleware.PermissionResolver
	// mfa 双因子认证（未配置时相关接口返回 503）。
	mfa *mfasvc.Service
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

// WithMFA 注入双因子认证服务。
func (h *AuthHandler) WithMFA(svc *mfasvc.Service) *AuthHandler {
	h.mfa = svc
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

	if result.MFARequired {
		// 密码已通过但尚未完成第二因子：不种 Cookie，只返回短期票据。
		if h.audit != nil {
			h.audit.Write(r, audit.Entry{
				ActorAdminID: result.Admin.ID.String(),
				Action:       "admin.login.mfa_challenge",
				ObjectType:   "admin",
				ObjectID:     result.Admin.ID.String(),
				SourceIP:     ip,
				Result:       domain.AuditSuccess,
			})
		}
		response.OK(w, map[string]any{
			"mfa_required": true,
			"mfa_ticket":   result.MFATicket,
		})
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

	h.respondLogin(w, r, result)
}

// respondLogin 种下会话 Cookie 并返回登录态（密码登录与 MFA 登录共用）。
func (h *AuthHandler) respondLogin(w http.ResponseWriter, r *http.Request, result *auth.LoginResult) {
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
		"id":                   result.Admin.ID,
		"user_no":              result.Admin.UserNo,
		"username":             result.Admin.Username,
		"nickname":             result.Admin.Nickname,
		"avatar_url":           result.Admin.AvatarURL,
		"role":                 result.Admin.Role,
		"must_change_password": result.Admin.MustChangePassword,
		"permissions":          h.permissionsFor(r.Context(), result.Admin.Role),
		"csrf_token":           result.CSRFToken,
		"expires_at":           result.ExpiresAt.Format(time.RFC3339),
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
	// 展示资料（昵称 / 头像）不缓存在会话中，实时回源以保证改资料后各端一致。
	// 回源失败不阻断会话恢复：退化为空串，前端回退到 username 展示。
	var nickname, avatarURL, userNo string
	if profile, err := h.authService.Profile(r.Context(), session.AccountID); err == nil && profile != nil {
		nickname = profile.Nickname
		avatarURL = profile.AvatarURL
		userNo = profile.UserNo
	}

	response.OK(w, map[string]any{
		"id":                   session.AccountID,
		"user_no":              userNo,
		"username":             session.Username,
		"nickname":             nickname,
		"avatar_url":           avatarURL,
		"role":                 session.Role,
		"must_change_password": session.MustChangePassword,
		"permissions":          h.permissionsFor(r.Context(), session.Role),
		"csrf_token":           session.CSRFToken,
	})
}

// updateProfileRequest 更新展示资料请求体（白名单字段）。
type updateProfileRequest struct {
	Nickname  string `json:"nickname"`
	AvatarURL string `json:"avatar_url"`
}

// UpdateProfile POST /api/v1/admin/auth/profile
// 更新当前后台账号的昵称与头像地址；仅后台账号可用（普通用户走用户侧接口）。
func (h *AuthHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	session := middleware.SessionFrom(r.Context())
	if !isAdminSide(session) {
		response.Unauthorized(w, r)
		return
	}

	// 头像以 data URI 随请求体上传（前端已压缩），故上限放宽到 MaxAvatarDataURI + 余量。
	var req updateProfileRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, auth.MaxAvatarDataURI+4096)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}

	admin, err := h.authService.UpdateProfile(r.Context(), session.AccountID, req.Nickname, req.AvatarURL)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrInvalidNickname):
			response.Validation(w, r, []response.ErrorDetail{
				{Field: "nickname", Message: err.Error()},
			})
		case errors.Is(err, auth.ErrInvalidAvatar):
			response.Validation(w, r, []response.ErrorDetail{
				{Field: "avatar_url", Message: err.Error()},
			})
		default:
			response.HandleError(w, r, err)
		}
		return
	}

	if h.audit != nil {
		h.audit.Write(r, audit.Entry{
			ActorAdminID: session.AccountID,
			Action:       "admin.update_profile",
			ObjectType:   "admin",
			ObjectID:     session.AccountID,
			SourceIP:     clientIP(r),
			Result:       domain.AuditSuccess,
		})
	}

	response.OK(w, map[string]any{
		"id":         admin.ID,
		"user_no":    admin.UserNo,
		"username":   admin.Username,
		"nickname":   admin.Nickname,
		"avatar_url": admin.AvatarURL,
		"role":       admin.Role,
	})
}

// changePasswordRequest 修改密码请求体。
type changePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

// changePasswordFailureReason 把改密错误映射成可审计的短标签（不含用户输入）。
func changePasswordFailureReason(err error) string {
	switch {
	case errors.Is(err, auth.ErrCredentials):
		return "old_password_mismatch"
	case errors.Is(err, auth.ErrWeakPassword):
		return "weak_password"
	default:
		return "internal_error"
	}
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
		// 改密失败属于安全事件：记录账号与来源 IP，不记录任何密码内容。
		if h.audit != nil {
			h.audit.Write(r, audit.Entry{
				ActorAdminID: session.AccountID,
				Action:       "admin.password.change",
				ObjectType:   "admin",
				ObjectID:     session.AccountID,
				Result:       domain.AuditFailure,
				After:        map[string]any{"reason": changePasswordFailureReason(err)},
			})
		}
		if errors.Is(err, auth.ErrCredentials) {
			response.Validation(w, r, []response.ErrorDetail{
				{Field: "old_password", Message: "旧密码不正确"},
			})
			return
		}
		if errors.Is(err, auth.ErrWeakPassword) {
			response.Validation(w, r, []response.ErrorDetail{
				{Field: "new_password", Message: err.Error()},
			})
			return
		}
		response.HandleError(w, r, err)
		return
	}

	// 改密成功：写安全审计（含强制改密标记的清除）。
	if h.audit != nil {
		h.audit.Write(r, audit.Entry{
			ActorAdminID: session.AccountID,
			Action:       "admin.password.change",
			ObjectType:   "admin",
			ObjectID:     session.AccountID,
			Result:       domain.AuditSuccess,
			After:        map[string]any{"forced": session.MustChangePassword},
		})
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

// sessionItem 会话列表项。
//
// 刻意不返回会话 ID 原文：会话 ID 就是 Cookie 中的 bearer 凭据，
// 回显到页面会扩大泄露面。这里只返回不可逆指纹，供用户辨认与「撤销其它会话」使用。
type sessionItem struct {
	ID         string `json:"id"`
	Role       string `json:"role"`
	CreatedAt  string `json:"created_at"`
	LastSeenAt string `json:"last_seen_at"`
	ExpiresAt  string `json:"expires_at"`
	Current    bool   `json:"current"`
}

// sessionFingerprint 会话 ID 的展示指纹（实现位于 auth 包，后台会话管理复用）。
func sessionFingerprint(sessionID string) string {
	return auth.SessionFingerprint(sessionID)
}

// Sessions POST /api/v1/admin/auth/sessions/list
// 列出当前后台账号的全部有效会话（当前会话标记 current = true）。
func (h *AuthHandler) Sessions(w http.ResponseWriter, r *http.Request) {
	session := middleware.SessionFrom(r.Context())
	if !isAdminSide(session) {
		response.Unauthorized(w, r)
		return
	}
	sessions, err := h.authService.ListSessions(r.Context(), session.AccountID)
	if err != nil {
		response.HandleError(w, r, err)
		return
	}
	items := make([]sessionItem, 0, len(sessions))
	for _, s := range sessions {
		items = append(items, sessionItem{
			ID:         sessionFingerprint(s.SessionID),
			Role:       s.Role,
			CreatedAt:  s.CreatedAt.Format(time.RFC3339),
			LastSeenAt: s.LastSeenAt.Format(time.RFC3339),
			ExpiresAt:  s.ExpiresAt.Format(time.RFC3339),
			Current:    s.SessionID == session.SessionID,
		})
	}
	response.OK(w, map[string]any{"sessions": items, "total": len(items)})
}

// revokeSessionsRequest 撤销会话请求体。
type revokeSessionsRequest struct {
	// Scope 撤销范围：others（默认，保留当前会话）或 all（含当前会话）。
	Scope string `json:"scope"`
}

// RevokeSessions POST /api/v1/admin/auth/sessions/revoke
// 撤销当前后台账号的会话（他人设备或全部设备）。
func (h *AuthHandler) RevokeSessions(w http.ResponseWriter, r *http.Request) {
	session := middleware.SessionFrom(r.Context())
	if !isAdminSide(session) {
		response.Unauthorized(w, r)
		return
	}
	var req revokeSessionsRequest
	// 允许空 body：默认 scope = others。
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req)
	}
	scope := strings.ToLower(strings.TrimSpace(req.Scope))
	if scope == "" {
		scope = "others"
	}

	var revoked int
	switch scope {
	case "others":
		n, err := h.authService.RevokeOtherSessions(r.Context(), session.AccountID, session.SessionID)
		if err != nil {
			response.HandleError(w, r, err)
			return
		}
		revoked = n
	case "all":
		if err := h.authService.RevokeAllSessions(r.Context(), session.AccountID); err != nil {
			response.HandleError(w, r, err)
			return
		}
		revoked = -1
	default:
		response.Validation(w, r, []response.ErrorDetail{
			{Field: "scope", Message: "scope 只支持 others 或 all"},
		})
		return
	}

	if h.audit != nil {
		h.audit.Write(r, audit.Entry{
			ActorAdminID: session.AccountID,
			Action:       "admin.revoke_sessions",
			ObjectType:   "admin",
			ObjectID:     session.AccountID,
			SourceIP:     clientIP(r),
			Result:       domain.AuditSuccess,
			After:        map[string]any{"scope": scope},
		})
	}

	// scope = all 时当前会话也已失效，清 Cookie 让前端回到登录页。
	if scope == "all" {
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
	}
	response.OK(w, map[string]any{"scope": scope, "revoked": revoked})
}
