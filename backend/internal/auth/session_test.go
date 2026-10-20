package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMemorySessionStoreTTL(t *testing.T) {
	store := NewMemorySessionStoreWithOptions(SessionOptions{TTL: time.Hour})
	if store.TTL() != time.Hour {
		t.Fatalf("TTL 应为 1h，实际 %v", store.TTL())
	}
	ctx := context.Background()

	if err := store.Create(ctx, Session{SessionID: "s1", AccountID: "acc-1"}); err != nil {
		t.Fatalf("Create 失败: %v", err)
	}
	got, err := store.Get(ctx, "s1")
	if err != nil || got == nil {
		t.Fatalf("Get 应命中: got=%v err=%v", got, err)
	}
	// Create 应自动填充派生字段（CreatedAt / LastSeenAt / ExpiresAt）。
	if got.CreatedAt.IsZero() || got.LastSeenAt.IsZero() || got.ExpiresAt.IsZero() {
		t.Fatalf("派生时间字段未填充: %+v", got)
	}
	if got.ExpiresAt.Sub(got.CreatedAt) != time.Hour {
		t.Fatalf("绝对有效期应为 1h，实际 %v", got.ExpiresAt.Sub(got.CreatedAt))
	}
}

func TestMemorySessionStoreAbsoluteExpiry(t *testing.T) {
	store := NewMemorySessionStore()
	ctx := context.Background()
	// 显式给出已过期的 ExpiresAt：Get 应视为失效并删除。
	if err := store.Create(ctx, Session{
		SessionID: "expired",
		AccountID: "acc-1",
		ExpiresAt: time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("Create 失败: %v", err)
	}
	got, err := store.Get(ctx, "expired")
	if err != nil || got != nil {
		t.Fatalf("过期会话应返回 nil: got=%v err=%v", got, err)
	}
	if n, _ := store.PurgeExpired(ctx); n != 0 {
		t.Fatalf("过期会话已被 Get 清理，PurgeExpired 应返回 0，实际 %d", n)
	}
}

func TestMemorySessionStoreIdleTimeout(t *testing.T) {
	store := NewMemorySessionStoreWithOptions(SessionOptions{
		TTL:     time.Hour,
		IdleTTL: 30 * time.Millisecond,
	})
	ctx := context.Background()
	if err := store.Create(ctx, Session{SessionID: "idle", AccountID: "acc-1"}); err != nil {
		t.Fatalf("Create 失败: %v", err)
	}
	time.Sleep(60 * time.Millisecond)
	got, err := store.Get(ctx, "idle")
	if err != nil || got != nil {
		t.Fatalf("空闲超时应返回 nil: got=%v err=%v", got, err)
	}
}

func TestMemorySessionStoreListAndRevoke(t *testing.T) {
	store := NewMemorySessionStore()
	ctx := context.Background()
	for _, id := range []string{"a", "b", "c"} {
		if err := store.Create(ctx, Session{SessionID: id, AccountID: "acc-1"}); err != nil {
			t.Fatalf("Create 失败: %v", err)
		}
	}
	// 其它账号的会话不应出现在列表中。
	if err := store.Create(ctx, Session{SessionID: "other", AccountID: "acc-2"}); err != nil {
		t.Fatalf("Create 失败: %v", err)
	}

	list, err := store.ListByAccountID(ctx, "acc-1")
	if err != nil {
		t.Fatalf("ListByAccountID 失败: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("应列出 3 个会话，实际 %d", len(list))
	}

	if err := store.DeleteByAccountID(ctx, "acc-1"); err != nil {
		t.Fatalf("DeleteByAccountID 失败: %v", err)
	}
	if list, _ := store.ListByAccountID(ctx, "acc-1"); len(list) != 0 {
		t.Fatalf("撤销后应无会话，实际 %d", len(list))
	}
	// 其它账号不受影响。
	if got, _ := store.Get(ctx, "other"); got == nil {
		t.Fatal("其它账号会话不应被撤销")
	}
}

func TestMemorySessionStorePurgeExpired(t *testing.T) {
	store := NewMemorySessionStore()
	ctx := context.Background()
	_ = store.Create(ctx, Session{SessionID: "live", AccountID: "acc-1"})
	_ = store.Create(ctx, Session{SessionID: "dead", AccountID: "acc-1", ExpiresAt: time.Now().Add(-time.Second)})
	n, err := store.PurgeExpired(ctx)
	if err != nil {
		t.Fatalf("PurgeExpired 失败: %v", err)
	}
	if n != 1 {
		t.Fatalf("应清理 1 个过期会话，实际 %d", n)
	}
}

func TestMemorySessionStoreListAllFilter(t *testing.T) {
	store := NewMemorySessionStore()
	ctx := context.Background()
	rows := []Session{
		{SessionID: "u1", AccountID: "u", Username: "alice", Role: "user"},
		{SessionID: "a1", AccountID: "a", Username: "admin", Role: "admin"},
		{SessionID: "a2", AccountID: "a2", Username: "editor", Role: "editor"},
	}
	for _, s := range rows {
		if err := store.Create(ctx, s); err != nil {
			t.Fatalf("Create 失败: %v", err)
		}
	}
	if err := store.Create(ctx, Session{
		SessionID: "expired", AccountID: "x", Username: "ghost", Role: "user",
		ExpiresAt: time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("Create 失败: %v", err)
	}

	all, total, err := store.ListAll(ctx, SessionListFilter{Page: 1, PageSize: 2})
	if err != nil {
		t.Fatalf("ListAll 失败: %v", err)
	}
	if total != 3 || len(all) != 2 {
		t.Fatalf("全站分页不符: total=%d len=%d", total, len(all))
	}

	users, total, err := store.ListAll(ctx, SessionListFilter{AccountType: SessionAccountUser})
	if err != nil || total != 1 || len(users) != 1 || users[0].Username != "alice" {
		t.Fatalf("普通用户筛选不符: total=%d rows=%+v err=%v", total, users, err)
	}
	admins, total, err := store.ListAll(ctx, SessionListFilter{AccountType: SessionAccountAdmin})
	if err != nil || total != 2 || len(admins) != 2 {
		t.Fatalf("管理员筛选不符: total=%d rows=%+v err=%v", total, admins, err)
	}
	search, total, err := store.ListAll(ctx, SessionListFilter{Q: "EDIT"})
	if err != nil || total != 1 || len(search) != 1 || search[0].Username != "editor" {
		t.Fatalf("用户名搜索不符: total=%d rows=%+v err=%v", total, search, err)
	}
}

func TestRevokeSessionByFingerprint(t *testing.T) {
	store := NewMemorySessionStore()
	ctx := context.Background()
	for _, s := range []Session{
		{SessionID: "user-session", AccountID: "u", Username: "alice", Role: "user"},
		{SessionID: "admin-session", AccountID: "a", Username: "admin", Role: "admin"},
	} {
		if err := store.Create(ctx, s); err != nil {
			t.Fatalf("Create 失败: %v", err)
		}
	}
	svc := NewService(nil, store, 5, 900)

	target, err := svc.RevokeSessionByFingerprint(ctx, SessionFingerprint("user-session"), "admin-session")
	if err != nil {
		t.Fatalf("按指纹撤销失败: %v", err)
	}
	if target.Username != "alice" {
		t.Fatalf("撤销目标不符: %+v", target)
	}
	if got, _ := store.Get(ctx, "user-session"); got != nil {
		t.Fatal("目标会话应已删除")
	}
	if got, _ := store.Get(ctx, "admin-session"); got == nil {
		t.Fatal("当前会话不应被删除")
	}

	if _, err := svc.RevokeSessionByFingerprint(ctx, SessionFingerprint("admin-session"), "admin-session"); !errors.Is(err, ErrCannotRevokeCurrent) {
		t.Fatalf("撤销当前会话应返回 ErrCannotRevokeCurrent，实际 %v", err)
	}
	if got, _ := store.Get(ctx, "admin-session"); got == nil {
		t.Fatal("拒绝撤销后当前会话仍应存在")
	}
	if _, err := svc.RevokeSessionByFingerprint(ctx, "0123456789abcdef", ""); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("未知指纹应返回 ErrSessionNotFound，实际 %v", err)
	}
}
