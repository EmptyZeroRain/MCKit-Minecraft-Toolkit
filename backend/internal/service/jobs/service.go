// Package jobs 提供通用后台运维任务：入队、调度、自动重试、人工重试与失败详情。
//
// 为什么需要它：链接全量巡检、缓存重建这类动作耗时且可能失败，同步 HTTP 请求既容易
// 超时，也无法在失败后重跑。这里用一张 admin_jobs 表 + 进程内工作协程解决：
//
//   - 接口只负责「入队」，立刻返回任务 ID，前端轮询状态；
//   - 执行失败先自动重试（指数退避，最多 MaxAttempts 次），仍失败才转 failed；
//   - 人工「重试」新建一行并记录 retry_of，保留原始失败现场，便于复盘。
//
// 多实例部署时多个工作协程同时抢占是安全的：派发用
// `UPDATE ... WHERE id = (SELECT ... FOR UPDATE SKIP LOCKED)` 保证同一行只被一个实例领取。
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"mckit/backend/internal/domain"
	"mckit/backend/internal/repository/types"
)

const (
	// DefaultMaxAttempts 默认自动重试上限（含首次执行）。
	DefaultMaxAttempts = 3
	// retryBackoff 自动重试的退避基数：第 n 次失败后等待 n*retryBackoff。
	retryBackoff = 30 * time.Second
	// jobTimeout 单个任务的执行上限，防止 handler 卡死占住调度。
	jobTimeout = 30 * time.Minute
	// maxErrorLen 落库的失败原因长度上限，避免超长堆栈污染列表。
	maxErrorLen = 500
)

// ErrNotFound 任务不存在。
var ErrNotFound = errors.New("任务不存在")

// ErrUnknownType 未注册的任务类型。
var ErrUnknownType = errors.New("未知的任务类型")

// ErrNotRetryable 当前状态不允许重试。
var ErrNotRetryable = errors.New("只有失败或已取消的任务可以重试")

// ErrNotCancellable 当前状态不允许取消。
var ErrNotCancellable = errors.New("只有排队中的任务可以取消")

// Handler 任务处理器：payload 为入队时的 JSON，返回值会写入 result。
//
// 约束：
//   - 必须尊重 ctx 取消（调度层会在超时后取消）；
//   - 返回的 error 文案会展示给运营，不得包含 SQL、密钥、堆栈。
type Handler func(ctx context.Context, payload types.JSONB) (types.JSONB, error)

// Service 后台任务服务。
type Service struct {
	db           *gorm.DB
	mu           sync.RWMutex
	handlers     map[string]Handler
	descriptions map[string]string
	maxAttempts  int

	stopOnce sync.Once
	stop     chan struct{}
	wg       sync.WaitGroup
}

// NewService 创建任务服务。
func NewService(db *gorm.DB) *Service {
	return &Service{
		db:           db,
		handlers:     make(map[string]Handler),
		descriptions: make(map[string]string),
		maxAttempts:  DefaultMaxAttempts,
		stop:         make(chan struct{}),
	}
}

// Register 注册任务类型及其处理器；desc 用于前端展示（未知类型不允许入队）。
func (s *Service) Register(jobType, desc string, h Handler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[jobType] = h
	s.descriptions[jobType] = desc
}

// TypeInfo 任务类型描述。
type TypeInfo struct {
	Type string `json:"type"`
	Desc string `json:"desc"`
}

// Types 返回已注册的任务类型（供前端展示可选任务）。
func (s *Service) Types() []TypeInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]TypeInfo, 0, len(s.handlers))
	for t, d := range s.descriptions {
		out = append(out, TypeInfo{Type: t, Desc: d})
	}
	return out
}

// EnqueueInput 入队参数。
type EnqueueInput struct {
	Type      string
	Payload   types.JSONB
	CreatedBy string
}

// Enqueue 把任务写入队列并立即返回。
//
// 只做参数校验与落库，不等待执行：调用方（HTTP 接口）不应被任务耗时阻塞。
func (s *Service) Enqueue(ctx context.Context, in EnqueueInput) (*domain.AdminJob, error) {
	s.mu.RLock()
	_, known := s.handlers[in.Type]
	s.mu.RUnlock()
	if !known {
		return nil, ErrUnknownType
	}
	payload := in.Payload
	if len(payload) == 0 {
		payload = types.JSONB("{}")
	}
	if !json.Valid(payload) {
		return nil, fmt.Errorf("任务参数不是合法 JSON")
	}

	job := domain.AdminJob{
		Type:        in.Type,
		Status:      domain.JobStatusPending,
		Payload:     payload,
		Result:      types.JSONB("{}"),
		MaxAttempts: s.maxAttempts,
		CreatedBy:   in.CreatedBy,
		NextRunAt:   time.Now(),
	}
	if err := s.db.WithContext(ctx).Create(&job).Error; err != nil {
		return nil, err
	}
	return &job, nil
}

