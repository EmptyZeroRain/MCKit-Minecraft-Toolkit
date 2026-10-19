// Package useradmin 提供普通用户账号的管理端查询与维护（后台「普通用户管理」）。
//
// 设计说明：
//   - 权限由路由层（RequireAuth + RequirePermission + CSRF）保证；
//   - 统一账号（迁移 0022）后普通用户与后台账号同表，靠 role 区分：
//     本包一律过滤 role = 'user'，绝不读写后台账号；
//   - 个人数据计数（收藏 / 最近使用 / 常用种子）按页批量聚合，避免 N+1 查询；
//   - 删除账号依赖外键 ON DELETE CASCADE 级联清除个人数据；
//   - 密码沿用普通用户口径（8–128 位），仅存 Argon2id 哈希。
package useradmin

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"mckit/backend/internal/auth"
	"mckit/backend/internal/domain"
)

// usernameRe 用户名规则：与普通用户注册一致（字母开头，3-40 位，字母/数字/下划线/连字符）。
var usernameRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{2,39}$`)

// 状态筛选值。
const (
	StatusAll      = ""         // 不限
	StatusActive   = "active"   // 仅启用
	StatusDisabled = "disabled" // 仅禁用
)

// 密码长度口径：与普通用户自助改密一致（管理员账号为 12）。
const (
	MinPasswordLength = 8
	MaxPasswordLength = 128
)

// exportMax 单次导出上限，避免一次拉取过多数据。
const exportMax = 10000

// 服务级错误（处理层据此映射状态码）。
var (
	// ErrNotFound 目标普通用户不存在。
	ErrNotFound = errors.New("用户不存在")
	// ErrWeakPassword 密码强度不足（统一实现见 auth.ValidatePassword）。
	ErrWeakPassword = auth.ErrWeakPassword
	// ErrBadUsername 用户名格式非法（与普通用户注册同一规则）。
	ErrBadUsername = errors.New("用户名需为 3-40 位，以字母开头，仅含字母、数字、下划线或连字符")
	// ErrUsernameTaken 用户名已被占用（accounts 全局唯一）。
	ErrUsernameTaken = errors.New("用户名已被占用")
)

// Service 普通用户管理服务。
type Service struct {
	db *gorm.DB
	// policy 普通用户密码策略（默认 DefaultUserPolicy，可由装配层按配置覆盖）。
	policy auth.PasswordPolicy
	// revoker 会话撤销器（可选）；停用 / 重置密码 / 删除后强制下线。
	revoker SessionRevoker
}

// SessionRevoker 撤销 / 统计某账号的会话（由 auth.SessionStore 满足，避免包间循环依赖）。
type SessionRevoker interface {
	DeleteByAccountID(ctx context.Context, accountID string) error
	ListByAccountID(ctx context.Context, accountID string) ([]auth.Session, error)
}

// NewService 创建服务。
func NewService(db *gorm.DB) *Service { return &Service{db: db, policy: auth.DefaultUserPolicy()} }

// WithPasswordPolicy 覆盖普通用户密码策略（装配层从配置注入）。
func (s *Service) WithPasswordPolicy(p auth.PasswordPolicy) *Service {
	s.policy = p
	return s
}

// WithSessionRevoker 注入会话撤销器（未注入时跳过撤销，不报错）。
func (s *Service) WithSessionRevoker(r SessionRevoker) *Service {
	s.revoker = r
	return s
}

// revokeSessions 尽力撤销账号会话；失败不阻断主流程。
func (s *Service) revokeSessions(ctx context.Context, accountID string) {
	if s.revoker == nil || accountID == "" {
		return
	}
	_ = s.revoker.DeleteByAccountID(ctx, accountID)
}

// ListFilter 列表筛选条件。
type ListFilter struct {
	Q        string
	Status   string
	Page     int
	PageSize int
}

// Item 列表条目：账号基础信息 + 个人数据计数。
type Item struct {
	ID            string     `json:"id"`
	Username      string     `json:"username"`
	IsActive      bool       `json:"is_active"`
	LastLoginAt   *time.Time `json:"last_login_at"`
	CreatedAt     time.Time  `json:"created_at"`
	FavoriteCount int64      `json:"favorite_count"`
	RecentCount   int64      `json:"recent_count"`
	SeedCount     int64      `json:"seed_count"`
}

// FavoriteItem 收藏明细。
type FavoriteItem struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	ObjectID  string    `json:"object_id"`
	Label     string    `json:"label"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
}

// RecentItem 最近使用明细。
type RecentItem struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	ObjectID  string    `json:"object_id"`
	Label     string    `json:"label"`
	URL       string    `json:"url"`
	VisitedAt time.Time `json:"visited_at"`
}

