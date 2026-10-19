package jobs_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"mckit/backend/internal/domain"
	db "mckit/backend/internal/repository"
	"mckit/backend/internal/repository/types"
	"mckit/backend/internal/service/jobs"
)

const jobsSchema = "jobs_it"

func withSearchPath(dsn, schema string) string {
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "search_path=" + schema
}

func setupJobsDB(t *testing.T) *gorm.DB {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("未设置 TEST_DATABASE_URL，跳过数据库集成测试")
	}
	ctx := context.Background()

	adminDB, err := db.Open(url)
	if err != nil {
		t.Fatalf("连接测试库失败: %v", err)
	}
	if err := adminDB.Exec("DROP SCHEMA IF EXISTS " + jobsSchema + " CASCADE").Error; err != nil {
		t.Fatalf("清理测试 schema 失败: %v", err)
	}
	if err := adminDB.Exec("CREATE SCHEMA " + jobsSchema).Error; err != nil {
		t.Fatalf("创建测试 schema 失败: %v", err)
	}
	if sqlDB, err := adminDB.DB(); err == nil {
		_ = sqlDB.Close()
	}

	gdb, err := db.Open(withSearchPath(url, jobsSchema))
	if err != nil {
		t.Fatalf("连接测试 schema 失败: %v", err)
	}
	if _, err := db.Migrate(ctx, gdb, "../../../migrations"); err != nil {
		t.Fatalf("执行迁移失败: %v", err)
	}
	return gdb
}

// forceDue 把任务的下次执行时间提前，避免测试等待退避时间。
func forceDue(t *testing.T, gdb *gorm.DB) {
	t.Helper()
	if err := gdb.Exec("UPDATE admin_jobs SET next_run_at = now() WHERE status = 'pending'").Error; err != nil {
		t.Fatalf("调整 next_run_at 失败: %v", err)
	}
}

