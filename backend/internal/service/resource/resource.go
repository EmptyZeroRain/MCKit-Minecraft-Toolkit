// Package resource 提供资源中心的业务逻辑（需求 §8.4 管理 API 与公开 API）。
package resource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"mckit/backend/internal/domain"
	"mckit/backend/internal/repository/types"
	"mckit/backend/internal/service/revision"
	"mckit/backend/internal/service/tagging"
	"mckit/backend/internal/versioning"
)

// ErrSlugTaken slug 冲突。
var ErrSlugTaken = errors.New("slug 已被占用")

// ErrNotFound 资源不存在。
var ErrNotFound = errors.New("资源不存在")

// Service 资源服务。
type Service struct {
	db *gorm.DB
}

// NewService 创建资源服务。
func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// CreateInput 创建资源入参。
// JSONB 数组字段以 []string / map 形式接收，内部统一序列化。
type CreateInput struct {
	Slug              string
	Name              string
	Summary           string
	Description       string
	ResourceType      string
	CoverImageURL     string
	AuthorName        string
	SourcePlatform    string
	SourceURL         string
	License           string
	GameEdition       string
	SupportedVersions []string
	LoaderTypes       []string
	// Dependencies 依赖项（前端为对象数组，保持任意 JSON 结构）。
	Dependencies json.RawMessage
	InstallGuide string
	Tags         []string
	// CompatibilityNote 人工核验备注（可选，TODO §6）。
	CompatibilityNote string
	Status            string
	CreatedBy         string
	// ActorName 操作者用户名（写入历史版本，便于审计回溯）。
	ActorName string
}

// UpdateInput 更新资源入参（白名单字段，需求 §6.3）。
type UpdateInput struct {
	Slug              *string
	Name              *string
	Summary           *string
	Description       *string
	ResourceType      *string
	CoverImageURL     *string
	AuthorName        *string
	SourcePlatform    *string
	SourceURL         *string
	License           *string
	GameEdition       *string
	SupportedVersions *[]string
	LoaderTypes       *[]string
	Dependencies      json.RawMessage
	InstallGuide      *string
	Tags              *[]string
	CompatibilityNote *string
	Status            *string
	UpdatedBy         string
	// ActorName 操作者用户名（写入历史版本）。
	ActorName string
	// Action 历史版本动作，为空按 update 处理；回滚时传 rollback。
	Action string
	// Note 历史版本备注。
	Note string
}

// validateCreate 创建时校验必填项与枚举（需求 §6.3）。
func validateCreate(in *CreateInput) error {
	if strings.TrimSpace(in.Slug) == "" {
		return errors.New("slug 必填")
	}
	if strings.TrimSpace(in.Name) == "" {
		return errors.New("name 必填")
	}
	if strings.TrimSpace(in.Summary) == "" {
		return errors.New("summary 必填")
	}
	if strings.TrimSpace(in.SourceURL) == "" {
		return errors.New("source_url 必填（资源原始来源）")
	}
	if !validResourceType(in.ResourceType) {
		return fmt.Errorf("resource_type 非法: %s", in.ResourceType)
	}
	if !validStatus(in.Status, true) {
		return fmt.Errorf("status 非法: %s", in.Status)
	}
	if !validHTTPURL(in.SourceURL) {
		return errors.New("source_url 必须是 http/https 链接")
	}
	return nil
}

func validResourceType(t string) bool {
	return ValidResourceType(t)
}

func validStatus(s string, allowHidden bool) bool {
	switch s {
	case domain.StatusDraft, domain.StatusPublished:
		return true
	case domain.StatusHidden:
		return allowHidden
	case domain.StatusRemoved:
		return false // 由软删除/下架接口管理
	}
	return false
}

func validHTTPURL(u string) bool {
	return ValidHTTPURL(u)
}

