package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"mckit/backend/internal/domain"
	"mckit/backend/internal/service"
)

// Service 认证业务逻辑。
type Service struct {
	adminRepo service.AdminRepo
	sessions  SessionStore
	throttle  *LoginThrottle
	// policy 后台账号密码策略（默认 DefaultAdminPolicy，可由装配层按配置覆盖）。
	policy PasswordPolicy
	// mfa 双因子认证（可为 nil，表示启用 MFA 的账号无法登录后台）。
	mfa MFAProvider
}

// MFAProvider 双因子认证能力（由 internal/service/mfa 实现）。
//
// 在 auth 包内以接口形式声明，避免 auth 反向依赖具体 MFA 存储实现。
type MFAProvider interface {
	// IsEnabled 账号是否已启用 MFA。
	IsEnabled(ctx context.Context, accountID uuid.UUID) (bool, error)
	// CreateTicket 为「密码已通过」的账号签发第二因子登录票据。
	CreateTicket(ctx context.Context, accountID uuid.UUID) (string, error)
	// RedeemTicket 校验验证码并消费票据，返回账号 ID（成功前允许有限次重试）。
	RedeemTicket(ctx context.Context, token, code string) (uuid.UUID, error)
	// Verify 校验 TOTP 或恢复码。
	Verify(ctx context.Context, accountID uuid.UUID, code string) error
}

// WithMFA 注入双因子认证能力（装配层从配置决定是否启用）。
func (s *Service) WithMFA(m MFAProvider) *Service {
	s.mfa = m
	return s
}

// NewService 创建认证服务。maxLoginTries / loginWindowSeconds 控制登录限流。
func NewService(adminRepo service.AdminRepo, sessions SessionStore, maxLoginTries int, loginWindowSeconds int) *Service {
	if maxLoginTries <= 0 {
		maxLoginTries = 5
	}
	if loginWindowSeconds <= 0 {
		loginWindowSeconds = 900
	}
	return &Service{
		adminRepo: adminRepo,
		sessions:  sessions,
		throttle:  NewLoginThrottle(maxLoginTries, time.Duration(loginWindowSeconds)*time.Second),
		policy:    DefaultAdminPolicy(),
	}
}

// WithPasswordPolicy 覆盖后台账号密码策略（装配层从配置注入）。
func (s *Service) WithPasswordPolicy(p PasswordPolicy) *Service {
	s.policy = p
	return s
}

// LoginResult 登录成功返回的会话信息。
type LoginResult struct {
	SessionID string
	Admin     domain.Admin
	CSRFToken string
	ExpiresAt time.Time
	// Remaining 登录被限流时剩余可尝试次数。
	Remaining int
	Locked    bool
	// MFARequired 密码已通过，但还需要第二因子；此时不签发会话。
	MFARequired bool
	// MFATicket 第二因子票据（仅 MFARequired 时返回），提交到 VerifyMFA 使用。
	MFATicket string
}

// ErrLocked 账号已被临时锁定。
var ErrLocked = errors.New("登录尝试次数过多，请稍后再试")

// ErrMFANotConfigured 账号已启用 MFA，但服务端缺少加密密钥，无法校验。
//
// 这种情况下必须拒绝登录（安全失败），不能静默降级为单因子。
var ErrMFANotConfigured = errors.New("该账号已启用双因子认证，但服务端未配置 MFA 密钥，请联系管理员")

// ErrCredentials 用户名或密码错误。
var ErrCredentials = errors.New("用户名或密码错误")

// MaxNicknameLength 昵称最大长度（按 Unicode 字符计，与迁移列宽 60 对应）。
const MaxNicknameLength = 60

// ErrInvalidNickname 昵称不合法（超长）。
var ErrInvalidNickname = fmt.Errorf("昵称长度不能超过 %d 个字符", MaxNicknameLength)

// Login 校验凭证并创建会话。
// 用户名或密码错误时统一返回 ErrCredentials，不泄露具体是哪个错。
func (s *Service) Login(ctx context.Context, username, password, ip string) (*LoginResult, error) {
	if s.throttle.IsLocked(username, ip) {
		remaining := s.throttle.Remaining(username, ip)
		return &LoginResult{Locked: true, Remaining: remaining}, ErrLocked
	}

	admin, err := s.adminRepo.FindByUsername(username)
	if err != nil {
		return nil, err
	}
	if admin == nil || !admin.IsActive || admin.PasswordHash == "" {
		s.throttle.RecordFailure(username, ip)
		return &LoginResult{Remaining: s.throttle.Remaining(username, ip)}, ErrCredentials
	}

	if err := VerifyPassword(admin.PasswordHash, password); err != nil {
		s.throttle.RecordFailure(username, ip)
		if errors.Is(err, ErrInvalidPassword) {
			return &LoginResult{Remaining: s.throttle.Remaining(username, ip)}, ErrCredentials
		}
		// 存储哈希损坏属于服务端异常
		return nil, err
	}

	s.throttle.RecordSuccess(username, ip)

	// 已启用 MFA：先签发短期票据，等第二因子通过后再建立会话。
	if s.mfa != nil {
		enabled, err := s.mfa.IsEnabled(ctx, admin.ID)
		if err != nil {
			return nil, err
		}
		if enabled {
			// 已绑定但服务端缺密钥：安全失败，绝不降级为单因子。
			if !s.mfaConfigured() {
				return nil, ErrMFANotConfigured
			}
			ticket, err := s.mfa.CreateTicket(ctx, admin.ID)
			if err != nil {
				return nil, err
			}
			return &LoginResult{MFARequired: true, MFATicket: ticket, Admin: *admin}, nil
		}
	}

	return s.issueSession(ctx, admin)
}

