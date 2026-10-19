package handler

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"mckit/backend/internal/auth"
	"mckit/backend/internal/captcha"
	"mckit/backend/internal/domain"
	"mckit/backend/internal/http/middleware"
	"mckit/backend/internal/http/response"
	"mckit/backend/internal/service/user"
)

// UserHandler 普通用户账号与个人数据处理器（需求 §12）。
//
// 安全边界（统一账号迁移 0022 后）：
//   - 普通用户与后台账号同表（accounts），靠 role = 'user' 区分；
//     路由层用 RequireAuth + RequireUserRole 双重限制，管理员会话无法进入；
//   - 会话 Cookie 与后台共用（SESSION_COOKIE_NAME），不再有独立 Cookie；
//   - 写操作要求 CSRF（中间件校验会话绑定 token）；
//   - 个人数据一律以当前会话账号 ID 过滤，无越权入口；
//   - 登录失败按 (用户名, IP) 限流；注册与登录叠加全局限流；
//   - 注册要求图形验证码（captcha 为 nil 表示未启用，此时跳过校验）。
type UserHandler struct {
	svc     *user.Service
	captcha *captcha.Manager
	secure  bool
	// growth 成长事件接收端（可选注入；未注入时成长事件为空操作）。
	growth domain.GrowthEventSink
}

// NewUserHandler 创建用户处理器。
// Cookie 名统一取 middleware.SessionCookieName()（与鉴权中间件同一来源）；
// captcha 为 nil 表示未启用图形验证码（注册将不校验验证码）。
func NewUserHandler(svc *user.Service, captchaMgr *captcha.Manager, secure bool) *UserHandler {
	return &UserHandler{svc: svc, captcha: captchaMgr, secure: secure}
}

// WithGrowthSink 注入成长事件接收端（main 装配；链式调用）。
func (h *UserHandler) WithGrowthSink(sink domain.GrowthEventSink) *UserHandler {
	h.growth = sink
	return h
}

// maxUserBody 用户接口请求体上限（远小于上传类接口）。
const maxUserBody = 8 << 10

// ---- 请求体 ----

type userRegisterRequest struct {
	Username string `json:"username"`
	Nickname string `json:"nickname"`
	Email    string `json:"email"`
	Password string `json:"password"`
	// CaptchaID / CaptchaAnswer 图形验证码（未启用时忽略）。
	CaptchaID     string `json:"captcha_id"`
	CaptchaAnswer string `json:"captcha_answer"`
}

type userLoginRequest struct {
	// Identifier 登录标识：用户名或邮箱。
	Identifier string `json:"identifier"`
	// Username 兼容旧客户端字段；Identifier 为空时回退使用。
	Username string `json:"username"`
	Password string `json:"password"`
}

type userChangePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

// userProfileRequest 更新展示资料请求体（白名单字段）。
type userProfileRequest struct {
	Nickname  string `json:"nickname"`
	AvatarURL string `json:"avatar_url"`
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
	case errors.Is(err, user.ErrEmailTaken):
		response.Error(w, r, http.StatusConflict, "EMAIL_TAKEN", err.Error(), nil)
	case errors.Is(err, user.ErrBadUsername),
		errors.Is(err, user.ErrBadEmail),
		errors.Is(err, user.ErrBadNickname),
		errors.Is(err, user.ErrWeakPassword),
		errors.Is(err, user.ErrBadKind),
		errors.Is(err, user.ErrBadInput):
		response.Validation(w, r, []response.ErrorDetail{{Message: err.Error()}})
	case errors.Is(err, auth.ErrInvalidAvatar):
		response.Validation(w, r, []response.ErrorDetail{
			{Field: "avatar_url", Message: err.Error()},
		})
	default:
		response.HandleError(w, r, err)
	}
}

// setSessionCookie 种下会话 Cookie（与后台共用的统一会话）。
func (h *UserHandler) setSessionCookie(w http.ResponseWriter, sessionID string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.SessionCookieName(),
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  expiresAt,
	})
}

