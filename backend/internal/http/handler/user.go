package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"mckit/backend/internal/auth"
	"mckit/backend/internal/http/middleware"
	"mckit/backend/internal/http/response"
	"mckit/backend/internal/service/user"
)

// UserHandler 普通用户账号与个人数据处理器（需求 §12）。
//
// 安全边界：
//   - 与管理员完全分离，用户会话角色固定为 "user"，不参与任何后台鉴权；
//   - 写操作要求 CSRF（中间件校验会话绑定 token）；
//   - 个人数据一律以当前会话用户 ID 过滤，无越权入口；
//   - 登录失败按 (用户名, IP) 限流；注册与登录叠加全局限流。
type UserHandler struct {
	svc        *user.Service
	secure     bool
	cookieName string
}

// NewUserHandler 创建用户处理器。cookieName 为空时取默认值。
func NewUserHandler(svc *user.Service, secure bool, cookieName string) *UserHandler {
	if cookieName == "" {
		cookieName = "mc_user_session"
	}
	return &UserHandler{svc: svc, secure: secure, cookieName: cookieName}
}

// maxUserBody 用户接口请求体上限（远小于上传类接口）。
const maxUserBody = 8 << 10

// ---- 请求体 ----

type userRegisterRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type userLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type userChangePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

type userDeleteRequest struct {
	Password string `json:"password"`
}

type userFavoriteRequest struct {
	Kind     string `json:"kind"`
	ObjectID string `json:"object_id"`
	Label    string `json:"label"`
	URL      string `json:"url"`
}

type userObjectRequest struct {
	Kind     string `json:"kind"`
	ObjectID string `json:"object_id"`
}

type userSeedRequest struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Seed string `json:"seed"`
	Note string `json:"note"`
}

type userSeedDeleteRequest struct {
	ID string `json:"id"`
}

// decodeBody 解析 JSON 请求体（限制大小），失败返回 false 并已写响应。
func decodeBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxUserBody)).Decode(dst); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return false
	}
	return true
}

// writeUserError 统一映射用户服务错误为 HTTP 响应。
func writeUserError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, auth.ErrLocked):
		response.Error(w, r, http.StatusTooManyRequests, "LOGIN_LOCKED", "登录尝试次数过多，请稍后再试", nil)
	case errors.Is(err, auth.ErrCredentials):
		response.Unauthorized(w, r)
	case errors.Is(err, user.ErrUsernameTaken):
		response.Error(w, r, http.StatusConflict, "USERNAME_TAKEN", err.Error(), nil)
	case errors.Is(err, user.ErrBadUsername),
		errors.Is(err, user.ErrWeakPassword),
		errors.Is(err, user.ErrBadKind),
		errors.Is(err, user.ErrBadInput):
		response.Validation(w, r, []response.ErrorDetail{{Message: err.Error()}})
	default:
		response.HandleError(w, r, err)
	}
}

// setSessionCookie 种下用户会话 Cookie。
func (h *UserHandler) setSessionCookie(w http.ResponseWriter, sessionID string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     h.cookieName,
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  expiresAt,
	})
}

// clearSessionCookie 清除用户会话 Cookie。
func (h *UserHandler) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     h.cookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   h.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Expires:  time.Unix(1, 0),
	})
}

