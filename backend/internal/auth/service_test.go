package auth

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"mckit/backend/internal/domain"
)

// fakeAdminRepo 内存版管理员仓库，避免依赖数据库。
type fakeAdminRepo struct {
	byName    map[string]*domain.Admin
	byID      map[string]*domain.Admin
	lastLogin string
}

func newFakeRepo(admins ...*domain.Admin) *fakeAdminRepo {
	r := &fakeAdminRepo{byName: map[string]*domain.Admin{}, byID: map[string]*domain.Admin{}}
	for _, a := range admins {
		r.byName[a.Username] = a
		r.byID[a.ID.String()] = a
	}
	return r
}

func (r *fakeAdminRepo) FindByUsername(username string) (*domain.Admin, error) {
	return r.byName[username], nil
}

func (r *fakeAdminRepo) FindByID(id string) (*domain.Admin, error) {
	return r.byID[id], nil
}

func (r *fakeAdminRepo) UpdateLastLogin(adminID string) error {
	r.lastLogin = adminID
	return nil
}

func (r *fakeAdminRepo) ChangePassword(adminID, newHash string) error {
	if a, ok := r.byID[adminID]; ok {
		a.PasswordHash = newHash
	}
	return nil
}

func (r *fakeAdminRepo) UpdateProfile(adminID, nickname, avatarURL string) error {
	if a, ok := r.byID[adminID]; ok {
		a.Nickname = nickname
		a.AvatarURL = avatarURL
	}
	return nil
}

func mustHash(t *testing.T, pw string) string {
	t.Helper()
	h, err := HashPassword(pw)
	if err != nil {
		t.Fatalf("生成哈希失败: %v", err)
	}
	return h
}

func newAdmin(t *testing.T, username, password, role string) *domain.Admin {
	t.Helper()
	return &domain.Admin{
		ID:           uuid.New(),
		Username:     username,
		PasswordHash: mustHash(t, password),
		Role:         role,
		IsActive:     true,
	}
}

// TestLoginSuccess 覆盖需求 §6.1：登录成功创建会话与 CSRF token。
func TestLoginSuccess(t *testing.T) {
	admin := newAdmin(t, "admin", "correct-horse-battery", domain.RoleAdmin)
	repo := newFakeRepo(admin)
	store := NewMemorySessionStore()
	svc := NewService(repo, store, 5, 900)

	res, err := svc.Login(context.Background(), "admin", "correct-horse-battery", "203.0.113.1")
	if err != nil {
		t.Fatalf("登录应成功: %v", err)
	}
	if res.SessionID == "" || res.CSRFToken == "" {
		t.Fatal("应返回会话 ID 与 CSRF token")
	}
	if res.Admin.Role != domain.RoleAdmin {
		t.Fatalf("角色应为 admin，实际 %s", res.Admin.Role)
	}
	if repo.lastLogin != admin.ID.String() {
		t.Fatal("应记录最后登录时间")
	}

	sess, err := svc.GetSession(context.Background(), res.SessionID)
	if err != nil || sess == nil {
		t.Fatalf("会话应可读取: %v", err)
	}
	if sess.CSRFToken != res.CSRFToken {
		t.Fatal("会话 CSRF token 应与返回一致")
	}
}

// TestLoginWrongPassword 凭证错误统一返回 ErrCredentials，不泄露细节。
func TestLoginWrongPassword(t *testing.T) {
	admin := newAdmin(t, "admin", "correct", domain.RoleAdmin)
	svc := NewService(newFakeRepo(admin), NewMemorySessionStore(), 5, 900)

	_, err := svc.Login(context.Background(), "admin", "wrong", "203.0.113.1")
	if !errors.Is(err, ErrCredentials) {
		t.Fatalf("错误密码应返回 ErrCredentials，实际 %v", err)
	}

	// 不存在的用户名同样返回 ErrCredentials
	_, err = svc.Login(context.Background(), "ghost", "whatever", "203.0.113.2")
	if !errors.Is(err, ErrCredentials) {
		t.Fatalf("不存在用户应返回 ErrCredentials，实际 %v", err)
	}
}