// Create 创建资源。
func (s *Service) Create(ctx context.Context, in *CreateInput) (*domain.Resource, error) {
	if err := validateCreate(in); err != nil {
		return nil, err
	}
	if err := s.checkSlugFree(in.Slug); err != nil {
		return nil, err
	}

	// TODO §6：统一字段校验与规范化，避免把非法加载器 / 版本 / 依赖结构写入库。
	edition, err := NormalizeGameEdition(in.GameEdition)
	if err != nil {
		return nil, err
	}
	versions, err := NormalizeGameVersions(in.SupportedVersions)
	if err != nil {
		return nil, err
	}
	loaders, err := NormalizeLoaderTypes(in.LoaderTypes)
	if err != nil {
		return nil, err
	}
	deps, err := NormalizeDependencies(in.Dependencies)
	if err != nil {
		return nil, err
	}

	res := &domain.Resource{
		Slug:              in.Slug,
		Name:              in.Name,
		Summary:           in.Summary,
		Description:       in.Description,
		ResourceType:      in.ResourceType,
		CoverImageURL:     in.CoverImageURL,
		AuthorName:        in.AuthorName,
		SourcePlatform:    in.SourcePlatform,
		SourceURL:         in.SourceURL,
		License:           in.License,
		GameEdition:       edition,
		SupportedVersions: types.JSONSlice(versions),
		LoaderTypes:       types.JSONSlice(loaders),
		Dependencies:      types.JSONBArray(deps),
		InstallGuide:      in.InstallGuide,
		Tags:              types.JSONSlice(in.Tags),
		CompatibilityNote: in.CompatibilityNote,
		Status:            in.Status,
		CreatedBy:         parseUUID(in.CreatedBy),
	}
	if in.Status == domain.StatusPublished {
		now := time.Now()
		res.PublishedAt = &now
	}

	// 与标签关联表同事务写入，保证 JSONB tags 与关联行原子一致。
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(res).Error; err != nil {
			return err
		}
		if err := tagging.Sync(tx, tagging.ResourceTarget, res.ID.String(), in.Tags); err != nil {
			return err
		}
		// 历史版本与内容同事务写入（§15 / §16.2）。
		_, err := revision.RecordTx(tx, revision.Input{
			ContentType: domain.RevisionContentResource,
			ContentID:   res.ID,
			Action:      domain.RevisionActionCreate,
			Snapshot:    res,
			ActorID:     in.CreatedBy,
			ActorName:   in.ActorName,
		})
		return err
	}); err != nil {
		return nil, err
	}
	return res, nil
}

// checkSlugFree 校验 slug 未被使用（含软删除记录）。
func (s *Service) checkSlugFree(slug string) error {
	var count int64
	if err := s.db.Model(&domain.Resource{}).
		Unscoped().Where("slug = ?", slug).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrSlugTaken
	}
	return nil
}