// Register POST /api/v1/user/register
func (h *UserHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req userRegisterRequest
	if !decodeBody(w, r, &req) {
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	u, err := h.svc.Register(r.Context(), req.Username, req.Password)
	if err != nil {
		writeUserError(w, r, err)
		return
	}
	response.OK(w, map[string]any{
		"id":       u.ID,
		"username": u.Username,
	})
}

// Login POST /api/v1/user/login
func (h *UserHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req userLoginRequest
	if !decodeBody(w, r, &req) {
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
	result, err := h.svc.Login(r.Context(), req.Username, req.Password, clientIP(r))
	if err != nil {
		writeUserError(w, r, err)
		return
	}
	h.setSessionCookie(w, result.SessionID, result.ExpiresAt)
	response.OK(w, map[string]any{
		"id":         result.User.ID,
		"username":   result.User.Username,
		"csrf_token": result.CSRFToken,
		"expires_at": result.ExpiresAt.Format(time.RFC3339),
	})
}

// Logout POST /api/v1/user/logout
func (h *UserHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if session := middleware.UserSessionFrom(r.Context()); session != nil {
		_ = h.svc.Logout(r.Context(), session.SessionID)
	}
	h.clearSessionCookie(w)
	response.NoContent(w)
}

// Me POST /api/v1/user/me
func (h *UserHandler) Me(w http.ResponseWriter, r *http.Request) {
	session := middleware.UserSessionFrom(r.Context())
	if session == nil {
		response.Unauthorized(w, r)
		return
	}
	response.OK(w, map[string]any{
		"id":         session.AdminID,
		"username":   session.Username,
		"csrf_token": session.CSRFToken,
	})
}

// ChangePassword POST /api/v1/user/change-password（CSRF）
func (h *UserHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	session := middleware.UserSessionFrom(r.Context())
	if session == nil {
		response.Unauthorized(w, r)
		return
	}
	var req userChangePasswordRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if err := h.svc.ChangePassword(r.Context(), session.AdminID, req.OldPassword, req.NewPassword); err != nil {
		writeUserError(w, r, err)
		return
	}
	// 改密后全部会话失效（含当前），清除 Cookie。
	h.clearSessionCookie(w)
	response.NoContent(w)
}

// DeleteAccount POST /api/v1/user/delete（CSRF，需密码二次确认）
func (h *UserHandler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	session := middleware.UserSessionFrom(r.Context())
	if session == nil {
		response.Unauthorized(w, r)
		return
	}
	var req userDeleteRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if err := h.svc.DeleteAccount(r.Context(), session.AdminID, req.Password); err != nil {
		writeUserError(w, r, err)
		return
	}
	h.clearSessionCookie(w)
	response.NoContent(w)
}

// ListFavorites POST /api/v1/user/favorites/list
func (h *UserHandler) ListFavorites(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUserID(w, r)
	if !ok {
		return
	}
	items, err := h.svc.ListFavorites(r.Context(), userID)
	if err != nil {
		response.HandleError(w, r, err)
		return
	}
	response.OK(w, map[string]any{"items": items, "total": len(items)})
}

// ToggleFavorite POST /api/v1/user/favorites/toggle（CSRF）
func (h *UserHandler) ToggleFavorite(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUserID(w, r)
	if !ok {
		return
	}
	var req userFavoriteRequest
	if !decodeBody(w, r, &req) {
		return
	}
	favorited, err := h.svc.ToggleFavorite(r.Context(), userID, req.Kind, req.ObjectID, req.Label, req.URL)
	if err != nil {
		writeUserError(w, r, err)
		return
	}
	response.OK(w, map[string]any{"favorited": favorited})
}

// RemoveFavorite POST /api/v1/user/favorites/remove（CSRF）
func (h *UserHandler) RemoveFavorite(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUserID(w, r)
	if !ok {
		return
	}
	var req userObjectRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if err := h.svc.RemoveFavorite(r.Context(), userID, req.Kind, req.ObjectID); err != nil {
		writeUserError(w, r, err)
		return
	}
	response.NoContent(w)
}

// ListRecents POST /api/v1/user/recents/list
func (h *UserHandler) ListRecents(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUserID(w, r)
	if !ok {
		return
	}
	items, err := h.svc.ListRecents(r.Context(), userID)
	if err != nil {
		response.HandleError(w, r, err)
		return
	}
	response.OK(w, map[string]any{"items": items, "total": len(items)})
}

// RecordRecent POST /api/v1/user/recents/record（CSRF）
func (h *UserHandler) RecordRecent(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUserID(w, r)
	if !ok {
		return
	}
	var req userFavoriteRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if err := h.svc.RecordRecent(r.Context(), userID, req.Kind, req.ObjectID, req.Label, req.URL); err != nil {
		writeUserError(w, r, err)
		return
	}
	response.NoContent(w)
}

// ClearRecents POST /api/v1/user/recents/clear（CSRF）
func (h *UserHandler) ClearRecents(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUserID(w, r)
	if !ok {
		return
	}
	if err := h.svc.ClearRecents(r.Context(), userID); err != nil {
		response.HandleError(w, r, err)
		return
	}
	response.NoContent(w)
}

// ListSeeds POST /api/v1/user/seeds/list
func (h *UserHandler) ListSeeds(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUserID(w, r)
	if !ok {
		return
	}
	items, err := h.svc.ListSeeds(r.Context(), userID)
	if err != nil {
		response.HandleError(w, r, err)
		return
	}
	response.OK(w, map[string]any{"items": items, "total": len(items)})
}

// SaveSeed POST /api/v1/user/seeds/save（CSRF）
func (h *UserHandler) SaveSeed(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUserID(w, r)
	if !ok {
		return
	}
	var req userSeedRequest
	if !decodeBody(w, r, &req) {
		return
	}
	seed, err := h.svc.SaveSeed(r.Context(), userID, req.ID, req.Name, req.Seed, req.Note)
	if err != nil {
		writeUserError(w, r, err)
		return
	}
	response.OK(w, seed)
}

// DeleteSeed POST /api/v1/user/seeds/delete（CSRF）
func (h *UserHandler) DeleteSeed(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUserID(w, r)
	if !ok {
		return
	}
	var req userSeedDeleteRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if err := h.svc.DeleteSeed(r.Context(), userID, req.ID); err != nil {
		writeUserError(w, r, err)
		return
	}
	response.NoContent(w)
}

// currentUserID 从会话读取用户 ID；缺失返回 401 并返回 ok=false。
func currentUserID(w http.ResponseWriter, r *http.Request) (string, bool) {
	session := middleware.UserSessionFrom(r.Context())
	if session == nil || session.AdminID == "" {
		response.Unauthorized(w, r)
		return "", false
	}
	return session.AdminID, true
}
