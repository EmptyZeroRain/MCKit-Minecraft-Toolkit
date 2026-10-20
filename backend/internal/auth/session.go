package auth

import (
	"context"
	"errors"
	"sort"
	"strings"
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

// ErrSessionNotFound 目标会话不存在或已过期（后台踢下线场景）。
var ErrSessionNotFound = errors.New("会话不存在或已失效")

// ErrCannotRevokeCurrent 不允许在后台会话管理中踢下线当前会话。
var ErrCannotRevokeCurrent = errors.New("不能踢下线当前会话，请使用退出登录")

// 会话列表的账号类型筛选值。
const (
	SessionAccountAll   = "all"
	SessionAccountUser  = "user"
	SessionAccountAdmin = "admin"
)

// SessionListFilter 后台全站会话列表筛选。
//
// AccountType：all / user / admin；user 指 role='user'，admin 指其余后台角色。
// Q 仅匹配用户名（会话表不保存昵称 / user_no）。Page / PageSize 由服务层归一化。
type SessionListFilter struct {
	Q           string
	AccountType string
	Page        int
	PageSize    int
}

// normalizeSessionListFilter 归一化筛选与分页，避免各存储实现重复处理边界。
func normalizeSessionListFilter(f SessionListFilter) SessionListFilter {
	f.Q = strings.TrimSpace(f.Q)
	switch f.AccountType {
	case SessionAccountUser, SessionAccountAdmin, SessionAccountAll:
	default:
		f.AccountType = SessionAccountAll
	}
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize <= 0 {
		f.PageSize = 20
	}
	if f.PageSize > 100 {
		f.PageSize = 100
	}
	return f
}

// matchSession 判断会话是否满足关键字与账号类型筛选。
func matchSession(s Session, f SessionListFilter) bool {
	if f.Q != "" && !strings.Contains(strings.ToLower(s.Username), strings.ToLower(f.Q)) {
		return false
	}
	switch f.AccountType {
	case SessionAccountUser:
		return s.Role == "user"
	case SessionAccountAdmin:
		return s.Role != "user"
	default:
		return true
	}
}

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
	// ListAll 后台全站会话列表（分页 + 用户名 / 账号类型筛选）。
	ListAll(ctx context.Context, f SessionListFilter) ([]Session, int64, error)
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

// ListAll 后台全站有效会话（按最近活动倒序分页）。
func (m *MemorySessionStore) ListAll(_ context.Context, f SessionListFilter) ([]Session, int64, error) {
	f = normalizeSessionListFilter(f)
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	all := make([]Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		if m.opts.expired(&s, now) || !matchSession(s, f) {
			continue
		}
		all = append(all, s)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].LastSeenAt.After(all[j].LastSeenAt) })
	total := int64(len(all))
	start := (f.Page - 1) * f.PageSize
	if start >= len(all) {
		return []Session{}, total, nil
	}
	end := start + f.PageSize
	if end > len(all) {
		end = len(all)
	}
	return all[start:end], total, nil
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
