package auth

import (
	"sync"
	"time"
)

// LoginThrottle 按 (用户名, IP) 维度限制登录失败次数。
type LoginThrottle struct {
	mu       sync.Mutex
	maxTries int
	window   time.Duration
	attempts map[string]*throttleEntry
}

type throttleEntry struct {
	failures  int
	firstFail time.Time
	lockedAt  time.Time
}

// NewLoginThrottle 创建限流器：窗口内失败达到 maxTries 即锁定。
func NewLoginThrottle(maxTries int, window time.Duration) *LoginThrottle {
	return &LoginThrottle{
		maxTries: maxTries,
		window:   window,
		attempts: make(map[string]*throttleEntry),
	}
}

// key 使用 用户名|IP 组合。
func (t *LoginThrottle) key(username, ip string) string {
	return username + "|" + ip
}

// IsLocked 返回该用户当前是否被锁定。
func (t *LoginThrottle) IsLocked(username, ip string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.attempts[t.key(username, ip)]
	if !ok {
		return false
	}
	// 从未锁定过，或达到阈值但没到锁定时刻（未设置 lockedAt），不算锁定。
	if e.lockedAt.IsZero() {
		return false
	}
	// 锁定窗口过期则解锁
	if time.Since(e.lockedAt) > t.window {
		delete(t.attempts, t.key(username, ip))
		return false
	}
	return e.failures >= t.maxTries
}

// RecordFailure 记录一次失败。
func (t *LoginThrottle) RecordFailure(username, ip string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	k := t.key(username, ip)
	e, ok := t.attempts[k]
	if !ok || time.Since(e.firstFail) > t.window {
		e = &throttleEntry{firstFail: time.Now()}
		t.attempts[k] = e
	}
	e.failures++
	if e.failures >= t.maxTries {
		e.lockedAt = time.Now()
	}
	return e.failures >= t.maxTries
}

// RecordSuccess 登录成功后清除计数。
func (t *LoginThrottle) RecordSuccess(username, ip string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.attempts, t.key(username, ip))
}

// Remaining 返回剩余尝试次数（窗口内）。
func (t *LoginThrottle) Remaining(username, ip string) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.attempts[t.key(username, ip)]
	if !ok || time.Since(e.firstFail) > t.window {
		return t.maxTries
	}
	left := t.maxTries - e.failures
	if left < 0 {
		return 0
	}
	return left
}
