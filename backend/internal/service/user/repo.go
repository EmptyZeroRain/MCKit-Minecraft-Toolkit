// Package user 提供账号个人数据（收藏 / 最近使用 / 常用种子）与前台自助服务（需求 §12）。
//
// 安全边界：
//   - 统一账号（迁移 0022）后普通用户与后台账号同表（accounts），靠 role 区分；
//   - 账号查询按主键 / 用户名 / 邮箱全局检索（用户名与邮箱本就全局唯一），
//     由服务层决定哪些入口允许后台账号使用；
//   - 前台登录（/user/login）仍只接受 role = 'user'，后台账号必须走
//     /admin/auth/login，避免绕过后台的 MFA；
//   - 本包不涉及任何后台权限；个人数据查询一律以 user_id 过滤，杜绝越权访问他人数据。
package user

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"mckit/backend/internal/domain"
)

// Repo 用户与个人数据仓储接口（消费端定义，便于测试替身）。
type Repo interface {
	FindByUsername(ctx context.Context, username string) (*domain.User, error)
	// FindByEmail 按邮箱查询普通用户（邮箱统一小写存储）；不存在返回 (nil, nil)。
	FindByEmail(ctx context.Context, email string) (*domain.User, error)
	FindByID(ctx context.Context, id string) (*domain.User, error)
	Create(ctx context.Context, u *domain.User) error
	UpdateLastLogin(ctx context.Context, id string) error
	ChangePassword(ctx context.Context, id, hash string) error
	// UpdateProfile 更新展示资料（昵称 / 头像 URL），仅限白名单字段与普通用户。
	UpdateProfile(ctx context.Context, id, nickname, avatarURL string) error
	// DeleteUser 删除账号；个人数据由数据库外键 ON DELETE CASCADE 级联清除。
	DeleteUser(ctx context.Context, id string) error

	ListFavorites(ctx context.Context, userID string) ([]domain.UserFavorite, error)
	FavoriteExists(ctx context.Context, userID, kind, objectID string) (bool, error)
	UpsertFavorite(ctx context.Context, f *domain.UserFavorite) error
	DeleteFavorite(ctx context.Context, userID, kind, objectID string) error

	ListRecents(ctx context.Context, userID string) ([]domain.UserRecent, error)
	UpsertRecent(ctx context.Context, r *domain.UserRecent) error
	PruneRecents(ctx context.Context, userID string, keep int) error
	ClearRecents(ctx context.Context, userID string) error

	ListSeeds(ctx context.Context, userID string) ([]domain.UserSeed, error)
	CreateSeed(ctx context.Context, s *domain.UserSeed) error
	UpdateSeed(ctx context.Context, userID, seedID, name, seed, note string) error
	DeleteSeed(ctx context.Context, userID, seedID string) error
}

// GormRepo 基于 GORM 的仓储实现。
type GormRepo struct{ db *gorm.DB }

// NewRepo 创建 GORM 仓储。
func NewRepo(db *gorm.DB) *GormRepo { return &GormRepo{db: db} }

// FindByUsername 按用户名查询账号（用户名全局唯一）；不存在返回 (nil, nil)。
// 是否允许该角色使用调用入口由服务层判定（例如前台登录只接受 role = 'user'）。
func (r *GormRepo) FindByUsername(ctx context.Context, username string) (*domain.User, error) {
	var u domain.User
	err := r.db.WithContext(ctx).
		Where("username = ?", username).
		First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// FindByEmail 按邮箱查询账号（邮箱全局唯一，后台账号可为空串）；不存在返回 (nil, nil)。
func (r *GormRepo) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	if email == "" {
		return nil, nil
	}
	var u domain.User
	err := r.db.WithContext(ctx).
		Where("email = ?", email).
		First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// FindByID 按 ID 查询账号（任意角色）；不存在返回 (nil, nil)。
// 前台自助功能（资料 / 改密 / 显示信息）对管理员同样开放。
func (r *GormRepo) FindByID(ctx context.Context, id string) (*domain.User, error) {
	var u domain.User
	err := r.db.WithContext(ctx).
		Where("id = ?", id).
		First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// Create 新建用户。
func (r *GormRepo) Create(ctx context.Context, u *domain.User) error {
	return r.db.WithContext(ctx).Create(u).Error
}

// UpdateLastLogin 记录最后登录时间（失败不阻断登录）。
func (r *GormRepo) UpdateLastLogin(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Model(&domain.User{}).
		Where("id = ?", id).Update("last_login_at", gorm.Expr("now()")).Error
}

// ChangePassword 更新密码哈希（任意角色；密码策略由服务层按角色选择），并清空强制改密标记。
func (r *GormRepo) ChangePassword(ctx context.Context, id, hash string) error {
	return r.db.WithContext(ctx).Model(&domain.User{}).
		Where("id = ?", id).
		Updates(map[string]any{"password_hash": hash, "must_change_password": false}).Error
}

// UpdateProfile 更新展示资料（昵称 / 头像 URL；任意角色，管理员也有展示资料）。
// 仅写入白名单字段，避免把请求体其余键带入 UPDATE（防字段注入）。
func (r *GormRepo) UpdateProfile(ctx context.Context, id, nickname, avatarURL string) error {
	return r.db.WithContext(ctx).Model(&domain.User{}).
		Where("id = ?", id).
		Updates(map[string]any{"nickname": nickname, "avatar_url": avatarURL}).Error
}

// DeleteUser 删除普通用户账号（个人数据级联删除；仅普通用户）。
func (r *GormRepo) DeleteUser(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).
		Where("id = ? AND role = ?", id, domain.RoleUser).
		Delete(&domain.User{}).Error
}

// ListFavorites 列出收藏，按创建时间倒序。
func (r *GormRepo) ListFavorites(ctx context.Context, userID string) ([]domain.UserFavorite, error) {
	var out []domain.UserFavorite
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).
		Order("created_at DESC").Limit(MaxFavorites).Find(&out).Error
	return out, err
}

