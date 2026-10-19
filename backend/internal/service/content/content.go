// Package content 提供工具/百科/分类/标签/反馈（管理端 CRUD）业务逻辑。
package content

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"mckit/backend/internal/domain"
	"mckit/backend/internal/repository/types"
)

// 通用错误。
var (
	ErrNotFound   = errors.New("记录不存在")
	ErrSlugTaken  = errors.New("slug 已被占用")
	ErrBadEnum    = errors.New("枚举值非法")
	ErrBadContent = errors.New("内容校验失败")
	// ErrDuplicateSubmission 同一来源在时间窗内重复提交（TODO §17 反垃圾）。
	ErrDuplicateSubmission = errors.New("重复提交")
)

// Service 内容管理服务。
type Service struct {
	db *gorm.DB
}

// NewService 创建内容管理服务。
func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// ---- 工具 ----

// ToolInput 创建/更新工具入参。
type ToolInput struct {
	Slug              string
	Name              string
	Summary           string
	Description       string
	Category          string
	Icon              string
	Tags              []string
	SupportedVersions []string
	// InputSchema/OutputSchema 前端为字段描述数组，保持任意 JSON 结构。
	InputSchema  json.RawMessage
	OutputSchema json.RawMessage
	IsFeatured   bool
	SortOrder    int
	Status       string
}

// CreateTool 创建工具。
func (s *Service) CreateTool(ctx context.Context, in *ToolInput) (*domain.Tool, error) {
	if err := validateTool(in); err != nil {
		return nil, err
	}
	if in.Status == "" {
		in.Status = domain.StatusPublished
	}
	if !validToolStatus(in.Status) {
		return nil, fmt.Errorf("%w: status=%s", ErrBadEnum, in.Status)
	}
	if err := s.checkSlugFree(&domain.Tool{}, "", in.Slug); err != nil {
		return nil, err
	}
	tool := &domain.Tool{
		Slug:              in.Slug,
		Name:              in.Name,
		Summary:           in.Summary,
		Description:       in.Description,
		Category:          in.Category,
		Icon:              in.Icon,
		Tags:              types.JSONSlice(in.Tags),
		SupportedVersions: types.JSONSlice(in.SupportedVersions),
		InputSchema:       types.JSONBArray(in.InputSchema),
		OutputSchema:      types.JSONBArray(in.OutputSchema),
		IsFeatured:        in.IsFeatured,
		SortOrder:         in.SortOrder,
		Status:            in.Status,
	}
	if err := s.db.WithContext(ctx).Create(tool).Error; err != nil {
		return nil, err
	}
	return tool, nil
}

// UpdateTool 更新工具。
func (s *Service) UpdateTool(ctx context.Context, id string, in *ToolInput) (*domain.Tool, error) {
	tool, err := s.GetToolByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if in.Category != "" && !validToolCategory(in.Category) {
		return nil, fmt.Errorf("%w: category=%s", ErrBadEnum, in.Category)
	}
	if in.Status != "" && !validToolStatus(in.Status) {
		return nil, fmt.Errorf("%w: status=%s", ErrBadEnum, in.Status)
	}
	if in.Slug != "" && in.Slug != tool.Slug {
		if err := s.checkSlugFree(&domain.Tool{}, id, in.Slug); err != nil {
			return nil, err
		}
		tool.Slug = in.Slug
	}
	if in.Name != "" {
		tool.Name = in.Name
	}
	if in.Summary != "" {
		tool.Summary = in.Summary
	}
	if in.Description != "" {
		tool.Description = in.Description
	}
	if in.Category != "" {
		tool.Category = in.Category
	}
	if in.Icon != "" {
		tool.Icon = in.Icon
	}
	if in.Tags != nil {
		tool.Tags = types.JSONSlice(in.Tags)
	}
	if in.SupportedVersions != nil {
		tool.SupportedVersions = types.JSONSlice(in.SupportedVersions)
	}
	if in.InputSchema != nil {
		tool.InputSchema = types.JSONBArray(in.InputSchema)
	}
	if in.OutputSchema != nil {
		tool.OutputSchema = types.JSONBArray(in.OutputSchema)
	}
	tool.IsFeatured = in.IsFeatured
	tool.SortOrder = in.SortOrder
	if in.Status != "" {
		tool.Status = in.Status
	}
	if err := s.db.WithContext(ctx).Save(tool).Error; err != nil {
		return nil, err
	}
	return tool, nil
}