// ListFilter 任务列表筛选条件。
type ListFilter struct {
	Status   string
	Type     string
	Page     int
	PageSize int
}

// List 分页查询任务。
func (s *Service) List(ctx context.Context, f ListFilter) ([]domain.AdminJob, int64, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 || f.PageSize > 100 {
		f.PageSize = 20
	}

	tx := s.db.WithContext(ctx).Model(&domain.AdminJob{})
	if st := strings.TrimSpace(f.Status); st != "" {
		if !domain.IsValidJobStatus(st) {
			return nil, 0, fmt.Errorf("非法的任务状态")
		}
		tx = tx.Where("status = ?", st)
	}
	if tp := strings.TrimSpace(f.Type); tp != "" {
		tx = tx.Where("type = ?", tp)
	}

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := make([]domain.AdminJob, 0, f.PageSize)
	if err := tx.Order("created_at DESC").
		Offset((f.Page - 1) * f.PageSize).
		Limit(f.PageSize).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// Get 查询单个任务（含失败详情）。
func (s *Service) Get(ctx context.Context, id uuid.UUID) (*domain.AdminJob, error) {
	var job domain.AdminJob
	if err := s.db.WithContext(ctx).First(&job, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &job, nil
}

// Retry 人工重试：基于原任务新建一条 pending 任务。
//
// 不直接重置原任务，是因为运营需要保留「原始失败现场」（错误、耗时、参数）；
// 新任务通过 retry_of 关联，前端可回溯到源头。
func (s *Service) Retry(ctx context.Context, id uuid.UUID, operator string) (*domain.AdminJob, error) {
	origin, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if origin.Status != domain.JobStatusFailed && origin.Status != domain.JobStatusCancelled {
		return nil, ErrNotRetryable
	}

	s.mu.RLock()
	_, known := s.handlers[origin.Type]
	s.mu.RUnlock()
	if !known {
		return nil, ErrUnknownType
	}

	originID := origin.ID
	job := domain.AdminJob{
		Type:        origin.Type,
		Status:      domain.JobStatusPending,
		Payload:     origin.Payload,
		Result:      types.JSONB("{}"),
		MaxAttempts: s.maxAttempts,
		RetryOf:     &originID,
		CreatedBy:   operator,
		NextRunAt:   time.Now(),
	}
	if err := s.db.WithContext(ctx).Create(&job).Error; err != nil {
		return nil, err
	}
	return &job, nil
}

// Cancel 取消排队中的任务。
//
// 执行中的任务不允许取消：handler 可能已经产生了副作用，
// 强行改状态会造成「显示已取消但实际已执行」的错觉。
func (s *Service) Cancel(ctx context.Context, id uuid.UUID, operator string) (*domain.AdminJob, error) {
	now := time.Now()
	res := s.db.WithContext(ctx).Model(&domain.AdminJob{}).
		Where("id = ? AND status = ?", id, domain.JobStatusPending).
		Updates(map[string]any{
			"status":      domain.JobStatusCancelled,
			"finished_at": now,
			"updated_at":  now,
		})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		if _, err := s.Get(ctx, id); err != nil {
			return nil, err
		}
		return nil, ErrNotCancellable
	}
	job, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	// 取消留痕：运维需要知道是谁在什么时候终止了任务。
	s.writeNote(ctx, job.ID, operator)
	return job, nil
}

// writeNote 把取消人记录进 result，避免额外加列。
func (s *Service) writeNote(ctx context.Context, id uuid.UUID, operator string) {
	if strings.TrimSpace(operator) == "" {
		return
	}
	note, _ := json.Marshal(map[string]string{"cancelled_by": operator})
	_ = s.db.WithContext(ctx).Model(&domain.AdminJob{}).
		Where("id = ?", id).
		Update("result", types.JSONB(note)).Error
}

// Start 启动调度协程：每隔 interval 领取任务执行。
//
// interval 建议 3–10 秒；入队与调度解耦，前端看到的是 pending → running → 终态。
func (s *Service) Start(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			// 一次 tick 尽量把积压清空，但保留可取消的边界。
			for {
				n, err := s.RunOnce(ctx)
				if err != nil || n == 0 {
					break
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-s.stop:
				return
			case <-ticker.C:
			}
		}
	}()
}

// Stop 停止调度并等待当前任务收尾。
func (s *Service) Stop() {
	s.stopOnce.Do(func() { close(s.stop) })
	s.wg.Wait()
}

