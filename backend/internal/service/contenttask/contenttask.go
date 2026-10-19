// Package contenttask 提供内容建设任务（搜索缺口 → 运营任务）的业务逻辑。
//
// 闭环（gj.md §8.3）：无结果搜索词达到阈值后创建任务 → 指派负责人并跟踪状态 →
// 新增内容后「重新检测」搜索结果 → 有结果即标记完成。
// 任务只保存归一化关键词与聚合计数，不含 IP / 会话 / 用户标识。
package contenttask

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"mckit/backend/internal/domain"
	"mckit/backend/internal/search"
	"mckit/backend/internal/service/searchstats"
)

// 错误。
var (
	// ErrNotFound 任务不存在。
	ErrNotFound = errors.New("任务不存在")
	// ErrBadKeyword 关键词为空或无法归一化。
	ErrBadKeyword = errors.New("关键词不能为空")
	// ErrDuplicate 该关键词已存在未完结任务。
	ErrDuplicate = errors.New("该关键词已存在未完结任务")
	// ErrBadStatus 任务状态非法。
	ErrBadStatus = errors.New("任务状态非法")
	// ErrBadContentType 关联内容类型非法。
	ErrBadContentType = errors.New("关联内容类型非法")
	// ErrBadAssignee 负责人账号不存在或不是后台账号。
	ErrBadAssignee = errors.New("负责人必须是有效的后台账号")
	// ErrBadRelatedContent 关联内容 ID 非法（非 UUID）。
	ErrBadRelatedContent = errors.New("关联内容 ID 非法")
	// ErrSearcherUnavailable 检索服务未配置（无法重新检测）。
	ErrSearcherUnavailable = errors.New("检索服务未配置")
)

// Searcher 关键词检索能力（用于「重新检测」）。由 search.Service 实现。
type Searcher interface {
	Search(ctx context.Context, f search.Filter) (*search.Result, error)
}

// Service 内容建设任务服务。
type Service struct {
	db     *gorm.DB
	search Searcher
}

// NewService 创建任务服务；searcher 可为 nil（此时不支持「重新检测」）。
func NewService(db *gorm.DB, searcher Searcher) *Service {
	return &Service{db: db, search: searcher}
}

// CreateInput 创建任务入参。
type CreateInput struct {
	// Keyword 目标关键词（原始输入，内部按搜索词口径归一化）。
	Keyword string
	Note    string
	// AssigneeID 负责人账号 ID（可选）；展示名由服务端按账号解析，不信任客户端传入。
	AssigneeID string
	// CreatedBy 创建者账号 ID。
	CreatedBy string
}

// UpdateInput 更新任务入参（仅非 nil 字段生效）。
type UpdateInput struct {
	Status *string
	// AssigneeID 负责人账号 ID；空串表示清空。展示名由服务端按账号解析。
	AssigneeID         *string
	RelatedContentType *string
	RelatedContentID   *string // 空串表示清空关联内容
	Note               *string
}

// ListFilter 任务列表筛选。
type ListFilter struct {
	Status   string
	Q        string
	Page     int
	PageSize int
}

// Candidate 无结果词候选（可转为内容建设任务）。
type Candidate struct {
	Term         string    `json:"term"`
	Hits         int64     `json:"hits"`
	NoResultHits int64     `json:"no_result_hits"`
	LastSeenAt   time.Time `json:"last_seen_at"`
	// TaskID 该关键词已存在的未完结任务 ID（空串表示可直接创建）。
	TaskID string `json:"task_id"`
}

// RecheckResult 「重新检测」结果。
type RecheckResult struct {
	TaskID      string `json:"task_id"`
	Keyword     string `json:"keyword"`
	ResultCount int64  `json:"result_count"`
	// HasResults 当前是否已有搜索结果（true 时可将任务标记完成）。
	HasResults bool `json:"has_results"`
}