// SeedItem 常用种子明细。
type SeedItem struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Seed      string    `json:"seed"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Detail 用户详情：账号基础信息 + 个人数据明细。
//
// 明细列表受个人数据容量上限约束（收藏 ≤500、最近使用 ≤50、常用种子 ≤200），
// 因此可一次性返回，无需再分页。
type Detail struct {
	ID            string         `json:"id"`
	Username      string         `json:"username"`
	IsActive      bool           `json:"is_active"`
	LastLoginAt   *time.Time     `json:"last_login_at"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	FavoriteCount int64          `json:"favorite_count"`
	RecentCount   int64          `json:"recent_count"`
	SeedCount     int64          `json:"seed_count"`
	Favorites     []FavoriteItem `json:"favorites"`
	Recents       []RecentItem   `json:"recents"`
	Seeds         []SeedItem     `json:"seeds"`
}

// CreateInput 创建普通用户入参。
// 角色固定为 user（由服务端决定），调用方无法指定后台角色。
type CreateInput struct {
	Username string
	Password string
}

// Create 创建普通用户账号：校验用户名规则/全局唯一与密码强度，角色固定为 user。
// 用户名在 accounts 中全局唯一，故需与后台账号一并查重。
func (s *Service) Create(ctx context.Context, in CreateInput) (*Item, error) {
	username := strings.TrimSpace(in.Username)
	if !usernameRe.MatchString(username) {
		return nil, ErrBadUsername
	}
	if err := auth.ValidatePassword(in.Password, s.policy, in.Username); err != nil {
		return nil, err
	}

	var existing int64
	if err := s.db.WithContext(ctx).Model(&domain.User{}).
		Where("username = ?", username).Count(&existing).Error; err != nil {
		return nil, err
	}
	if existing > 0 {
		return nil, ErrUsernameTaken
	}

	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return nil, err
	}
	u := &domain.User{
		Username:     username,
		PasswordHash: hash,
		Role:         domain.RoleUser,
		IsActive:     true,
	}
	if err := s.db.WithContext(ctx).Create(u).Error; err != nil {
		return nil, err
	}
	return &Item{
		ID:          u.ID.String(),
		Username:    u.Username,
		IsActive:    u.IsActive,
		LastLoginAt: u.LastLoginAt,
		CreatedAt:   u.CreatedAt,
	}, nil
}

// List 分页查询普通用户（用户名模糊匹配 + 状态筛选），并附带个人数据计数。
func (s *Service) List(ctx context.Context, f ListFilter) ([]Item, int64, error) {
	page, pageSize := normalize(f.Page, f.PageSize)

	tx := s.baseQuery(ctx, f)
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var users []domain.User
	if err := tx.Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&users).Error; err != nil {
		return nil, 0, err
	}

	items, err := s.attachCounts(ctx, users)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// Export 导出符合条件的全部普通用户（不分页），返回条目与是否被截断。
func (s *Service) Export(ctx context.Context, f ListFilter) ([]Item, bool, error) {
	tx := s.baseQuery(ctx, f)
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, false, err
	}

	var users []domain.User
	if err := tx.Order("created_at DESC").Limit(exportMax).Find(&users).Error; err != nil {
		return nil, false, err
	}
	items, err := s.attachCounts(ctx, users)
	if err != nil {
		return nil, false, err
	}
	return items, total > int64(exportMax), nil
}

