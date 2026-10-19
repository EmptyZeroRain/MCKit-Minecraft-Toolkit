package user

import (
	"context"
	"errors"
	"strings"
	"testing"

	"mckit/backend/internal/auth"
	"mckit/backend/internal/domain"
)

// newTestService 构造带内存仓储与内存会话存储的服务。
func newTestService() (*Service, *MemoryRepo, auth.SessionStore) {
	repo := NewMemoryRepo()
	sessions := auth.NewMemorySessionStore()
	return NewService(repo, sessions, 5, 900), repo, sessions
}

// reg 便捷注册：昵称取用户名，邮箱取 <username>@example.com。
func reg(ctx context.Context, svc *Service, username, password string) (*domain.User, error) {
	return svc.Register(ctx, RegisterInput{
		Username: username,
		Nickname: username,
		Email:    username + "@example.com",
		Password: password,
	})
}

func TestRegisterValidation(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()

	for _, name := range []string{"a", "1abc", "ab", "with space", "用户名"} {
		if _, err := reg(ctx, svc, name, "password123"); !errors.Is(err, ErrBadUsername) {
			t.Errorf("用户名 %q 应被拒绝，实际 %v", name, err)
		}
	}
	if _, err := reg(ctx, svc, "alice", "short"); !errors.Is(err, ErrWeakPassword) {
		t.Errorf("弱密码应被拒绝，实际 %v", err)
	}
	if _, err := reg(ctx, svc, "alice", "password123"); err != nil {
		t.Fatalf("合法注册失败: %v", err)
	}
	if _, err := reg(ctx, svc, "alice", "password123"); !errors.Is(err, ErrUsernameTaken) {
		t.Errorf("重复用户名应被拒绝，实际 %v", err)
	}
}

func TestLoginFlow(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()

	if _, err := reg(ctx, svc, "bob", "password123"); err != nil {
		t.Fatalf("注册失败: %v", err)
	}

	res, err := svc.Login(ctx, "bob", "password123", "1.2.3.4")
	if err != nil {
		t.Fatalf("正确凭证登录失败: %v", err)
	}
	if res.SessionID == "" || res.CSRFToken == "" {
		t.Fatal("会话 ID / CSRF token 不应为空")
	}
	if res.User.Username != "bob" {
		t.Errorf("用户名不符: %s", res.User.Username)
	}

	if _, err := svc.Login(ctx, "bob", "wrong-password", "1.2.3.4"); !errors.Is(err, auth.ErrCredentials) {
		t.Errorf("错误密码应返回 ErrCredentials，实际 %v", err)
	}
	if _, err := svc.Login(ctx, "ghost", "password123", "1.2.3.4"); !errors.Is(err, auth.ErrCredentials) {
		t.Errorf("不存在用户应返回 ErrCredentials，实际 %v", err)
	}
}

func TestLoginLockout(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()

	// 连续失败达到阈值后应锁定（对不存在的用户不触发哈希校验，速度可控）。
	for i := 0; i < 5; i++ {
		_, _ = svc.Login(ctx, "ghost", "x", "9.9.9.9")
	}
	if _, err := svc.Login(ctx, "ghost", "x", "9.9.9.9"); !errors.Is(err, auth.ErrLocked) {
		t.Errorf("达到失败阈值应锁定，实际 %v", err)
	}
}

func TestToggleFavorite(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	u, _ := reg(ctx, svc, "carol", "password123")
	id := u.ID.String()

	added, err := svc.ToggleFavorite(ctx, id, "tool", "seed-tools", "种子工具", "/seed-tools")
	if err != nil || !added {
		t.Fatalf("首次收藏应成功: added=%v err=%v", added, err)
	}
	list, _ := svc.ListFavorites(ctx, id)
	if len(list) != 1 || list[0].ObjectID != "seed-tools" {
		t.Fatalf("收藏列表不符: %+v", list)
	}

	added, err = svc.ToggleFavorite(ctx, id, "tool", "seed-tools", "种子工具", "/seed-tools")
	if err != nil || added {
		t.Fatalf("再次收藏应取消: added=%v err=%v", added, err)
	}
	list, _ = svc.ListFavorites(ctx, id)
	if len(list) != 0 {
		t.Fatalf("取消后列表应为空: %+v", list)
	}

	// 非法类型应被拒绝。
	if _, err := svc.ToggleFavorite(ctx, id, "unknown", "x", "", ""); !errors.Is(err, ErrBadKind) {
		t.Errorf("非法类型应返回 ErrBadKind，实际 %v", err)
	}
}