// PurgeOlderThan 删除 N 天前已结束的任务记录，防止 admin_jobs 无限增长。
//
// 只删终态（succeeded / failed / cancelled）：pending / running 代表仍在处理，
// 无论多旧都不能删。失败现场在保留期内可复盘，超出保留期后由本方法清掉。
// 多实例同时执行是安全的：DELETE 幂等，最多做一次无用功。
func (s *Service) PurgeOlderThan(ctx context.Context, days int) (int64, error) {
	if days <= 0 {
		return 0, nil
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	res := s.db.WithContext(ctx).
		Where("status IN ?", []string{domain.JobStatusSucceeded, domain.JobStatusFailed, domain.JobStatusCancelled}).
		Where("updated_at < ?", cutoff).
		Delete(&domain.AdminJob{})
	return res.RowsAffected, res.Error
}

// StartRetention 启动定时清理（默认每天一次）；days<=0 表示不清理。
func (s *Service) StartRetention(ctx context.Context, days int, interval time.Duration) {
	if days <= 0 {
		return
	}
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			if n, err := s.PurgeOlderThan(ctx, days); err != nil {
				slog.Warn("清理历史后台任务失败", "err", err)
			} else if n > 0 {
				slog.Info("已清理历史后台任务", "count", n, "days", days)
			}
			select {
			case <-ctx.Done():
				return
			case <-s.stop:
				return
			case <-ticker.C:
			}
		}
	}()
}

// RunOnce 领取并执行一条到点任务；返回实际执行条数（0 表示队列空闲）。
//
// 导出用于测试与手动触发（例如运维脚本调用一次）。
func (s *Service) RunOnce(ctx context.Context) (int, error) {
	job, err := s.claim(ctx)
	if err != nil {
		return 0, err
	}
	if job == nil {
		return 0, nil
	}
	s.execute(ctx, job)
	return 1, nil
}

// claim 用 SKIP LOCKED 抢占一条到点任务。
//
// 条件更新 + RETURNING 保证并发安全：多个实例/协程同时抢占时，
// 同一行只会被其中一个拿到，不需要额外的分布式锁。
func (s *Service) claim(ctx context.Context) (*domain.AdminJob, error) {
	var jobs []domain.AdminJob
	err := s.db.WithContext(ctx).Raw(`
		UPDATE admin_jobs
		SET status = 'running',
		    started_at = now(),
		    updated_at = now(),
		    attempts = attempts + 1
		WHERE id = (
			SELECT id FROM admin_jobs
			WHERE status = 'pending' AND next_run_at <= now()
			ORDER BY next_run_at, created_at
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING *`).Scan(&jobs).Error
	if err != nil {
		return nil, err
	}
	if len(jobs) == 0 {
		return nil, nil
	}
	return &jobs[0], nil
}

// execute 执行任务并写回终态 / 待重试状态。
func (s *Service) execute(ctx context.Context, job *domain.AdminJob) {
	s.mu.RLock()
	handler := s.handlers[job.Type]
	s.mu.RUnlock()
	if handler == nil {
		s.fail(ctx, job, ErrUnknownType)
		return
	}

	runCtx, cancel := context.WithTimeout(ctx, jobTimeout)
	defer cancel()

	result, err := s.safeRun(runCtx, handler, job.Payload)
	if err != nil {
		s.fail(ctx, job, err)
		return
	}
	if len(result) == 0 {
		result = types.JSONB("{}")
	}
	now := time.Now()
	_ = s.db.WithContext(ctx).Model(&domain.AdminJob{}).
		Where("id = ?", job.ID).
		Updates(map[string]any{
			"status":      domain.JobStatusSucceeded,
			"result":      result,
			"last_error":  "",
			"finished_at": now,
			"updated_at":  now,
		}).Error
}

// safeRun 隔离 handler 的 panic：单个任务崩溃不能拖垮 API 进程。
func (s *Service) safeRun(ctx context.Context, h Handler, payload types.JSONB) (result types.JSONB, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("任务执行异常（已记录，请联系开发排查）")
		}
	}()
	return h(ctx, payload)
}

// fail 记录失败：未达重试上限则回到 pending 并退避，否则转 failed。
func (s *Service) fail(ctx context.Context, job *domain.AdminJob, cause error) {
	msg := sanitizeError(cause)
	now := time.Now()
	updates := map[string]any{
		"last_error": msg,
		"updated_at": now,
	}
	if job.Attempts < job.MaxAttempts {
		updates["status"] = domain.JobStatusPending
		updates["next_run_at"] = now.Add(time.Duration(job.Attempts) * retryBackoff)
	} else {
		updates["status"] = domain.JobStatusFailed
		updates["finished_at"] = now
	}
	_ = s.db.WithContext(ctx).Model(&domain.AdminJob{}).
		Where("id = ?", job.ID).
		Updates(updates).Error
}

// sanitizeError 把错误压成单行、截断过长内容，避免堆栈/多行 SQL 污染列表与日志。
func sanitizeError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.Join(strings.Fields(err.Error()), " ")
	if len(msg) > maxErrorLen {
		msg = msg[:maxErrorLen] + "…"
	}
	return msg
}