// Detail 查询用户详情（含个人数据明细）；非普通用户视为不存在。
func (s *Service) Detail(ctx context.Context, id string) (*Detail, error) {
	u, err := s.findByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, ErrNotFound
	}

	var favorites []domain.UserFavorite
	if err := s.db.WithContext(ctx).Where("user_id = ?", u.ID).
		Order("created_at DESC").Find(&favorites).Error; err != nil {
		return nil, err
	}
	var recents []domain.UserRecent
	if err := s.db.WithContext(ctx).Where("user_id = ?", u.ID).
		Order("visited_at DESC").Find(&recents).Error; err != nil {
		return nil, err
	}
	var seeds []domain.UserSeed
	if err := s.db.WithContext(ctx).Where("user_id = ?", u.ID).
		Order("created_at DESC").Find(&seeds).Error; err != nil {
		return nil, err
	}

	detail := &Detail{
		ID:            u.ID.String(),
		Username:      u.Username,
		IsActive:      u.IsActive,
		LastLoginAt:   u.LastLoginAt,
		CreatedAt:     u.CreatedAt,
		UpdatedAt:     u.UpdatedAt,
		FavoriteCount: int64(len(favorites)),
		RecentCount:   int64(len(recents)),
		SeedCount:     int64(len(seeds)),
		Favorites:     make([]FavoriteItem, 0, len(favorites)),
		Recents:       make([]RecentItem, 0, len(recents)),
		Seeds:         make([]SeedItem, 0, len(seeds)),
	}
	for _, f := range favorites {
		detail.Favorites = append(detail.Favorites, FavoriteItem{
			ID: f.ID.String(), Kind: f.Kind, ObjectID: f.ObjectID,
			Label: f.Label, URL: f.URL, CreatedAt: f.CreatedAt,
		})
	}
	for _, r := range recents {
		detail.Recents = append(detail.Recents, RecentItem{
			ID: r.ID.String(), Kind: r.Kind, ObjectID: r.ObjectID,
			Label: r.Label, URL: r.URL, VisitedAt: r.VisitedAt,
		})
	}
	for _, sd := range seeds {
		detail.Seeds = append(detail.Seeds, SeedItem{
			ID: sd.ID.String(), Name: sd.Name, Seed: sd.Seed,
			Note: sd.Note, CreatedAt: sd.CreatedAt, UpdatedAt: sd.UpdatedAt,
		})
	}
	return detail, nil
}

// Update 更新启用状态；返回更新后的列表条目。
// 仅作用于普通用户（role = 'user'），后台账号一律视为不存在。
func (s *Service) Update(ctx context.Context, id string, isActive bool) (*Item, error) {
	u, err := s.findByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, ErrNotFound
	}

	if u.IsActive != isActive {
		if err := s.db.WithContext(ctx).Model(&domain.User{}).
			Where("id = ? AND role = ?", id, domain.RoleUser).
			Updates(map[string]any{
				"is_active":  isActive,
				"updated_at": gorm.Expr("now()"),
			}).Error; err != nil {
			return nil, err
		}
		u.IsActive = isActive
		// 停用后立即撤销全部会话（gj.md §10.2）。
		if !isActive {
			s.revokeSessions(ctx, id)
		}
	}

	items, err := s.attachCounts(ctx, []domain.User{*u})
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, ErrNotFound
	}
	return &items[0], nil
}

// ResetPassword 重置指定用户密码（无需旧密码，由管理员执行）。
//
// 账号恢复流程（gj.md §10.2）：重置后置 must_change_password = true，
// 用户下次登录必须自行改密；同时撤销其全部会话。
func (s *Service) ResetPassword(ctx context.Context, id, newPassword string) error {
	u, err := s.findByID(ctx, id)
	if err != nil {
		return err
	}
	if u == nil {
		return ErrNotFound
	}
	if err := auth.ValidatePassword(newPassword, s.policy, u.Username); err != nil {
		return err
	}

	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		return err
	}
	if err := s.db.WithContext(ctx).Model(&domain.User{}).
		Where("id = ? AND role = ?", id, domain.RoleUser).
		Updates(map[string]any{
			"password_hash":        hash,
			"must_change_password": true,
			"updated_at":           gorm.Expr("now()"),
		}).Error; err != nil {
		return err
	}
	s.revokeSessions(ctx, id)
	return nil
}

// Delete 删除普通用户账号；个人数据由外键 ON DELETE CASCADE 一并清除。
func (s *Service) Delete(ctx context.Context, id string) error {
	u, err := s.findByID(ctx, id)
	if err != nil {
		return err
	}
	if u == nil {
		return ErrNotFound
	}
	if err := s.db.WithContext(ctx).
		Where("id = ? AND role = ?", id, domain.RoleUser).
		Delete(&domain.User{}).Error; err != nil {
		return err
	}
	// 删除后撤销全部会话（共享存储由外键级联兜底，内存存储需显式清理）。
	s.revokeSessions(ctx, id)
	return nil
}

// DeletePreview 删除影响预览（gj.md §12）：展示删除会一并清理的关联数据范围。
type DeletePreview struct {
	UserID      string   `json:"user_id"`
	Username    string   `json:"username"`
	IsActive    bool     `json:"is_active"`
	Favorites   int64    `json:"favorites"`
	Recents     int64    `json:"recents"`
	Seeds       int64    `json:"seeds"`
	Submissions int64    `json:"submissions"`
	Sessions    int      `json:"sessions"`
	Reversible  bool     `json:"reversible"`
	Notes       []string `json:"notes"`
}

