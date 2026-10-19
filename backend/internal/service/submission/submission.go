// Package submission 提供「用户资源投稿」的业务逻辑（结构化投稿 → 人工审核 → 转为资源）。
//
// 与既有投稿能力的边界：
//   - feedback（internal/service/... / handler.SubmitFeedback）是自由文本纠错 / 举报，
//     不携带资源结构化字段，也不会生成 resources 记录；
//   - 本包面向「用户提交一个新资源」，字段与 resources 对齐，审核通过时调用
//     resource.Service.Create 生成正式资源（默认 draft，可显式发布），
//     从而完整复用资源的校验、标签关联与历史版本链路。
//
// 反垃圾：同一账号对同一来源链接同时只允许一个待审核投稿（部分唯一索引兜底），
// 且待审核投稿总量设上限，避免刷单淹没审核队列。
package submission

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"mckit/backend/internal/domain"
	"mckit/backend/internal/repository/types"
	"mckit/backend/internal/service/resource"
)

// 错误。
var (
	// ErrNotFound 投稿不存在。
	ErrNotFound = errors.New("投稿不存在")
	// ErrDuplicate 同一来源链接已存在待审核投稿。
	ErrDuplicate = errors.New("该来源链接已有待审核的投稿，请等待审核完成")
	// ErrTooManyPending 待审核投稿数量超限。
	ErrTooManyPending = errors.New("待审核投稿过多，请等待现有投稿审核完成")
	// ErrAlreadyReviewed 投稿已被审核，不能重复审核。
	ErrAlreadyReviewed = errors.New("投稿已被审核")
	// ErrBadStatus 目标审核状态非法（仅支持通过 / 驳回）。
	ErrBadStatus = errors.New("审核状态非法")
)

// 上限（与迁移中的列宽 / 反垃圾策略一致）。
const (
	// MaxPendingPerAccount 单账号待审核投稿上限。
	MaxPendingPerAccount = 10
	// maxNameLength 资源名称长度上限（对应 name VARCHAR(200)）。
	maxNameLength = 200
	// maxSummaryLength 简介长度上限（对应 summary VARCHAR(500)）。
	maxSummaryLength = 500
	// maxBodyLength 长文本字段上限（描述 / 安装说明），防止超大请求体入库。
	maxBodyLength = 20000
	// maxNoteLength 提交说明长度上限。
	maxNoteLength = 2000
	// maxAuthorNameLength 作者名长度上限。
	maxAuthorNameLength = 160
	// maxPlatformLength 来源平台长度上限。
	maxPlatformLength = 100
	// maxLicenseLength 许可协议长度上限。
	maxLicenseLength = 160
	// maxSourceURLLength 来源链接长度上限。既防超长 URL 入库，也避免超出
	// 部分唯一索引 (account_id, source_url) 的 btree 单条键长上限（约 2704 字节）。
	maxSourceURLLength = 2000
	// maxSlugLength 自动生成 slug 的长度上限。
	maxSlugLength = 80
	// maxSlugAttempts slug 冲突时的后缀重试次数上限。
	maxSlugAttempts = 50
)

// FieldError 带字段名的校验错误，供处理器回填到表单字段。
type FieldError struct {
	Field   string
	Message string
}

func (e *FieldError) Error() string { return e.Message }

// CreateInput 用户提交资源投稿的入参。
type CreateInput struct {
	AccountID     uuid.UUID
	SubmitterName string

	Name           string
	Summary        string
	Description    string
	ResourceType   string
	AuthorName     string
	SourcePlatform string
	SourceURL      string
	License        string
	GameEdition    string
	// SupportedVersions / LoaderTypes 由 resource 包的归一化函数校验。
	SupportedVersions []string
	LoaderTypes       []string
	InstallGuide      string
	Note              string
	// LicenseVersion 投稿时接受的授权条款版本，须等于 domain.SubmissionLicenseVersion。
	LicenseVersion string
}

// Filter 后台审核列表筛选条件。
type Filter struct {
	Status       string
	ResourceType string
	Q            string
	Page         int
	PageSize     int
}