// mfaConfigured 判断 MFA 提供方是否可用（未配置密钥时应拒绝启用 MFA 的账号登录）。
func (s *Service) mfaConfigured() bool {
	type configured interface{ Configured() bool }
	if c, ok := s.mfa.(configured); ok {
		return c.Configured()
	}
	return true
}

// VerifyMFA 使用第二因子票据 + 验证码完成登录。
//
// 票据允许有限次错误重试（MaxTicketAttempts），达到上限后作废；
// 由于每次登录都要先通过密码校验（受限流约束），6 位验证码无法被暴力枚举。
func (s *Service) VerifyMFA(ctx context.Context, ticket, code string) (*LoginResult, error) {
	if s.mfa == nil {
		return nil, ErrMFANotConfigured
	}
	accountID, err := s.mfa.RedeemTicket(ctx, ticket, code)
	if err != nil {
		return nil, err
	}

	admin, err := s.adminRepo.FindByID(accountID.String())
	if err != nil {
		return nil, err
	}
	// 票据有效期内账号可能被停用或删除，必须重新校验。
	if admin == nil || !admin.IsActive || admin.PasswordHash == "" {
		return nil, ErrCredentials
	}
	return s.issueSession(ctx, admin)
}

// issueSession 创建会话并返回登录结果（密码 / MFA 两条路径共用）。
func (s *Service) issueSession(ctx context.Context, admin *domain.Admin) (*LoginResult, error) {
	sessionID, err := RandomToken(32)
	if err != nil {
		return nil, err
	}
	csrfToken, err := RandomToken(32)
	if err != nil {
		return nil, err
	}

	// 有效期策略由会话存储提供（单实例内存 / 多实例共享存储装配时注入）。
	expiresAt := time.Now().Add(s.sessions.TTL())
	sess := Session{
		SessionID:          sessionID,
		AccountID:          admin.ID.String(),
		Username:           admin.Username,
		Role:               admin.Role,
		CSRFToken:          csrfToken,
		MustChangePassword: admin.MustChangePassword,
		ExpiresAt:          expiresAt,
	}
	if err := s.sessions.Create(ctx, sess); err != nil {
		return nil, err
	}

	// 记录最后登录时间；失败仅记录，不阻断登录。
	_ = s.adminRepo.UpdateLastLogin(admin.ID.String())

	return &LoginResult{
		SessionID: sessionID,
		Admin:     *admin,
		CSRFToken: csrfToken,
		ExpiresAt: expiresAt,
	}, nil
}

// GetSession 读取会话。
func (s *Service) GetSession(ctx context.Context, sessionID string) (*Session, error) {
	return s.sessions.Get(ctx, sessionID)
}

// Logout 注销会话。
func (s *Service) Logout(ctx context.Context, sessionID string) error {
	return s.sessions.Delete(ctx, sessionID)
}

// ListSessions 列出账号的全部有效会话（供「会话管理」展示与撤销）。
func (s *Service) ListSessions(ctx context.Context, accountID string) ([]Session, error) {
	return s.sessions.ListByAccountID(ctx, accountID)
}

// ListAllSessions 列出全站有效会话（后台会话管理，含普通用户与管理员）。
func (s *Service) ListAllSessions(ctx context.Context, f SessionListFilter) ([]Session, int64, error) {
	return s.sessions.ListAll(ctx, normalizeSessionListFilter(f))
}

