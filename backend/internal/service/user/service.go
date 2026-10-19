package user

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"mckit/backend/internal/auth"
	"mckit/backend/internal/domain"
)

// 个人数据容量与保留策略（见 Function.md「个人数据保留期限」）。
const (
	// MaxRecents 最近使用记录上限：超出按访问时间淘汰最旧。
	MaxRecents = 50
	// MaxFavorites 收藏上限：防止单账号无限增长。
	MaxFavorites = 500
	// MaxSeeds 常用种子上限。
	MaxSeeds = 200
	// MinPasswordLength 用户密码最小长度（管理员为 12，普通用户取 8）。
	MinPasswordLength = 8
	maxPasswordLength = 128
	// maxNameLen / maxSeedLen / maxNoteLen 字段长度上限。
	maxNameLen  = 120
	maxSeedLen  = 64
	maxNoteLen  = 500
	maxObjIDLen = 200
	maxLabelLen = 200
	maxURLLen   = 500
)

// 服务级错误（处理层据此映射状态码）。
var (
	ErrUsernameTaken = errors.New("用户名已被占用")
	ErrBadUsername   = errors.New("用户名需为 3-40 位，以字母开头，仅含字母、数字、下划线或连字符")
	ErrWeakPassword  = errors.New("密码长度需为 8-128 位")
	ErrBadKind       = errors.New("不支持的对象类型")
	ErrBadInput      = errors.New("输入不合法")
)