// ReviewInput 审核入参。
type ReviewInput struct {
	ID         uuid.UUID
	Status     string
	Reviewer   string
	ReviewerID uuid.UUID
	Note       string
	// Publish 审核通过时是否直接发布资源（默认生成草稿，需人工核验后发布）。
	Publish bool
}

// Service 资源投稿服务。
type Service struct {
	db        *gorm.DB
	resources *resource.Service
}

// NewService 创建投稿服务。resources 用于审核通过时生成正式资源。
func NewService(db *gorm.DB, resources *resource.Service) *Service {
	return &Service{db: db, resources: resources}
}

// Create 提交一条资源投稿（落库为 pending）。
func (s *Service) Create(ctx context.Context, in *CreateInput) (*domain.ResourceSubmission, error) {
	if in.AccountID == uuid.Nil {
		return nil, &FieldError{Field: "account_id", Message: "缺少提交者账号"}
	}

	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, &FieldError{Field: "name", Message: "资源名称必填"}
	}
	if len([]rune(name)) > maxNameLength {
		return nil, &FieldError{Field: "name", Message: fmt.Sprintf("资源名称不能超过 %d 个字符", maxNameLength)}
	}

	summary := strings.TrimSpace(in.Summary)
	if summary == "" {
		return nil, &FieldError{Field: "summary", Message: "一句话简介必填"}
	}
	if len([]rune(summary)) > maxSummaryLength {
		return nil, &FieldError{Field: "summary", Message: fmt.Sprintf("简介不能超过 %d 个字符", maxSummaryLength)}
	}

	resourceType := strings.TrimSpace(in.ResourceType)
	if !resource.ValidResourceType(resourceType) {
		return nil, &FieldError{Field: "resource_type", Message: "资源类型非法"}
	}

	sourceURL := strings.TrimSpace(in.SourceURL)
	if sourceURL == "" {
		return nil, &FieldError{Field: "source_url", Message: "资源原始来源链接必填"}
	}
	if !resource.ValidHTTPURL(sourceURL) {
		return nil, &FieldError{Field: "source_url", Message: "来源链接必须是 http/https 链接"}
	}
	if len(sourceURL) > maxSourceURLLength {
		return nil, &FieldError{Field: "source_url", Message: fmt.Sprintf("来源链接不能超过 %d 个字符", maxSourceURLLength)}
	}

	edition, err := resource.NormalizeGameEdition(in.GameEdition)
	if err != nil {
		return nil, &FieldError{Field: "game_edition", Message: err.Error()}
	}
	versions, err := resource.NormalizeGameVersions(in.SupportedVersions)
	if err != nil {
		return nil, &FieldError{Field: "supported_versions", Message: err.Error()}
	}
	loaders, err := resource.NormalizeLoaderTypes(in.LoaderTypes)
	if err != nil {
		return nil, &FieldError{Field: "loader_types", Message: err.Error()}
	}

	authorName := strings.TrimSpace(in.AuthorName)
	if len([]rune(authorName)) > maxAuthorNameLength {
		return nil, &FieldError{Field: "author_name", Message: fmt.Sprintf("作者名不能超过 %d 个字符", maxAuthorNameLength)}
	}
	platform := strings.TrimSpace(in.SourcePlatform)
	if len([]rune(platform)) > maxPlatformLength {
		return nil, &FieldError{Field: "source_platform", Message: fmt.Sprintf("来源平台不能超过 %d 个字符", maxPlatformLength)}
	}
	license := strings.TrimSpace(in.License)
	if len([]rune(license)) > maxLicenseLength {
		return nil, &FieldError{Field: "license", Message: fmt.Sprintf("许可协议不能超过 %d 个字符", maxLicenseLength)}
	}
	description := strings.TrimSpace(in.Description)
	if len([]rune(description)) > maxBodyLength {
		return nil, &FieldError{Field: "description", Message: "资源描述过长"}
	}
	installGuide := strings.TrimSpace(in.InstallGuide)
	if len([]rune(installGuide)) > maxBodyLength {
		return nil, &FieldError{Field: "install_guide", Message: "安装说明过长"}
	}
	note := strings.TrimSpace(in.Note)
	if len([]rune(note)) > maxNoteLength {
		return nil, &FieldError{Field: "note", Message: fmt.Sprintf("补充说明不能超过 %d 个字符", maxNoteLength)}
	}

	// 投稿属于「向本站贡献内容」，须先接受当前版本的授权条款（TODO §17）。
	if strings.TrimSpace(in.LicenseVersion) != domain.SubmissionLicenseVersion {
		return nil, &FieldError{Field: "license_version", Message: "请先阅读并接受投稿授权条款"}
	}

	// 待审核总量上限（软限制，最终仍由审核队列人工消化）。
	var pending int64
	if err := s.db.WithContext(ctx).Model(&domain.ResourceSubmission{}).
		Where("account_id = ? AND status = ?", in.AccountID, domain.SubmissionStatusPending).
		Count(&pending).Error; err != nil {
		return nil, err
	}
	if pending >= MaxPendingPerAccount {
		return nil, ErrTooManyPending
	}

	now := time.Now()
	sub := &domain.ResourceSubmission{
		AccountID:         in.AccountID,
		SubmitterName:     strings.TrimSpace(in.SubmitterName),
		Name:              name,
		Summary:           summary,
		Description:       description,
		ResourceType:      resourceType,
		AuthorName:        authorName,
		SourcePlatform:    platform,
		SourceURL:         sourceURL,
		License:           license,
		GameEdition:       edition,
		SupportedVersions: types.JSONSlice(versions),
		LoaderTypes:       types.JSONSlice(loaders),
		InstallGuide:      installGuide,
		Note:              note,
		LicenseVersion:    domain.SubmissionLicenseVersion,
		LicenseAcceptedAt: &now, Status: domain.SubmissionStatusPending,
	}
	if err := s.db.WithContext(ctx).Create(sub).Error; err != nil {
		if isDuplicatePending(err) {
			return nil, ErrDuplicate
		}
		return nil, err
	}
	return sub, nil
}

