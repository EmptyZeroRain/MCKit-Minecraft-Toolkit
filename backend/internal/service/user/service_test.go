package user

import (
	"context"
	"errors"
	"testing"

	"mckit/backend/internal/auth"
)

// newTestService 构造带内存仓储与内存会话存储的服务。
func newTestService() (*Service, *MemoryRepo, auth.SessionStore) {
	repo := NewMemoryRepo()
	sessions := auth.NewMemorySessionStore()
	return NewService(repo, sessions, 5, 900), repo, sessions
}

func TestRegisterValidation(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()

	for _, name := range []string{"a", "1abc", "ab", "with space", "用户名"} {
		if _, err := svc.Register(ctx, name, "password123"); !errors.Is(err, ErrBadUsername) {
			t.Errorf("用户名 %q 应被拒绝，实际 %v", name, err)
		}
	}
	if _, err := svc.Register(ctx, "alice", "short"); !errors.Is(err, ErrWeakPassword) {
		t.Errorf("弱密码应被拒绝，实际 %v", err)
	}
	if _, err := svc.Register(ctx, "alice", "password123"); err != nil {
		t.Fatalf("合法注册失败: %v", err)
	}
	if _, err := svc.Register(ctx, "alice", "password123"); !errors.Is(err, ErrUsernameTaken) {
		t.Errorf("重复用户名应被拒绝，实际 %v", err)
	}
}

func TestLoginFlow(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()

	if _, err := svc.Register(ctx, "bob", "password123"); err != nil {
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
	u, _ := svc.Register(ctx, "carol", "password123")
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
	a, _ := svc.Register(ctx, "dave", "password123")
	b, _ := svc.Register(ctx, "erin", "password123")

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
	u, _ := svc.Register(ctx, "frank", "password123")
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
	u, _ := svc.Register(ctx, "grace", "password123")
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
	u, _ := svc.Register(ctx, "heidi", "password123")

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
	u, _ := svc.Register(ctx, "ivan", "password123")
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