// RemoveTool 下架工具（软状态）。
func (s *Service) RemoveTool(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).Model(&domain.Tool{}).
		Where("id = ?", id).Update("status", domain.StatusHidden).Error
}

// GetToolByID 工具详情。
func (s *Service) GetToolByID(ctx context.Context, id string) (*domain.Tool, error) {
	var tool domain.Tool
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&tool).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &tool, nil
}

// GetToolBySlug 工具按 slug 查询。
func (s *Service) GetToolBySlug(ctx context.Context, slug string, publishedOnly bool) (*domain.Tool, error) {
	var tool domain.Tool
	q := s.db.WithContext(ctx).Where("slug = ?", slug)
	if publishedOnly {
		q = q.Where("status = ?", domain.StatusPublished)
	}
	err := q.First(&tool).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &tool, nil
}

// ToolFilter 工具列表筛选。
type ToolFilter struct {
	Category      string
	Q             string
	PublishedOnly bool
	Page          int
	PageSize      int
}

// ListTools 工具列表。
func (s *Service) ListTools(ctx context.Context, f ToolFilter) ([]domain.Tool, int64, error) {
	q := s.db.WithContext(ctx).Model(&domain.Tool{})
	if f.PublishedOnly {
		q = q.Where("status = ?", domain.StatusPublished)
	}
	if f.Category != "" {
		q = q.Where("category = ?", f.Category)
	}
	if f.Q != "" {
		kw := "%" + f.Q + "%"
		q = q.Where("name ILIKE ? OR summary ILIKE ? OR slug ILIKE ?", kw, kw, kw)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	page, pageSize := normalizePage(f.Page, f.PageSize)
	var items []domain.Tool
	err := q.Order("sort_order ASC, created_at DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func validateTool(in *ToolInput) error {
	if strings.TrimSpace(in.Slug) == "" {
		return errors.New("slug 必填")
	}
	if strings.TrimSpace(in.Name) == "" {
		return errors.New("name 必填")
	}
	if strings.TrimSpace(in.Summary) == "" {
		return errors.New("summary 必填")
	}
	if !validToolCategory(in.Category) {
		return fmt.Errorf("%w: category=%s", ErrBadEnum, in.Category)
	}
	return nil
}

func validToolCategory(c string) bool {
	switch c {
	case domain.ToolCategoryQuery, domain.ToolCategoryCalculation, domain.ToolCategoryGeneration,
		domain.ToolCategoryEditing, domain.ToolCategoryAuxiliary:
		return true
	}
	return false
}

// validToolStatus 工具状态白名单（removed 由删除接口管理）。
func validToolStatus(s string) bool {
	switch s {
	case domain.StatusDraft, domain.StatusPublished, domain.StatusHidden:
		return true
	}
	return false
}

// validWikiStatus 百科状态白名单（removed 由删除接口管理）。
func validWikiStatus(s string) bool {
	switch s {
	case domain.StatusDraft, domain.StatusPublished, domain.StatusHidden:
		return true
	}
	return false
}

// ---- 百科 ----

// WikiInput 百科条目入参。
type WikiInput struct {
	Slug          string
	Title         string
	Summary       string
	Content       string
	ContentFormat string
	ContentType   string
	GameEdition   string
	GameVersions  []string
	Tags          []string
	// SourceReferences 来源参考（前端为 {title,url} 对象数组，保持任意 JSON）。
	SourceReferences json.RawMessage
	Status           string
}

// CreateWiki 创建百科条目。
func (s *Service) CreateWiki(ctx context.Context, in *WikiInput, actorID string) (*domain.WikiEntry, error) {
	if strings.TrimSpace(in.Slug) == "" || strings.TrimSpace(in.Title) == "" {
		return nil, ErrBadContent
	}
	if strings.TrimSpace(in.Content) == "" {
		return nil, errors.New("content 必填")
	}
	if in.Status == "" {
		in.Status = domain.StatusDraft
	}
	if !validWikiStatus(in.Status) {
		return nil, fmt.Errorf("%w: status=%s", ErrBadEnum, in.Status)
	}
	if err := s.checkSlugFree(&domain.WikiEntry{}, "", in.Slug); err != nil {
		return nil, err
	}
	entry := &domain.WikiEntry{
		Slug:             in.Slug,
		Title:            in.Title,
		Summary:          in.Summary,
		Content:          in.Content,
		ContentFormat:    orDefault(in.ContentFormat, "markdown"),
		ContentType:      in.ContentType,
		GameEdition:      orDefault(in.GameEdition, "java"),
		GameVersions:     types.JSONSlice(in.GameVersions),
		Tags:             types.JSONSlice(in.Tags),
		SourceReferences: types.JSONBArray(in.SourceReferences),
		Status:           in.Status,
		CreatedBy:        parseUUID(actorID),
	}
	if in.Status == domain.StatusPublished {
		now := time.Now()
		entry.PublishedAt = &now
	}
	if err := s.db.WithContext(ctx).Create(entry).Error; err != nil {
		return nil, err
	}
	return entry, nil
}

// UpdateWiki 更新百科条目。
func (s *Service) UpdateWiki(ctx context.Context, id string, in *WikiInput, actorID string) (*domain.WikiEntry, error) {
	entry, err := s.GetWikiByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if in.Slug != "" && in.Slug != entry.Slug {
		if err := s.checkSlugFree(&domain.WikiEntry{}, id, in.Slug); err != nil {
			return nil, err
		}
		entry.Slug = in.Slug
	}
	if in.Title != "" {
		entry.Title = in.Title
	}
	if in.Summary != "" {
		entry.Summary = in.Summary
	}
	if in.Content != "" {
		entry.Content = in.Content
	}
	if in.ContentFormat != "" {
		entry.ContentFormat = in.ContentFormat
	}
	if in.ContentType != "" {
		entry.ContentType = in.ContentType
	}
	if in.GameEdition != "" {
		entry.GameEdition = in.GameEdition
	}
	if in.GameVersions != nil {
		entry.GameVersions = types.JSONSlice(in.GameVersions)
	}
	if in.Tags != nil {
		entry.Tags = types.JSONSlice(in.Tags)
	}
	if in.SourceReferences != nil {
		entry.SourceReferences = types.JSONBArray(in.SourceReferences)
	}
	if in.Status != "" {
		if !validWikiStatus(in.Status) {
			return nil, fmt.Errorf("%w: status=%s", ErrBadEnum, in.Status)
		}
		entry.Status = in.Status
		if in.Status == domain.StatusPublished && entry.PublishedAt == nil {
			now := time.Now()
			entry.PublishedAt = &now
		}
	}
	entry.UpdatedBy = parseUUID(actorID)
	if err := s.db.WithContext(ctx).Save(entry).Error; err != nil {
		return nil, err
	}
	return entry, nil
}

// RemoveWiki 软删除百科。
func (s *Service) RemoveWiki(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).Model(&domain.WikiEntry{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":     domain.StatusRemoved,
			"deleted_at": time.Now(),
		}).Error
}

// GetWikiByID 百科详情。
func (s *Service) GetWikiByID(ctx context.Context, id string) (*domain.WikiEntry, error) {
	var entry domain.WikiEntry
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&entry).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &entry, nil
}

