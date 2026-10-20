// Package accountadmin 提供统一账号管理（后台「账号管理」单列表）。
//
// 背景：迁移 0022 之后 admins / users 已合并为 accounts，身份仅由 role 决定：
//   - role = 'user' 为普通用户；
//   - 其余角色（内置 admin / editor 与自定义角色）为后台账号。
//
// 本包在同一个列表里管理两类账号，并支持把普通用户提升为后台角色、把后台
// 账号降级为普通用户。
//
// 安全边界：
//   - 本包只做数据与规则校验，调用方（handler）负责按账号类型做权限点判定：
//     普通用户需要 accounts.users，后台账号需要 accounts.admins；
//   - 「最后一名启用 admin」保护与既有 adminaccount 包共用同一把咨询锁键，
//     保证两条调用路径并发时仍至少保留一名可管理的 admin；
//   - 密码使用 Argon2id 哈希；普通用户与后台账号分别使用各自的最小长度策略。
package accountadmin

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"mckit/backend/internal/accountid"
	"mckit/backend/internal/auth"
	"mckit/backend/internal/domain"
)

// 列表范围：用于把「全部 / 后台账号 / 普通用户」映射为 role 条件。
const (
	ScopeAll   = ""
	ScopeAdmin = "admin" // role <> 'user'
	ScopeUser  = "user"  // role = 'user'
)

// 状态筛选值。
const (
	StatusAll      = ""
	StatusActive   = "active"
	StatusDisabled = "disabled"
)

// exportMax 单次导出上限。
const exportMax = 10000

