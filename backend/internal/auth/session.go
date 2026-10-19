package auth

import (
	"context"
	"sync"
	"time"
)

// Session 表示一个已登录账号的会话（统一账号体系：管理员与普通用户共用）。
type Session struct {
	SessionID string
	AccountID string
	Username  string
	Role      string
	CSRFToken string
	ExpiresAt time.Time
}

// SessionStore 会话存储接口。MVP 使用内存实现；多实例部署时应替换为共享存储。
type SessionStore interface {
	Create(ctx context.Context, s Session) error
	Get(ctx context.Context, sessionID string) (*Session, error)
	Delete(ctx context.Context, sessionID string) error
	// Touch 续期会话。
	Touch(ctx context.Context, sessionID string, ttl time.Duration) error
	// DeleteByAccountID 使某账号的全部会话失效（改密、被停用/改角色/删除时调用）。
	DeleteByAccountID(ctx context.Context, accountID string) error
}

// sessionTTL 默认会话有效期。
const sessionTTL = 72 * time.Hour

// MemorySessionStore 内存会话存储（单实例适用）。
type MemorySessionStore struct {
	mu       sync.Mutex
	sessions map[string]Session
	ttl      time.Duration
}

// NewMemorySessionStore 创建带默认 TTL 的内存会话存储。
func NewMemorySessionStore() *MemorySessionStore {
	return &MemorySessionStore{
		sessions: make(map[string]Session),
		ttl:      sessionTTL,
	}
}

// Create 保存会话。
func (m *MemorySessionStore) Create(_ context.Context, s Session) error {
	if s.ExpiresAt.IsZero() {
		s.ExpiresAt = time.Now().Add(m.ttl)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[s.SessionID] = s
	return nil
}

// Get 读取会话并检查有效期。
func (m *MemorySessionStore) Get(_ context.Context, sessionID string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[sessionID]
	if !ok {
		return nil, nil
	}
	if time.Now().After(s.ExpiresAt) {
		delete(m.sessions, sessionID)
		return nil, nil
	}
	session := s
	return &session, nil
}

// Delete 删除会话。
func (m *MemorySessionStore) Delete(_ context.Context, sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, sessionID)
	return nil
}

// Touch 续期会话。
func (m *MemorySessionStore) Touch(_ context.Context, sessionID string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[sessionID]
	if !ok {
		return nil
	}
	if ttl == 0 {
		ttl = m.ttl
	}
	s.ExpiresAt = time.Now().Add(ttl)
	m.sessions[sessionID] = s
	return nil
}

// DeleteByAccountID 删除指定账号的全部会话。
// 内存实现需遍历所有会话；共享存储（如 Redis）应改用集合索引避免全扫。
func (m *MemorySessionStore) DeleteByAccountID(_ context.Context, accountID string) error {
	if accountID == "" {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for sid, s := range m.sessions {
		if s.AccountID == accountID {
			delete(m.sessions, sid)
		}
	}
	return nil
}