// DeletePreview 计算删除普通用户的影响（不执行删除）。
func (s *Service) DeletePreview(ctx context.Context, id string) (*DeletePreview, error) {
	u, err := s.findByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, ErrNotFound
	}
	p := &DeletePreview{
		UserID:     u.ID.String(),
		Username:   u.Username,
		IsActive:   u.IsActive,
		Reversible: false,
	}
	if p.Favorites, err = s.countOne(ctx, &domain.UserFavorite{}, u.ID); err != nil {
		return nil, err
	}
	if p.Recents, err = s.countOne(ctx, &domain.UserRecent{}, u.ID); err != nil {
		return nil, err
	}
	if p.Seeds, err = s.countOne(ctx, &domain.UserSeed{}, u.ID); err != nil {
		return nil, err
	}
	if p.Submissions, err = s.countOne(ctx, &domain.ResourceSubmission{}, u.ID); err != nil {
		return nil, err
	}
	if s.revoker != nil {
		if sessions, err := s.revoker.ListByAccountID(ctx, id); err == nil {
			p.Sessions = len(sessions)
		}
	}
	p.Notes = []string{
		"删除为物理删除，不可恢复",
		"收藏 / 最近使用 / 常用种子 / 投稿记录将随账号一并清理",
		"该账号的全部登录会话将被立即撤销",
	}
	return p, nil
}

// countOne 统计某用户在给定表中的记录数。
func (s *Service) countOne(ctx context.Context, model any, userID uuid.UUID) (int64, error) {
	var n int64
	column := "user_id"
	if _, ok := model.(*domain.ResourceSubmission); ok {
		column = "account_id"
	}
	err := s.db.WithContext(ctx).Model(model).Where(column+" = ?", userID).Count(&n).Error
	return n, err
}

// baseQuery 构造普通用户基础查询（固定叠加 role = 'user'，再按条件过滤）。
func (s *Service) baseQuery(ctx context.Context, f ListFilter) *gorm.DB {
	tx := s.db.WithContext(ctx).Model(&domain.User{}).Where("role = ?", domain.RoleUser)
	if q := strings.TrimSpace(f.Q); q != "" {
		tx = tx.Where("username ILIKE ?", "%"+q+"%")
	}
	switch f.Status {
	case StatusActive:
		tx = tx.Where("is_active = true")
	case StatusDisabled:
		tx = tx.Where("is_active = false")
	}
	return tx
}

// attachCounts 为一批用户补齐个人数据计数（批量聚合，避免 N+1）。
func (s *Service) attachCounts(ctx context.Context, users []domain.User) ([]Item, error) {
	ids := make([]uuid.UUID, 0, len(users))
	for _, u := range users {
		ids = append(ids, u.ID)
	}
	favCounts, err := s.countByUser(ctx, &domain.UserFavorite{}, ids)
	if err != nil {
		return nil, err
	}
	recCounts, err := s.countByUser(ctx, &domain.UserRecent{}, ids)
	if err != nil {
		return nil, err
	}
	seedCounts, err := s.countByUser(ctx, &domain.UserSeed{}, ids)
	if err != nil {
		return nil, err
	}

	items := make([]Item, 0, len(users))
	for _, u := range users {
		key := u.ID.String()
		items = append(items, Item{
			ID:            key,
			Username:      u.Username,
			IsActive:      u.IsActive,
			LastLoginAt:   u.LastLoginAt,
			CreatedAt:     u.CreatedAt,
			FavoriteCount: favCounts[key],
			RecentCount:   recCounts[key],
			SeedCount:     seedCounts[key],
		})
	}
	return items, nil
}

// findByID 按主键查询普通用户（排除后台账号），未命中返回 (nil, nil)。
// 非法 UUID 视为不存在，避免把格式错误当成 500。
func (s *Service) findByID(ctx context.Context, id string) (*domain.User, error) {
	id = strings.TrimSpace(id)
	if _, err := uuid.Parse(id); err != nil {
		return nil, nil
	}
	var u domain.User
	err := s.db.WithContext(ctx).
		Where("id = ? AND role = ?", id, domain.RoleUser).
		First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// countByUser 按 user_id 聚合计数；ids 为空时直接返回空表，避免无谓查询。
func (s *Service) countByUser(ctx context.Context, model any, ids []uuid.UUID) (map[string]int64, error) {
	out := make(map[string]int64, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var rows []struct {
		UserID string
		N      int64
	}
	if err := s.db.WithContext(ctx).Model(model).
		Select("user_id::text AS user_id, count(*) AS n").
		Where("user_id IN ?", ids).
		Group("user_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.UserID] = row.N
	}
	return out, nil
}

// normalize 归一化分页参数（与其余管理端接口一致：page ≥ 1，page_size 默认 20、上限 100）。
func normalize(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return page, pageSize
}