// usernameRe 普通用户用户名规则（与注册、useradmin 保持一致）。
var usernameRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{2,39}$`)

// 服务级错误（处理层据此映射状态码）。
var (
	// ErrNotFound 目标账号不存在。
	ErrNotFound = errors.New("账号不存在")
	// ErrUsernameTaken 用户名已被占用。
	ErrUsernameTaken = errors.New("用户名已被占用")
	// ErrBadRole 角色不存在。
	ErrBadRole = errors.New("角色不存在，请先在「角色管理」中创建")
	// ErrBadUsername 后台账号用户名格式非法。
	ErrBadUsername = errors.New("用户名长度需在 1-80 之间")
	// ErrBadUserUsername 普通用户用户名格式非法。
	ErrBadUserUsername = errors.New("用户名需为 3-40 位，以字母开头，仅含字母、数字、下划线或连字符")
	// ErrWeakPassword 密码强度不足。
	ErrWeakPassword = auth.ErrWeakPassword
	// ErrLastAdmin 会造成没有任何启用状态 admin 账号。
	ErrLastAdmin = errors.New("必须保留至少一个启用状态的 admin 账号")
)

// SessionRevoker 撤销 / 统计账号会话（由 auth.SessionStore 满足）。
type SessionRevoker interface {
	DeleteByAccountID(ctx context.Context, accountID string) error
	ListByAccountID(ctx context.Context, accountID string) ([]auth.Session, error)
}

// Service 统一账号管理服务。
type Service struct {
	db *gorm.DB
	// adminPolicy 后台账号密码策略；userPolicy 普通用户密码策略。
	adminPolicy auth.PasswordPolicy
	userPolicy  auth.PasswordPolicy
	revoker     SessionRevoker
}

// NewService 创建统一账号服务（默认使用内置密码策略）。
func NewService(db *gorm.DB) *Service {
	return &Service{
		db:          db,
		adminPolicy: auth.DefaultAdminPolicy(),
		userPolicy:  auth.DefaultUserPolicy(),
	}
}

// WithPasswordPolicies 覆盖两类账号的密码策略（装配层从配置注入）。
func (s *Service) WithPasswordPolicies(adminPolicy, userPolicy auth.PasswordPolicy) *Service {
	s.adminPolicy = adminPolicy
	s.userPolicy = userPolicy
	return s
}

// WithSessionRevoker 注入会话撤销器（未注入时跳过撤销）。
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

// policyFor 按账号角色返回对应的密码策略。
// 普通用户沿用 8 位口径；后台角色（含被提升的账号）必须满足后台策略。
func (s *Service) policyFor(role string) auth.PasswordPolicy {
	if role == domain.RoleUser {
		return s.userPolicy
	}
	return s.adminPolicy
}

// ListFilter 统一列表筛选条件。
type ListFilter struct {
	Q        string
	Status   string
	Scope    string // ScopeAll / ScopeAdmin / ScopeUser
	Page     int
	PageSize int
}

// Item 列表条目：账号基础信息 + 个人数据计数。
type Item struct {
	ID            string     `json:"id"`
	UserNo        string     `json:"user_no"`
	Username      string     `json:"username"`
	Nickname      string     `json:"nickname"`
	Role          string     `json:"role"`
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

// Detail 账号详情：基础信息 + 个人数据明细。
type Detail struct {
	ID            string         `json:"id"`
	UserNo        string         `json:"user_no"`
	Username      string         `json:"username"`
	Nickname      string         `json:"nickname"`
	Email         string         `json:"email"`
	AvatarURL     string         `json:"avatar_url"`
	Role          string         `json:"role"`
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

// CreateInput 创建账号入参；Role 为空时按普通用户创建。
type CreateInput struct {
	Username string
	Password string
	Role     string
}

// UpdateInput 更新账号入参（指针区分「未提供」与「置零」）。
type UpdateInput struct {
	Role     *string
	IsActive *bool
}

// DeletePreview 删除影响预览。
type DeletePreview struct {
	AccountID      string   `json:"account_id"`
	Username       string   `json:"username"`
	Role           string   `json:"role"`
	IsActive       bool     `json:"is_active"`
	IsSelf         bool     `json:"is_self"`
	IsLastAdmin    bool     `json:"is_last_admin"`
	Favorites      int64    `json:"favorites"`
	Recents        int64    `json:"recents"`
	Seeds          int64    `json:"seeds"`
	Submissions    int64    `json:"submissions"`
	ActiveSessions int      `json:"active_sessions"`
	Reversible     bool     `json:"reversible"`
	Notes          []string `json:"notes"`
}

// Get 按主键查询任意账号（不区分角色）；不存在返回 (nil, nil)。
func (s *Service) Get(ctx context.Context, id string) (*domain.Account, error) {
	return findAccountByID(s.db.WithContext(ctx), id)
}

// List 分页查询账号（支持用户名模糊、状态与范围筛选）。
func (s *Service) List(ctx context.Context, f ListFilter) ([]Item, int64, error) {
	page, pageSize := normalizePage(f.Page, f.PageSize)

	tx := s.listQuery(ctx, f)
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var accounts []domain.Account
	if err := tx.Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&accounts).Error; err != nil {
		return nil, 0, err
	}

	items, err := s.attachCounts(ctx, accounts)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// Export 导出当前筛选下的账号（不分页，最多 exportMax 条）。
func (s *Service) Export(ctx context.Context, f ListFilter) ([]Item, bool, error) {
	var accounts []domain.Account
	if err := s.listQuery(ctx, f).Order("created_at DESC").Limit(exportMax + 1).Find(&accounts).Error; err != nil {
		return nil, false, err
	}
	truncated := len(accounts) > exportMax
	if truncated {
		accounts = accounts[:exportMax]
	}
	items, err := s.attachCounts(ctx, accounts)
	if err != nil {
		return nil, false, err
	}
	return items, truncated, nil
}

// Detail 查询账号详情与个人数据明细。
func (s *Service) Detail(ctx context.Context, id string) (*Detail, error) {
	acc, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if acc == nil {
		return nil, ErrNotFound
	}

	out := &Detail{
		ID:          acc.ID.String(),
		UserNo:      acc.UserNo,
		Username:    acc.Username,
		Nickname:    acc.Nickname,
		Email:       acc.Email,
		AvatarURL:   acc.AvatarURL,
		Role:        acc.Role,
		IsActive:    acc.IsActive,
		LastLoginAt: acc.LastLoginAt,
		CreatedAt:   acc.CreatedAt,
		UpdatedAt:   acc.UpdatedAt,
	}

	var favorites []domain.UserFavorite
	if err := s.db.WithContext(ctx).Where("user_id = ?", acc.ID).
		Order("created_at DESC").Limit(500).Find(&favorites).Error; err != nil {
		return nil, err
	}
	out.FavoriteCount = int64(len(favorites))
	out.Favorites = make([]FavoriteItem, 0, len(favorites))
	for _, f := range favorites {
		out.Favorites = append(out.Favorites, FavoriteItem{
			ID: f.ID.String(), Kind: f.Kind, ObjectID: f.ObjectID,
			Label: f.Label, URL: f.URL, CreatedAt: f.CreatedAt,
		})
	}

	var recents []domain.UserRecent
	if err := s.db.WithContext(ctx).Where("user_id = ?", acc.ID).
		Order("visited_at DESC").Limit(50).Find(&recents).Error; err != nil {
		return nil, err
	}
	out.RecentCount = int64(len(recents))
	out.Recents = make([]RecentItem, 0, len(recents))
	for _, r := range recents {
		out.Recents = append(out.Recents, RecentItem{
			ID: r.ID.String(), Kind: r.Kind, ObjectID: r.ObjectID,
			Label: r.Label, URL: r.URL, VisitedAt: r.VisitedAt,
		})
	}

	var seeds []domain.UserSeed
	if err := s.db.WithContext(ctx).Where("user_id = ?", acc.ID).
		Order("created_at DESC").Limit(200).Find(&seeds).Error; err != nil {
		return nil, err
	}
	out.SeedCount = int64(len(seeds))
	out.Seeds = make([]SeedItem, 0, len(seeds))
	for _, sd := range seeds {
		out.Seeds = append(out.Seeds, SeedItem{
			ID: sd.ID.String(), Name: sd.Name, Seed: sd.Seed, Note: sd.Note,
			CreatedAt: sd.CreatedAt, UpdatedAt: sd.UpdatedAt,
		})
	}
	return out, nil
}

// Create 创建账号：校验用户名、角色与密码策略，用户名全局唯一。
func (s *Service) Create(ctx context.Context, in CreateInput) (*Item, error) {
	role := strings.TrimSpace(in.Role)
	if role == "" {
		role = domain.RoleUser
	}
	if err := validateRole(ctx, s.db, role); err != nil {
		return nil, err
	}

	username := strings.TrimSpace(in.Username)
	if role == domain.RoleUser {
		if !usernameRe.MatchString(username) {
			return nil, ErrBadUserUsername
		}
	} else if username == "" || len(username) > 80 {
		return nil, ErrBadUsername
	}

	if err := auth.ValidatePassword(in.Password, s.policyFor(role), username); err != nil {
		return nil, err
	}

	var existing int64
	if err := s.db.WithContext(ctx).Model(&domain.Account{}).
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
	userNo, err := accountid.Generate()
	if err != nil {
		return nil, err
	}
	acc := &domain.Account{
		Username:     username,
		UserNo:       userNo,
		PasswordHash: hash,
		Role:         role,
		IsActive:     true,
	}
	if err := s.db.WithContext(ctx).Create(acc).Error; err != nil {
		return nil, err
	}
	return &Item{
		ID:          acc.ID.String(),
		UserNo:      acc.UserNo,
		Username:    acc.Username,
		Nickname:    acc.Nickname,
		Role:        acc.Role,
		IsActive:    acc.IsActive,
		LastLoginAt: acc.LastLoginAt,
		CreatedAt:   acc.CreatedAt,
	}, nil
}

// Update 更新角色与启用状态。
//
// 关键规则：
//   - 把普通用户提升为后台角色时，标记 must_change_password：原密码可能只有
//     8 位，不满足后台策略，下次登录必须先改成 ≥ 后台最小长度；
//   - 把启用中的 admin 降级或停用时，必须保留至少一名启用 admin（咨询锁串行化）；
//   - 角色或状态变化后撤销该账号全部会话。
func (s *Service) Update(ctx context.Context, id string, in UpdateInput) (*Item, error) {
	var (
		result          *domain.Account
		securityChanged bool
	)
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockAdminGuard(tx); err != nil {
			return err
		}
		acc, err := findAccountByID(tx, id)
		if err != nil {
			return err
		}
		if acc == nil {
			return ErrNotFound
		}

		updates := map[string]any{}

		if in.Role != nil && strings.TrimSpace(*in.Role) != "" {
			newRole := strings.TrimSpace(*in.Role)
			if newRole != acc.Role {
				if err := validateRoleTx(tx, newRole); err != nil {
					return err
				}
				if acc.Role == domain.RoleAdmin && acc.IsActive && newRole != domain.RoleAdmin {
					if n, err := countActiveAdminsTx(tx); err != nil {
						return err
					} else if n <= 1 {
						return ErrLastAdmin
					}
				}
				updates["role"] = newRole
				// 普通用户 → 后台角色：强制下次登录改密，补足后台密码强度。
				if acc.Role == domain.RoleUser && newRole != domain.RoleUser {
					updates["must_change_password"] = true
				}
				securityChanged = true
			}
		}

		if in.IsActive != nil && *in.IsActive != acc.IsActive {
			if !*in.IsActive && acc.Role == domain.RoleAdmin {
				if n, err := countActiveAdminsTx(tx); err != nil {
					return err
				} else if n <= 1 {
					return ErrLastAdmin
				}
			}
			updates["is_active"] = *in.IsActive
			securityChanged = true
		}

		if len(updates) == 0 {
			result = acc
			return nil
		}
		updates["updated_at"] = gorm.Expr("now()")
		if err := tx.Model(&domain.Account{}).Where("id = ?", acc.ID).Updates(updates).Error; err != nil {
			return err
		}
		result, err = findAccountByID(tx, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	if securityChanged {
		s.revokeSessions(ctx, id)
	}

	items, err := s.attachCounts(ctx, []domain.Account{*result})
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, ErrNotFound
	}
	return &items[0], nil
}

// ResetPassword 重置任意账号密码（无需旧密码，由管理员执行）。
// 密码策略按目标账号当前角色选择；重置后强制改密并撤销全部会话。
func (s *Service) ResetPassword(ctx context.Context, id, newPassword string) error {
	acc, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if acc == nil {
		return ErrNotFound
	}
	if err := auth.ValidatePassword(newPassword, s.policyFor(acc.Role), acc.Username); err != nil {
		return err
	}
	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		return err
	}
	if err := s.db.WithContext(ctx).Model(&domain.Account{}).
		Where("id = ?", acc.ID).
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

// Delete 物理删除任意账号；个人数据由外键 ON DELETE CASCADE 清除。
// 删除启用中的最后一名 admin 会被拒绝。
func (s *Service) Delete(ctx context.Context, id string) error {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockAdminGuard(tx); err != nil {
			return err
		}
		acc, err := findAccountByID(tx, id)
		if err != nil {
			return err
		}
		if acc == nil {
			return ErrNotFound
		}
		if acc.Role == domain.RoleAdmin && acc.IsActive {
			if n, err := countActiveAdminsTx(tx); err != nil {
				return err
			} else if n <= 1 {
				return ErrLastAdmin
			}
		}
		return tx.Where("id = ?", acc.ID).Delete(&domain.Account{}).Error
	})
	if err != nil {
		return err
	}
	s.revokeSessions(ctx, id)
	return nil
}

// ResetMFA 解绑任意账号的双因子认证（账号恢复）。
func (s *Service) ResetMFA(ctx context.Context, id string) error {
	acc, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if acc == nil {
		return ErrNotFound
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("account_id = ?", acc.ID).Delete(&domain.MFARecoveryCode{}).Error; err != nil {
			return err
		}
		return tx.Where("account_id = ?", acc.ID).Delete(&domain.AccountMFA{}).Error
	})
}

// DeletePreview 计算删除账号的影响（不执行删除）。
func (s *Service) DeletePreview(ctx context.Context, id, actorID string) (*DeletePreview, error) {
	acc, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if acc == nil {
		return nil, ErrNotFound
	}
	p := &DeletePreview{
		AccountID:  acc.ID.String(),
		Username:   acc.Username,
		Role:       acc.Role,
		IsActive:   acc.IsActive,
		IsSelf:     actorID != "" && actorID == acc.ID.String(),
		Reversible: false,
	}
	if acc.Role == domain.RoleAdmin && acc.IsActive {
		n, err := countActiveAdmins(ctx, s.db)
		if err != nil {
			return nil, err
		}
		p.IsLastAdmin = n <= 1
	}
	if p.Favorites, err = countByUserColumn(ctx, s.db, &domain.UserFavorite{}, acc.ID); err != nil {
		return nil, err
	}
	if p.Recents, err = countByUserColumn(ctx, s.db, &domain.UserRecent{}, acc.ID); err != nil {
		return nil, err
	}
	if p.Seeds, err = countByUserColumn(ctx, s.db, &domain.UserSeed{}, acc.ID); err != nil {
		return nil, err
	}
	if p.Submissions, err = countByUserColumn(ctx, s.db, &domain.ResourceSubmission{}, acc.ID); err != nil {
		return nil, err
	}
	if s.revoker != nil {
		if sessions, err := s.revoker.ListByAccountID(ctx, id); err == nil {
			p.ActiveSessions = len(sessions)
		}
	}
	notes := []string{"删除为物理删除，不可恢复"}
	if p.IsSelf {
		notes = append(notes, "不能删除当前登录账号")
	}
	if p.IsLastAdmin {
		notes = append(notes, "系统必须保留至少一个启用状态的 admin 账号")
	}
	notes = append(notes, "收藏 / 最近使用 / 常用种子 / 投稿记录将随账号一并清理")
	notes = append(notes, "该账号的全部登录会话将被立即撤销")
	p.Notes = notes
	return p, nil
}

// listQuery 构造列表查询（角色范围 + 关键字 + 状态）。
func (s *Service) listQuery(ctx context.Context, f ListFilter) *gorm.DB {
	tx := s.db.WithContext(ctx).Model(&domain.Account{})
	switch strings.TrimSpace(f.Scope) {
	case ScopeAdmin:
		tx = tx.Where("role <> ?", domain.RoleUser)
	case ScopeUser:
		tx = tx.Where("role = ?", domain.RoleUser)
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		// 显式加括号：避免与角色 / 状态条件组合时 OR 抢先结合导致越界查询。
		tx = tx.Where("(username ILIKE ? OR user_no ILIKE ?)", "%"+q+"%", "%"+q+"%")
	}
	switch f.Status {
	case StatusActive:
		tx = tx.Where("is_active = true")
	case StatusDisabled:
		tx = tx.Where("is_active = false")
	}
	return tx
}

// attachCounts 批量补齐个人数据计数，避免 N+1。
func (s *Service) attachCounts(ctx context.Context, accounts []domain.Account) ([]Item, error) {
	ids := make([]uuid.UUID, 0, len(accounts))
	for _, a := range accounts {
		ids = append(ids, a.ID)
	}
	favCounts, err := countByUserIDs(ctx, s.db, &domain.UserFavorite{}, ids)
	if err != nil {
		return nil, err
	}
	recCounts, err := countByUserIDs(ctx, s.db, &domain.UserRecent{}, ids)
	if err != nil {
		return nil, err
	}
	seedCounts, err := countByUserIDs(ctx, s.db, &domain.UserSeed{}, ids)
	if err != nil {
		return nil, err
	}

	items := make([]Item, 0, len(accounts))
	for _, a := range accounts {
		key := a.ID.String()
		items = append(items, Item{
			ID:            key,
			UserNo:        a.UserNo,
			Username:      a.Username,
			Nickname:      a.Nickname,
			Role:          a.Role,
			IsActive:      a.IsActive,
			LastLoginAt:   a.LastLoginAt,
			CreatedAt:     a.CreatedAt,
			FavoriteCount: favCounts[key],
			RecentCount:   recCounts[key],
			SeedCount:     seedCounts[key],
		})
	}
	return items, nil
}

// findAccountByID 按主键查询任意账号；非法 UUID 视为不存在。
func findAccountByID(tx *gorm.DB, id string) (*domain.Account, error) {
	id = strings.TrimSpace(id)
	if _, err := uuid.Parse(id); err != nil {
		return nil, nil
	}
	var acc domain.Account
	err := tx.Where("id = ?", id).First(&acc).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &acc, nil
}

// validateRole 校验角色在 roles 表中存在。
func validateRole(ctx context.Context, db *gorm.DB, role string) error {
	return validateRoleTx(db.WithContext(ctx), role)
}

// validateRoleTx 基于给定事务/连接校验角色存在。
func validateRoleTx(tx *gorm.DB, role string) error {
	role = strings.TrimSpace(role)
	if role == "" {
		return ErrBadRole
	}
	var n int64
	if err := tx.Model(&domain.Role{}).Where("name = ?", role).Count(&n).Error; err != nil {
		return err
	}
	if n == 0 {
		return ErrBadRole
	}
	return nil
}

// countActiveAdmins 统计启用状态的 admin 账号数。
func countActiveAdmins(ctx context.Context, db *gorm.DB) (int64, error) {
	return countActiveAdminsTx(db.WithContext(ctx))
}

// countActiveAdminsTx 基于给定事务/连接统计启用 admin 数。
func countActiveAdminsTx(tx *gorm.DB) (int64, error) {
	var n int64
	err := tx.Model(&domain.Account{}).
		Where("role = ? AND is_active = true", domain.RoleAdmin).
		Count(&n).Error
	return n, err
}

// adminGuardLockKey 与 adminaccount 包保持一致，串行化「减少启用 admin」的操作。
const adminGuardLockKey int64 = 0x6d636b6974 // "mckit"

// lockAdminGuard 在事务内获取咨询锁。
func lockAdminGuard(tx *gorm.DB) error {
	return tx.Exec("SELECT pg_advisory_xact_lock(?)", adminGuardLockKey).Error
}

// countByUserIDs 按 user_id 聚合计数；ids 为空时返回空表。
func countByUserIDs(ctx context.Context, db *gorm.DB, model any, ids []uuid.UUID) (map[string]int64, error) {
	out := make(map[string]int64, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var rows []struct {
		UserID string
		N      int64
	}
	if err := db.WithContext(ctx).Model(model).
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

// countByUserColumn 统计单个账号在指定表中的记录数。
// 资源投稿的归属列是 account_id，其余个人数据表使用 user_id。
func countByUserColumn(ctx context.Context, db *gorm.DB, model any, accountID uuid.UUID) (int64, error) {
	column := "user_id"
	if _, ok := model.(*domain.ResourceSubmission); ok {
		column = "account_id"
	}
	var n int64
	err := db.WithContext(ctx).Model(model).Where(column+" = ?", accountID).Count(&n).Error
	return n, err
}

// normalizePage 与其余管理端接口一致：page ≥ 1，page_size 默认 20、上限 100。
func normalizePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return page, pageSize
}