// GetBySlug 按 slug 查询（公开接口只返回 published）。
func (s *Service) GetBySlug(ctx context.Context, slug string, publishedOnly bool) (*domain.Resource, error) {
	var res domain.Resource
	q := s.db.WithContext(ctx).Where("slug = ?", slug)
	if publishedOnly {
		q = q.Where("status = ?", domain.StatusPublished)
	}
	err := q.First(&res).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// GetByID 按 ID 查询（管理端）。
func (s *Service) GetByID(ctx context.Context, id string) (*domain.Resource, error) {
	var res domain.Resource
	err := s.db.WithContext(ctx).
		Where("id = ?", id).First(&res).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// ListFilter 列表筛选。
type ListFilter struct {
	Q              string
	ResourceType   string
	GameVersion    string
	Loader         string
	SourcePlatform string
	// Tag 按标签精确筛选（走 resource_tags 关联表，可走索引）。
	Tag string
	// Status 精确筛选资源状态（draft / published / offline 等），空表示不限。
	Status   string
	Sort     string
	Page     int
	PageSize int
	// PublishedOnly 仅公开页面使用，排除草稿/下架。
	PublishedOnly bool
}

// ExportMax 单次导出上限：资源量级较大，避免一次导出把进程内存打满。
const ExportMax = 5000

// filteredQuery 构造列表 / 导出共用的筛选查询。
//
// 抽出来是为了避免 List 与 Export 各写一套筛选条件后逐渐漂移
// （导出内容与页面看到的不一致是运营最难排查的问题之一）。
func (s *Service) filteredQuery(ctx context.Context, f ListFilter) *gorm.DB {
	q := s.db.WithContext(ctx).Model(&domain.Resource{})
	if f.PublishedOnly {
		q = q.Where("status = ?", domain.StatusPublished)
	}
	if f.Q != "" {
		kw := "%" + f.Q + "%"
		q = q.Where("name ILIKE ? OR summary ILIKE ? OR slug ILIKE ?", kw, kw, kw)
	}
	if f.ResourceType != "" {
		q = q.Where("resource_type = ?", f.ResourceType)
	}
	// 状态筛选：后台列表页一直在传 status，此前后端未接收（静默失效），一并修复。
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	// 版本/加载器筛选用 JSONB 包含查询（首期简化，需求量级小）。
	// 声明「所有版本」（all）的资源对任何具体版本筛选都应命中。
	if f.GameVersion != "" {
		if f.GameVersion == versioning.AllKeyword {
			q = q.Where("supported_versions::text LIKE ?", "%\""+versioning.AllKeyword+"\"%")
		} else {
			q = q.Where(
				"(supported_versions::text LIKE ? OR supported_versions::text LIKE ?)",
				"%"+f.GameVersion+"%", "%\""+versioning.AllKeyword+"\"%",
			)
		}
	}
	if f.Loader != "" {
		q = q.Where("loader_types::text LIKE ?", "%"+f.Loader+"%")
	}
	if f.SourcePlatform != "" {
		q = q.Where("source_platform ILIKE ?", "%"+f.SourcePlatform+"%")
	}
	if f.Tag != "" {
		clause, tag := tagging.ResourceTarget.ExistsClause(f.Tag)
		q = q.Where(clause, tag)
	}
	return q
}

// List 分页查询资源。
func (s *Service) List(ctx context.Context, f ListFilter) ([]domain.Resource, int64, error) {
	q := s.filteredQuery(ctx, f)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	page, pageSize := normalizePage(f.Page, f.PageSize)
	sortCol := "updated_at DESC"
	switch f.Sort {
	case "name_asc":
		sortCol = "name ASC"
	default:
		sortCol = "updated_at DESC"
	}

	var items []domain.Resource
	err := q.Order(sortCol).
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&items).Error
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// Export 按筛选条件导出资源（不分页，最多 ExportMax 条）。
// truncated 表示结果被上限截断。
func (s *Service) Export(ctx context.Context, f ListFilter) (items []domain.Resource, truncated bool, err error) {
	q := s.filteredQuery(ctx, f)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, false, err
	}

	sortCol := "updated_at DESC"
	if f.Sort == "name_asc" {
		sortCol = "name ASC"
	}
	var rows []domain.Resource
	if err := q.Order(sortCol).Limit(ExportMax).Find(&rows).Error; err != nil {
		return nil, false, err
	}
	return rows, total > int64(len(rows)), nil
}

// Update 更新资源（白名单字段，软删除记录不更新）。
func (s *Service) Update(ctx context.Context, id string, in *UpdateInput) (*domain.Resource, error) {
	existing, err := s.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	updates := map[string]any{}
	if in.Slug != nil {
		if err := s.checkSlugFreeExcept(*in.Slug, id); err != nil {
			return nil, err
		}
		updates["slug"] = *in.Slug
	}
	if in.Name != nil {
		updates["name"] = *in.Name
	}
	if in.Summary != nil {
		updates["summary"] = *in.Summary
	}
	if in.Description != nil {
		updates["description"] = *in.Description
	}
	if in.ResourceType != nil {
		if !validResourceType(*in.ResourceType) {
			return nil, fmt.Errorf("resource_type 非法")
		}
		updates["resource_type"] = *in.ResourceType
	}
	if in.CoverImageURL != nil {
		updates["cover_image_url"] = *in.CoverImageURL
	}
	if in.AuthorName != nil {
		updates["author_name"] = *in.AuthorName
	}
	if in.SourcePlatform != nil {
		updates["source_platform"] = *in.SourcePlatform
	}
	if in.SourceURL != nil {
		if !validHTTPURL(*in.SourceURL) {
			return nil, errors.New("source_url 必须是 http/https 链接")
		}
		updates["source_url"] = *in.SourceURL
	}
	if in.License != nil {
		updates["license"] = *in.License
	}
	if in.GameEdition != nil {
		edition, err := NormalizeGameEdition(*in.GameEdition)
		if err != nil {
			return nil, err
		}
		updates["game_edition"] = edition
	}
	if in.SupportedVersions != nil {
		versions, err := NormalizeGameVersions(*in.SupportedVersions)
		if err != nil {
			return nil, err
		}
		updates["supported_versions"] = types.JSONSlice(versions)
	}
	if in.LoaderTypes != nil {
		loaders, err := NormalizeLoaderTypes(*in.LoaderTypes)
		if err != nil {
			return nil, err
		}
		updates["loader_types"] = types.JSONSlice(loaders)
	}
	if in.Dependencies != nil {
		deps, err := NormalizeDependencies(in.Dependencies)
		if err != nil {
			return nil, err
		}
		updates["dependencies"] = types.JSONBArray(deps)
	}
	if in.InstallGuide != nil {
		updates["install_guide"] = *in.InstallGuide
	}
	if in.CompatibilityNote != nil {
		updates["compatibility_note"] = *in.CompatibilityNote
	}
	if in.Tags != nil {
		updates["tags"] = types.JSONSlice(*in.Tags)
	}
	if in.Status != nil {
		if !validStatus(*in.Status, true) {
			return nil, fmt.Errorf("status 非法: %s", *in.Status)
		}
		updates["status"] = *in.Status
		if *in.Status == domain.StatusPublished && existing.PublishedAt == nil {
			now := time.Now()
			updates["published_at"] = now
		}
		_ = existing.PublishedAt
	}
	if in.UpdatedBy != "" {
		updates["updated_by"] = parseUUID(in.UpdatedBy)
	}

	if len(updates) == 0 {
		return existing, nil
	}

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&domain.Resource{}).
			Where("id = ?", id).Updates(updates).Error; err != nil {
			return err
		}
		// 仅当本次显式提交了 tags 时重建关联行，避免误清空。
		if in.Tags != nil {
			if err := tagging.Sync(tx, tagging.ResourceTarget, id, *in.Tags); err != nil {
				return err
			}
		}
		// 读取写入后的完整行作为历史快照（§15）。
		var after domain.Resource
		if err := tx.Where("id = ?", id).First(&after).Error; err != nil {
			return err
		}
		action := in.Action
		if action == "" {
			action = domain.RevisionActionUpdate
		}
		_, err := revision.RecordTx(tx, revision.Input{
			ContentType: domain.RevisionContentResource,
			ContentID:   after.ID,
			Action:      action,
			Snapshot:    after,
			ActorID:     in.UpdatedBy,
			ActorName:   in.ActorName,
			Note:        in.Note,
		})
		return err
	}); err != nil {
		return nil, err
	}
	return s.GetByID(ctx, id)
}

