// Package adminaccount 提供后台账号管理（需求 §6.1 身份认证 / §6.2 权限）。
// 路由层用 RequirePermission(accounts.admins) 限制。
//
// 统一账号（迁移 0022）后 admins 与 users 合并为 accounts：
//   - 本包只管理「后台账号」（role <> 'user'），所有查询都显式排除普通用户，
//     否则普通用户会出现在管理员列表里，甚至被重置密码 / 删除；
//   - 内置角色 user 属普通用户，不允许经本接口创建（改由 /user/register 或
//     「普通用户管理」维护）。
//
// 所有写操作使用参数化查询；密码使用 Argon2id 哈希，绝不落库明文。
package adminaccount

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"

	"mckit/backend/internal/auth"
	"mckit/backend/internal/domain"
)

// MinPasswordLength 管理员账号创建/重置密码的最小长度。
// 与初始化命令 cmd/create-admin 保持一致（>用户自助改密的 8 位）。
const MinPasswordLength = 12

var (
	// ErrNotFound 目标管理员不存在。
	ErrNotFound = errors.New("管理员不存在")
	// ErrUsernameTaken 用户名已被占用。
	ErrUsernameTaken = errors.New("用户名已被占用")
	// ErrBadRole 角色非法。
	ErrBadRole = errors.New("角色不存在，请先在「角色管理」中创建")
	// ErrReservedRole 内置角色 user 为普通用户身份，不经后台账号接口维护。
	ErrReservedRole = errors.New("内置角色 user 属普通用户，请在「普通用户管理」中维护")
	// ErrWeakPassword 密码强度不足（统一实现见 auth.ValidatePassword）。
	ErrWeakPassword = auth.ErrWeakPassword
	// ErrBadUsername 用户名格式非法。
	ErrBadUsername = errors.New("用户名长度需在 1-80 之间")
	// ErrLastAdmin 会造成没有任何启用状态 admin 账号，拒绝以保证系统可管理。
	ErrLastAdmin = errors.New("必须保留至少一个启用状态的 admin 账号")
)

// Service 管理员账号服务。
type Service struct {
	db *gorm.DB
	// policy 后台账号密码策略（默认 DefaultAdminPolicy，可由装配层按配置覆盖）。
	policy auth.PasswordPolicy
	// revoker 会话撤销器（可选）；改角色 / 停用 / 重置密码 / 删除后强制下线。
	revoker SessionRevoker
}

// SessionRevoker 撤销 / 统计某账号的会话。
// 由装配层注入（auth.SessionStore 满足该接口），避免包间循环依赖。
type SessionRevoker interface {
	DeleteByAccountID(ctx context.Context, accountID string) error
	ListByAccountID(ctx context.Context, accountID string) ([]auth.Session, error)
}

// NewService 创建账号服务。
func NewService(db *gorm.DB) *Service { return &Service{db: db, policy: auth.DefaultAdminPolicy()} }

// WithPasswordPolicy 覆盖后台账号密码策略（装配层从配置注入）。
func (s *Service) WithPasswordPolicy(p auth.PasswordPolicy) *Service {
	s.policy = p
	return s
}

// WithSessionRevoker 注入会话撤销器（未注入时跳过撤销，不报错）。
func (s *Service) WithSessionRevoker(r SessionRevoker) *Service {
	s.revoker = r
	return s
}

// revokeSessions 尽力撤销账号会话；失败不阻断主流程（会话仍有绝对有效期兜底）。
func (s *Service) revokeSessions(ctx context.Context, accountID string) {
	if s.revoker == nil || accountID == "" {
		return
	}
	_ = s.revoker.DeleteByAccountID(ctx, accountID)
}

// CreateInput 创建管理员入参。
type CreateInput struct {
	Username string
	Password string
	Role     string
}

// UpdateInput 更新管理员入参（仅白名单字段，指针区分「未提供」与「置零」）。
type UpdateInput struct {
	Role     *string
	IsActive *bool
}

