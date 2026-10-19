package metrics

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"mckit/backend/internal/domain"
)

// fakeStore 记录落库调用，供断言。
type fakeStore struct {
	mu        sync.Mutex
	api       []domain.APIStatDaily
	tool      []domain.ToolStatDaily
	res       []domain.ResourceStatDaily
	search    []domain.SearchStatDaily
	pruneCut  time.Time
	pruneCall int
}

func (f *fakeStore) UpsertAPIStats(_ context.Context, rows []domain.APIStatDaily) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.api = append(f.api, rows...)
	return nil
}

func (f *fakeStore) UpsertToolStats(_ context.Context, rows []domain.ToolStatDaily) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tool = append(f.tool, rows...)
	return nil
}

func (f *fakeStore) UpsertResourceStats(_ context.Context, rows []domain.ResourceStatDaily) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.res = append(f.res, rows...)
	return nil
}

func (f *fakeStore) UpsertSearchStats(_ context.Context, rows []domain.SearchStatDaily) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.search = append(f.search, rows...)
	return nil
}

func (f *fakeStore) PruneBefore(_ context.Context, cutoff time.Time) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pruneCut = cutoff
	f.pruneCall++
	return 1, nil
}

// fixedNow 让采集器使用固定时间。
func fixedNow(t time.Time) func() time.Time { return func() time.Time { return t } }

func TestObserveAggregatesStatusAndLatency(t *testing.T) {
	store := &fakeStore{}
	c := New(store, Options{FlushInterval: time.Hour})
	now := time.Date(2026, 10, 17, 10, 0, 0, 0, time.Local)
	c.now = fixedNow(now)

	c.Observe("/api/v1/tools/{slug}", http.MethodPost, http.StatusOK, 30*time.Millisecond)
	c.Observe("/api/v1/tools/{slug}", http.MethodPost, http.StatusInternalServerError, 800*time.Millisecond)
	c.Observe("/api/v1/tools/{slug}", http.MethodPost, http.StatusNotFound, 1200*time.Millisecond)
	c.Observe("/api/v1/tools/{slug}", http.MethodPost, http.StatusTooManyRequests, 5*time.Millisecond)
	c.Observe("/api/v1/tools/{slug}", http.MethodPost, http.StatusOK, 9000*time.Millisecond)

	if err := c.Flush(context.Background()); err != nil {
		t.Fatalf("Flush 失败: %v", err)
	}
	if len(store.api) != 1 {
		t.Fatalf("期望 1 行 API 聚合，实际 %d", len(store.api))
	}
	row := store.api[0]
	if row.Total != 5 {
		t.Errorf("total = %d, 期望 5", row.Total)
	}
	if row.Errors != 1 {
		t.Errorf("errors = %d, 期望 1（仅 5xx）", row.Errors)
	}
	if row.ClientErrors != 1 {
		t.Errorf("client_errors = %d, 期望 1（404，不含 429）", row.ClientErrors)
	}
	if row.RateLimited != 1 {
		t.Errorf("rate_limited = %d, 期望 1", row.RateLimited)
	}
	if row.LatencyMsMax != 9000 {
		t.Errorf("latency_ms_max = %d, 期望 9000", row.LatencyMsMax)
	}
	if row.LatencyMsSum != 30+800+1200+5+9000 {
		t.Errorf("latency_ms_sum = %d", row.LatencyMsSum)
	}
	// 分桶：30<=50、5<=50 归入 le_50；800<=1000、1200 归入 le_3000? 1200<=3000；
	// 9000 > 3000 归入 gt_3000。
	if row.Le50 != 2 {
		t.Errorf("le_50 = %d, 期望 2", row.Le50)
	}
	if row.Le1000 != 1 {
		t.Errorf("le_1000 = %d, 期望 1", row.Le1000)
	}
	if row.Le3000 != 1 {
		t.Errorf("le_3000 = %d, 期望 1", row.Le3000)
	}
	if row.Gt3000 != 1 {
		t.Errorf("gt_3000 = %d, 期望 1", row.Gt3000)
	}
	// 落库后缓冲应清空，二次 flush 无新行。
	if err := c.Flush(context.Background()); err != nil {
		t.Fatalf("二次 Flush 失败: %v", err)
	}
	if len(store.api) != 1 {
		t.Errorf("落库后缓冲未清空，行数 = %d", len(store.api))
	}
	if !row.StatDate.Equal(fixedNow(now)()) {
		// 允许时区归一：断言日期部分一致。
		y1, m1, d1 := row.StatDate.Date()
		y2, m2, d2 := now.Date()
		if y1 != y2 || m1 != m2 || d1 != d2 {
			t.Errorf("stat_date = %v, 期望日期 %v", row.StatDate, now)
		}
	}
}