// TestLoginThrottleLocks 覆盖需求 §6.1 / §13.1：登录失败限流防暴力破解。
func TestLoginThrottleLocks(t *testing.T) {
	admin := newAdmin(t, "admin", "correct", domain.RoleAdmin)
	svc := NewService(newFakeRepo(admin), NewMemorySessionStore(), 5, 900)

	for i := 0; i < 5; i++ {
		if _, err := svc.Login(context.Background(), "admin", "bad", "198.51.100.9"); !errors.Is(err, ErrCredentials) {
			t.Fatalf("第 %d 次失败应为 ErrCredentials，实际 %v", i+1, err)
		}
	}
	// 第 6 次应被锁定
	res, err := svc.Login(context.Background(), "admin", "correct", "198.51.100.9")
	if !errors.Is(err, ErrLocked) {
		t.Fatalf("达到阈值后应锁定，实际 %v", err)
	}
	if !res.Locked {
		t.Fatal("结果应标记 Locked")
	}

	// 其他 IP 不受影响（按 用户名|IP 隔离）
	if _, err := svc.Login(context.Background(), "admin", "correct", "198.51.100.10"); err != nil {
		t.Fatalf("不同 IP 不应被锁定: %v", err)
	}
}

// TestLogout 退出后会话失效。
func TestLogout(t *testing.T) {
	admin := newAdmin(t, "admin", "correct", domain.RoleAdmin)
	svc := NewService(newFakeRepo(admin), NewMemorySessionStore(), 5, 900)

	res, err := svc.Login(context.Background(), "admin", "correct", "203.0.113.1")
	if err != nil {
		t.Fatalf("登录失败: %v", err)
	}
	if err := svc.Logout(context.Background(), res.SessionID); err != nil {
		t.Fatalf("退出失败: %v", err)
	}
	sess, _ := svc.GetSession(context.Background(), res.SessionID)
	if sess != nil {
		t.Fatal("退出后会话应失效")
	}
}

// TestChangePassword 覆盖需求 §6.1：旧密码校验、长度校验与哈希更新。
func TestChangePassword(t *testing.T) {
	admin := newAdmin(t, "admin", "old-password", domain.RoleAdmin)
	repo := newFakeRepo(admin)
	svc := NewService(repo, NewMemorySessionStore(), 5, 900)
	ctx := context.Background()

	// 旧密码错误
	if err := svc.ChangePassword(ctx, admin.ID.String(), "wrong", "new-password-123"); !errors.Is(err, ErrCredentials) {
		t.Fatalf("旧密码错误应返回 ErrCredentials，实际 %v", err)
	}
	// 新密码过短
	if err := svc.ChangePassword(ctx, admin.ID.String(), "old-password", "short"); err == nil {
		t.Fatal("过短新密码应被拒绝")
	}
	// 正常修改
	if err := svc.ChangePassword(ctx, admin.ID.String(), "old-password", "new-password-123"); err != nil {
		t.Fatalf("修改密码失败: %v", err)
	}
	if err := VerifyPassword(admin.PasswordHash, "new-password-123"); err != nil {
		t.Fatalf("新密码应可校验通过: %v", err)
	}
	if err := VerifyPassword(admin.PasswordHash, "old-password"); !errors.Is(err, ErrInvalidPassword) {
		t.Fatal("旧密码不应再可用")
	}
}

// TestChangePasswordInvalidatesAllSessions 覆盖需求 §6.1：
// 改密成功后该管理员的全部会话（含其他设备）立即失效。
func TestChangePasswordInvalidatesAllSessions(t *testing.T) {
	admin := newAdmin(t, "admin", "old-password", domain.RoleAdmin)
	svc := NewService(newFakeRepo(admin), NewMemorySessionStore(), 5, 900)
	ctx := context.Background()

	first, err := svc.Login(ctx, "admin", "old-password", "203.0.113.1")
	if err != nil {
		t.Fatalf("首次登录失败: %v", err)
	}
	second, err := svc.Login(ctx, "admin", "old-password", "203.0.113.2")
	if err != nil {
		t.Fatalf("二次登录失败: %v", err)
	}

	if err := svc.ChangePassword(ctx, admin.ID.String(), "old-password", "brand-new-pass-123"); err != nil {
		t.Fatalf("改密失败: %v", err)
	}

	for name, sid := range map[string]string{"会话1": first.SessionID, "会话2": second.SessionID} {
		if sess, _ := svc.GetSession(ctx, sid); sess != nil {
			t.Fatalf("改密后 %s 应失效", name)
		}
	}
}