// GetWikiBySlug 按 slug 查询百科（公开只返回已发布）。
func (s *Service) GetWikiBySlug(ctx context.Context, slug string, publishedOnly bool) (*domain.WikiEntry, error) {
	var entry domain.WikiEntry
	q := s.db.WithContext(ctx).Where("slug = ?", slug)
	if publishedOnly {
		q = q.Where("status = ?", domain.StatusPublished)
	}
	err := q.First(&entry).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &entry, nil
}

// ListWiki 百科列表。
func (s *Service) ListWiki(ctx context.Context, f ToolFilter) ([]domain.WikiEntry, int64, error) {
	q := s.db.WithContext(ctx).Model(&domain.WikiEntry{})
	if f.PublishedOnly {
		q = q.Where("status = ?", domain.StatusPublished)
	}
	if f.Q != "" {
		kw := "%" + f.Q + "%"
		q = q.Where("title ILIKE ? OR summary ILIKE ? OR slug ILIKE ?", kw, kw, kw)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	page, pageSize := normalizePage(f.Page, f.PageSize)
	var items []domain.WikiEntry
	err := q.Order("updated_at DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// ---- 分类与标签 ----

// CategoryInput 分类入参。
type CategoryInput struct {
	Name        string
	Slug        string
	ContentType string // tool | resource | wiki
	SortOrder   int
}

// CreateCategory 创建分类。
func (s *Service) CreateCategory(ctx context.Context, in *CategoryInput) (*domain.Category, error) {
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Slug) == "" {
		return nil, ErrBadContent
	}
	switch in.ContentType {
	case "tool", "resource", "wiki":
	default:
		return nil, ErrBadEnum
	}
	var count int64
	if err := s.db.WithContext(ctx).Model(&domain.Category{}).
		Where("content_type = ? AND slug = ?", in.ContentType, in.Slug).
		Count(&count).Error; err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, ErrSlugTaken
	}
	cat := &domain.Category{
		Name:        in.Name,
		Slug:        in.Slug,
		ContentType: in.ContentType,
		SortOrder:   in.SortOrder,
	}
	if err := s.db.WithContext(ctx).Create(cat).Error; err != nil {
		return nil, err
	}
	return cat, nil
}

// ListCategories 分类列表。
func (s *Service) ListCategories(ctx context.Context, contentType string) ([]domain.Category, error) {
	q := s.db.WithContext(ctx).Model(&domain.Category{}).Order("sort_order ASC, name ASC")
	if contentType != "" {
		q = q.Where("content_type = ?", contentType)
	}
	var items []domain.Category
	if err := q.Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

// UpdateCategory 更新分类（白名单字段）。
func (s *Service) UpdateCategory(ctx context.Context, id string, in *CategoryInput) (*domain.Category, error) {
	var cat domain.Category
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&cat).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	if in.Name != "" {
		cat.Name = in.Name
	}
	if in.ContentType != "" {
		switch in.ContentType {
		case "tool", "resource", "wiki":
		default:
			return nil, ErrBadEnum
		}
		cat.ContentType = in.ContentType
	}
	if in.Slug != "" && in.Slug != cat.Slug {
		var count int64
		if err := s.db.WithContext(ctx).Model(&domain.Category{}).
			Where("content_type = ? AND slug = ? AND id <> ?", cat.ContentType, in.Slug, id).
			Count(&count).Error; err != nil {
			return nil, err
		}
		if count > 0 {
			return nil, ErrSlugTaken
		}
		cat.Slug = in.Slug
	}
	cat.SortOrder = in.SortOrder
	if err := s.db.WithContext(ctx).Save(&cat).Error; err != nil {
		return nil, err
	}
	return &cat, nil
}

// DeleteCategory 删除分类（分类为轻量元数据，直接删除）。
func (s *Service) DeleteCategory(ctx context.Context, id string) error {
	res := s.db.WithContext(ctx).Where("id = ?", id).Delete(&domain.Category{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// TagInput 标签入参。
type TagInput struct {
	Name string
	Slug string
}

// CreateTag 创建标签。
func (s *Service) CreateTag(ctx context.Context, in *TagInput) (*domain.Tag, error) {
	if strings.TrimSpace(in.Name) == "" && strings.TrimSpace(in.Slug) == "" {
		return nil, ErrBadContent
	}
	slug := in.Slug
	if slug == "" {
		slug = slugify(in.Name)
	}
	var count int64
	if err := s.db.WithContext(ctx).Model(&domain.Tag{}).
		Where("slug = ?", slug).Count(&count).Error; err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, ErrSlugTaken
	}
	tag := &domain.Tag{Name: in.Name, Slug: slug}
	if err := s.db.WithContext(ctx).Create(tag).Error; err != nil {
		return nil, err
	}
	return tag, nil
}

// ListTags 标签列表。
func (s *Service) ListTags(ctx context.Context, q string) ([]domain.Tag, error) {
	query := s.db.WithContext(ctx).Model(&domain.Tag{}).Order("name ASC")
	if q != "" {
		query = query.Where("name ILIKE ? OR slug ILIKE ?", "%"+q+"%", "%"+q+"%")
	}
	var items []domain.Tag
	if err := query.Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

// UpdateTag 更新标签（白名单字段）。
func (s *Service) UpdateTag(ctx context.Context, id string, in *TagInput) (*domain.Tag, error) {
	var tag domain.Tag
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&tag).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if in.Name != "" {
		tag.Name = in.Name
	}
	if in.Slug != "" && in.Slug != tag.Slug {
		var count int64
		if err := s.db.WithContext(ctx).Model(&domain.Tag{}).
			Where("slug = ? AND id <> ?", in.Slug, id).Count(&count).Error; err != nil {
			return nil, err
		}
		if count > 0 {
			return nil, ErrSlugTaken
		}
		tag.Slug = in.Slug
	}
	if err := s.db.WithContext(ctx).Save(&tag).Error; err != nil {
		return nil, err
	}
	return &tag, nil
}

// DeleteTag 删除标签（标签为轻量元数据，直接删除）。
func (s *Service) DeleteTag(ctx context.Context, id string) error {
	res := s.db.WithContext(ctx).Where("id = ?", id).Delete(&domain.Tag{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- 反馈 ----

// FeedbackStatusUpdate 反馈状态更新。
type FeedbackStatusUpdate struct {
	Status string
}

// ListFeedback 反馈列表。
func (s *Service) ListFeedback(ctx context.Context, status, feedbackType string, page, pageSize int) ([]domain.Feedback, int64, error) {
	q := s.db.WithContext(ctx).Model(&domain.Feedback{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if feedbackType != "" {
		q = q.Where("feedback_type = ?", feedbackType)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	page, pageSize = normalizePage(page, pageSize)
	var items []domain.Feedback
	err := q.Order("created_at DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// UpdateFeedbackStatus 更新反馈状态（保留：仅改状态、不写审核历史）。
// 管理端新流程请使用 ReviewFeedback（会记录审核人与变更历史）。
func (s *Service) UpdateFeedbackStatus(ctx context.Context, id, status string) (*domain.Feedback, error) {
	var fb domain.Feedback
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&fb).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if !domain.IsValidFeedbackStatus(status) {
		return nil, ErrBadEnum
	}
	fb.Status = status
	if err := s.db.WithContext(ctx).Save(&fb).Error; err != nil {
		return nil, err
	}
	return &fb, nil
}

// ReviewFeedback 审核投稿（TODO §17）：更新状态与最近审核信息，并**追加**一条审核历史。
// 状态未变化时也会记录一次（便于留痕「再次确认」）。
func (s *Service) ReviewFeedback(ctx context.Context, id, status, reviewer, note string) (*domain.Feedback, error) {
	if !domain.IsValidFeedbackStatus(status) {
		return nil, ErrBadEnum
	}
	var fb domain.Feedback
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&fb).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	from := fb.Status
	now := time.Now()
	fb.Status = status
	fb.Reviewer = strings.TrimSpace(reviewer)
	fb.ReviewedAt = &now
	fb.ReviewNote = strings.TrimSpace(note)

	// 状态更新与历史写入必须同事务，避免出现「有状态无历史」。
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&fb).Error; err != nil {
			return err
		}
		return tx.Create(&domain.FeedbackReview{
			FeedbackID: fb.ID,
			FromStatus: from,
			ToStatus:   status,
			Reviewer:   fb.Reviewer,
			Note:       fb.ReviewNote,
		}).Error
	})
	if err != nil {
		return nil, err
	}
	return &fb, nil
}

// ListFeedbackReviews 返回某条投稿的审核历史（新→旧）。
func (s *Service) ListFeedbackReviews(ctx context.Context, feedbackID string) ([]domain.FeedbackReview, error) {
	if strings.TrimSpace(feedbackID) == "" {
		return nil, ErrNotFound
	}
	var items []domain.FeedbackReview
	err := s.db.WithContext(ctx).
		Where("feedback_id = ?", feedbackID).
		Order("created_at DESC").Find(&items).Error
	if err != nil {
		return nil, err
	}
	return items, nil
}

// SubmitFeedback 公开提交反馈 / 投稿（需求 §5.10、§17）。
//
// 投稿默认进入待审核状态；须先接受授权条款（LicenseVersion 为当前版本），
// 并做时间窗内重复提交检查（反垃圾）。
func (s *Service) SubmitFeedback(ctx context.Context, fb *domain.Feedback) (*domain.Feedback, error) {
	if err := validateFeedback(fb); err != nil {
		return nil, err
	}
	now := time.Now()
	fb.Status = domain.FeedbackStatusNew
	fb.LicenseAcceptedAt = &now
	fb.DedupHash = feedbackDedupHash(fb)

	dup, err := s.feedbackDuplicate(ctx, fb.DedupHash, now)
	if err != nil {
		return nil, err
	}
	if dup {
		return nil, ErrDuplicateSubmission
	}

	if err := s.db.WithContext(ctx).Create(fb).Error; err != nil {
		return nil, err
	}
	return fb, nil
}

// feedbackDedupWindow 重复提交判定时间窗。
const feedbackDedupWindow = 24 * time.Hour

// feedbackDuplicate 判断时间窗内是否已存在相同摘要的投稿。
func (s *Service) feedbackDuplicate(ctx context.Context, hash string, now time.Time) (bool, error) {
	if hash == "" {
		return false, nil
	}
	var count int64
	err := s.db.WithContext(ctx).Model(&domain.Feedback{}).
		Where("dedup_hash = ? AND created_at > ?", hash, now.Add(-feedbackDedupWindow)).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// feedbackDedupHash 生成反垃圾摘要：类型 + 目标 + 标题 + 正文（归一化空白与大小写）。
func feedbackDedupHash(fb *domain.Feedback) string {
	parts := []string{
		strings.ToLower(strings.TrimSpace(fb.FeedbackType)),
		strings.ToLower(strings.TrimSpace(fb.TargetKind)),
		strings.ToLower(strings.TrimSpace(fb.TargetRef)),
		strings.Join(strings.Fields(strings.ToLower(fb.Title)), " "),
		strings.Join(strings.Fields(strings.ToLower(fb.Content)), " "),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return hex.EncodeToString(sum[:])
}

// validateFeedback 反馈 / 投稿字段校验（需求 §5.10、§17）。
func validateFeedback(fb *domain.Feedback) error {
	if strings.TrimSpace(fb.Title) == "" || strings.TrimSpace(fb.Content) == "" {
		return errors.New("标题与内容必填")
	}
	if !domain.IsValidFeedbackType(fb.FeedbackType) {
		return ErrBadEnum
	}
	if fb.TargetKind == "" {
		fb.TargetKind = domain.FeedbackTargetOther
	}
	if !domain.IsValidFeedbackTargetKind(fb.TargetKind) {
		return ErrBadEnum
	}
	if len([]rune(fb.Title)) > 200 || len([]rune(fb.Content)) > 5000 {
		return errors.New("标题或内容超长")
	}
	if len([]rune(fb.TargetRef)) > 200 || len([]rune(fb.Contact)) > 200 {
		return errors.New("关联标识或联系方式超长")
	}
	// 投稿 / 纠错须明确接受授权条款（TODO §17：明确授权条款与版权处理流程）。
	if fb.FeedbackType == domain.FeedbackTypeDataCorrection ||
		fb.FeedbackType == domain.FeedbackTypeResourceSupplement {
		if strings.TrimSpace(fb.LicenseVersion) != domain.FeedbackLicenseVersion {
			return errors.New("请先阅读并接受投稿授权条款")
		}
	}
	// 恶意链接防护：只接受 http/https 链接，拒绝脚本伪协议与内网地址。
	if err := validateExternalLink(fb.RelatedURL); err != nil {
		return err
	}
	return nil
}

// validateExternalLink 校验外部链接：仅允许 http/https，拒绝脚本伪协议与内网 / 环回地址。
// 空链接视为合法（并非所有投稿都需要附链接）。
func validateExternalLink(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return errors.New("链接格式不合法")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return errors.New("链接仅支持 http / https")
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return errors.New("链接缺少主机名")
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return errors.New("链接不得指向本机地址")
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			return errors.New("链接不得指向内网地址")
		}
	}
	return nil
}

// ---- 通用辅助 ----

// checkSlugFree 校验指定模型的 slug 唯一。
func (s *Service) checkSlugFree(model any, exceptID, slug string) error {
	tx := s.db.WithContext(context.Background())
	if exceptID != "" {
		tx = tx.Where("slug = ? AND id <> ?", slug, exceptID)
	} else {
		tx = tx.Where("slug = ?", slug)
	}
	var count int64
	if err := tx.Model(model).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrSlugTaken
	}
	return nil
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func parseUUID(s string) *uuid.UUID {
	if s == "" {
		return nil
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return nil
	}
	return &id
}

func normalizePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 50 {
		pageSize = 50
	}
	return page, pageSize
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}
