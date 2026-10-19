// Package revision 提供内容历史版本（快照）的记录、查询与回滚（gj.md §15 / §16.2）。
//
// 记录：内容服务在自己的写入事务内调用 RecordTx，保证「内容 + 历史」原子一致。
// 查询：按 (content_type, content_id) 分页返回历史，新→旧。
// 回滚：委托给各内容服务实现的 Restorer，走「追加新版本」而非覆盖历史。
package revision

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"mckit/backend/internal/domain"
	"mckit/backend/internal/repository/types"
)

// 错误。
var (
	// ErrNotFound 历史版本不存在。
	ErrNotFound = errors.New("历史版本不存在")
	// ErrBadTarget 内容类型非法。
	ErrBadTarget = errors.New("内容类型非法")
	// ErrUnsupported 该内容类型暂不支持回滚。
	ErrUnsupported = errors.New("该内容类型暂不支持回滚")
)

// revisionLockNamespace 是版本号分配的会话级咨询锁「命名空间」。
// 第二个键由 (content_type, content_id) 派生，从而按内容对象串行化，
// 避免并发更新读到相同 MAX(revision_no) 而产生重复版本号。
const revisionLockNamespace int32 = 0x72657631 // "rev1"

// Input 记录一次版本所需的入参。
type Input struct {
	ContentType string
	ContentID   uuid.UUID
	// Action 版本动作；为空时按 update 处理。
	Action string
	// Snapshot 该次写入后的完整内容（任意可 JSON 序列化结构）。
	Snapshot any
	// ActorID / ActorName 操作者账号 ID 与用户名（可空）。
	ActorID   string
	ActorName string
	// Note 备注（回滚时用于说明回滚原因 / 目标版本）。
	Note string
}

// RecordTx 在给定事务内追加一条版本记录。
//
// 必须在内容写入的同一事务中调用，保证「内容 + 历史」原子一致。
// change_summary 通过对比上一版快照自动计算（顶层字段名）。
func RecordTx(tx *gorm.DB, in Input) (*domain.ContentRevision, error) {
	if !domain.IsValidRevisionContentType(in.ContentType) {
		return nil, ErrBadTarget
	}
	action := strings.TrimSpace(in.Action)
	if action == "" {
		action = domain.RevisionActionUpdate
	}
	snapshot, err := json.Marshal(in.Snapshot)
	if err != nil {
		return nil, fmt.Errorf("序列化历史快照失败: %w", err)
	}

	// 按内容对象串行化版本号分配（事务结束自动释放）。
	lockKey := in.ContentType + ":" + in.ContentID.String()
	if err := tx.Exec(
		"SELECT pg_advisory_xact_lock(?, hashtext(?))", revisionLockNamespace, lockKey,
	).Error; err != nil {
		return nil, err
	}

	var latest domain.ContentRevision
	hasLatest := true
	err = tx.Where("content_type = ? AND content_id = ?", in.ContentType, in.ContentID).
		Order("revision_no DESC").First(&latest).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		hasLatest = false
	} else if err != nil {
		return nil, err
	}

	// 无上一版（新建）时 change_summary 为空；否则对比上一版快照计算。
	changes := []string{}
	if hasLatest {
		changes = diffKeys(latest.Snapshot, types.JSONB(snapshot))
	}

	rec := &domain.ContentRevision{
		ContentType:   in.ContentType,
		ContentID:     in.ContentID,
		RevisionNo:    latest.RevisionNo + 1,
		Action:        action,
		Snapshot:      types.JSONB(snapshot),
		ChangeSummary: types.JSONSlice(changes),
		ActorID:       parseUUID(in.ActorID),
		ActorName:     strings.TrimSpace(in.ActorName),
		Note:          strings.TrimSpace(in.Note),
	}
	if err := tx.Create(rec).Error; err != nil {
		return nil, err
	}
	return rec, nil
}

// diffKeys 对比两份 JSON 快照，返回发生变化的顶层字段名（有序）。
func diffKeys(before, after types.JSONB) []string {
	var b, a map[string]json.RawMessage
	if len(before) > 0 {
		_ = json.Unmarshal(before, &b)
	}
	if len(after) > 0 {
		_ = json.Unmarshal(after, &a)
	}
	changed := make(map[string]struct{})
	for k, bv := range b {
		av, ok := a[k]
		if !ok || !jsonEqual(bv, av) {
			changed[k] = struct{}{}
		}
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			changed[k] = struct{}{}
		}
	}
	keys := make([]string, 0, len(changed))
	for k := range changed {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// jsonEqual 语义化比较两段 JSON（忽略键顺序与空白差异）。
func jsonEqual(a, b json.RawMessage) bool {
	var av, bv any
	if err := json.Unmarshal(a, &av); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &bv); err != nil {
		return false
	}
	return reflect.DeepEqual(av, bv)
}

// Restorer 由各内容服务实现：把某个历史快照还原到当前内容。
//
// 定义在 revision 包内、由 main 装配注册，避免 revision 与内容服务之间的循环依赖。
type Restorer interface {
	RestoreRevision(ctx context.Context, contentType, contentID string, snapshot json.RawMessage, actorID, actorName, note string) error
}

