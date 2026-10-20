// Package contentfilter 提供内容关键字屏蔽规则与实时匹配。
//
// 规则由后台维护；社区帖子 / 留言创建时由服务端同步校验，前端无法绕过。
// 匹配使用小写 + 空白折叠后的文本，关键字本身不区分大小写。
package contentfilter

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"mckit/backend/internal/domain"
)

const (
	maxKeywordLength = 100
	maxNoteLength    = 255
)

var (
	ErrNotFound  = errors.New("关键字不存在")
	ErrBadInput  = errors.New("关键字需为 1-100 个字符")
	ErrDuplicate = errors.New("关键字已存在")
	ErrBadNote   = errors.New("备注不能超过 255 个字符")
)

// Service 关键字屏蔽服务。
type Service struct {
	db *gorm.DB
}

// NewService 创建服务。
func NewService(db *gorm.DB) *Service { return &Service{db: db} }

// List 返回全部关键字（启用优先，按创建时间倒序）。
func (s *Service) List(ctx context.Context) ([]domain.ContentKeyword, error) {
	var out []domain.ContentKeyword
	err := s.db.WithContext(ctx).
		Order("enabled DESC, created_at DESC").
		Find(&out).Error
	if out == nil {
		out = []domain.ContentKeyword{}
	}
	return out, err
}

// Create 新增关键字。重复判断基于 normalized，大小写与多余空白不会产生重复项。
func (s *Service) Create(ctx context.Context, keyword, note, operatorID string) (*domain.ContentKeyword, error) {
	keyword, normalized, err := normalizeInput(keyword, note)
	if err != nil {
		return nil, err
	}
	var count int64
	if err := s.db.WithContext(ctx).Model(&domain.ContentKeyword{}).
		Where("normalized = ?", normalized).Count(&count).Error; err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, ErrDuplicate
	}
	row := &domain.ContentKeyword{
		Keyword:    keyword,
		Normalized: normalized,
		Enabled:    true,
		Note:       strings.TrimSpace(note),
		CreatedBy:  parseUUIDPtr(operatorID),
	}
	if err := s.db.WithContext(ctx).Create(row).Error; err != nil {
		return nil, err
	}
	return row, nil
}

// Update 更新关键字、备注与启用状态。
func (s *Service) Update(ctx context.Context, id, keyword, note string, enabled bool) (*domain.ContentKeyword, error) {
	kid, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return nil, ErrNotFound
	}
	keyword, normalized, err := normalizeInput(keyword, note)
	if err != nil {
		return nil, err
	}
	var count int64
	if err := s.db.WithContext(ctx).Model(&domain.ContentKeyword{}).
		Where("normalized = ? AND id <> ?", normalized, kid).Count(&count).Error; err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, ErrDuplicate
	}
	res := s.db.WithContext(ctx).Model(&domain.ContentKeyword{}).
		Where("id = ?", kid).
		Updates(map[string]any{
			"keyword":    keyword,
			"normalized": normalized,
			"note":       strings.TrimSpace(note),
			"enabled":    enabled,
		})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, ErrNotFound
	}
	var row domain.ContentKeyword
	if err := s.db.WithContext(ctx).Where("id = ?", kid).First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

// Delete 物理删除关键字（仅规则本身，不影响已发布内容）。
func (s *Service) Delete(ctx context.Context, id string) error {
	kid, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return ErrNotFound
	}
	res := s.db.WithContext(ctx).Where("id = ?", kid).Delete(&domain.ContentKeyword{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// CheckText 返回第一个命中的启用关键字；未命中返回空串。
//
// 每次内容创建都读取启用规则，保证后台启停立即生效。规则数量按运营场景
// 控制在可控范围，若后续增长到数千级可再引入缓存与失效通知。
func (s *Service) CheckText(ctx context.Context, text string) (string, error) {
	haystack := normalizeText(text)
	if haystack == "" {
		return "", nil
	}
	var rows []domain.ContentKeyword
	if err := s.db.WithContext(ctx).
		Select("keyword", "normalized").
		Where("enabled = true").
		Find(&rows).Error; err != nil {
		return "", err
	}
	for _, row := range rows {
		if row.Normalized != "" && strings.Contains(haystack, row.Normalized) {
			return row.Keyword, nil
		}
	}
	return "", nil
}

func normalizeInput(keyword, note string) (string, string, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" || utf8.RuneCountInString(keyword) > maxKeywordLength {
		return "", "", ErrBadInput
	}
	if utf8.RuneCountInString(strings.TrimSpace(note)) > maxNoteLength {
		return "", "", ErrBadNote
	}
	return keyword, normalizeText(keyword), nil
}

// normalizeText 小写并折叠连续空白，降低通过大小写 / 多余空格绕过规则的概率。
func normalizeText(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

func parseUUIDPtr(s string) *uuid.UUID {
	raw := strings.TrimSpace(s)
	if raw == "" {
		return nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil
	}
	return &id
}
