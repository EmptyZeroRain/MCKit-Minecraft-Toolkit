package auth

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

// Session 表示一个已登录账号的会话（统一账号体系：管理员与普通用户共用）。
//
// 有效期语义（gj.md §3.4，必须有明确配置说明）：
//   - 绝对有效期：CreatedAt + SessionOptions.TTL，任何续期都不能越过该上限；
//   - 空闲超时：SessionOptions.IdleTTL（<=0 表示不启用），超过则视为失效。
type Session struct {
	SessionID string
	AccountID string
	Username  string
	Role      string
	CSRFToken string
	// MustChangePassword 账号处于「必须修改密码」状态（管理员重置密码后的首次登录）。
	// 会话上冗余保存，使鉴权中间件无需额外查询账号表。
	MustChangePassword bool
	// CreatedAt 会话创建时间（绝对有效期基准）。
	CreatedAt time.Time
	// LastSeenAt 最近一次活动时间（空闲超时基准）。
	LastSeenAt time.Time
	// ExpiresAt 绝对过期时间 = CreatedAt + TTL。
	ExpiresAt time.Time
}

// SessionOptions 会话有效期策略。由装配层从配置注入，避免散落的硬编码常量。
type SessionOptions struct {
	// TTL 绝对有效期。<=0 时回退 DefaultSessionTTL。
	TTL time.Duration
	// IdleTTL 空闲超时；<=0 表示不启用空闲超时。
	IdleTTL time.Duration
}

// DefaultSessionTTL 默认会话绝对有效期（72 小时）。
const DefaultSessionTTL = 72 * time.Hour

// lastSeenRefreshInterval 最近活动时间的写回节流间隔。
//
// 每次请求都写回 LastSeenAt 会造成共享存储的写放大；节流到分钟级即可满足
// 空闲超时精度（空闲超时以分钟计），同时把写压力降到每会话每分钟最多一次。
const lastSeenRefreshInterval = time.Minute

// ErrSessionStoreUnavailable 会话共享存储不可用。
//
// 出现该错误时必须**安全失败**（拒绝请求），不得跳过认证（gj.md §3.4）。
var ErrSessionStoreUnavailable = errors.New("会话存储不可用")

// SessionStore 会话存储接口。
//
// 单实例部署使用 MemorySessionStore（进程重启即失效）；
// 多实例部署必须使用 PostgresSessionStore（共享存储，会话跨实例一致）。
type SessionStore interface {
	// TTL 返回会话绝对有效期，供登录时计算 Cookie 过期时间。
	TTL() time.Duration
	// Create 保存会话（CreatedAt / LastSeenAt / ExpiresAt 为零值时自动填充）。
	Create(ctx context.Context, s Session) error
	// Get 读取会话；过期（绝对或空闲）返回 (nil, nil)。
	Get(ctx context.Context, sessionID string) (*Session, error)
	// Delete 删除单个会话（登出）。
	Delete(ctx context.Context, sessionID string) error
	// DeleteByAccountID 使某账号的全部会话失效（改密、被停用/改角色/删除时调用）。
	DeleteByAccountID(ctx context.Context, accountID string) error
	// ListByAccountID 列出账号的全部有效会话元数据（用于「查看与撤销会话」）。
	ListByAccountID(ctx context.Context, accountID string) ([]Session, error)
	// PurgeExpired 清理已过期会话，返回清理条数。
	PurgeExpired(ctx context.Context) (int64, error)
}

// normalize 填充派生字段，保证 Create 的入参可以是「只带业务字段」的会话。
func (o SessionOptions) normalize(s *Session) {
	now := time.Now()
	if s.CreatedAt.IsZero() {
		s.CreatedAt = now
	}
	if s.LastSeenAt.IsZero() {
		s.LastSeenAt = s.CreatedAt
	}
	if s.ExpiresAt.IsZero() {
		s.ExpiresAt = s.CreatedAt.Add(o.TTL)
	}
}

// expired 判断会话是否已失效（绝对有效期或空闲超时）。
func (o SessionOptions) expired(s *Session, now time.Time) bool {
	if !s.ExpiresAt.IsZero() && now.After(s.ExpiresAt) {
		return true
	}
	if o.IdleTTL > 0 && !s.LastSeenAt.IsZero() && now.Sub(s.LastSeenAt) > o.IdleTTL {
		return true
	}
	return false
}

// MemorySessionStore 内存会话存储（单实例适用；进程重启后全部会话失效）。
type MemorySessionStore struct {
	mu       sync.Mutex
	sessions map[string]Session
	opts     SessionOptions
}

// NewMemorySessionStore 创建使用默认策略的内存会话存储。
func NewMemorySessionStore() *MemorySessionStore {
	return NewMemorySessionStoreWithOptions(SessionOptions{})
}

// NewMemorySessionStoreWithOptions 创建带指定策略的内存会话存储。
func NewMemorySessionStoreWithOptions(opts SessionOptions) *MemorySessionStore {
	if opts.TTL <= 0 {
		opts.TTL = DefaultSessionTTL
	}
	return &MemorySessionStore{
		sessions: make(map[string]Session),
		opts:     opts,
	}
}

// TTL 返回会话绝对有效期。
func (m *MemorySessionStore) TTL() time.Duration { return m.opts.TTL }

// Create 保存会话。
func (m *MemorySessionStore) Create(_ context.Context, s Session) error {
	m.opts.normalize(&s)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[s.SessionID] = s
	return nil
}

// Get 读取会话并检查有效期；过期会话被顺带删除。
func (m *MemorySessionStore) Get(_ context.Context, sessionID string) (*Session, error) {
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[sessionID]
	if !ok {
		return nil, nil
	}
	if m.opts.expired(&s, now) {
		delete(m.sessions, sessionID)
		return nil, nil
	}
	// 节流写回最近活动时间（空闲超时基准）。
	if now.Sub(s.LastSeenAt) >= lastSeenRefreshInterval {
		s.LastSeenAt = now
		m.sessions[sessionID] = s
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

// DeleteByAccountID 删除指定账号的全部会话。
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

// ListByAccountID 列出指定账号的全部有效会话（按最近活动倒序）。
func (m *MemorySessionStore) ListByAccountID(_ context.Context, accountID string) ([]Session, error) {
	if accountID == "" {
		return nil, nil
	}
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Session, 0, 4)
	for _, s := range m.sessions {
		if s.AccountID != accountID {
			continue
		}
		if m.opts.expired(&s, now) {
			continue
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastSeenAt.After(out[j].LastSeenAt) })
	return out, nil
}

// PurgeExpired 清理已过期会话。
func (m *MemorySessionStore) PurgeExpired(_ context.Context) (int64, error) {
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	var n int64
	for sid, s := range m.sessions {
		if m.opts.expired(&s, now) {
			delete(m.sessions, sid)
			n++
		}
	}
	return n, nil
}