func TestFavoritesIsolatedByUser(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	a, _ := reg(ctx, svc, "dave", "password123")
	b, _ := reg(ctx, svc, "erin", "password123")

	_, _ = svc.ToggleFavorite(ctx, a.ID.String(), "wiki", "redstone", "红石", "/wiki/redstone")

	la, _ := svc.ListFavorites(ctx, a.ID.String())
	lb, _ := svc.ListFavorites(ctx, b.ID.String())
	if len(la) != 1 || len(lb) != 0 {
		t.Fatalf("收藏应按用户隔离: a=%d b=%d", len(la), len(lb))
	}
}

func TestRecordRecentPrune(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	u, _ := reg(ctx, svc, "frank", "password123")
	id := u.ID.String()

	for i := 0; i < MaxRecents+5; i++ {
		objID := string(rune('a'+i%26)) + "-" + itoa(i)
		if err := svc.RecordRecent(ctx, id, "item", objID, "物品", "/items/"+objID); err != nil {
			t.Fatalf("记录最近使用失败: %v", err)
		}
	}
	list, _ := svc.ListRecents(ctx, id)
	if len(list) > MaxRecents {
		t.Fatalf("最近使用应裁剪到 %d 条，实际 %d", MaxRecents, len(list))
	}
	for _, item := range list {
		if item.VisitedAt.IsZero() {
			t.Fatal("访问时间不应为零值（GORM 不会自动维护 visited_at）")
		}
	}

	// 重复访问同一对象不重复计入。
	if err := svc.RecordRecent(ctx, id, "item", "a-0", "物品", "/items/a-0"); err != nil {
		t.Fatal(err)
	}
	before, _ := svc.ListRecents(ctx, id)
	if err := svc.RecordRecent(ctx, id, "item", "a-0", "物品", "/items/a-0"); err != nil {
		t.Fatal(err)
	}
	after, _ := svc.ListRecents(ctx, id)
	if len(before) != len(after) {
		t.Fatalf("重复记录不应增加条数: %d -> %d", len(before), len(after))
	}

	if err := svc.ClearRecents(ctx, id); err != nil {
		t.Fatal(err)
	}
	list, _ = svc.ListRecents(ctx, id)
	if len(list) != 0 {
		t.Fatalf("清空后应为空: %+v", list)
	}
}

func TestSeedCRUD(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	u, _ := reg(ctx, svc, "grace", "password123")
	id := u.ID.String()

	if _, err := svc.SaveSeed(ctx, id, "", "", "123", ""); !errors.Is(err, ErrBadInput) {
		t.Errorf("缺少名称应报错，实际 %v", err)
	}
	created, err := svc.SaveSeed(ctx, id, "", "我的世界", "9007199254740993", "备注")
	if err != nil {
		t.Fatalf("新建种子失败: %v", err)
	}
	if created.Seed != "9007199254740993" {
		t.Fatalf("大种子应原样保存: %s", created.Seed)
	}
	list, _ := svc.ListSeeds(ctx, id)
	if len(list) != 1 {
		t.Fatalf("种子列表不符: %+v", list)
	}

	if _, err := svc.SaveSeed(ctx, id, list[0].ID.String(), "改名", "42", ""); err != nil {
		t.Fatalf("更新种子失败: %v", err)
	}
	list, _ = svc.ListSeeds(ctx, id)
	if list[0].Name != "改名" || list[0].Seed != "42" {
		t.Fatalf("更新未生效: %+v", list[0])
	}

	if err := svc.DeleteSeed(ctx, id, list[0].ID.String()); err != nil {
		t.Fatal(err)
	}
	list, _ = svc.ListSeeds(ctx, id)
	if len(list) != 0 {
		t.Fatalf("删除后应为空: %+v", list)
	}
}

