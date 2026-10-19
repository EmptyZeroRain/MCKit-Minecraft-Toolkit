package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// sessionRow sessions 表的存储模型（仅本包内使用）。
//
// 与内存实现一一对应；account_id 外键指向 accounts(id) 并级联删除，
// 因此删除账号会自动清理其会话（gj.md §10.1「删除后立即撤销会话」）。
type sessionRow struct {
	SessionID          string    `gorm:"primaryKey;size:128"`
	AccountID          uuid.UUID `gorm:"type:uuid;not null;index"`
	Username           string    `gorm:"size:80;not null"`
	Role               string    `gorm:"size:40;not null"`
	CSRFToken          string    `gorm:"size:128;not null"`
	MustChangePassword bool      `gorm:"not null;default:false"`
	CreatedAt          time.Time `gorm:"not null"`
	LastSeenAt         time.Time `gorm:"not null"`
	ExpiresAt          time.Time `gorm:"not null;index"`
}

// TableName 指定表名。
func (sessionRow) TableName() string { return "sessions" }

func (r sessionRow) toSession() Session {
	return Session{
		SessionID:          r.SessionID,
		AccountID:          r.AccountID.String(),
		Username:           r.Username,
		Role:               r.Role,
		CSRFToken:          r.CSRFToken,
		MustChangePassword: r.MustChangePassword,
		CreatedAt:          r.CreatedAt,
		LastSeenAt:         r.LastSeenAt,
		ExpiresAt:          r.ExpiresAt,
	}
}

// PostgresSessionStore 基于 PostgreSQL 的共享会话存储。
//
// 用于多实例部署：会话在所有实例间共享，创建 / 续期 / 撤销即时一致，
// 进程重启不会丢失会话。单实例部署可用 MemorySessionStore。
type PostgresSessionStore struct {
	db   *gorm.DB
	opts SessionOptions
}

// NewPostgresSessionStore 创建共享会话存储。
func NewPostgresSessionStore(db *gorm.DB, opts SessionOptions) *PostgresSessionStore {
	if opts.TTL <= 0 {
		opts.TTL = DefaultSessionTTL
	}
	return &PostgresSessionStore{db: db, opts: opts}
}

// TTL 返回会话绝对有效期。
func (p *PostgresSessionStore) TTL() time.Duration { return p.opts.TTL }

// Create 保存会话。
func (p *PostgresSessionStore) Create(ctx context.Context, s Session) error {
	p.opts.normalize(&s)
	accountID, err := uuid.Parse(s.AccountID)
	if err != nil {
		return fmt.Errorf("会话账号 ID 非法: %w", err)
	}
	row := sessionRow{
		SessionID:          s.SessionID,
		AccountID:          accountID,
		Username:           s.Username,
		Role:               s.Role,
		CSRFToken:          s.CSRFToken,
		MustChangePassword: s.MustChangePassword,
		CreatedAt:          s.CreatedAt,
		LastSeenAt:         s.LastSeenAt,
		ExpiresAt:          s.ExpiresAt,
	}
	if err := p.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("%w: %v", ErrSessionStoreUnavailable, err)
	}
	return nil
}

// Get 读取会话并检查有效期；过期会话惰性删除后返回 (nil, nil)。
func (p *PostgresSessionStore) Get(ctx context.Context, sessionID string) (*Session, error) {
	if sessionID == "" {
		return nil, nil
	}
	var row sessionRow
	err := p.db.WithContext(ctx).Where("session_id = ?", sessionID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSessionStoreUnavailable, err)
	}

	s := row.toSession()
	now := time.Now()
	if p.opts.expired(&s, now) {
		// 读取路径上的惰性清理：失败不影响本次鉴权结果（返回「无会话」）。
		_ = p.db.WithContext(ctx).Where("session_id = ?", sessionID).Delete(&sessionRow{}).Error
		return nil, nil
	}
	// 节流写回最近活动时间，避免每请求一次写入。
	if now.Sub(s.LastSeenAt) >= lastSeenRefreshInterval {
		s.LastSeenAt = now
		_ = p.db.WithContext(ctx).Model(&sessionRow{}).
			Where("session_id = ?", sessionID).
			Update("last_seen_at", now).Error
	}
	return &s, nil
}

// Delete 删除会话。
func (p *PostgresSessionStore) Delete(ctx context.Context, sessionID string) error {
	if err := p.db.WithContext(ctx).Where("session_id = ?", sessionID).Delete(&sessionRow{}).Error; err != nil {
		return fmt.Errorf("%w: %v", ErrSessionStoreUnavailable, err)
	}
	return nil
}

// DeleteByAccountID 删除指定账号的全部会话。
func (p *PostgresSessionStore) DeleteByAccountID(ctx context.Context, accountID string) error {
	if accountID == "" {
		return nil
	}
	id, err := uuid.Parse(accountID)
	if err != nil {
		return nil
	}
	if err := p.db.WithContext(ctx).Where("account_id = ?", id).Delete(&sessionRow{}).Error; err != nil {
		return fmt.Errorf("%w: %v", ErrSessionStoreUnavailable, err)
	}
	return nil
}

// ListByAccountID 列出账号的有效会话（按最近活动倒序）。
func (p *PostgresSessionStore) ListByAccountID(ctx context.Context, accountID string) ([]Session, error) {
	if accountID == "" {
		return nil, nil
	}
	id, err := uuid.Parse(accountID)
	if err != nil {
		return nil, nil
	}
	now := time.Now()
	q := p.db.WithContext(ctx).
		Where("account_id = ? AND expires_at > ?", id, now)
	if p.opts.IdleTTL > 0 {
		q = q.Where("last_seen_at > ?", now.Add(-p.opts.IdleTTL))
	}
	var rows []sessionRow
	if err := q.Order("last_seen_at DESC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSessionStoreUnavailable, err)
	}
	out := make([]Session, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.toSession())
	}
	return out, nil
}

// PurgeExpired 清理已过期会话。
func (p *PostgresSessionStore) PurgeExpired(ctx context.Context) (int64, error) {
	now := time.Now()
	q := p.db.WithContext(ctx).Where("expires_at <= ?", now)
	if p.opts.IdleTTL > 0 {
		q = q.Or("last_seen_at <= ?", now.Add(-p.opts.IdleTTL))
	}
	res := q.Delete(&sessionRow{})
	if res.Error != nil {
		return 0, fmt.Errorf("%w: %v", ErrSessionStoreUnavailable, res.Error)
	}
	return res.RowsAffected, nil
}