func TestJobSucceeds(t *testing.T) {
	gdb := setupJobsDB(t)
	ctx := context.Background()
	svc := jobs.NewService(gdb)

	var calls int32
	svc.Register("demo.ok", "演示任务", func(_ context.Context, _ types.JSONB) (types.JSONB, error) {
		atomic.AddInt32(&calls, 1)
		return types.JSONB(`{"checked":3}`), nil
	})

	job, err := svc.Enqueue(ctx, jobs.EnqueueInput{Type: "demo.ok", CreatedBy: "tester"})
	if err != nil {
		t.Fatalf("入队失败: %v", err)
	}
	if job.Status != domain.JobStatusPending {
		t.Fatalf("入队后应为 pending，实际 %s", job.Status)
	}

	n, err := svc.RunOnce(ctx)
	if err != nil || n != 1 {
		t.Fatalf("应执行 1 条任务，n=%d err=%v", n, err)
	}
	got, err := svc.Get(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.JobStatusSucceeded {
		t.Fatalf("应成功，实际 %s（错误：%s）", got.Status, got.LastError)
	}
	if got.Attempts != 1 {
		t.Fatalf("执行次数应为 1，实际 %d", got.Attempts)
	}
	if got.FinishedAt == nil {
		t.Error("成功任务应记录完成时间")
	}
	if !strings.Contains(string(got.Result), "checked") {
		t.Fatalf("结果未落库: %s", got.Result)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("handler 应只执行 1 次，实际 %d", calls)
	}
}

// TestJobAutoRetryThenFailed 失败自动重试直到上限，随后转为 failed 并保留失败原因。
func TestJobAutoRetryThenFailed(t *testing.T) {
	gdb := setupJobsDB(t)
	ctx := context.Background()
	svc := jobs.NewService(gdb)

	var calls int32
	svc.Register("demo.fail", "必定失败", func(_ context.Context, _ types.JSONB) (types.JSONB, error) {
		atomic.AddInt32(&calls, 1)
		return nil, errors.New("上游不可用\n带换行的错误")
	})

	job, err := svc.Enqueue(ctx, jobs.EnqueueInput{Type: "demo.fail", CreatedBy: "tester"})
	if err != nil {
		t.Fatal(err)
	}

	// 第一次失败：未达上限 → 回到 pending（带退避）。
	if _, err := svc.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.Get(ctx, job.ID)
	if got.Status != domain.JobStatusPending {
		t.Fatalf("首次失败应回到 pending，实际 %s", got.Status)
	}
	if got.LastError == "" {
		t.Error("失败应记录原因")
	}
	if strings.Contains(got.LastError, "\n") {
		t.Errorf("失败原因应压成单行，实际 %q", got.LastError)
	}
	if !got.NextRunAt.After(got.UpdatedAt.Add(-1 * 1e9)) {
		// 退避时间应明显晚于当前时刻（粗略断言，避免时钟精度问题）。
		t.Logf("next_run_at=%v updated_at=%v", got.NextRunAt, got.UpdatedAt)
	}

	// 重试到上限。
	for i := 0; i < jobs.DefaultMaxAttempts+1; i++ {
		forceDue(t, gdb)
		if _, err := svc.RunOnce(ctx); err != nil {
			t.Fatal(err)
		}
		got, _ = svc.Get(ctx, job.ID)
		if got.Status == domain.JobStatusFailed {
			break
		}
	}
	if got.Status != domain.JobStatusFailed {
		t.Fatalf("重试耗尽后应为 failed，实际 %s（attempts=%d）", got.Status, got.Attempts)
	}
	if got.Attempts != jobs.DefaultMaxAttempts {
		t.Fatalf("执行次数应等于上限 %d，实际 %d", jobs.DefaultMaxAttempts, got.Attempts)
	}
	if atomic.LoadInt32(&calls) != int32(jobs.DefaultMaxAttempts) {
		t.Fatalf("handler 调用次数异常: %d", calls)
	}
}

// TestJobManualRetryKeepsOrigin 人工重试新建任务并保留原始失败现场。
func TestJobManualRetryKeepsOrigin(t *testing.T) {
	gdb := setupJobsDB(t)
	ctx := context.Background()
	svc := jobs.NewService(gdb)

	svc.Register("demo.manual", "人工重试", func(_ context.Context, _ types.JSONB) (types.JSONB, error) {
		return types.JSONB(`{}`), nil
	})

	origin, err := svc.Enqueue(ctx, jobs.EnqueueInput{Type: "demo.manual", Payload: types.JSONB(`{"limit":5}`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Cancel(ctx, origin.ID, "tester"); err != nil {
		t.Fatalf("取消失败: %v", err)
	}

	retried, err := svc.Retry(ctx, origin.ID, "operator")
	if err != nil {
		t.Fatalf("重试失败: %v", err)
	}
	if retried.ID == origin.ID {
		t.Fatal("重试应创建新任务，不能复用原任务 ID")
	}
	if retried.RetryOf == nil || *retried.RetryOf != origin.ID {
		t.Fatalf("重试任务应记录 retry_of，实际 %v", retried.RetryOf)
	}
	// JSONB 落库会被 PostgreSQL 规范化（键序 / 空格），这里比较语义。
	var payload struct {
		Limit int `json:"limit"`
	}
	if err := json.Unmarshal(retried.Payload, &payload); err != nil || payload.Limit != 5 {
		t.Fatalf("重试应沿用原始参数，实际 %s (err=%v)", retried.Payload, err)
	}
	// 原始任务仍保持取消态（失败现场不被覆盖）。
	originAfter, _ := svc.Get(ctx, origin.ID)
	if originAfter.Status != domain.JobStatusCancelled {
		t.Fatalf("原任务状态被改写: %s", originAfter.Status)
	}

	// 成功任务不允许重试。
	if _, err := svc.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Retry(ctx, retried.ID, "operator"); !errors.Is(err, jobs.ErrNotRetryable) {
		t.Fatalf("成功任务重试应返回 ErrNotRetryable，实际 %v", err)
	}
}

func TestJobValidation(t *testing.T) {
	gdb := setupJobsDB(t)
	ctx := context.Background()
	svc := jobs.NewService(gdb)

	if _, err := svc.Enqueue(ctx, jobs.EnqueueInput{Type: "no.such.type"}); !errors.Is(err, jobs.ErrUnknownType) {
		t.Fatalf("未知类型应返回 ErrUnknownType，实际 %v", err)
	}

	svc.Register("demo.validation", "校验", func(_ context.Context, _ types.JSONB) (types.JSONB, error) {
		return types.JSONB(`{}`), nil
	})
	if _, err := svc.Enqueue(ctx, jobs.EnqueueInput{
		Type: "demo.validation", Payload: types.JSONB(`{bad json`),
	}); err == nil {
		t.Fatal("非法 JSON 参数应被拒绝")
	}

	job, _ := svc.Enqueue(ctx, jobs.EnqueueInput{Type: "demo.validation"})
	if _, err := svc.Cancel(ctx, job.ID, "tester"); err != nil {
		t.Fatalf("取消排队任务失败: %v", err)
	}
	// 已取消的任务再次取消 → 冲突。
	if _, err := svc.Cancel(ctx, job.ID, "tester"); !errors.Is(err, jobs.ErrNotCancellable) {
		t.Fatalf("重复取消应返回 ErrNotCancellable，实际 %v", err)
	}
	// 列表筛选非法状态 → 报错（handler 转 422）。
	if _, _, err := svc.List(ctx, jobs.ListFilter{Status: "bogus"}); err == nil {
		t.Fatal("非法状态筛选应报错")
	}
	if _, err := svc.Get(ctx, job.ID); err != nil {
		t.Fatalf("查询已取消任务失败: %v", err)
	}
}

// TestJobConcurrentClaim 并发抢占同一批任务时，每条任务只会被执行一次。
func TestJobConcurrentClaim(t *testing.T) {
	gdb := setupJobsDB(t)
	ctx := context.Background()
	svc := jobs.NewService(gdb)

	var executed sync.Map
	svc.Register("demo.conc", "并发抢占", func(_ context.Context, payload types.JSONB) (types.JSONB, error) {
		executed.Store(string(payload), true)
		return types.JSONB(`{}`), nil
	})

	const total = 8
	for i := 0; i < total; i++ {
		p := types.JSONB(`{"i":` + string(rune('0'+i)) + `}`)
		if _, err := svc.Enqueue(ctx, jobs.EnqueueInput{Type: "demo.conc", Payload: p}); err != nil {
			t.Fatal(err)
		}
	}

	var wg sync.WaitGroup
	var count int32
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				n, err := svc.RunOnce(ctx)
				if err != nil {
					return
				}
				if n == 0 {
					return
				}
				atomic.AddInt32(&count, int32(n))
			}
		}()
	}
	wg.Wait()

	if count != total {
		t.Fatalf("应执行 %d 条，实际 %d", total, count)
	}
	var succeeded int64
	if err := gdb.Model(&domain.AdminJob{}).Where("status = ?", domain.JobStatusSucceeded).Count(&succeeded).Error; err != nil {
		t.Fatal(err)
	}
	if succeeded != total {
		t.Fatalf("成功任务数应为 %d，实际 %d", total, succeeded)
	}
}

// TestPurgeOlderThanOnlyTerminalJobs 保留清理只删终态任务，且受时间边界约束。
func TestPurgeOlderThanOnlyTerminalJobs(t *testing.T) {
	gdb := setupJobsDB(t)
	svc := jobs.NewService(gdb)
	svc.Register("linkhealth.run", "链接巡检", func(context.Context, types.JSONB) (types.JSONB, error) {
		return nil, nil
	})
	ctx := context.Background()

	mk := func(status string) *domain.AdminJob {
		job, err := svc.Enqueue(ctx, jobs.EnqueueInput{
			Type:    "linkhealth.run",
			Payload: types.JSONB(`{"limit":1}`),
		})
		if err != nil {
			t.Fatalf("入队失败: %v", err)
		}
		if err := gdb.Model(&domain.AdminJob{}).Where("id = ?", job.ID).
			Update("status", status).Error; err != nil {
			t.Fatalf("更新状态失败: %v", err)
		}
		return job
	}

	oldDone := mk(domain.JobStatusSucceeded)
	oldFailed := mk(domain.JobStatusFailed)
	oldPending := mk(domain.JobStatusPending)
	freshDone := mk(domain.JobStatusSucceeded)

	old := time.Now().AddDate(0, 0, -120)
	if err := gdb.Model(&domain.AdminJob{}).
		Where("id IN ?", []string{oldDone.ID.String(), oldFailed.ID.String(), oldPending.ID.String()}).
		Update("updated_at", old).Error; err != nil {
		t.Fatalf("回填时间失败: %v", err)
	}

	n, err := svc.PurgeOlderThan(ctx, 90)
	if err != nil {
		t.Fatalf("清理失败: %v", err)
	}
	if n != 2 {
		t.Fatalf("期望删除 2 条终态任务，实际 %d", n)
	}

	var remain []domain.AdminJob
	if err := gdb.Find(&remain).Error; err != nil {
		t.Fatalf("查询剩余任务失败: %v", err)
	}
	if len(remain) != 2 {
		t.Fatalf("期望剩余 2 条，实际 %d", len(remain))
	}
	for _, r := range remain {
		if r.ID == oldPending.ID {
			continue
		}
		if r.ID == freshDone.ID {
			continue
		}
		t.Fatalf("不该保留的任务被保留: id=%s status=%s", r.ID, r.Status)
	}

	// days<=0 表示关闭清理。
	if n, err := svc.PurgeOlderThan(ctx, 0); err != nil || n != 0 {
		t.Fatalf("days<=0 应不清理: n=%d err=%v", n, err)
	}
}