// RestoreRevision 实现 revision.Restorer：把历史快照还原为当前资源（回滚）。
//
// 直接按快照写入内容字段并同步标签关联，保证历史数据不受当前校验规则变化影响。
// link_status / last_checked_at 属于运行态观测数据，不在内容快照内，因此不受回滚影响。
// 还原本身也会追加一条 action=rollback 的新版本，不覆盖历史（§16.2）。
func (s *Service) RestoreRevision(ctx context.Context, contentType, contentID string, snapshot json.RawMessage, actorID, actorName, note string) error {
	if contentType != domain.RevisionContentResource {
		return revision.ErrUnsupported
	}
	var snap domain.Resource
	if err := json.Unmarshal(snapshot, &snap); err != nil {
		return fmt.Errorf("解析历史快照失败: %w", err)
	}
	if snap.Slug != "" {
		if err := s.checkSlugFreeExcept(snap.Slug, contentID); err != nil {
			return err
		}
	}

	updates := map[string]any{
		"slug":               snap.Slug,
		"name":               snap.Name,
		"summary":            snap.Summary,
		"description":        snap.Description,
		"resource_type":      snap.ResourceType,
		"cover_image_url":    snap.CoverImageURL,
		"author_name":        snap.AuthorName,
		"source_platform":    snap.SourcePlatform,
		"source_url":         snap.SourceURL,
		"license":            snap.License,
		"game_edition":       snap.GameEdition,
		"supported_versions": snap.SupportedVersions,
		"loader_types":       snap.LoaderTypes,
		"dependencies":       snap.Dependencies,
		"install_guide":      snap.InstallGuide,
		"compatibility_note": snap.CompatibilityNote,
		"tags":               snap.Tags,
		"status":             snap.Status,
		"published_at":       snap.PublishedAt,
	}
	if actorID != "" {
		updates["updated_by"] = parseUUID(actorID)
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&domain.Resource{}).
			Where("id = ?", contentID).Updates(updates).Error; err != nil {
			return err
		}
		if err := tagging.Sync(tx, tagging.ResourceTarget, contentID, []string(snap.Tags)); err != nil {
			return err
		}
		var after domain.Resource
		if err := tx.Where("id = ?", contentID).First(&after).Error; err != nil {
			return err
		}
		_, err := revision.RecordTx(tx, revision.Input{
			ContentType: domain.RevisionContentResource,
			ContentID:   after.ID,
			Action:      domain.RevisionActionRollback,
			Snapshot:    after,
			ActorID:     actorID,
			ActorName:   actorName,
			Note:        note,
		})
		return err
	})
}

// checkSlugFreeExcept 校验 slug 不被其他资源占用。
func (s *Service) checkSlugFreeExcept(slug, exceptID string) error {
	var count int64
	if err := s.db.Model(&domain.Resource{}).
		Unscoped().
		Where("slug = ? AND id <> ?", slug, exceptID).
		Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrSlugTaken
	}
	return nil
}

// RemoveByID 软删除（需求 §6.3：删除优先软删除或下架）。
func (s *Service) RemoveByID(ctx context.Context, id string) error {
	if _, err := s.GetByID(ctx, id); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Model(&domain.Resource{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":     domain.StatusRemoved,
			"deleted_at": time.Now(),
		}).Error
}

// UpdateLinkStatus 更新链接检查状态。
func (s *Service) UpdateLinkStatus(ctx context.Context, id, linkStatus string, checkedAt time.Time) error {
	return s.db.WithContext(ctx).Model(&domain.Resource{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"link_status":     linkStatus,
			"last_checked_at": checkedAt,
		}).Error
}

func parseUUID(s string) *uuid.UUID {
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