func TestChangePasswordInvalidatesSessions(t *testing.T) {
	svc, _, sessions := newTestService()
	ctx := context.Background()
	u, _ := reg(ctx, svc, "heidi", "password123")

	res, err := svc.Login(ctx, "heidi", "password123", "1.1.1.1")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ChangePassword(ctx, u.ID.String(), "password123", "newpassword123"); err != nil {
		t.Fatalf("改密失败: %v", err)
	}
	sess, _ := sessions.Get(ctx, res.SessionID)
	if sess != nil {
		t.Fatal("改密后旧会话应失效")
	}
	if _, err := svc.Login(ctx, "heidi", "newpassword123", "1.1.1.1"); err != nil {
		t.Fatalf("新密码应可登录: %v", err)
	}
}

func TestDeleteAccountCascades(t *testing.T) {
	svc, _, sessions := newTestService()
	ctx := context.Background()
	u, _ := reg(ctx, svc, "ivan", "password123")
	id := u.ID.String()

	res, _ := svc.Login(ctx, "ivan", "password123", "2.2.2.2")
	_, _ = svc.ToggleFavorite(ctx, id, "tool", "recipes", "配方", "/recipes")
	_ = svc.RecordRecent(ctx, id, "tool", "recipes", "配方", "/recipes")
	_, _ = svc.SaveSeed(ctx, id, "", "种子", "1", "")

	if err := svc.DeleteAccount(ctx, id, "wrong"); !errors.Is(err, auth.ErrCredentials) {
		t.Errorf("密码错误不应删除账号，实际 %v", err)
	}
	if err := svc.DeleteAccount(ctx, id, "password123"); err != nil {
		t.Fatalf("删除账号失败: %v", err)
	}

	if got, _ := svc.ListFavorites(ctx, id); len(got) != 0 {
		t.Errorf("删除账号应清除收藏: %+v", got)
	}
	if got, _ := svc.ListRecents(ctx, id); len(got) != 0 {
		t.Errorf("删除账号应清除最近使用: %+v", got)
	}
	if got, _ := svc.ListSeeds(ctx, id); len(got) != 0 {
		t.Errorf("删除账号应清除常用种子: %+v", got)
	}
	if _, err := svc.Login(ctx, "ivan", "password123", "2.2.2.2"); !errors.Is(err, auth.ErrCredentials) {
		t.Errorf("账号删除后不应能登录，实际 %v", err)
	}
	if sess, _ := sessions.Get(ctx, res.SessionID); sess != nil {
		t.Error("删除账号后会话应失效")
	}
}

// TestRegisterEmailValidation 邮箱格式、唯一性与归一化。
func TestRegisterEmailValidation(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()

	base := RegisterInput{Username: "zoe", Nickname: "佐伊", Email: "zoe@example.com", Password: "password123"}

	// 非法邮箱。
	for _, bad := range []string{"", "no-at", "a@b", "a@b.", "a b@c.com", "a@b..com"} {
		in := base
		in.Email = bad
		if _, err := svc.Register(ctx, in); !errors.Is(err, ErrBadEmail) {
			t.Errorf("邮箱 %q 应被拒绝，实际 %v", bad, err)
		}
	}
	// 超长邮箱（>254）。
	in := base
	in.Email = strings.Repeat("a", 250) + "@example.com"
	if _, err := svc.Register(ctx, in); !errors.Is(err, ErrBadEmail) {
		t.Errorf("超长邮箱应被拒绝，实际 %v", err)
	}

	// 合法注册后邮箱按小写存储。
	in = base
	in.Email = "Zoe@Example.COM"
	u, err := svc.Register(ctx, in)
	if err != nil {
		t.Fatalf("合法注册失败: %v", err)
	}
	if u.Email != "zoe@example.com" {
		t.Errorf("邮箱应归一化为小写，实际 %q", u.Email)
	}
	if u.Nickname != "佐伊" {
		t.Errorf("昵称不符: %q", u.Nickname)
	}

	// 邮箱重复（大小写不敏感）。
	dup := base
	dup.Username = "zoe2"
	dup.Email = "ZOE@example.com"
	if _, err := svc.Register(ctx, dup); !errors.Is(err, ErrEmailTaken) {
		t.Errorf("重复邮箱应被拒绝，实际 %v", err)
	}
}

