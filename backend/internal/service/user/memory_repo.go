package user

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"mckit/backend/internal/domain"
)

// MemoryRepo Repo 的内存实现。
//
// 用途：单元测试与「无数据库」的本地演示/冒烟测试。
// 生产环境使用 GormRepo（见 repo.go）。行为需与 GormRepo 保持一致：
//   - 账号查询叠加 role = 'user'（与 GormRepo 一致，排除后台账号）；
//   - 删除用户时级联清除其收藏 / 最近使用 / 常用种子；
//   - 收藏与最近使用按 (user, kind, object) 去重；
//   - 列表按时间倒序；最近使用按上限裁剪。
type MemoryRepo struct {
	mu        sync.Mutex
	users     map[string]domain.User
	favorites map[string]domain.UserFavorite
	recents   map[string]domain.UserRecent
	seeds     map[string]domain.UserSeed
}

// NewMemoryRepo 创建内存仓储。
func NewMemoryRepo() *MemoryRepo {
	return &MemoryRepo{
		users:     map[string]domain.User{},
		favorites: map[string]domain.UserFavorite{},
		recents:   map[string]domain.UserRecent{},
		seeds:     map[string]domain.UserSeed{},
	}
}

// FindByUsername 按用户名查询普通用户；不存在返回 (nil, nil)。
func (m *MemoryRepo) FindByUsername(_ context.Context, username string) (*domain.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.Username == username && u.Role == domain.RoleUser {
			cp := u
			return &cp, nil
		}
	}
	return nil, nil
}

// FindByEmail 按邮箱查询普通用户；邮箱统一小写，不存在返回 (nil, nil)。
func (m *MemoryRepo) FindByEmail(_ context.Context, email string) (*domain.User, error) {
	if email == "" {
		return nil, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.Email == email && u.Role == domain.RoleUser {
			cp := u
			return &cp, nil
		}
	}
	return nil, nil
}

// FindByID 按 ID 查询普通用户。
func (m *MemoryRepo) FindByID(_ context.Context, id string) (*domain.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[id]
	if !ok || u.Role != domain.RoleUser {
		return nil, nil
	}
	cp := u
	return &cp, nil
}

// Create 新建用户。
func (m *MemoryRepo) Create(_ context.Context, u *domain.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	// 内存实现不经过数据库默认值，需在此补齐角色，保持与 GormRepo 一致。
	if u.Role == "" {
		u.Role = domain.RoleUser
	}
	u.CreatedAt = time.Now()
	u.UpdatedAt = u.CreatedAt
	m.users[u.ID.String()] = *u
	return nil
}

// UpdateLastLogin 记录最后登录时间。
func (m *MemoryRepo) UpdateLastLogin(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if u, ok := m.users[id]; ok {
		now := time.Now()
		u.LastLoginAt = &now
		m.users[id] = u
	}
	return nil
}

// ChangePassword 更新密码哈希。
func (m *MemoryRepo) ChangePassword(_ context.Context, id, hash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if u, ok := m.users[id]; ok {
		u.PasswordHash = hash
		u.MustChangePassword = false
		u.UpdatedAt = time.Now()
		m.users[id] = u
	}
	return nil
}

// UpdateProfile 更新展示资料（昵称 / 头像 URL）。
func (m *MemoryRepo) UpdateProfile(_ context.Context, id, nickname, avatarURL string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if u, ok := m.users[id]; ok && u.Role == domain.RoleUser {
		u.Nickname, u.AvatarURL = nickname, avatarURL
		u.UpdatedAt = time.Now()
		m.users[id] = u
	}
	return nil
}

// DeleteUser 删除账号并级联清除个人数据。
func (m *MemoryRepo) DeleteUser(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.users, id)
	for k, v := range m.favorites {
		if v.UserID.String() == id {
			delete(m.favorites, k)
		}
	}
	for k, v := range m.recents {
		if v.UserID.String() == id {
			delete(m.recents, k)
		}
	}
	for k, v := range m.seeds {
		if v.UserID.String() == id {
			delete(m.seeds, k)
		}
	}
	return nil
}

// ListFavorites 列出收藏，按创建时间倒序。
func (m *MemoryRepo) ListFavorites(_ context.Context, userID string) ([]domain.UserFavorite, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domain.UserFavorite, 0)
	for _, v := range m.favorites {
		if v.UserID.String() == userID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if len(out) > MaxFavorites {
		out = out[:MaxFavorites]
	}
	return out, nil
}