// clearSessionCookie 清除会话 Cookie。
func (h *UserHandler) clearSessionCookie(w http.ResponseWriter) {
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

// Captcha POST /api/v1/user/captcha
// 签发一张图形验证码：返回一次性 captcha_id 与 data URI 形式的 PNG 图片。
func (h *UserHandler) Captcha(w http.ResponseWriter, r *http.Request) {
	if h.captcha == nil {
		response.Error(w, r, http.StatusServiceUnavailable, "CAPTCHA_DISABLED", "验证码功能未启用", nil)
		return
	}
	res, err := h.captcha.Issue(r.Context())
	if err != nil {
		response.HandleError(w, r, err)
		return
	}
	response.OK(w, map[string]any{
		"captcha_id": res.ID,
		"image":      "data:image/png;base64," + base64.StdEncoding.EncodeToString(res.Image),
		"expires_in": int(res.ExpiresIn.Seconds()),
	})
}

// Register POST /api/v1/user/register
// 注册需通过图形验证码（启用时）；昵称与邮箱必填，邮箱全局唯一并可作为后续登录标识。
func (h *UserHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req userRegisterRequest
	if !decodeBody(w, r, &req) {
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if h.captcha != nil && !h.captcha.Verify(req.CaptchaID, req.CaptchaAnswer) {
		response.Error(w, r, http.StatusUnprocessableEntity, "CAPTCHA_INVALID",
			"验证码错误或已过期，请重新获取", nil)
		return
	}
	u, err := h.svc.Register(r.Context(), user.RegisterInput{
		Username: req.Username,
		Nickname: req.Nickname,
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		writeUserError(w, r, err)
		return
	}
	// 成长事件：注册成功（服务端事实确认后投递，subject 为账号自身，终身只发一次）。
	emitGrowth(r.Context(), h.growth, "user", domain.EventUserRegistered, u.ID.String(), "account", u.ID.String())
	response.OK(w, map[string]any{
		"id":         u.ID,
		"username":   u.Username,
		"nickname":   u.Nickname,
		"email":      u.Email,
		"avatar_url": u.AvatarURL,
	})
}

// Login POST /api/v1/user/login
// identifier 支持用户名或邮箱；username 为兼容字段。
func (h *UserHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req userLoginRequest
	if !decodeBody(w, r, &req) {
		return
	}
	identifier := strings.TrimSpace(req.Identifier)
	if identifier == "" {
		identifier = strings.TrimSpace(req.Username)
	}
	if identifier == "" || req.Password == "" {
		response.Validation(w, r, []response.ErrorDetail{
			{Field: "identifier", Message: "用户名或邮箱必填"},
			{Field: "password", Message: "密码必填"},
		})
		return
	}
	result, err := h.svc.Login(r.Context(), identifier, req.Password, clientIP(r))
	if err != nil {
		writeUserError(w, r, err)
		return
	}
	h.setSessionCookie(w, result.SessionID, result.ExpiresAt)
	// 成长事件：登录成功（每日登录奖励与「每日登录」任务由成长侧按业务日期幂等处理）。
	emitGrowth(r.Context(), h.growth, "user", domain.EventUserLoginSucceeded, result.User.ID.String(), "account", result.User.ID.String())
	response.OK(w, map[string]any{
		"id":                   result.User.ID,
		"username":             result.User.Username,
		"nickname":             result.User.Nickname,
		"email":                result.User.Email,
		"avatar_url":           result.User.AvatarURL,
		"must_change_password": result.User.MustChangePassword,
		"csrf_token":           result.CSRFToken,
		"expires_at":           result.ExpiresAt.Format(time.RFC3339),
	})
}

// Logout POST /api/v1/user/logout
func (h *UserHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if session := middleware.SessionFrom(r.Context()); session != nil {
		_ = h.svc.Logout(r.Context(), session.SessionID)
	}
	h.clearSessionCookie(w)
	response.NoContent(w)
}

// Me POST /api/v1/user/me
func (h *UserHandler) Me(w http.ResponseWriter, r *http.Request) {
	session := middleware.SessionFrom(r.Context())
	if session == nil {
		response.Unauthorized(w, r)
		return
	}
	// 昵称 / 邮箱随会话一并返回：会话只携带登录标识，展示资料需回查账号。
	out := map[string]any{
		"id":                   session.AccountID,
		"username":             session.Username,
		"must_change_password": session.MustChangePassword,
		"csrf_token":           session.CSRFToken,
	}
	if u, err := h.svc.FindByID(r.Context(), session.AccountID); err == nil && u != nil {
		out["nickname"] = u.Nickname
		out["email"] = u.Email
		out["avatar_url"] = u.AvatarURL
	}
	response.OK(w, out)
}

// UpdateProfile POST /api/v1/user/profile（CSRF）
// 自助维护展示资料：昵称与头像（头像只能上传图片，以 data URI 提交）。
func (h *UserHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	session := middleware.SessionFrom(r.Context())
	if session == nil {
		response.Unauthorized(w, r)
		return
	}
	var req userProfileRequest
	// 头像以 data URI 随请求体上传（前端已压缩），故上限放宽到 MaxAvatarDataURI + 余量。
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, auth.MaxAvatarDataURI+4096)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	u, err := h.svc.UpdateProfile(r.Context(), session.AccountID, req.Nickname, req.AvatarURL)
	if err != nil {
		writeUserError(w, r, err)
		return
	}
	// 成长事件：完善个人资料（subject 为账号自身，终身只发一次）。
	emitGrowth(r.Context(), h.growth, "user", domain.EventUserProfileCompleted, session.AccountID, "account", session.AccountID)
	response.OK(w, map[string]any{
		"id":         u.ID,
		"username":   u.Username,
		"nickname":   u.Nickname,
		"email":      u.Email,
		"avatar_url": u.AvatarURL,
	})
}

// ChangePassword POST /api/v1/user/change-password（CSRF）
func (h *UserHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	session := middleware.SessionFrom(r.Context())
	if session == nil {
		response.Unauthorized(w, r)
		return
	}
	var req userChangePasswordRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if err := h.svc.ChangePassword(r.Context(), session.AccountID, req.OldPassword, req.NewPassword); err != nil {
		writeUserError(w, r, err)
		return
	}
	// 改密后全部会话失效（含当前），清除 Cookie。
	h.clearSessionCookie(w)
	response.NoContent(w)
}

// DeleteAccount POST /api/v1/user/delete（CSRF，需密码二次确认）
func (h *UserHandler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	session := middleware.SessionFrom(r.Context())
	if session == nil {
		response.Unauthorized(w, r)
		return
	}
	var req userDeleteRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if err := h.svc.DeleteAccount(r.Context(), session.AccountID, req.Password); err != nil {
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
	// 成长事件：收藏资源（仅在「新收藏」时投递；取消收藏不奖励，
	// 成长侧以「账号 + 目标对象」为幂等键，取消后重收藏也不会重复发奖）。
	if favorited {
		emitGrowth(r.Context(), h.growth, "user", domain.EventResourceFavorited, userID, strings.ToLower(strings.TrimSpace(req.Kind)), strings.TrimSpace(req.ObjectID))
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
	session := middleware.SessionFrom(r.Context())
	if session == nil || session.AccountID == "" {
		response.Unauthorized(w, r)
		return "", false
	}
	return session.AccountID, true
}