// TestDeleteSessionsForAdmin 覆盖账号停用/删除场景：按管理员批量清理会话。
func TestDeleteSessionsForAdmin(t *testing.T) {
	admin := newAdmin(t, "admin", "correct", domain.RoleAdmin)
	other := newAdmin(t, "editor", "correct", domain.RoleEditor)
	svc := NewService(newFakeRepo(admin, other), NewMemorySessionStore(), 5, 900)
	ctx := context.Background()

	s1, _ := svc.Login(ctx, "admin", "correct", "203.0.113.1")
	s2, _ := svc.Login(ctx, "admin", "correct", "203.0.113.2")
	e1, _ := svc.Login(ctx, "editor", "correct", "203.0.113.3")

	if err := svc.DeleteSessionsForAdmin(ctx, admin.ID.String()); err != nil {
		t.Fatalf("清理会话失败: %v", err)
	}
	if sess, _ := svc.GetSession(ctx, s1.SessionID); sess != nil {
		t.Fatal("admin 会话1 应被清理")
	}
	if sess, _ := svc.GetSession(ctx, s2.SessionID); sess != nil {
		t.Fatal("admin 会话2 应被清理")
	}
	if sess, _ := svc.GetSession(ctx, e1.SessionID); sess == nil {
		t.Fatal("其他管理员会话不应被清理")
	}
}

// tinyPNGDataURI 1x1 PNG 的合法头像 data URI（内容嗅探为 image/png）。
const tinyPNGDataURI = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

// TestUpdateProfile 覆盖「用户信息」自助维护：昵称 / 头像（仅图片）写入与校验。
func TestUpdateProfile(t *testing.T) {
	admin := newAdmin(t, "admin", "correct", domain.RoleAdmin)
	repo := newFakeRepo(admin)
	svc := NewService(repo, NewMemorySessionStore(), 5, 900)
	ctx := context.Background()

	// 正常写入（昵称含首尾空白应被裁剪）。
	updated, err := svc.UpdateProfile(ctx, admin.ID.String(), "  小明  ", tinyPNGDataURI)
	if err != nil {
		t.Fatalf("更新资料失败: %v", err)
	}
	if updated.Nickname != "小明" {
		t.Fatalf("昵称应被裁剪为「小明」，实际 %q", updated.Nickname)
	}
	if updated.AvatarURL != tinyPNGDataURI {
		t.Fatalf("头像 data URI 写入错误: %q", updated.AvatarURL)
	}
	// 回源读取应反映最新资料。
	got, err := svc.Profile(ctx, admin.ID.String())
	if err != nil || got == nil || got.Nickname != "小明" {
		t.Fatalf("Profile 应返回最新昵称，实际 %+v err=%v", got, err)
	}

	// 空昵称 / 空头像合法（表示清空，前端回退到 username）。
	if _, err := svc.UpdateProfile(ctx, admin.ID.String(), "", ""); err != nil {
		t.Fatalf("清空资料应被接受: %v", err)
	}

	// 昵称超长（按 Unicode 字符计）。
	long := strings.Repeat("字", MaxNicknameLength+1)
	if _, err := svc.UpdateProfile(ctx, admin.ID.String(), long, ""); !errors.Is(err, ErrInvalidNickname) {
		t.Fatalf("超长昵称应返回 ErrInvalidNickname，实际 %v", err)
	}
	// 头像只能上传图片：外链 URL、非图片 data URI、坏 base64 均应被拒绝。
	badAvatars := []string{
		"https://cdn.example.com/a.png",
		"javascript:alert(1)",
		"data:text/plain;base64,aGVsbG8=",
		"data:image/png;base64,bm90LWJhc2U2NA==",
		"data:image/png,notbase64",
	}
	for _, bad := range badAvatars {
		if _, err := svc.UpdateProfile(ctx, admin.ID.String(), "n", bad); !errors.Is(err, ErrInvalidAvatar) {
			t.Fatalf("非法头像 %q 应返回 ErrInvalidAvatar，实际 %v", bad, err)
		}
	}
}

// ===== 双因子认证（MFA）登录流程 =====

// fakeMFA 内存版 MFAProvider：票据 -> 账号，验证码固定为 123456。
type fakeMFA struct {
	enabled    bool
	configured bool
	tickets    map[string]uuid.UUID
	used       map[string]bool
	attempts   map[string]int
	code       string
}

func newFakeMFA(enabled bool) *fakeMFA {
	return &fakeMFA{
		enabled:    enabled,
		configured: true,
		tickets:    map[string]uuid.UUID{},
		used:       map[string]bool{},
		attempts:   map[string]int{},
		code:       "123456",
	}
}

func (f *fakeMFA) Configured() bool { return f.configured }

func (f *fakeMFA) IsEnabled(_ context.Context, _ uuid.UUID) (bool, error) {
	return f.enabled, nil
}

func (f *fakeMFA) CreateTicket(_ context.Context, accountID uuid.UUID) (string, error) {
	token := "ticket-" + accountID.String()[:8]
	f.tickets[token] = accountID
	return token, nil
}