// TestRegisterNicknameValidation 昵称必填、限长、拒绝控制字符。
func TestRegisterNicknameValidation(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()

	base := RegisterInput{Username: "nick", Nickname: "尼", Email: "nick@example.com", Password: "password123"}

	in := base
	in.Nickname = "   "
	if _, err := svc.Register(ctx, in); !errors.Is(err, ErrBadNickname) {
		t.Errorf("空昵称应被拒绝，实际 %v", err)
	}

	in = base
	in.Nickname = strings.Repeat("好", MaxNicknameLength+1)
	if _, err := svc.Register(ctx, in); !errors.Is(err, ErrBadNickname) {
		t.Errorf("超长昵称应被拒绝，实际 %v", err)
	}

	in = base
	in.Nickname = "坏\x00名"
	if _, err := svc.Register(ctx, in); !errors.Is(err, ErrBadNickname) {
		t.Errorf("含控制字符的昵称应被拒绝，实际 %v", err)
	}

	// 首尾空白被去除。
	in = base
	in.Nickname = "  昵称  "
	u, err := svc.Register(ctx, in)
	if err != nil {
		t.Fatalf("合法昵称注册失败: %v", err)
	}
	if u.Nickname != "昵称" {
		t.Errorf("昵称应去除首尾空白，实际 %q", u.Nickname)
	}
}

// TestLoginByIdentifier 用户名与邮箱均可登录；邮箱大小写不敏感。
func TestLoginByIdentifier(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()

	if _, err := svc.Register(ctx, RegisterInput{
		Username: "yuri", Nickname: "尤里", Email: "Yuri@Example.com", Password: "password123",
	}); err != nil {
		t.Fatalf("注册失败: %v", err)
	}

	for _, id := range []string{"yuri", "yuri@example.com", "YURI@EXAMPLE.COM"} {
		if _, err := svc.Login(ctx, id, "password123", "3.3.3.3"); err != nil {
			t.Errorf("以 %q 登录应成功，实际 %v", id, err)
		}
	}
	if _, err := svc.Login(ctx, "other@example.com", "password123", "3.3.3.3"); !errors.Is(err, auth.ErrCredentials) {
		t.Errorf("未知邮箱应返回 ErrCredentials，实际 %v", err)
	}
}

// TestUpdateProfile 覆盖用户侧「用户信息」自助维护：昵称 / 头像（仅图片）写入与校验。
func TestUpdateProfile(t *testing.T) {
	svc, repo, _ := newTestService()
	ctx := context.Background()
	u, err := reg(ctx, svc, "alice", "password123")
	if err != nil {
		t.Fatalf("注册失败: %v", err)
	}

	// 1x1 PNG 的合法头像 data URI（内容嗅探为 image/png）。
	const png = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

	updated, err := svc.UpdateProfile(ctx, u.ID.String(), "  爱丽丝  ", png)
	if err != nil {
		t.Fatalf("更新资料失败: %v", err)
	}
	if updated.Nickname != "爱丽丝" || updated.AvatarURL != png {
		t.Fatalf("更新结果不符: nickname=%q avatar=%q", updated.Nickname, updated.AvatarURL)
	}
	// 回源读取应反映最新资料。
	if got, _ := repo.FindByID(ctx, u.ID.String()); got == nil || got.Nickname != "爱丽丝" {
		t.Fatalf("仓储未持久化昵称: %+v", got)
	}

	// 昵称非法：空 / 超长。
	if _, err := svc.UpdateProfile(ctx, u.ID.String(), "", png); !errors.Is(err, ErrBadNickname) {
		t.Fatalf("空昵称应返回 ErrBadNickname，实际 %v", err)
	}
	long := strings.Repeat("字", MaxNicknameLength+1)
	if _, err := svc.UpdateProfile(ctx, u.ID.String(), long, png); !errors.Is(err, ErrBadNickname) {
		t.Fatalf("超长昵称应返回 ErrBadNickname，实际 %v", err)
	}

	// 头像只能上传图片：外链 URL、非图片 data URI 均被拒绝。
	for _, bad := range []string{
		"https://cdn.example.com/a.png",
		"data:text/plain;base64,aGVsbG8=",
	} {
		if _, err := svc.UpdateProfile(ctx, u.ID.String(), "n", bad); !errors.Is(err, auth.ErrInvalidAvatar) {
			t.Fatalf("非法头像 %q 应返回 ErrInvalidAvatar，实际 %v", bad, err)
		}
	}
}

// itoa 简单的非负整数转字符串，避免测试引入 strconv 之外的依赖。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
