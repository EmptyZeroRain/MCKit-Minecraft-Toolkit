package auth

import (
	"context"
	"errors"
	"time"

	"mckit/backend/internal/domain"
	"mckit/backend/internal/service"
)

// Service 认证业务逻辑。
type Service struct {
	adminRepo service.AdminRepo
	sessions  SessionStore
	throttle  *LoginThrottle
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
	}
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
}

// ErrLocked 账号已被临时锁定。
var ErrLocked = errors.New("登录尝试次数过多，请稍后再试")

// ErrCredentials 用户名或密码错误。
var ErrCredentials = errors.New("用户名或密码错误")

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

	sessionID, err := RandomToken(32)
	if err != nil {
		return nil, err
	}
	csrfToken, err := RandomToken(32)
	if err != nil {
		return nil, err
	}

	expiresAt := time.Now().Add(sessionTTL)
	sess := Session{
		SessionID: sessionID,
		AdminID:   admin.ID.String(),
		Username:  admin.Username,
		Role:      admin.Role,
		CSRFToken: csrfToken,
		ExpiresAt: expiresAt,
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

// Touch 续期会话。
func (s *Service) Touch(ctx context.Context, sessionID string) error {
	return s.sessions.Touch(ctx, sessionID, sessionTTL)
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
	// 新密码最小长度校验
	if len(newPassword) < 8 {
		return errors.New("新密码长度至少 8 位")
	}
	newHash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	if err := s.adminRepo.ChangePassword(adminID, newHash); err != nil {
		return err
	}
	// 使该管理员所有会话失效：已登录的设备（含当前设备）必须用新密码重新登录。
	if err := s.sessions.DeleteByAdminID(ctx, adminID); err != nil {
		return err
	}
	return nil
}

// DeleteSessionsForAdmin 使指定管理员的全部会话失效。
// 供账号管理（停用、改角色、重置密码、删除）调用。
func (s *Service) DeleteSessionsForAdmin(ctx context.Context, adminID string) error {
	return s.sessions.DeleteByAdminID(ctx, adminID)
}