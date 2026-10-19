package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"mckit/backend/internal/audit"
	"mckit/backend/internal/domain"
	"mckit/backend/internal/http/middleware"
	"mckit/backend/internal/http/response"
	mfasvc "mckit/backend/internal/service/mfa"
)

// maxMFABody 请求体上限：验证码与密码都很短，限制体积防止滥用。
const maxMFABody = 4096

// mfaVerifyRequest 第二因子校验请求。
type mfaVerifyRequest struct {
	// Ticket 密码校验后签发的短期票据。
	Ticket string `json:"ticket"`
	// Code 6 位 TOTP 或一次性恢复码。
	Code string `json:"code"`
}

// MFAVerify POST /api/v1/admin/auth/mfa/verify
//
// 公开接口（不需要已有会话）：凭证是「密码阶段签发的票据 + 第二因子」。
// 成功后才种下会话 Cookie，因此不经过 RequireAuth，但仍受全局限流保护。
func (h *AuthHandler) MFAVerify(w http.ResponseWriter, r *http.Request) {
	var req mfaVerifyRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxMFABody)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	req.Ticket = strings.TrimSpace(req.Ticket)
	req.Code = strings.TrimSpace(req.Code)
	if req.Ticket == "" || req.Code == "" {
		response.Validation(w, r, []response.ErrorDetail{
			{Field: "ticket", Message: "登录票据必填"},
			{Field: "code", Message: "验证码必填"},
		})
		return
	}

	result, err := h.authService.VerifyMFA(r.Context(), req.Ticket, req.Code)
	if err != nil {
		if h.audit != nil {
			h.audit.Write(r, audit.Entry{
				Action:     "admin.login.mfa",
				ObjectType: "admin",
				SourceIP:   clientIP(r),
				Result:     domain.AuditFailure,
			})
		}
		switch {
		case errors.Is(err, mfasvc.ErrTicketInvalid):
			response.Error(w, r, http.StatusUnauthorized, "MFA_TICKET_INVALID",
				"登录票据无效或已过期，请重新登录", nil)
		case errors.Is(err, mfasvc.ErrInvalidCode):
			response.Error(w, r, http.StatusUnauthorized, "MFA_INVALID_CODE",
				"验证码错误或已失效", nil)
		case errors.Is(err, mfasvc.ErrNotConfigured):
			response.Error(w, r, http.StatusServiceUnavailable, "MFA_NOT_CONFIGURED",
				"服务端未配置双因子认证密钥，请联系管理员", nil)
		default:
			response.HandleError(w, r, err)
		}
		return
	}

	if h.audit != nil {
		h.audit.Write(r, audit.Entry{
			ActorAdminID: result.Admin.ID.String(),
			Action:       "admin.login",
			ObjectType:   "admin",
			ObjectID:     result.Admin.ID.String(),
			SourceIP:     clientIP(r),
			Result:       domain.AuditSuccess,
		})
	}
	h.respondLogin(w, r, result)
}

// MFAStatus POST /api/v1/admin/auth/mfa/status
//
// 返回当前账号的 MFA 绑定状态（不含密钥材料）。
func (h *AuthHandler) MFAStatus(w http.ResponseWriter, r *http.Request) {
	session := middleware.SessionFrom(r.Context())
	if !isAdminSide(session) {
		response.Unauthorized(w, r)
		return
	}
	accountID, err := uuid.Parse(session.AccountID)
	if err != nil {
		response.HandleError(w, r, err)
		return
	}
	if h.mfa == nil {
		response.OK(w, mfasvc.Status{Configured: false})
		return
	}
	status, err := h.mfa.Status(r.Context(), accountID)
	if err != nil {
		response.HandleError(w, r, err)
		return
	}
	response.OK(w, status)
}

// MFASetup POST /api/v1/admin/auth/mfa/setup
//
// 生成新的 TOTP secret（待启用），返回 otpauth URI 供前端渲染二维码。
func (h *AuthHandler) MFASetup(w http.ResponseWriter, r *http.Request) {
	session := middleware.SessionFrom(r.Context())
	if !isAdminSide(session) {
		response.Unauthorized(w, r)
		return
	}
	if h.mfa == nil || !h.mfa.Configured() {
		response.Error(w, r, http.StatusServiceUnavailable, "MFA_NOT_CONFIGURED",
			"服务端未配置双因子认证密钥，暂不可绑定", nil)
		return
	}
	accountID, err := uuid.Parse(session.AccountID)
	if err != nil {
		response.HandleError(w, r, err)
		return
	}

	res, err := h.mfa.Setup(r.Context(), accountID, session.Username)
	if err != nil {
		switch {
		case errors.Is(err, mfasvc.ErrAlreadyEnabled):
			response.Error(w, r, http.StatusConflict, "MFA_ALREADY_ENABLED",
				"该账号已启用双因子认证，如需重新绑定请先解绑", nil)
		default:
			response.HandleError(w, r, err)
		}
		return
	}

	if h.audit != nil {
		h.audit.Write(r, audit.Entry{
			ActorAdminID: session.AccountID,
			Action:       "admin.mfa.setup",
			ObjectType:   "admin",
			ObjectID:     session.AccountID,
			SourceIP:     clientIP(r),
			Result:       domain.AuditSuccess,
		})
	}
	response.OK(w, res)
}

