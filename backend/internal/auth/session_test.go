package auth

import (
	"context"
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
