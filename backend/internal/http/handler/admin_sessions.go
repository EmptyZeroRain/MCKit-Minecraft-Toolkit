package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"mckit/backend/internal/audit"
	"mckit/backend/internal/auth"
	"mckit/backend/internal/domain"
	"mckit/backend/internal/http/middleware"
	"mckit/backend/internal/http/response"
)

// SessionAdminHandler 后台全站会话管理处理器。
//
// 能力：列出普通用户与管理员的有效会话；按展示指纹踢下线单个会话。
// 安全约束：
//   - 仅 ops.sessions 权限可访问；
//   - 不返回会话 ID 原文，只返回不可逆指纹；
//   - 不允许踢下线当前会话（避免误操作导致管理员自锁）；
//   - 每次踢下线写审计日志。
type SessionAdminHandler struct {
	svc   *auth.Service
	audit *audit.Service
}

// NewSessionAdminHandler 创建处理器。
func NewSessionAdminHandler(svc *auth.Service) *SessionAdminHandler {
	return &SessionAdminHandler{svc: svc}
}

// WithAudit 注入审计服务。
func (h *SessionAdminHandler) WithAudit(svc *audit.Service) *SessionAdminHandler {
	h.audit = svc
	return h
}

type adminSessionListRequest struct {
	Q           string `json:"q"`
	AccountType string `json:"account_type"`
	Page        int    `json:"page"`
	PageSize    int    `json:"page_size"`
}

type adminSessionRevokeRequest struct {
	ID string `json:"id"`
}

// adminSessionItem 后台会话列表项（不包含 SessionID 原文）。
type adminSessionItem struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	Role        string `json:"role"`
	AccountType string `json:"account_type"`
	CreatedAt   string `json:"created_at"`
	LastSeenAt  string `json:"last_seen_at"`
	ExpiresAt   string `json:"expires_at"`
	Current     bool   `json:"current"`
}

// List POST /api/v1/admin/sessions/list
func (h *SessionAdminHandler) List(w http.ResponseWriter, r *http.Request) {
	var req adminSessionListRequest
	_ = decodeOptionalBody(w, r, &req)
	accountType := strings.ToLower(strings.TrimSpace(req.AccountType))
	switch accountType {
	case "", auth.SessionAccountAll, auth.SessionAccountUser, auth.SessionAccountAdmin:
	default:
		response.Validation(w, r, []response.ErrorDetail{
			{Field: "account_type", Message: "account_type 只支持 all / user / admin"},
		})
		return
	}
	page, pageSize := normalizePage(req.Page, req.PageSize)
	sessions, total, err := h.svc.ListAllSessions(r.Context(), auth.SessionListFilter{
		Q:           strings.TrimSpace(req.Q),
		AccountType: accountType,
		Page:        page,
		PageSize:    pageSize,
	})
	if err != nil {
		if errors.Is(err, auth.ErrSessionStoreUnavailable) {
			response.ServiceUnavailable(w, r, "")
			return
		}
		response.HandleError(w, r, err)
		return
	}
	current := middleware.SessionFrom(r.Context())
	items := make([]adminSessionItem, 0, len(sessions))
	for _, sess := range sessions {
		items = append(items, adminSessionItem{
			ID:          auth.SessionFingerprint(sess.SessionID),
			Username:    sess.Username,
			Role:        sess.Role,
			AccountType: sessionAccountType(sess.Role),
			CreatedAt:   sess.CreatedAt.Format(time.RFC3339),
			LastSeenAt:  sess.LastSeenAt.Format(time.RFC3339),
			ExpiresAt:   sess.ExpiresAt.Format(time.RFC3339),
			Current:     current != nil && sess.SessionID == current.SessionID,
		})
	}
	writeGrowthPage(w, r, items, total, page, pageSize)
}

// Revoke POST /api/v1/admin/sessions/revoke
// 参数：id（列表返回的展示指纹）。踢下线后目标会话立即失效。
func (h *SessionAdminHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	var req adminSessionRevokeRequest
	if !decodeBody(w, r, &req) {
		return
	}
	fingerprint := strings.TrimSpace(req.ID)
	if fingerprint == "" {
		response.Validation(w, r, []response.ErrorDetail{
			{Field: "id", Message: "id 必填"},
		})
		return
	}
	current := middleware.SessionFrom(r.Context())
	keep := ""
	if current != nil {
		keep = current.SessionID
	}
	target, err := h.svc.RevokeSessionByFingerprint(r.Context(), fingerprint, keep)
	switch {
	case errors.Is(err, auth.ErrSessionNotFound):
		response.NotFound(w, r)
		return
	case errors.Is(err, auth.ErrCannotRevokeCurrent):
		response.Conflict(w, r, err.Error())
		return
	case errors.Is(err, auth.ErrSessionStoreUnavailable):
		response.ServiceUnavailable(w, r, "")
		return
	case err != nil:
		response.HandleError(w, r, err)
		return
	}
	if h.audit != nil {
		h.audit.Write(r, audit.Entry{
			ActorAdminID: actorID(r),
			Action:       "session.admin_revoke",
			ObjectType:   "session",
			ObjectID:     fingerprint,
			SourceIP:     clientIP(r),
			Result:       domain.AuditSuccess,
			After: map[string]any{
				"username":     target.Username,
				"role":         target.Role,
				"account_type": sessionAccountType(target.Role),
			},
		})
	}
	response.OK(w, map[string]any{"revoked": true})
}

func sessionAccountType(role string) string {
	if role == domain.RoleUser {
		return auth.SessionAccountUser
	}
	return auth.SessionAccountAdmin
}