// Create 从关键词创建任务，并快照当前搜索统计。
func (s *Service) Create(ctx context.Context, in *CreateInput) (*domain.ContentTask, error) {
	keyword := searchstats.Normalize(in.Keyword)
	if keyword == "" {
		return nil, ErrBadKeyword
	}
	assigneeName, err := s.resolveAssignee(ctx, in.AssigneeID)
	if err != nil {
		return nil, err
	}

	var term domain.SearchTerm
	hasTerm := true
	err = s.db.WithContext(ctx).Where("term = ?", keyword).First(&term).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		hasTerm = false
	} else if err != nil {
		return nil, err
	}

	task := &domain.ContentTask{
		Keyword:      keyword,
		Status:       domain.ContentTaskStatusOpen,
		Note:         strings.TrimSpace(in.Note),
		AssigneeID:   parseUUID(in.AssigneeID),
		AssigneeName: assigneeName,
		CreatedBy:    parseUUID(in.CreatedBy),
	}
	if hasTerm {
		task.SearchTermID = &term.ID
		task.Hits = term.Hits
		task.NoResultHits = term.NoResultHits
		seen := term.LastSeenAt
		task.LastSeenAt = &seen
	}

	if err := s.db.WithContext(ctx).Create(task).Error; err != nil {
		if isUniqueViolation(err) {
			return nil, ErrDuplicate
		}
		return nil, err
	}
	return task, nil
}

// GetByID 按 ID 查询任务。
func (s *Service) GetByID(ctx context.Context, id string) (*domain.ContentTask, error) {
	var task domain.ContentTask
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&task).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &task, nil
}

// Update 更新任务（状态 / 负责人 / 关联内容 / 备注）。
func (s *Service) Update(ctx context.Context, id string, in *UpdateInput) (*domain.ContentTask, error) {
	if _, err := s.GetByID(ctx, id); err != nil {
		return nil, err
	}

	updates := map[string]any{}
	if in.Status != nil {
		if !domain.IsValidContentTaskStatus(*in.Status) {
			return nil, ErrBadStatus
		}
		updates["status"] = *in.Status
	}
	if in.AssigneeID != nil {
		name, err := s.resolveAssignee(ctx, *in.AssigneeID)
		if err != nil {
			return nil, err
		}
		updates["assignee_id"] = parseUUID(*in.AssigneeID)
		updates["assignee_name"] = name
	}
	if in.RelatedContentType != nil {
		if !domain.IsValidContentTaskContentType(*in.RelatedContentType) {
			return nil, ErrBadContentType
		}
		updates["related_content_type"] = *in.RelatedContentType
	}
	if in.RelatedContentID != nil {
		trimmed := strings.TrimSpace(*in.RelatedContentID)
		if trimmed != "" && parseUUID(trimmed) == nil {
			return nil, ErrBadRelatedContent
		}
		updates["related_content_id"] = parseUUID(trimmed)
	}
	if in.Note != nil {
		updates["note"] = strings.TrimSpace(*in.Note)
	}

	if len(updates) == 0 {
		return s.GetByID(ctx, id)
	}
	if err := s.db.WithContext(ctx).Model(&domain.ContentTask{}).
		Where("id = ?", id).Updates(updates).Error; err != nil {
		if isUniqueViolation(err) {
			// 仅当把已完成/已忽略的任务改回未完结时才可能撞上部分唯一索引。
			return nil, ErrDuplicate
		}
		return nil, err
	}
	return s.GetByID(ctx, id)
}