// List 分页查询后台账号（按用户名模糊匹配，排除普通用户）。
func (s *Service) List(ctx context.Context, q string, page, pageSize int) ([]domain.Admin, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	tx := s.db.WithContext(ctx).Model(&domain.Admin{}).Where("role <> ?", domain.RoleUser)
	if q = strings.TrimSpace(q); q != "" {
		tx = tx.Where("username ILIKE ?", "%"+q+"%")
	}

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var items []domain.Admin
	if err := tx.Order("created_at ASC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// GetByID 按 ID 查询。
func (s *Service) GetByID(ctx context.Context, id string) (*domain.Admin, error) {
	if id == "" {
		return nil, ErrNotFound
	}
	return s.findByID(ctx, id)
}

// Create 创建后台账号：校验用户名唯一、角色合法（非普通用户）与密码强度。
func (s *Service) Create(ctx context.Context, in CreateInput) (*domain.Admin, error) {
	username := strings.TrimSpace(in.Username)
	if username == "" || len(username) > 80 {
		return nil, ErrBadUsername
	}
	if strings.TrimSpace(in.Role) == domain.RoleUser {
		return nil, ErrReservedRole
	}
	if err := s.validateRole(ctx, in.Role); err != nil {
		return nil, err
	}
	if err := auth.ValidatePassword(in.Password, s.policy, username); err != nil {
		return nil, err
	}

	// 用户名在 accounts 中全局唯一，需与普通用户一并查重。
	var existing int64
	if err := s.db.WithContext(ctx).Model(&domain.Admin{}).
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
	admin := &domain.Admin{
		Username:     username,
		PasswordHash: hash,
		Role:         in.Role,
		IsActive:     true,
	}
	if err := s.db.WithContext(ctx).Create(admin).Error; err != nil {
		return nil, err
	}
	return admin, nil
}

// Update 更新角色与启用状态；拒绝把系统最后一名启用 admin 降级或停用。
//
// 并发安全：整个判定 + 写入在**单个事务**内完成，并先获取会话级咨询锁
// （pg_advisory_xact_lock）串行化所有「可能减少启用 admin 数量」的操作。
// 否则两个并发请求会各自读到 2 名启用 admin 后双双通过，最终系统无可用 admin。
func (s *Service) Update(ctx context.Context, id string, in UpdateInput) (*domain.Admin, error) {
	var result *domain.Admin
	securityChanged := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockAdminGuard(tx); err != nil {
			return err
		}

		admin, err := findByIDTx(tx, id)
		if err != nil {
			return err
		}
		if admin == nil {
			return ErrNotFound
		}

		updates := map[string]any{}

		if in.Role != nil && *in.Role != admin.Role {
			if strings.TrimSpace(*in.Role) == domain.RoleUser {
				return ErrReservedRole
			}
			if err := validateRoleTx(tx, *in.Role); err != nil {
				return err
			}
			// 降级最后一名启用 admin 同样会导致无人可管理，需拦截。
			if admin.Role == domain.RoleAdmin && admin.IsActive {
				if n, err := countActiveAdminsTx(tx); err != nil {
					return err
				} else if n <= 1 {
					return ErrLastAdmin
				}
			}
			updates["role"] = *in.Role
			securityChanged = true
		}

		if in.IsActive != nil && *in.IsActive != admin.IsActive {
			if !*in.IsActive && admin.Role == domain.RoleAdmin {
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
			result = admin
			return nil
		}
		updates["updated_at"] = gorm.Expr("now()")
		if err := tx.Model(&domain.Admin{}).
			Where("id = ? AND role <> ?", id, domain.RoleUser).Updates(updates).Error; err != nil {
			return err
		}
		result, err = findByIDTx(tx, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	// 改角色 / 停用后立即撤销该账号全部会话（gj.md §10.1）：权限边界变化后
	// 旧会话携带的角色信息已过期，必须重新登录获取新权限。
	if securityChanged {
		s.revokeSessions(ctx, id)
	}
	return result, nil
}

// ResetPassword 重置指定后台账号密码（不需旧密码，由管理员执行）。
//
// 账号恢复流程（gj.md §10.1）：重置后置 must_change_password = true，
// 该账号下次登录必须自行改密；同时撤销其全部会话（强制下线）。
func (s *Service) ResetPassword(ctx context.Context, id, newPassword string) error {
	admin, err := s.findByID(ctx, id)
	if err != nil {
		return err
	}
	if admin == nil {
		return ErrNotFound
	}
	if err := auth.ValidatePassword(newPassword, s.policy, admin.Username); err != nil {
		return err
	}

	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		return err
	}
	if err := s.db.WithContext(ctx).Model(&domain.Admin{}).
		Where("id = ? AND role <> ?", id, domain.RoleUser).
		Updates(map[string]any{
			"password_hash":        hash,
			"must_change_password": true,
			"updated_at":           gorm.Expr("now()"),
		}).Error; err != nil {
		return err
	}
	// 撤销全部会话：旧会话立即失效，必须用新密码重新登录。
	s.revokeSessions(ctx, id)
	return nil
}

// Delete 删除后台账号；拒绝删除系统最后一名启用 admin。
// accounts 表采用物理删除，调用方（handler）需叠加「不可删除自己」的约束。
//
// 并发安全同 Update：事务内先取咨询锁再统计，避免与并发降级/停用叠加后无 admin。
func (s *Service) Delete(ctx context.Context, id string) error {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockAdminGuard(tx); err != nil {
			return err
		}
		admin, err := findByIDTx(tx, id)
		if err != nil {
			return err
		}
		if admin == nil {
			return ErrNotFound
		}
		if admin.Role == domain.RoleAdmin && admin.IsActive {
			if n, err := countActiveAdminsTx(tx); err != nil {
				return err
			} else if n <= 1 {
				return ErrLastAdmin
			}
		}
		return tx.Where("id = ? AND role <> ?", id, domain.RoleUser).
			Delete(&domain.Admin{}).Error
	})
	if err != nil {
		return err
	}
	// 删除后撤销全部会话（共享存储由外键级联兜底，内存存储需显式清理）。
	s.revokeSessions(ctx, id)
	return nil
}

// ResetMFA 解绑指定后台账号的双因子认证（账号恢复）。
//
// 适用场景：管理员更换手机且恢复码丢失。这是高权限操作，必须由持有
// accounts.admins 的管理员执行并写入审计；解绑后该账号下次登录只需密码。
// 已撤销的会话不受影响：MFA 只影响登录，不会延长既有会话。
func (s *Service) ResetMFA(ctx context.Context, id string) error {
	admin, err := findByIDTx(s.db.WithContext(ctx), id)
	if err != nil {
		return err
	}
	if admin == nil {
		return ErrNotFound
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("account_id = ?", id).Delete(&domain.MFARecoveryCode{}).Error; err != nil {
			return err
		}
		return tx.Where("account_id = ?", id).Delete(&domain.AccountMFA{}).Error
	})
}