func TestRecordToolAndResource(t *testing.T) {
	store := &fakeStore{}
	c := New(store, Options{FlushInterval: time.Hour})
	c.now = fixedNow(time.Date(2026, 10, 17, 0, 0, 0, 0, time.Local))

	c.RecordTool("region-inspect", ActionVisit)
	c.RecordTool("region-inspect", ActionUse)
	c.RecordTool("region-inspect", ActionUse)
	c.RecordTool("region-inspect", ActionFail)
	c.RecordTool("region-inspect", "bogus") // 未知动作忽略
	c.RecordTool("", ActionVisit)           // 空 slug 忽略
	c.RecordResourceView("jei")
	c.RecordResourceView("jei")

	if err := c.Flush(context.Background()); err != nil {
		t.Fatalf("Flush 失败: %v", err)
	}
	if len(store.tool) != 1 {
		t.Fatalf("期望 1 行工具聚合，实际 %d", len(store.tool))
	}
	tool := store.tool[0]
	if tool.Visits != 1 || tool.Uses != 2 || tool.Failures != 1 {
		t.Errorf("工具计数错误: %+v", tool)
	}
	if len(store.res) != 1 || store.res[0].Views != 2 {
		t.Errorf("资源访问计数错误: %+v", store.res)
	}
}

func TestRecordSearch(t *testing.T) {
	store := &fakeStore{}
	c := New(store, Options{FlushInterval: time.Hour})
	c.now = fixedNow(time.Date(2026, 10, 17, 12, 0, 0, 0, time.Local))

	c.RecordSearch(false, false)
	c.RecordSearch(true, false)
	c.RecordSearch(false, true)

	if err := c.Flush(context.Background()); err != nil {
		t.Fatalf("Flush 失败: %v", err)
	}
	if len(store.search) != 1 {
		t.Fatalf("期望 1 行搜索聚合，实际 %d", len(store.search))
	}
	s := store.search[0]
	if s.Queries != 3 || s.ZeroResult != 1 || s.Failed != 1 {
		t.Errorf("搜索计数错误: %+v", s)
	}
}

func TestFlushPrunesOncePerInterval(t *testing.T) {
	store := &fakeStore{}
	c := New(store, Options{
		FlushInterval: time.Hour,
		RetentionDays: 180,
		PruneInterval: 6 * time.Hour,
	})
	base := time.Date(2026, 10, 17, 0, 0, 0, 0, time.Local)
	c.now = fixedNow(base)

	if err := c.Flush(context.Background()); err != nil {
		t.Fatalf("Flush 失败: %v", err)
	}
	// 期望 cutoff = 今天 - (180-1) 天。
	wantCutoff := dayStart(base).AddDate(0, 0, -179)
	if !store.pruneCut.Equal(wantCutoff) {
		t.Errorf("cutoff = %v, 期望 %v", store.pruneCut, wantCutoff)
	}
	// 未超过清理周期时再次 flush 不再清理。
	if err := c.Flush(context.Background()); err != nil {
		t.Fatalf("Flush 失败: %v", err)
	}
	if store.pruneCall != 1 {
		t.Errorf("清理调用次数 = %d, 期望 1", store.pruneCall)
	}
	// 超过清理周期后再次清理。
	c.now = fixedNow(base.Add(7 * time.Hour))
	if err := c.Flush(context.Background()); err != nil {
		t.Fatalf("Flush 失败: %v", err)
	}
	if store.pruneCall != 2 {
		t.Errorf("清理调用次数 = %d, 期望 2", store.pruneCall)
	}
}

func TestRetentionDisabledSkipsPrune(t *testing.T) {
	store := &fakeStore{}
	c := New(store, Options{FlushInterval: time.Hour, RetentionDays: 0})
	if err := c.Flush(context.Background()); err != nil {
		t.Fatalf("Flush 失败: %v", err)
	}
	if store.pruneCall != 0 {
		t.Errorf("retention=0 时不应清理，调用次数 = %d", store.pruneCall)
	}
}

func TestStartStopFlushes(t *testing.T) {
	store := &fakeStore{}
	c := New(store, Options{FlushInterval: 20 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.Start(ctx)

	c.RecordTool("recipes", ActionUse)
	deadline := time.Now().Add(2 * time.Second)
	for {
		store.mu.Lock()
		n := len(store.tool)
		store.mu.Unlock()
		if n > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("后台循环未在预期时间内落库")
		}
		time.Sleep(10 * time.Millisecond)
	}
	c.Stop()
	// Stop 应幂等。
	c.Stop()
}

func TestPercentileFromBuckets(t *testing.T) {
	bounds := LatencyBucketBounds
	// 100 个样本：90 个 <=50ms，10 个 <=1000ms。
	buckets := []int64{90, 0, 0, 10, 0, 0}
	if got := PercentileFromBuckets(buckets, bounds, 100, 0.5); got != 50 {
		t.Errorf("p50 = %d, 期望 50", got)
	}
	if got := PercentileFromBuckets(buckets, bounds, 100, 0.95); got != 1000 {
		t.Errorf("p95 = %d, 期望 1000", got)
	}
	if got := PercentileFromBuckets(buckets, bounds, 0, 0.95); got != 0 {
		t.Errorf("空样本 p95 = %d, 期望 0", got)
	}
	// 全部溢出桶。
	overflow := []int64{0, 0, 0, 0, 0, 5}
	if got := PercentileFromBuckets(overflow, bounds, 5, 0.95); got != 3000 {
		t.Errorf("溢出桶 p95 = %d, 期望 3000（最大上界）", got)
	}
}