// ListMine 返回当前账号的投稿（新→旧）。
func (s *Service) ListMine(ctx context.Context, accountID uuid.UUID, page, pageSize int) ([]domain.ResourceSubmission, int64, error) {
	page, pageSize = normalizePage(page, pageSize)
	q := s.db.WithContext(ctx).Model(&domain.ResourceSubmission{}).Where("account_id = ?", accountID)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []domain.ResourceSubmission
	if err := q.Order("created_at DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// List 后台审核列表（按状态 / 类型 / 关键词筛选，新→旧）。
func (s *Service) List(ctx context.Context, f Filter) ([]domain.ResourceSubmission, int64, error) {
	page, pageSize := normalizePage(f.Page, f.PageSize)
	q := s.db.WithContext(ctx).Model(&domain.ResourceSubmission{})
	if f.Status != "" {
		if !domain.IsValidSubmissionStatus(f.Status) {
			return nil, 0, ErrBadStatus
		}
		q = q.Where("status = ?", f.Status)
	}
	if f.ResourceType != "" {
		q = q.Where("resource_type = ?", f.ResourceType)
	}
	if kw := strings.TrimSpace(f.Q); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("name ILIKE ? OR summary ILIKE ? OR submitter_name ILIKE ?", like, like, like)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []domain.ResourceSubmission
	if err := q.Order("created_at DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// GetByID 按 ID 查询投稿。
func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (*domain.ResourceSubmission, error) {
	var sub domain.ResourceSubmission
	if err := s.db.WithContext(ctx).First(&sub, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &sub, nil
}

// Review 审核投稿。
//
// 通过：先生成正式资源（默认 draft），再把投稿置为 approved 并回填 resource_id；
// 若并发下已被他人审核（UPDATE 影响 0 行），则回滚刚创建的资源并返回 ErrAlreadyReviewed。
// 驳回：仅更新状态与留痕，不产生资源。
//
// 返回审核后的投稿与（通过时）生成的资源。
func (s *Service) Review(ctx context.Context, in ReviewInput) (*domain.ResourceSubmission, *domain.Resource, error) {
	if in.Status != domain.SubmissionStatusApproved && in.Status != domain.SubmissionStatusRejected {
		return nil, nil, ErrBadStatus
	}
	sub, err := s.GetByID(ctx, in.ID)
	if err != nil {
		return nil, nil, err
	}
	if sub.Status != domain.SubmissionStatusPending {
		return nil, nil, ErrAlreadyReviewed
	}

	now := time.Now()
	if in.Status == domain.SubmissionStatusRejected {
		updated, err := s.finishReview(ctx, in.ID, domain.SubmissionStatusRejected, in.Reviewer, in.Note, nil, now)
		if err != nil {
			return nil, nil, err
		}
		if !updated {
			return nil, nil, ErrAlreadyReviewed
		}
		sub.Status = domain.SubmissionStatusRejected
		sub.Reviewer = in.Reviewer
		sub.ReviewNote = in.Note
		sub.ReviewedAt = &now
		return sub, nil, nil
	}

	// 通过：生成资源。
	res, err := s.createResource(ctx, sub, in)
	if err != nil {
		return nil, nil, err
	}
	updated, err := s.finishReview(ctx, in.ID, domain.SubmissionStatusApproved, in.Reviewer, in.Note, &res.ID, now)
	if err != nil {
		// 状态写入失败：回收刚创建的资源，避免留下无主草稿。
		_ = s.resources.RemoveByID(ctx, res.ID.String())
		return nil, nil, err
	}
	if !updated {
		_ = s.resources.RemoveByID(ctx, res.ID.String())
		return nil, nil, ErrAlreadyReviewed
	}
	sub.Status = domain.SubmissionStatusApproved
	sub.Reviewer = in.Reviewer
	sub.ReviewNote = in.Note
	sub.ReviewedAt = &now
	sub.ResourceID = &res.ID
	return sub, res, nil
}

// ErrNoIDs 批量操作未提供任何 ID。
var ErrNoIDs = errors.New("请至少选择一条记录")

// ErrTooManyIDs 批量操作超过单次上限。
var ErrTooManyIDs = errors.New("单次批量操作条数超出上限")

// BatchReviewInput 批量审核入参。
type BatchReviewInput struct {
	IDs        []uuid.UUID
	Status     string
	Reviewer   string
	ReviewerID uuid.UUID
	Note       string
	Publish    bool
	// OnApproved 审核通过并生成资源后的回调（用于成长事件等跨模块副作用）。
	// 回调必须幂等；其内部失败不应回滚已完成的审核。
	OnApproved func(sub *domain.ResourceSubmission, res *domain.Resource)
}

// BatchReview 批量审核投稿：逐条独立执行，单条失败不影响其余记录。
//
// 返回逐条结果而不是「全成功 / 全失败」：审核时最典型的部分失败是
// 「已被他人审核」（并发），前端需要准确告诉运营哪几条没生效。
func (s *Service) BatchReview(ctx context.Context, in BatchReviewInput) (*domain.BatchReviewResult, error) {
	if in.Status != domain.SubmissionStatusApproved && in.Status != domain.SubmissionStatusRejected {
		return nil, ErrBadStatus
	}
	ids := dedupeIDs(in.IDs)
	if len(ids) == 0 {
		return nil, ErrNoIDs
	}
	if len(ids) > domain.MaxBatchReview {
		return nil, ErrTooManyIDs
	}

	result := &domain.BatchReviewResult{Items: make([]domain.BatchItemResult, 0, len(ids))}
	for _, id := range ids {
		item := domain.BatchItemResult{ID: id.String()}
		sub, res, err := s.Review(ctx, ReviewInput{
			ID:         id,
			Status:     in.Status,
			Reviewer:   in.Reviewer,
			ReviewerID: in.ReviewerID,
			Note:       in.Note,
			Publish:    in.Publish,
		})
		switch {
		case err == nil:
			item.OK = true
			item.Label = sub.Name
			if in.OnApproved != nil && res != nil {
				in.OnApproved(sub, res)
			}
		case errors.Is(err, ErrAlreadyReviewed):
			item.Label = submissionLabel(ctx, s.db, id)
			item.Error = "已被审核，已跳过"
		case errors.Is(err, ErrNotFound):
			item.Error = "投稿不存在"
		default:
			item.Label = submissionLabel(ctx, s.db, id)
			item.Error = "审核失败，请稍后重试"
		}
		result.Add(item)
	}
	return result, nil
}

// submissionLabel 尽力取出投稿名称用于结果展示；查询失败不影响主流程。
func submissionLabel(ctx context.Context, db *gorm.DB, id uuid.UUID) string {
	var name string
	_ = db.WithContext(ctx).Model(&domain.ResourceSubmission{}).
		Select("name").Where("id = ?", id).Scan(&name).Error
	return name
}

// dedupeIDs 去重并保持原顺序，避免同一记录被批量操作执行两次。
func dedupeIDs(ids []uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(ids))
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if id == uuid.Nil {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// finishReview 以「仍为 pending」为条件更新审核结果，返回是否真正更新。
func (s *Service) finishReview(
	ctx context.Context,
	id uuid.UUID,
	status, reviewer, note string,
	resourceID *uuid.UUID,
	at time.Time,
) (bool, error) {
	updates := map[string]any{
		"status":      status,
		"reviewer":    reviewer,
		"review_note": note,
		"reviewed_at": at,
		"updated_at":  at,
	}
	if resourceID != nil {
		updates["resource_id"] = *resourceID
	}
	tx := s.db.WithContext(ctx).Model(&domain.ResourceSubmission{}).
		Where("id = ? AND status = ?", id, domain.SubmissionStatusPending).
		Updates(updates)
	if tx.Error != nil {
		return false, tx.Error
	}
	return tx.RowsAffected > 0, nil
}

// createResource 由投稿生成正式资源（slug 冲突时自动追加序号重试）。
func (s *Service) createResource(ctx context.Context, sub *domain.ResourceSubmission, in ReviewInput) (*domain.Resource, error) {
	status := domain.StatusDraft
	if in.Publish {
		status = domain.StatusPublished
	}
	author := sub.AuthorName
	if author == "" {
		author = sub.SubmitterName
	}
	base := slugify(sub.Name)

	// 审核人账号 ID 缺失时不写 created_by（避免零值 UUID 触发外键失败）。
	createdBy := ""
	if in.ReviewerID != uuid.Nil {
		createdBy = in.ReviewerID.String()
	}

	var lastErr error
	for i := 0; i < maxSlugAttempts; i++ {
		slug := base
		if i > 0 {
			slug = fmt.Sprintf("%s-%d", base, i+1)
		}
		res, err := s.resources.Create(ctx, &resource.CreateInput{
			Slug:              slug,
			Name:              sub.Name,
			Summary:           sub.Summary,
			Description:       sub.Description,
			ResourceType:      sub.ResourceType,
			AuthorName:        author,
			SourcePlatform:    sub.SourcePlatform,
			SourceURL:         sub.SourceURL,
			License:           sub.License,
			GameEdition:       sub.GameEdition,
			SupportedVersions: []string(sub.SupportedVersions),
			LoaderTypes:       []string(sub.LoaderTypes),
			InstallGuide:      sub.InstallGuide,
			Status:            status,
			CreatedBy:         createdBy,
			ActorName:         in.Reviewer,
		})
		if err == nil {
			return res, nil
		}
		lastErr = err
		if !errors.Is(err, resource.ErrSlugTaken) {
			return nil, err
		}
	}
	return nil, lastErr
}

// isDuplicatePending 判断是否为「同账号 + 同来源链接」待审核投稿的唯一索引冲突。
func isDuplicatePending(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "idx_resource_submissions_pending_unique")
}

// slugify 由资源名称生成 slug：保留 ASCII 字母数字与中日韩汉字，其余转为连字符。
// 与 wikicrawl 的标题 slug 规则保持一致（ASCII + CJK 可读，避免纯拼音/空串）。
func slugify(name string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		case unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r):
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if len([]rune(out)) > maxSlugLength {
		out = string([]rune(out)[:maxSlugLength])
		out = strings.Trim(out, "-")
	}
	if out == "" {
		out = "resource"
	}
	return out
}

// normalizePage 归一化分页参数（默认 1 / 20，上限 100）。
func normalizePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}