// DeletePreview 删除后台账号的影响预览（gj.md §12）。
type DeletePreview struct {
	AdminID        string   `json:"admin_id"`
	Username       string   `json:"username"`
	Role           string   `json:"role"`
	IsActive       bool     `json:"is_active"`
	IsSelf         bool     `json:"is_self"`
	IsLastAdmin    bool     `json:"is_last_admin"`
	ActiveSessions int      `json:"active_sessions"`
	Reversible     bool     `json:"reversible"`
	Notes          []string `json:"notes"`
}

// DeletePreview 计算删除后台账号的影响（不执行删除）。
func (s *Service) DeletePreview(ctx context.Context, id, actorID string) (*DeletePreview, error) {
	admin, err := s.findByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if admin == nil {
		return nil, ErrNotFound
	}
	p := &DeletePreview{
		AdminID:    admin.ID.String(),
		Username:   admin.Username,
		Role:       admin.Role,
		IsActive:   admin.IsActive,
		IsSelf:     actorID != "" && actorID == id,
		Reversible: false,
	}
	if admin.Role == domain.RoleAdmin && admin.IsActive {
		n, err := s.countActiveAdmins(ctx)
		if err != nil {
			return nil, err
		}
		p.IsLastAdmin = n <= 1
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
	notes = append(notes, "该账号的全部登录会话将被立即撤销")
	p.Notes = notes
	return p, nil
}

// adminGuardLockKey 是「最后一名启用 admin 保护」的会话级咨询锁键。
// 所有可能减少启用 admin 数量的操作共享同一键，从而串行化并发判定；
// 事务结束（提交或回滚）时 PostgreSQL 自动释放，无需显式解锁。
const adminGuardLockKey int64 = 0x6d636b6974 // "mckit"

// lockAdminGuard 在事务内获取咨询锁；必须在已开启的事务中调用。
func lockAdminGuard(tx *gorm.DB) error {
	return tx.Exec("SELECT pg_advisory_xact_lock(?)", adminGuardLockKey).Error
}

// findByID 按主键查询后台账号（排除普通用户），未命中返回 (nil, nil)。
func (s *Service) findByID(ctx context.Context, id string) (*domain.Admin, error) {
	return findByIDTx(s.db.WithContext(ctx), id)
}

// findByIDTx 基于给定事务/连接按主键查询后台账号（排除普通用户）。
func findByIDTx(tx *gorm.DB, id string) (*domain.Admin, error) {
	var admin domain.Admin
	err := tx.Where("id = ? AND role <> ?", id, domain.RoleUser).First(&admin).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &admin, nil
}

// countActiveAdmins 统计启用状态的 admin 账号数。
func (s *Service) countActiveAdmins(ctx context.Context) (int64, error) {
	return countActiveAdminsTx(s.db.WithContext(ctx))
}

// countActiveAdminsTx 基于给定事务/连接统计启用状态的 admin 账号数。
func countActiveAdminsTx(tx *gorm.DB) (int64, error) {
	var n int64
	err := tx.Model(&domain.Admin{}).
		Where("role = ? AND is_active = true", domain.RoleAdmin).
		Count(&n).Error
	return n, err
}

// validateRole 校验角色存在（以 roles 表为准，支持自定义角色）。
func (s *Service) validateRole(ctx context.Context, role string) error {
	return validateRoleTx(s.db.WithContext(ctx), role)
}

// validateRoleTx 基于给定事务/连接校验角色存在。
// accounts.role 是 roles(name) 的外键，因此这里必须要求角色行真实存在。
func validateRoleTx(tx *gorm.DB, role string) error {
	name := strings.TrimSpace(role)
	if name == "" {
		return ErrBadRole
	}
	var n int64
	if err := tx.Model(&domain.Role{}).
		Where("name = ?", name).Count(&n).Error; err != nil {
		return err
	}
	if n == 0 {
		return ErrBadRole
	}
	return nil
}