// Delete 删除任务（用于纠错；历史统计不受影响）。
func (s *Service) Delete(ctx context.Context, id string) error {
	res := s.db.WithContext(ctx).Where("id = ?", id).Delete(&domain.ContentTask{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// List 分页查询任务（更新时间倒序，最近变动的任务排在前面）。
func (s *Service) List(ctx context.Context, f ListFilter) ([]domain.ContentTask, int64, error) {
	page, pageSize := normalizePage(f.Page, f.PageSize)

	tx := s.db.WithContext(ctx).Model(&domain.ContentTask{})
	if st := strings.TrimSpace(f.Status); st != "" {
		if !domain.IsValidContentTaskStatus(st) {
			return nil, 0, ErrBadStatus
		}
		tx = tx.Where("status = ?", st)
	}
	if kw := strings.TrimSpace(f.Q); kw != "" {
		tx = tx.Where("keyword ILIKE ? ESCAPE '\\'", "%"+escapeLike(kw)+"%")
	}

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	out := make([]domain.ContentTask, 0, pageSize)
	err := tx.Order("updated_at DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&out).Error
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// Candidates 返回达到阈值、且尚无未完结任务的无结果搜索词（可直接转为任务）。
func (s *Service) Candidates(ctx context.Context, limit int) ([]Candidate, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var terms []domain.SearchTerm
	if err := s.db.WithContext(ctx).
		Where("no_result_hits >= ?", domain.ContentTaskMinNoResultHits).
		Order("no_result_hits DESC, hits DESC").
		Limit(limit).Find(&terms).Error; err != nil {
		return nil, err
	}
	if len(terms) == 0 {
		return []Candidate{}, nil
	}

	keywords := make([]string, 0, len(terms))
	for _, t := range terms {
		keywords = append(keywords, t.Term)
	}
	var tasks []domain.ContentTask
	if err := s.db.WithContext(ctx).
		Where("keyword IN ? AND status IN ?", keywords,
			[]string{domain.ContentTaskStatusOpen, domain.ContentTaskStatusInProgress}).
		Find(&tasks).Error; err != nil {
		return nil, err
	}
	activeByKeyword := make(map[string]string, len(tasks))
	for _, t := range tasks {
		activeByKeyword[t.Keyword] = t.ID.String()
	}

	out := make([]Candidate, 0, len(terms))
	for _, t := range terms {
		out = append(out, Candidate{
			Term:         t.Term,
			Hits:         t.Hits,
			NoResultHits: t.NoResultHits,
			LastSeenAt:   t.LastSeenAt,
			TaskID:       activeByKeyword[t.Term],
		})
	}
	return out, nil
}

// Recheck 重新检测任务关键词的搜索结果（§8.3：新增内容后可重新检测并标记完成）。
//
// 只做检测、不改变任务状态：是否完成由运营人员判断（避免误判自动关单）。
func (s *Service) Recheck(ctx context.Context, id string) (*RecheckResult, error) {
	task, err := s.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if s.search == nil {
		return nil, ErrSearcherUnavailable
	}
	res, err := s.search.Search(ctx, search.Filter{Q: task.Keyword, Page: 1, PageSize: 1})
	if err != nil {
		return nil, err
	}
	return &RecheckResult{
		TaskID:      task.ID.String(),
		Keyword:     task.Keyword,
		ResultCount: res.Total,
		HasResults:  res.Total > 0,
	}, nil
}

// resolveAssignee 校验负责人并返回其展示名（昵称优先，其次用户名）。
//
// 空串表示不指派，返回空名。展示名由服务端按账号解析，不采用客户端传入值，
// 避免伪造负责人显示名。
func (s *Service) resolveAssignee(ctx context.Context, id string) (string, error) {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return "", nil
	}
	parsed, err := uuid.Parse(trimmed)
	if err != nil {
		return "", ErrBadAssignee
	}
	var acc domain.Account
	err = s.db.WithContext(ctx).Select("nickname", "username").
		Where("id = ? AND role <> ?", parsed, domain.RoleUser).First(&acc).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", ErrBadAssignee
	}
	if err != nil {
		return "", err
	}
	if acc.Nickname != "" {
		return acc.Nickname, nil
	}
	return acc.Username, nil
}

// isUniqueViolation 判断是否为 PostgreSQL 唯一约束冲突（并发创建/恢复未完结任务）。
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func parseUUID(s string) *uuid.UUID {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return nil
	}
	return &id
}

// normalizePage 归一化分页参数，与处理器层（handler.normalizePage）保持同一口径，
// 避免响应 meta 与实际查询页大小不一致。
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

// escapeLike 转义 LIKE 通配符，避免关键词中的 % / _ 改变匹配语义。
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}