func (f *fakeMFA) RedeemTicket(_ context.Context, token, code string) (uuid.UUID, error) {
	id, ok := f.tickets[token]
	if !ok || f.used[token] || f.attempts[token] >= 5 {
		return uuid.Nil, errTicketInvalidForTest
	}
	if code != f.code {
		f.attempts[token]++
		return uuid.Nil, errInvalidCodeForTest
	}
	f.used[token] = true
	return id, nil
}

// Verify 供解绑等自助操作复用（本测试不涉及）。
func (f *fakeMFA) Verify(_ context.Context, _ uuid.UUID, code string) error {
	if code != f.code {
		return errInvalidCodeForTest
	}
	return nil
}

var (
	errInvalidCodeForTest   = errors.New("验证码错误")
	errTicketInvalidForTest = errors.New("票据无效")
)

func TestLoginRequiresMFAWhenEnabled(t *testing.T) {
	admin := newAdmin(t, "admin", "correct-horse-battery", domain.RoleAdmin)
	repo := newFakeRepo(admin)
	store := NewMemorySessionStore()
	mfa := newFakeMFA(true)
	svc := NewService(repo, store, 5, 900).WithMFA(mfa)

	res, err := svc.Login(context.Background(), "admin", "correct-horse-battery", "203.0.113.1")
	if err != nil {
		t.Fatalf("密码正确时不应报错: %v", err)
	}
	if !res.MFARequired {
		t.Fatal("启用 MFA 的账号应要求第二因子")
	}
	if res.MFATicket == "" {
		t.Fatal("应返回第二因子票据")
	}
	if res.SessionID != "" {
		t.Fatal("第二因子通过前不得签发会话")
	}
	// 未建立会话：不应记录最后登录时间。
	if repo.lastLogin != "" {
		t.Fatal("第二因子通过前不应记录最后登录")
	}
}

func TestVerifyMFAIssuesSession(t *testing.T) {
	admin := newAdmin(t, "admin", "correct-horse-battery", domain.RoleAdmin)
	repo := newFakeRepo(admin)
	store := NewMemorySessionStore()
	mfa := newFakeMFA(true)
	svc := NewService(repo, store, 5, 900).WithMFA(mfa)

	first, err := svc.Login(context.Background(), "admin", "correct-horse-battery", "203.0.113.1")
	if err != nil {
		t.Fatal(err)
	}

	// 错误验证码不建立会话。
	if _, err := svc.VerifyMFA(context.Background(), first.MFATicket, "000000"); err == nil {
		t.Fatal("错误验证码应失败")
	}

	res, err := svc.VerifyMFA(context.Background(), first.MFATicket, "123456")
	if err != nil {
		t.Fatalf("正确验证码应成功: %v", err)
	}
	if res.SessionID == "" || res.CSRFToken == "" {
		t.Fatal("应签发会话与 CSRF token")
	}
	sess, err := svc.GetSession(context.Background(), res.SessionID)
	if err != nil || sess == nil {
		t.Fatalf("会话应可读取: %v", err)
	}
	if sess.AccountID != admin.ID.String() {
		t.Fatal("会话应归属该账号")
	}
}

func TestVerifyMFARejectsDisabledAccount(t *testing.T) {
	admin := newAdmin(t, "admin", "correct-horse-battery", domain.RoleAdmin)
	repo := newFakeRepo(admin)
	mfa := newFakeMFA(true)
	svc := NewService(repo, NewMemorySessionStore(), 5, 900).WithMFA(mfa)

	first, err := svc.Login(context.Background(), "admin", "correct-horse-battery", "203.0.113.1")
	if err != nil {
		t.Fatal(err)
	}
	// 票据有效期内账号被停用，必须拒绝发放会话。
	admin.IsActive = false
	if _, err := svc.VerifyMFA(context.Background(), first.MFATicket, "123456"); err == nil {
		t.Fatal("停用账号不应通过第二因子登录")
	}
}

func TestLoginRejectsMFAWhenServerNotConfigured(t *testing.T) {
	admin := newAdmin(t, "admin", "correct-horse-battery", domain.RoleAdmin)
	mfa := newFakeMFA(true)
	mfa.configured = false
	svc := NewService(newFakeRepo(admin), NewMemorySessionStore(), 5, 900).WithMFA(mfa)

	if _, err := svc.Login(context.Background(), "admin", "correct-horse-battery", "203.0.113.1"); !errors.Is(err, ErrMFANotConfigured) {
		t.Fatalf("未配置密钥时应拒绝登录（安全失败），实际 %v", err)
	}
}
