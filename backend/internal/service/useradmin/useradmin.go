// Package useradmin 提供普通用户账号的管理端查询（后台「普通用户管理」）。
//
// 设计说明：
//   - 本包只服务管理端读取，权限由路由层（RequireAuth + RequirePermission + CSRF）保证；
//   - 统一账号（迁移 0022）后普通用户与后台账号同表，靠 role 区分：
//     本包一律过滤 role = 'user'，绝不返回后台账号；
//   - 个人数据计数（收藏 / 最近使用 / 常用种子）按页批量聚合，避免 N+1 查询。
package useradmin

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"mckit/backend/internal/domain"
)

// 状态筛选值。
const (
	StatusAll      = ""         // 不限
	StatusActive   = "active"   // 仅启用
	StatusDisabled = "disabled" // 仅禁用
)

// Service 普通用户管理服务（只读）。
type Service struct {
	db *gorm.DB
}

// NewService 创建服务。
func NewService(db *gorm.DB) *Service { return &Service{db: db} }

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

// List 分页查询普通用户（用户名模糊匹配 + 状态筛选），并附带个人数据计数。
func (s *Service) List(ctx context.Context, f ListFilter) ([]Item, int64, error) {
	page, pageSize := normalize(f.Page, f.PageSize)

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

	ids := make([]uuid.UUID, 0, len(users))
	for _, u := range users {
		ids = append(ids, u.ID)
	}

	favCounts, err := s.countByUser(ctx, &domain.UserFavorite{}, ids)
	if err != nil {
		return nil, 0, err
	}
	recCounts, err := s.countByUser(ctx, &domain.UserRecent{}, ids)
	if err != nil {
		return nil, 0, err
	}
	seedCounts, err := s.countByUser(ctx, &domain.UserSeed{}, ids)
	if err != nil {
		return nil, 0, err
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
	return items, total, nil
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