// mfaEnableRequest 激活绑定请求。
type mfaEnableRequest struct {
	Code string `json:"code"`
}

// MFAEnable POST /api/v1/admin/auth/mfa/enable
//
// 用首次验证码确认绑定，成功后一次性返回恢复码明文（仅此一次）。
func (h *AuthHandler) MFAEnable(w http.ResponseWriter, r *http.Request) {
	session := middleware.SessionFrom(r.Context())
	if !isAdminSide(session) {
		response.Unauthorized(w, r)
		return
	}
	if h.mfa == nil || !h.mfa.Configured() {
		response.Error(w, r, http.StatusServiceUnavailable, "MFA_NOT_CONFIGURED",
			"服务端未配置双因子认证密钥，暂不可绑定", nil)
		return
	}
	var req mfaEnableRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxMFABody)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	req.Code = strings.TrimSpace(req.Code)
	if req.Code == "" {
		response.Validation(w, r, []response.ErrorDetail{
			{Field: "code", Message: "验证码必填"},
		})
		return
	}
	accountID, err := uuid.Parse(session.AccountID)
	if err != nil {
		response.HandleError(w, r, err)
		return
	}

	codes, err := h.mfa.Enable(r.Context(), accountID, req.Code)
	if err != nil {
		switch {
		case errors.Is(err, mfasvc.ErrInvalidCode):
			response.Error(w, r, http.StatusUnprocessableEntity, "MFA_INVALID_CODE",
				"验证码错误，请确认认证器时间准确后重试", nil)
		case errors.Is(err, mfasvc.ErrAlreadyEnabled):
			response.Error(w, r, http.StatusConflict, "MFA_ALREADY_ENABLED",
				"该账号已启用双因子认证", nil)
		case errors.Is(err, mfasvc.ErrNotEnabled):
			response.Error(w, r, http.StatusConflict, "MFA_NOT_SETUP",
				"请先获取绑定二维码再提交验证码", nil)
		default:
			response.HandleError(w, r, err)
		}
		return
	}

	if h.audit != nil {
		h.audit.Write(r, audit.Entry{
			ActorAdminID: session.AccountID,
			Action:       "admin.mfa.enable",
			ObjectType:   "admin",
			ObjectID:     session.AccountID,
			SourceIP:     clientIP(r),
			Result:       domain.AuditSuccess,
		})
	}
	response.OK(w, map[string]any{"recovery_codes": codes})
}

// mfaDisableRequest 解绑请求。
type mfaDisableRequest struct {
	// Password 当前密码：防止会话被劫持后直接解绑。
	Password string `json:"password"`
	// Code 当前 TOTP 或恢复码：证明持有第二因子。
	Code string `json:"code"`
}

// MFADisable POST /api/v1/admin/auth/mfa/disable
//
// 解绑需要「密码 + 当前第二因子」双证据，避免仅凭被劫持的会话关闭 MFA。
func (h *AuthHandler) MFADisable(w http.ResponseWriter, r *http.Request) {
	session := middleware.SessionFrom(r.Context())
	if !isAdminSide(session) {
		response.Unauthorized(w, r)
		return
	}
	if h.mfa == nil {
		response.Error(w, r, http.StatusServiceUnavailable, "MFA_NOT_CONFIGURED",
			"服务端未配置双因子认证密钥", nil)
		return
	}
	var req mfaDisableRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxMFABody)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	if strings.TrimSpace(req.Password) == "" || strings.TrimSpace(req.Code) == "" {
		response.Validation(w, r, []response.ErrorDetail{
			{Field: "password", Message: "密码必填"},
			{Field: "code", Message: "验证码必填"},
		})
		return
	}
	accountID, err := uuid.Parse(session.AccountID)
	if err != nil {
		response.HandleError(w, r, err)
		return
	}

	if err := h.authService.VerifyPassword(r.Context(), session.AccountID, req.Password); err != nil {
		if h.audit != nil {
			h.audit.Write(r, audit.Entry{
				ActorAdminID: session.AccountID,
				Action:       "admin.mfa.disable",
				ObjectType:   "admin",
				ObjectID:     session.AccountID,
				SourceIP:     clientIP(r),
				Result:       domain.AuditFailure,
			})
		}
		response.Error(w, r, http.StatusUnauthorized, "PASSWORD_MISMATCH", "密码错误", nil)
		return
	}
	if err := h.mfa.Verify(r.Context(), accountID, req.Code); err != nil {
		response.Error(w, r, http.StatusUnprocessableEntity, "MFA_INVALID_CODE",
			"验证码错误或已失效", nil)
		return
	}
	if err := h.mfa.Disable(r.Context(), accountID); err != nil {
		response.HandleError(w, r, err)
		return
	}

	if h.audit != nil {
		h.audit.Write(r, audit.Entry{
			ActorAdminID: session.AccountID,
			Action:       "admin.mfa.disable",
			ObjectType:   "admin",
			ObjectID:     session.AccountID,
			SourceIP:     clientIP(r),
			Result:       domain.AuditSuccess,
		})
	}
	response.NoContent(w)
}