// Service 历史版本查询与回滚。
type Service struct {
	db        *gorm.DB
	restorers map[string]Restorer
}

// NewService 创建历史版本服务。
func NewService(db *gorm.DB) *Service {
	return &Service{db: db, restorers: map[string]Restorer{}}
}

// Register 注册某内容类型的还原器。
func (s *Service) Register(contentType string, r Restorer) {
	s.restorers[contentType] = r
}

// List 分页查询某内容的历史版本（新→旧）。
func (s *Service) List(ctx context.Context, contentType, contentID string, page, pageSize int) ([]domain.ContentRevision, int64, error) {
	if !domain.IsValidRevisionContentType(contentType) {
		return nil, 0, ErrBadTarget
	}
	q := s.db.WithContext(ctx).Model(&domain.ContentRevision{}).
		Where("content_type = ? AND content_id = ?", contentType, contentID)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	page, pageSize = normalizePage(page, pageSize)
	var items []domain.ContentRevision
	err := q.Order("revision_no DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// Get 查询指定版本。
func (s *Service) Get(ctx context.Context, contentType, contentID string, revisionNo int) (*domain.ContentRevision, error) {
	if !domain.IsValidRevisionContentType(contentType) {
		return nil, ErrBadTarget
	}
	var rec domain.ContentRevision
	err := s.db.WithContext(ctx).
		Where("content_type = ? AND content_id = ? AND revision_no = ?", contentType, contentID, revisionNo).
		First(&rec).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

// Rollback 把内容还原到指定历史版本，并返回回滚后追加的新版本。
//
// 还原通过对应内容服务完成（保留标签同步、审计与校验），因此历史只增不减：
// 回滚本身也会生成一条 action=rollback 的新记录（§16.2）。
func (s *Service) Rollback(ctx context.Context, contentType, contentID string, revisionNo int, actorID, actorName, note string) (*domain.ContentRevision, error) {
	target, err := s.Get(ctx, contentType, contentID, revisionNo)
	if err != nil {
		return nil, err
	}
	restorer, ok := s.restorers[contentType]
	if !ok {
		return nil, ErrUnsupported
	}
	restoreNote := strings.TrimSpace(note)
	if restoreNote == "" {
		restoreNote = fmt.Sprintf("回滚至版本 #%d", revisionNo)
	}
	if err := restorer.RestoreRevision(ctx, contentType, contentID, json.RawMessage(target.Snapshot), actorID, actorName, restoreNote); err != nil {
		return nil, err
	}

	var latest domain.ContentRevision
	if err := s.db.WithContext(ctx).
		Where("content_type = ? AND content_id = ?", contentType, contentID).
		Order("revision_no DESC").First(&latest).Error; err != nil {
		return nil, err
	}
	return &latest, nil
}

// FieldChange 单个字段的差异（Before 为空表示新增字段，After 为 null 表示删除字段）。
type FieldChange struct {
	Field  string          `json:"field"`
	Before json.RawMessage `json:"before"`
	After  json.RawMessage `json:"after"`
}

// Diff 某版本相对上一版的差异。
type Diff struct {
	Revision domain.ContentRevision  `json:"revision"`
	Previous *domain.ContentRevision `json:"previous,omitempty"`
	Changes  []FieldChange           `json:"changes"`
}

// Diff 计算指定版本相对上一版的字段级差异（首版无上一版，Changes 为空）。
func (s *Service) Diff(ctx context.Context, contentType, contentID string, revisionNo int) (*Diff, error) {
	current, err := s.Get(ctx, contentType, contentID, revisionNo)
	if err != nil {
		return nil, err
	}
	out := &Diff{Revision: *current, Changes: []FieldChange{}}
	if revisionNo > 1 {
		prev, err := s.Get(ctx, contentType, contentID, revisionNo-1)
		if err == nil {
			out.Previous = prev
			out.Changes = fieldChanges(prev.Snapshot, current.Snapshot)
		} else if !errors.Is(err, ErrNotFound) {
			return nil, err
		}
	}
	return out, nil
}

// fieldChanges 计算两份快照的字段级差异（按字段名有序）。
func fieldChanges(before, after types.JSONB) []FieldChange {
	var b, a map[string]json.RawMessage
	if len(before) > 0 {
		_ = json.Unmarshal(before, &b)
	}
	if len(after) > 0 {
		_ = json.Unmarshal(after, &a)
	}
	changed := make(map[string]FieldChange)
	for k, av := range a {
		bv, ok := b[k]
		if !ok || !jsonEqual(bv, av) {
			changed[k] = FieldChange{Field: k, Before: nonNullRaw(bv), After: av}
		}
	}
	for k, bv := range b {
		if _, ok := a[k]; !ok {
			changed[k] = FieldChange{Field: k, Before: bv, After: json.RawMessage("null")}
		}
	}
	keys := make([]string, 0, len(changed))
	for k := range changed {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]FieldChange, 0, len(keys))
	for _, k := range keys {
		out = append(out, changed[k])
	}
	return out
}

func nonNullRaw(r json.RawMessage) json.RawMessage {
	if len(r) == 0 {
		return json.RawMessage("null")
	}
	return r
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