// FavoriteExists 判断是否已收藏。
func (m *MemoryRepo) FavoriteExists(_ context.Context, userID, kind, objectID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.favorites[favKey(userID, kind, objectID)]
	return ok, nil
}

// UpsertFavorite 插入或更新收藏。
func (m *MemoryRepo) UpsertFavorite(_ context.Context, f *domain.UserFavorite) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := favKey(f.UserID.String(), f.Kind, f.ObjectID)
	if old, ok := m.favorites[key]; ok {
		old.Label, old.URL = f.Label, f.URL
		m.favorites[key] = old
		return nil
	}
	f.ID = uuid.New()
	f.CreatedAt = time.Now()
	m.favorites[key] = *f
	return nil
}

// DeleteFavorite 取消收藏。
func (m *MemoryRepo) DeleteFavorite(_ context.Context, userID, kind, objectID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.favorites, favKey(userID, kind, objectID))
	return nil
}

// ListRecents 列出最近使用，按访问时间倒序。
func (m *MemoryRepo) ListRecents(_ context.Context, userID string) ([]domain.UserRecent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domain.UserRecent, 0)
	for _, v := range m.recents {
		if v.UserID.String() == userID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].VisitedAt.After(out[j].VisitedAt) })
	if len(out) > MaxRecents {
		out = out[:MaxRecents]
	}
	return out, nil
}

// UpsertRecent 插入或刷新最近使用记录。
func (m *MemoryRepo) UpsertRecent(_ context.Context, r *domain.UserRecent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := favKey(r.UserID.String(), r.Kind, r.ObjectID)
	if old, ok := m.recents[key]; ok {
		old.Label, old.URL, old.VisitedAt = r.Label, r.URL, time.Now()
		m.recents[key] = old
		return nil
	}
	r.ID = uuid.New()
	r.VisitedAt = time.Now()
	m.recents[key] = *r
	return nil
}

// PruneRecents 仅保留最近 keep 条。
func (m *MemoryRepo) PruneRecents(_ context.Context, userID string, keep int) error {
	if keep <= 0 {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	list := make([]domain.UserRecent, 0)
	for _, v := range m.recents {
		if v.UserID.String() == userID {
			list = append(list, v)
		}
	}
	if len(list) <= keep {
		return nil
	}
	sort.Slice(list, func(i, j int) bool { return list[i].VisitedAt.After(list[j].VisitedAt) })
	for _, v := range list[keep:] {
		delete(m.recents, v.ID.String())
	}
	return nil
}

// ClearRecents 清空最近使用。
func (m *MemoryRepo) ClearRecents(_ context.Context, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, v := range m.recents {
		if v.UserID.String() == userID {
			delete(m.recents, k)
		}
	}
	return nil
}

// ListSeeds 列出常用种子，按创建时间倒序。
func (m *MemoryRepo) ListSeeds(_ context.Context, userID string) ([]domain.UserSeed, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domain.UserSeed, 0)
	for _, v := range m.seeds {
		if v.UserID.String() == userID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if len(out) > MaxSeeds {
		out = out[:MaxSeeds]
	}
	return out, nil
}

// CreateSeed 新建常用种子。
func (m *MemoryRepo) CreateSeed(_ context.Context, s *domain.UserSeed) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s.ID = uuid.New()
	s.CreatedAt = time.Now()
	s.UpdatedAt = s.CreatedAt
	m.seeds[s.ID.String()] = *s
	return nil
}

// UpdateSeed 更新常用种子（限本人）。
func (m *MemoryRepo) UpdateSeed(_ context.Context, userID, seedID, name, seed, note string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.seeds[seedID]
	if !ok || s.UserID.String() != userID {
		return nil
	}
	s.Name, s.Seed, s.Note, s.UpdatedAt = name, seed, note, time.Now()
	m.seeds[seedID] = s
	return nil
}

// DeleteSeed 删除常用种子（限本人）。
func (m *MemoryRepo) DeleteSeed(_ context.Context, userID, seedID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.seeds[seedID]; ok && s.UserID.String() == userID {
		delete(m.seeds, seedID)
	}
	return nil
}

// favKey 收藏 / 最近使用的去重键。
func favKey(userID, kind, objectID string) string {
	return userID + "|" + kind + "|" + objectID
}