// FavoriteExists 判断是否已收藏。
func (r *GormRepo) FavoriteExists(ctx context.Context, userID, kind, objectID string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&domain.UserFavorite{}).
		Where("user_id = ? AND kind = ? AND object_id = ?", userID, kind, objectID).
		Count(&count).Error
	return count > 0, err
}

// UpsertFavorite 插入或更新收藏（同一 user/kind/object 唯一）。
func (r *GormRepo) UpsertFavorite(ctx context.Context, f *domain.UserFavorite) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "kind"}, {Name: "object_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"label", "url"}),
	}).Create(f).Error
}

// DeleteFavorite 取消收藏。
func (r *GormRepo) DeleteFavorite(ctx context.Context, userID, kind, objectID string) error {
	return r.db.WithContext(ctx).Where("user_id = ? AND kind = ? AND object_id = ?", userID, kind, objectID).
		Delete(&domain.UserFavorite{}).Error
}

// ListRecents 列出最近使用，按访问时间倒序。
func (r *GormRepo) ListRecents(ctx context.Context, userID string) ([]domain.UserRecent, error) {
	var out []domain.UserRecent
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).
		Order("visited_at DESC").Limit(MaxRecents).Find(&out).Error
	return out, err
}

// UpsertRecent 插入或刷新最近使用记录。
// 说明：visited_at 使用服务端传入的时间（GORM 不会自动维护该字段名）。
func (r *GormRepo) UpsertRecent(ctx context.Context, rec *domain.UserRecent) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "kind"}, {Name: "object_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"label":      rec.Label,
			"url":        rec.URL,
			"visited_at": rec.VisitedAt,
		}),
	}).Create(rec).Error
}

// PruneRecents 仅保留最近 keep 条记录（个人数据保留期限 / 容量上限）。
func (r *GormRepo) PruneRecents(ctx context.Context, userID string, keep int) error {
	if keep <= 0 {
		return nil
	}
	return r.db.WithContext(ctx).Exec(`
		DELETE FROM user_recents
		WHERE user_id = ?
		  AND id NOT IN (
		    SELECT id FROM user_recents WHERE user_id = ? ORDER BY visited_at DESC LIMIT ?
		  )`, userID, userID, keep).Error
}

// ClearRecents 清空该用户全部最近使用记录。
func (r *GormRepo) ClearRecents(ctx context.Context, userID string) error {
	return r.db.WithContext(ctx).Where("user_id = ?", userID).Delete(&domain.UserRecent{}).Error
}

// ListSeeds 列出常用种子，按创建时间倒序。
func (r *GormRepo) ListSeeds(ctx context.Context, userID string) ([]domain.UserSeed, error) {
	var out []domain.UserSeed
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).
		Order("created_at DESC").Limit(MaxSeeds).Find(&out).Error
	return out, err
}

// CreateSeed 新建常用种子。
func (r *GormRepo) CreateSeed(ctx context.Context, s *domain.UserSeed) error {
	return r.db.WithContext(ctx).Omit("id").Create(s).Error
}

// UpdateSeed 更新常用种子（限本人）。
func (r *GormRepo) UpdateSeed(ctx context.Context, userID, seedID, name, seed, note string) error {
	return r.db.WithContext(ctx).Model(&domain.UserSeed{}).
		Where("id = ? AND user_id = ?", seedID, userID).
		Updates(map[string]any{"name": name, "seed": seed, "note": note, "updated_at": gorm.Expr("now()")}).Error
}

// DeleteSeed 删除常用种子（限本人）。
func (r *GormRepo) DeleteSeed(ctx context.Context, userID, seedID string) error {
	return r.db.WithContext(ctx).Where("id = ? AND user_id = ?", seedID, userID).
		Delete(&domain.UserSeed{}).Error
}