// usernameRe 用户名规则：字母开头，3-40 位，字母/数字/下划线/连字符。
var usernameRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{2,39}$`)

// validKinds 允许收藏 / 记录的对象类型。
var validKinds = map[string]bool{"tool": true, "item": true, "resource": true, "wiki": true, "potion": true, "villager": true, "mob": true, "redstone_design": true, "mod": true, "modpack": true}

// Service 普通用户业务逻辑。
type Service struct {
	repo     Repo
	sessions auth.SessionStore
	throttle *auth.LoginThrottle
}

// NewService 创建用户服务。maxLoginTries / loginWindowSeconds 控制登录限流。
func NewService(repo Repo, sessions auth.SessionStore, maxLoginTries, loginWindowSeconds int) *Service {
	if maxLoginTries <= 0 {
		maxLoginTries = 5
	}
	if loginWindowSeconds <= 0 {
		loginWindowSeconds = 900
	}
	return &Service{
		repo:     repo,
		sessions: sessions,
		throttle: auth.NewLoginThrottle(maxLoginTries, time.Duration(loginWindowSeconds)*time.Second),
	}
}

// LoginResult 登录成功返回的会话信息。
type LoginResult struct {
	SessionID string
	User      domain.User
	CSRFToken string
	ExpiresAt time.Time
	Remaining int
	Locked    bool
}

// sessionTTL 用户会话有效期（与管理员一致：72 小时）。
const sessionTTL = 72 * time.Hour

// Register 注册新普通用户（固定 role = 'user'，不授予任何后台权限）。
func (s *Service) Register(ctx context.Context, username, password string) (*domain.User, error) {
	username = strings.TrimSpace(username)
	if !usernameRe.MatchString(username) {
		return nil, ErrBadUsername
	}
	if len(password) < MinPasswordLength || len(password) > maxPasswordLength {
		return nil, ErrWeakPassword
	}
	existing, err := s.repo.FindByUsername(ctx, username)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrUsernameTaken
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return nil, err
	}
	u := &domain.User{
		Username:     username,
		PasswordHash: hash,
		Role:         domain.RoleUser,
		IsActive:     true,
	}
	if err := s.repo.Create(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

// Login 校验凭证并创建会话；错误统一返回 auth.ErrCredentials，不泄露具体原因。
func (s *Service) Login(ctx context.Context, username, password, ip string) (*LoginResult, error) {
	username = strings.TrimSpace(username)
	if s.throttle.IsLocked(username, ip) {
		return &LoginResult{Locked: true, Remaining: s.throttle.Remaining(username, ip)}, auth.ErrLocked
	}

	u, err := s.repo.FindByUsername(ctx, username)
	if err != nil {
		return nil, err
	}
	if u == nil || !u.IsActive || u.PasswordHash == "" {
		s.throttle.RecordFailure(username, ip)
		return &LoginResult{Remaining: s.throttle.Remaining(username, ip)}, auth.ErrCredentials
	}
	if err := auth.VerifyPassword(u.PasswordHash, password); err != nil {
		s.throttle.RecordFailure(username, ip)
		if errors.Is(err, auth.ErrInvalidPassword) {
			return &LoginResult{Remaining: s.throttle.Remaining(username, ip)}, auth.ErrCredentials
		}
		return nil, err
	}

	s.throttle.RecordSuccess(username, ip)

	sessionID, err := auth.RandomToken(32)
	if err != nil {
		return nil, err
	}
	csrfToken, err := auth.RandomToken(32)
	if err != nil {
		return nil, err
	}
	expiresAt := time.Now().Add(sessionTTL)
	sess := auth.Session{
		SessionID: sessionID,
		AccountID: u.ID.String(),
		Username:  u.Username,
		Role:      domain.RoleUser,
		CSRFToken: csrfToken,
		ExpiresAt: expiresAt,
	}
	if err := s.sessions.Create(ctx, sess); err != nil {
		return nil, err
	}
	_ = s.repo.UpdateLastLogin(ctx, u.ID.String())

	return &LoginResult{SessionID: sessionID, User: *u, CSRFToken: csrfToken, ExpiresAt: expiresAt}, nil
}

// GetSession 读取会话（供鉴权中间件使用）。
func (s *Service) GetSession(ctx context.Context, sessionID string) (*auth.Session, error) {
	return s.sessions.Get(ctx, sessionID)
}

// Logout 注销会话。
func (s *Service) Logout(ctx context.Context, sessionID string) error {
	return s.sessions.Delete(ctx, sessionID)
}

// ChangePassword 修改密码，成功后使该用户所有会话失效。
func (s *Service) ChangePassword(ctx context.Context, userID, oldPassword, newPassword string) error {
	u, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if u == nil {
		return auth.ErrCredentials
	}
	if err := auth.VerifyPassword(u.PasswordHash, oldPassword); err != nil {
		if errors.Is(err, auth.ErrInvalidPassword) {
			return auth.ErrCredentials
		}
		return err
	}
	if len(newPassword) < MinPasswordLength || len(newPassword) > maxPasswordLength {
		return ErrWeakPassword
	}
	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		return err
	}
	if err := s.repo.ChangePassword(ctx, userID, hash); err != nil {
		return err
	}
	return s.sessions.DeleteByAccountID(ctx, userID)
}

// DeleteAccount 删除账号及其全部个人数据（需密码二次确认）。
func (s *Service) DeleteAccount(ctx context.Context, userID, password string) error {
	u, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if u == nil {
		return auth.ErrCredentials
	}
	if err := auth.VerifyPassword(u.PasswordHash, password); err != nil {
		if errors.Is(err, auth.ErrInvalidPassword) {
			return auth.ErrCredentials
		}
		return err
	}
	if err := s.repo.DeleteUser(ctx, userID); err != nil {
		return err
	}
	return s.sessions.DeleteByAccountID(ctx, userID)
}

// ListFavorites 列出收藏。
func (s *Service) ListFavorites(ctx context.Context, userID string) ([]domain.UserFavorite, error) {
	out, err := s.repo.ListFavorites(ctx, userID)
	if out == nil {
		out = []domain.UserFavorite{}
	}
	return out, err
}

// ToggleFavorite 收藏 / 取消收藏，返回操作后的状态（true=已收藏）。
func (s *Service) ToggleFavorite(ctx context.Context, userID, kind, objectID, label, url string) (bool, error) {
	kind, objectID, label, url = normalizeObject(kind, objectID, label, url)
	if err := validateObject(kind, objectID, label, url); err != nil {
		return false, err
	}
	exists, err := s.repo.FavoriteExists(ctx, userID, kind, objectID)
	if err != nil {
		return false, err
	}
	if exists {
		return false, s.repo.DeleteFavorite(ctx, userID, kind, objectID)
	}
	f := &domain.UserFavorite{Kind: kind, ObjectID: objectID, Label: label, URL: url}
	if err := s.repo.UpsertFavorite(ctx, withUserID(f, userID)); err != nil {
		return false, err
	}
	return true, nil
}

// RemoveFavorite 取消收藏（幂等）。
func (s *Service) RemoveFavorite(ctx context.Context, userID, kind, objectID string) error {
	kind, objectID, _, _ = normalizeObject(kind, objectID, "", "")
	if err := validateObject(kind, objectID, "", ""); err != nil {
		return err
	}
	return s.repo.DeleteFavorite(ctx, userID, kind, objectID)
}

// ListRecents 列出最近使用。
func (s *Service) ListRecents(ctx context.Context, userID string) ([]domain.UserRecent, error) {
	out, err := s.repo.ListRecents(ctx, userID)
	if out == nil {
		out = []domain.UserRecent{}
	}
	return out, err
}

// RecordRecent 记录一次最近使用（幂等刷新），并按上限裁剪。
func (s *Service) RecordRecent(ctx context.Context, userID, kind, objectID, label, url string) error {
	kind, objectID, label, url = normalizeObject(kind, objectID, label, url)
	if err := validateObject(kind, objectID, label, url); err != nil {
		return err
	}
	rec := &domain.UserRecent{
		Kind: kind, ObjectID: objectID, Label: label, URL: url,
		// VisitedAt 由服务端统一写入：GORM 不识别该字段名，若不显式赋值会落成零值时间。
		VisitedAt: time.Now(),
	}
	if err := s.repo.UpsertRecent(ctx, withUserIDRecent(rec, userID)); err != nil {
		return err
	}
	return s.repo.PruneRecents(ctx, userID, MaxRecents)
}

// ClearRecents 清空最近使用。
func (s *Service) ClearRecents(ctx context.Context, userID string) error {
	return s.repo.ClearRecents(ctx, userID)
}

// ListSeeds 列出常用种子。
func (s *Service) ListSeeds(ctx context.Context, userID string) ([]domain.UserSeed, error) {
	out, err := s.repo.ListSeeds(ctx, userID)
	if out == nil {
		out = []domain.UserSeed{}
	}
	return out, err
}

// SaveSeed 新建或更新常用种子；seedID 为空表示新建。
func (s *Service) SaveSeed(ctx context.Context, userID, seedID, name, seed, note string) (*domain.UserSeed, error) {
	name = strings.TrimSpace(name)
	seed = strings.TrimSpace(seed)
	note = strings.TrimSpace(note)
	if name == "" || len(name) > maxNameLen {
		return nil, ErrBadInput
	}
	if seed == "" || len(seed) > maxSeedLen {
		return nil, ErrBadInput
	}
	if len(note) > maxNoteLen {
		return nil, ErrBadInput
	}
	if seedID == "" {
		rec := &domain.UserSeed{Name: name, Seed: seed, Note: note}
		if err := s.repo.CreateSeed(ctx, withUserIDSeed(rec, userID)); err != nil {
			return nil, err
		}
		return rec, nil
	}
	if err := s.repo.UpdateSeed(ctx, userID, seedID, name, seed, note); err != nil {
		return nil, err
	}
	// 回读以返回完整记录（含时间戳）；极端情况下（记录超出列表窗口）回退为构造值。
	if out, err := s.repo.ListSeeds(ctx, userID); err == nil {
		want := mustUUID(seedID)
		for i := range out {
			if out[i].ID == want {
				return &out[i], nil
			}
		}
	}
	return &domain.UserSeed{ID: mustUUID(seedID), Name: name, Seed: seed, Note: note}, nil
}

// DeleteSeed 删除常用种子。
func (s *Service) DeleteSeed(ctx context.Context, userID, seedID string) error {
	if strings.TrimSpace(seedID) == "" {
		return ErrBadInput
	}
	return s.repo.DeleteSeed(ctx, userID, seedID)
}

// normalizeObject 归一化对象字段（去空白、限长）。
func normalizeObject(kind, objectID, label, url string) (string, string, string, string) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	objectID = strings.TrimSpace(objectID)
	label = strings.TrimSpace(label)
	url = strings.TrimSpace(url)
	return kind, objectID, label, url
}

// validateObject 校验收藏 / 记录对象。
func validateObject(kind, objectID, label, url string) error {
	if !validKinds[kind] {
		return ErrBadKind
	}
	if objectID == "" || len(objectID) > maxObjIDLen {
		return ErrBadInput
	}
	if len(label) > maxLabelLen || len(url) > maxURLLen {
		return ErrBadInput
	}
	return nil
}

// mustUUID 解析 UUID；非法值返回 uuid.Nil（上层已校验来源为会话，正常不会出现）。
func mustUUID(s string) uuid.UUID {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil
	}
	return id
}

// withUserID / withUserIDRecent / withUserIDSeed 绑定归属用户，统一入口避免遗漏。
func withUserID(f *domain.UserFavorite, userID string) *domain.UserFavorite {
	f.UserID = mustUUID(userID)
	return f
}

func withUserIDRecent(r *domain.UserRecent, userID string) *domain.UserRecent {
	r.UserID = mustUUID(userID)
	return r
}

func withUserIDSeed(s *domain.UserSeed, userID string) *domain.UserSeed {
	s.UserID = mustUUID(userID)
	return s
}