// RevokeSessionByFingerprint 按展示指纹定位并删除单个会话。
//
// 不向前端暴露会话 ID 原文，撤销时逐页扫描有效会话比对指纹；该操作低频，
// 扫描成本可接受。keepSessionID 非空时拒绝撤销当前会话，避免管理员误踢自己。
func (s *Service) RevokeSessionByFingerprint(ctx context.Context, fingerprint, keepSessionID string) (*Session, error) {
	fingerprint = strings.ToLower(strings.TrimSpace(fingerprint))
	if len(fingerprint) != sessionFingerprintHexLength {
		return nil, ErrSessionNotFound
	}
	const scanPageSize = 100
	for page := 1; ; page++ {
		rows, total, err := s.sessions.ListAll(ctx, SessionListFilter{
			Page: page, PageSize: scanPageSize,
		})
		if err != nil {
			return nil, err
		}
		for i := range rows {
			sess := rows[i]
			if SessionFingerprint(sess.SessionID) != fingerprint {
				continue
			}
			if keepSessionID != "" && sess.SessionID == keepSessionID {
				return &sess, ErrCannotRevokeCurrent
			}
			if err := s.sessions.Delete(ctx, sess.SessionID); err != nil {
				return nil, err
			}
			return &sess, nil
		}
		if int64(page*scanPageSize) >= total {
			break
		}
	}
	return nil, ErrSessionNotFound
}

// RevokeOtherSessions 注销账号除 keepSessionID 外的全部会话，返回注销条数。
func (s *Service) RevokeOtherSessions(ctx context.Context, accountID, keepSessionID string) (int, error) {
	sessions, err := s.sessions.ListByAccountID(ctx, accountID)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, sess := range sessions {
		if sess.SessionID == keepSessionID {
			continue
		}
		if err := s.sessions.Delete(ctx, sess.SessionID); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// RevokeAllSessions 注销账号全部会话（账号恢复 / 强制下线）。
func (s *Service) RevokeAllSessions(ctx context.Context, accountID string) error {
	return s.sessions.DeleteByAccountID(ctx, accountID)
}

// VerifyPassword 校验指定账号的密码（用于敏感自助操作，如解绑 MFA）。
func (s *Service) VerifyPassword(ctx context.Context, adminID, password string) error {
	admin, err := s.adminRepo.FindByID(adminID)
	if err != nil {
		return err
	}
	if admin == nil || admin.PasswordHash == "" {
		return ErrCredentials
	}
	if err := VerifyPassword(admin.PasswordHash, password); err != nil {
		if errors.Is(err, ErrInvalidPassword) {
			return ErrCredentials
		}
		return err
	}
	return nil
}

// ChangePassword 修改当前管理员密码，成功后使该管理员所有会话失效。
func (s *Service) ChangePassword(ctx context.Context, adminID, oldPassword, newPassword string) error {
	admin, err := s.adminRepo.FindByID(adminID)
	if err != nil {
		return err
	}
	if admin == nil {
		return ErrCredentials
	}
	// 校验旧密码
	if err := VerifyPassword(admin.PasswordHash, oldPassword); err != nil {
		if errors.Is(err, ErrInvalidPassword) {
			return ErrCredentials
		}
		return err
	}
	// 新密码策略校验（统一实现，参数为后台账号策略）。
	if err := ValidatePassword(newPassword, s.policy, admin.Username); err != nil {
		return err
	}
	newHash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	if err := s.adminRepo.ChangePassword(adminID, newHash); err != nil {
		return err
	}
	// 使该管理员所有会话失效：已登录的设备（含当前设备）必须用新密码重新登录。
	if err := s.sessions.DeleteByAccountID(ctx, adminID); err != nil {
		return err
	}
	return nil
}

// Profile 读取账号展示资料（昵称 / 头像）。
// 会话中不缓存展示字段，避免改资料后各端数据不一致，故按 ID 实时回源。
func (s *Service) Profile(ctx context.Context, adminID string) (*domain.Admin, error) {
	return s.adminRepo.FindByID(adminID)
}

// UpdateProfile 更新当前后台账号的展示资料（昵称 / 头像），返回更新后的账号。
// 仅接受白名单字段；昵称超长、头像非合法图片数据 URI 时返回对应校验错误。
func (s *Service) UpdateProfile(ctx context.Context, adminID, nickname, avatar string) (*domain.Admin, error) {
	nickname = strings.TrimSpace(nickname)
	if utf8.RuneCountInString(nickname) > MaxNicknameLength {
		return nil, ErrInvalidNickname
	}
	// 头像只能上传图片：校验并归一化 data URI（空串表示清除）。
	avatarURL, err := NormalizeAvatar(avatar)
	if err != nil {
		return nil, err
	}

	admin, err := s.adminRepo.FindByID(adminID)
	if err != nil {
		return nil, err
	}
	if admin == nil {
		return nil, ErrCredentials
	}
	if err := s.adminRepo.UpdateProfile(adminID, nickname, avatarURL); err != nil {
		return nil, err
	}
	admin.Nickname = nickname
	admin.AvatarURL = avatarURL
	return admin, nil
}

// DeleteSessionsForAdmin 使指定管理员的全部会话失效。
// 供账号管理（停用、改角色、重置密码、删除）调用。
func (s *Service) DeleteSessionsForAdmin(ctx context.Context, adminID string) error {
	return s.sessions.DeleteByAccountID(ctx, adminID)
}
