package user

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"mckit/backend/internal/accountid"
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
	ErrEmailTaken    = errors.New("该邮箱已被注册")
	ErrBadUsername   = errors.New("用户名需为 3-40 位，以字母开头，仅含字母、数字、下划线或连字符")
	ErrBadEmail      = errors.New("邮箱格式不正确")
	ErrBadNickname   = errors.New("昵称需为 1-60 个字符，且不能包含控制字符")
	// ErrWeakPassword 密码不满足策略（统一实现见 auth.ValidatePassword）。
	ErrWeakPassword = auth.ErrWeakPassword
	ErrBadKind      = errors.New("不支持的对象类型")
	ErrBadInput     = errors.New("输入不合法")
	// ErrAdminSelfDelete 后台账号不能在个人中心自助注销，避免绕过「至少保留一名
	// 启用 admin」的保护；必须由其他管理员在后台账号管理中删除。
	ErrAdminSelfDelete = errors.New("后台账号不能在个人中心注销，请由其他管理员在后台账号管理中删除")
)

// MaxNicknameLength 昵称长度上限（按 Unicode 字符计，与 accounts.nickname 列宽、
// 后台账号资料保持一致）。
const MaxNicknameLength = auth.MaxNicknameLength

// usernameRe 用户名规则：字母开头，3-40 位，字母/数字/下划线/连字符。
var usernameRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{2,39}$`)

// emailRe 邮箱格式（宽松但要求本地部分 + 至少一段域名 + 顶级域）；
// 长度另由 maxEmailLen 约束（对齐 RFC 5321 上限）。
var emailRe = regexp.MustCompile(`^[A-Za-z0-9._%+\-]+@([A-Za-z0-9\-]+\.)+[A-Za-z]{2,}$`)

// maxEmailLen 邮箱长度上限（对齐 accounts.email VARCHAR(254)）。
const maxEmailLen = 254

// validKinds 允许收藏 / 记录的对象类型。
var validKinds = map[string]bool{"tool": true, "item": true, "resource": true, "wiki": true, "potion": true, "villager": true, "mob": true, "redstone_design": true, "mod": true, "modpack": true}

// Service 普通用户业务逻辑。
type Service struct {
	repo     Repo
	sessions auth.SessionStore
	throttle *auth.LoginThrottle
	// policy 普通用户密码策略（默认 DefaultUserPolicy，可由装配层按配置覆盖）。
	policy auth.PasswordPolicy
	// adminPolicy 后台账号密码策略：后台账号使用前台自助改密时按此校验，
	// 避免管理员通过前台把密码改成低于后台要求的长度。
	adminPolicy auth.PasswordPolicy
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
		repo:        repo,
		sessions:    sessions,
		throttle:    auth.NewLoginThrottle(maxLoginTries, time.Duration(loginWindowSeconds)*time.Second),
		policy:      auth.DefaultUserPolicy(),
		adminPolicy: auth.DefaultAdminPolicy(),
	}
}

// WithPasswordPolicy 覆盖普通用户密码策略（装配层从配置注入）。
func (s *Service) WithPasswordPolicy(p auth.PasswordPolicy) *Service {
	s.policy = p
	return s
}

// WithAdminPasswordPolicy 覆盖后台账号密码策略（前台自助改密时按角色选择）。
func (s *Service) WithAdminPasswordPolicy(p auth.PasswordPolicy) *Service {
	s.adminPolicy = p
	return s
}

// policyFor 按账号角色返回密码策略：普通用户 8 位口径，后台账号沿用后台策略。
func (s *Service) policyFor(role string) auth.PasswordPolicy {
	if role == domain.RoleUser {
		return s.policy
	}
	return s.adminPolicy
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

// RegisterInput 注册入参。
type RegisterInput struct {
	Username string
	Nickname string
	Email    string
	Password string
}

// Register 注册新普通用户（固定 role = 'user'，不授予任何后台权限）。
// 用户名与邮箱均全局唯一；邮箱统一按小写存储，并作为可选的登录标识。
func (s *Service) Register(ctx context.Context, in RegisterInput) (*domain.User, error) {
	username := strings.TrimSpace(in.Username)
	if !usernameRe.MatchString(username) {
		return nil, ErrBadUsername
	}
	nickname, err := normalizeNickname(in.Nickname)
	if err != nil {
		return nil, err
	}
	email := normalizeEmail(in.Email)
	if len(email) > maxEmailLen || !emailRe.MatchString(email) {
		return nil, ErrBadEmail
	}
	if err := auth.ValidatePassword(in.Password, s.policy, username); err != nil {
		return nil, err
	}
	existing, err := s.repo.FindByUsername(ctx, username)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrUsernameTaken
	}
	byEmail, err := s.repo.FindByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if byEmail != nil {
		return nil, ErrEmailTaken
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return nil, err
	}
	userNo, err := accountid.Generate()
	if err != nil {
		return nil, err
	}
	u := &domain.User{
		Username:     username,
		UserNo:       userNo,
		Nickname:     nickname,
		Email:        email,
		PasswordHash: hash,
		Role:         domain.RoleUser,
		IsActive:     true,
	}
	if err := s.repo.Create(ctx, u); err != nil {
		// 并发注册可能同时通过前置检查，最终由数据库唯一约束兜底；
		// 此处把冲突映射为语义化错误，避免向用户暴露 500。
		return nil, mapCreateError(err)
	}
	return u, nil
}

// Login 校验凭证并创建会话；identifier 支持用户名或邮箱。
// 错误统一返回 auth.ErrCredentials，不泄露具体原因（账号是否存在、是用户名还是邮箱）。
func (s *Service) Login(ctx context.Context, identifier, password, ip string) (*LoginResult, error) {
	identifier = strings.TrimSpace(identifier)
	if s.throttle.IsLocked(identifier, ip) {
		return &LoginResult{Locked: true, Remaining: s.throttle.Remaining(identifier, ip)}, auth.ErrLocked
	}

	u, err := s.findAccount(ctx, identifier)
	if err != nil {
		return nil, err
	}
	if u == nil || !u.IsActive || u.PasswordHash == "" {
		s.throttle.RecordFailure(identifier, ip)
		return &LoginResult{Remaining: s.throttle.Remaining(identifier, ip)}, auth.ErrCredentials
	}
	if err := auth.VerifyPassword(u.PasswordHash, password); err != nil {
		s.throttle.RecordFailure(identifier, ip)
		if errors.Is(err, auth.ErrInvalidPassword) {
			return &LoginResult{Remaining: s.throttle.Remaining(identifier, ip)}, auth.ErrCredentials
		}
		return nil, err
	}
	// 前台登录只接受普通用户：后台账号必须走 /admin/auth/login，
	// 否则会绕过后台的 MFA 与登录审计。统一返回凭证错误，不泄露账号身份。
	// 放在密码校验之后，避免通过响应耗时差异枚举后台账号。
	if u.Role != domain.RoleUser {
		s.throttle.RecordFailure(identifier, ip)
		return &LoginResult{Remaining: s.throttle.Remaining(identifier, ip)}, auth.ErrCredentials
	}

	s.throttle.RecordSuccess(identifier, ip)

	sessionID, err := auth.RandomToken(32)
	if err != nil {
		return nil, err
	}
	csrfToken, err := auth.RandomToken(32)
	if err != nil {
		return nil, err
	}
	// 有效期策略由会话存储提供（与后台账号共用同一存储与策略）。
	expiresAt := time.Now().Add(s.sessions.TTL())
	sess := auth.Session{
		SessionID: sessionID,
		AccountID: u.ID.String(),
		Username:  u.Username,
		Role:      u.Role,
		CSRFToken: csrfToken,
		ExpiresAt: expiresAt,
	}
	if err := s.sessions.Create(ctx, sess); err != nil {
		return nil, err
	}
	_ = s.repo.UpdateLastLogin(ctx, u.ID.String())

	return &LoginResult{SessionID: sessionID, User: *u, CSRFToken: csrfToken, ExpiresAt: expiresAt}, nil
}

// FindByIdentifier 按用户名或邮箱查询账号（任意角色）。
//
// 仅供统一登录入口在密码校验前判定分流（普通用户 / 后台账号），
// 不承担鉴权职责；账号不存在时返回 (nil, nil)。
func (s *Service) FindByIdentifier(ctx context.Context, identifier string) (*domain.User, error) {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return nil, nil
	}
	return s.findAccount(ctx, identifier)
}

// GetSession 读取会话（供鉴权中间件使用）。
func (s *Service) GetSession(ctx context.Context, sessionID string) (*auth.Session, error) {
	return s.sessions.Get(ctx, sessionID)
}

// FindByID 读取账号（供展示资料回查，例如 /user/me 返回昵称与邮箱）。
func (s *Service) FindByID(ctx context.Context, userID string) (*domain.User, error) {
	return s.repo.FindByID(ctx, userID)
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
	// 按账号角色选择密码策略：后台账号从后台页面或前台页面改密都必须满足
	// 后台最小长度，避免借前台绕过更强的密码要求。
	if err := auth.ValidatePassword(newPassword, s.policyFor(u.Role), u.Username); err != nil {
		return err
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

// UpdateProfile 更新当前用户的展示资料（昵称 / 头像），返回更新后的账号。
// 昵称沿用注册口径（1-60 字符、不含控制字符）；头像只能上传图片（data URI，空串表示清除）。
func (s *Service) UpdateProfile(ctx context.Context, userID, nickname, avatar string) (*domain.User, error) {
	nick, err := normalizeNickname(nickname)
	if err != nil {
		return nil, err
	}
	// 头像只能上传图片：校验并归一化 data URI（空串表示清除）。
	avatarURL, err := auth.NormalizeAvatar(avatar)
	if err != nil {
		return nil, err
	}
	u, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, auth.ErrCredentials
	}
	if err := s.repo.UpdateProfile(ctx, userID, nick, avatarURL); err != nil {
		return nil, err
	}
	u.Nickname, u.AvatarURL = nick, avatarURL
	return u, nil
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
	// 后台账号不走自助注销：必须由其他管理员在后台处理，
	// 以保留「至少一名启用 admin」的最后防线。
	if u.Role != domain.RoleUser {
		return ErrAdminSelfDelete
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

// normalizeEmail 归一化邮箱：去空白 + 统一小写（与存储 / 唯一索引一致）。
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// normalizeNickname 归一化并校验昵称：去首尾空白，1-60 个字符，不含控制字符。
func normalizeNickname(nickname string) (string, error) {
	nickname = strings.TrimSpace(nickname)
	if nickname == "" || utf8.RuneCountInString(nickname) > MaxNicknameLength {
		return "", ErrBadNickname
	}
	for _, r := range nickname {
		if unicode.IsControl(r) {
			return "", ErrBadNickname
		}
	}
	return nickname, nil
}

// findAccount 按登录标识查账号：含 "@" 视为邮箱，否则视为用户名。
// 用户名规则不允许 "@"，因此该判别不存在歧义。
func (s *Service) findAccount(ctx context.Context, identifier string) (*domain.User, error) {
	if strings.Contains(identifier, "@") {
		return s.repo.FindByEmail(ctx, normalizeEmail(identifier))
	}
	return s.repo.FindByUsername(ctx, identifier)
}

// mapCreateError 将并发注册下的唯一约束冲突映射为语义化错误。
// 数据库是唯一性的最终权威：前置检查通过后仍可能并发撞车。
func mapCreateError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		switch {
		case strings.Contains(pgErr.ConstraintName, "email"):
			return ErrEmailTaken
		case strings.Contains(pgErr.ConstraintName, "username"):
			return ErrUsernameTaken
		}
	}
	return err
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
