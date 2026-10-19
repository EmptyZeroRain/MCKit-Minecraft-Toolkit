// Package metrics 提供使用统计的采集与按天聚合（TODO §13）。
//
// 设计目标：
//   - 请求期间只做内存累加，按固定周期批量落库，避免每请求写库放大数据库压力；
//   - 只记录可聚合的最小信息（路由模板 / 状态码 / 耗时 / 公开 slug），
//     **不记录** IP、会话、用户标识、User-Agent、query、请求体或逐次时间序列；
//   - 延迟以固定分桶记录，用桶上界近似分位数，不保存原始逐次耗时；
//   - 提供保留期限清理：超期数据由后台任务定期删除。
package metrics

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"mckit/backend/internal/domain"
)

// LatencyBucketBounds 延迟分桶上界（毫秒）；超出最后一档计入「大于最大上界」桶。
// 修改本切片时须同步 migrations/0011_usage_stats.sql 的 api_stats_daily 分桶列。
var LatencyBucketBounds = []int64{50, 200, 500, 1000, 3000}

// 工具统计的动作类型（与前端上报契约一致）。
const (
	// ActionVisit 工具页面访问。
	ActionVisit = "visit"
	// ActionUse 工具实际使用（产出结果 / 执行关键动作）。
	ActionUse = "use"
	// ActionFail 工具执行失败。
	ActionFail = "fail"
)

// UnmatchedRoute 未匹配到路由模板时的归并标签（含全局限流提前返回的请求）。
const UnmatchedRoute = "<unmatched>"

type apiKey struct {
	date   string
	route  string
	method string
}

type toolKey struct {
	date string
	slug string
}

type resourceKey struct {
	date string
	slug string
}

// Options 采集器参数。
type Options struct {
	// FlushInterval 落库周期，默认 60s。
	FlushInterval time.Duration
	// RetentionDays 明细保留天数（含当天），默认 180；<=0 表示不清理。
	RetentionDays int
	// PruneInterval 清理检查周期，默认 6h。
	PruneInterval time.Duration
	// Logger 日志器，默认 slog.Default()。
	Logger *slog.Logger
}

// Collector 使用统计采集器：内存聚合 + 定时批量落库 + 超期清理。
// 零值不可用，须经 New 构造。
type Collector struct {
	store         Store
	log           *slog.Logger
	now           func() time.Time
	flushInterval time.Duration
	retentionDays int
	pruneInterval time.Duration

	mu        sync.Mutex
	api       map[apiKey]*domain.APIStatDaily
	tool      map[toolKey]*domain.ToolStatDaily
	res       map[resourceKey]*domain.ResourceStatDaily
	search    map[string]*domain.SearchStatDaily
	lastPrune time.Time

	stop    chan struct{}
	done    chan struct{}
	running bool
}

// New 创建采集器。store 为 nil 时不做落库（仅内存累加，用于关闭统计的场景）。
func New(store Store, opts Options) *Collector {
	if opts.FlushInterval <= 0 {
		opts.FlushInterval = 60 * time.Second
	}
	if opts.PruneInterval <= 0 {
		opts.PruneInterval = 6 * time.Hour
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Collector{
		store:         store,
		log:           logger,
		now:           time.Now,
		flushInterval: opts.FlushInterval,
		retentionDays: opts.RetentionDays,
		pruneInterval: opts.PruneInterval,
		api:           make(map[apiKey]*domain.APIStatDaily),
		tool:          make(map[toolKey]*domain.ToolStatDaily),
		res:           make(map[resourceKey]*domain.ResourceStatDaily),
		search:        make(map[string]*domain.SearchStatDaily),
	}
}

// dayStart 将时间归一到「服务器本地日期」的 UTC 零点，使 DATE 列稳定。
func dayStart(t time.Time) time.Time {
	y, m, d := t.In(time.Local).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func (c *Collector) today() time.Time { return dayStart(c.now()) }

// Observe 记录一次 API 调用。route 应为 chi 路由模板（不含具体参数值）。
func (c *Collector) Observe(route, method string, status int, d time.Duration) {
	if c == nil {
		return
	}
	if route == "" {
		route = UnmatchedRoute
	}
	day := c.today()
	ms := d.Milliseconds()

	c.mu.Lock()
	defer c.mu.Unlock()
	key := apiKey{date: day.Format("2006-01-02"), route: route, method: method}
	row := c.api[key]
	if row == nil {
		row = &domain.APIStatDaily{StatDate: day, Route: route, Method: method}
		c.api[key] = row
	}
	row.Total++
	switch {
	case status == http.StatusTooManyRequests:
		// 限流单独计数，不重复计入 client_errors。
		row.RateLimited++
	case status >= 500:
		row.Errors++
	case status >= 400:
		row.ClientErrors++
	}
	row.LatencyMsSum += ms
	if ms > row.LatencyMsMax {
		row.LatencyMsMax = ms
	}
	addLatencyBucket(row, ms)
}

// addLatencyBucket 按固定分桶累加一次耗时。
func addLatencyBucket(row *domain.APIStatDaily, ms int64) {
	cols := []*int64{&row.Le50, &row.Le200, &row.Le500, &row.Le1000, &row.Le3000}
	for i, bound := range LatencyBucketBounds {
		if ms <= bound {
			*cols[i]++
			return
		}
	}
	row.Gt3000++
}

// RecordTool 记录工具事件。action 仅接受 ActionVisit / ActionUse / ActionFail，其余忽略。
func (c *Collector) RecordTool(slug, action string) {
	if c == nil || slug == "" {
		return
	}
	switch action {
	case ActionVisit, ActionUse, ActionFail:
	default:
		return
	}
	day := c.today()

	c.mu.Lock()
	defer c.mu.Unlock()
	key := toolKey{date: day.Format("2006-01-02"), slug: slug}
	row := c.tool[key]
	if row == nil {
		row = &domain.ToolStatDaily{StatDate: day, ToolSlug: slug}
		c.tool[key] = row
	}
	switch action {
	case ActionUse:
		row.Uses++
	case ActionFail:
		row.Failures++
	default:
		row.Visits++
	}
}

// RecordResourceView 记录一次资源详情页访问。
func (c *Collector) RecordResourceView(slug string) {
	if c == nil || slug == "" {
		return
	}
	day := c.today()

	c.mu.Lock()
	defer c.mu.Unlock()
	key := resourceKey{date: day.Format("2006-01-02"), slug: slug}
	row := c.res[key]
	if row == nil {
		row = &domain.ResourceStatDaily{StatDate: day, ResourceSlug: slug}
		c.res[key] = row
	}
	row.Views++
}

// RecordSearch 记录一次搜索：zeroResult 表示查询成功但无结果，failed 表示查询链路失败。
// 两者互斥时按调用方语义传入（无结果用 zeroResult=true，失败用 failed=true）。
func (c *Collector) RecordSearch(zeroResult, failed bool) {
	if c == nil {
		return
	}
	day := c.today()
	key := day.Format("2006-01-02")

	c.mu.Lock()
	defer c.mu.Unlock()
	row := c.search[key]
	if row == nil {
		row = &domain.SearchStatDaily{StatDate: day}
		c.search[key] = row
	}
	row.Queries++
	if zeroResult {
		row.ZeroResult++
	}
	if failed {
		row.Failed++
	}
}

// Flush 将当前内存聚合批量落库（冲突时累加），并按保留期限清理超期数据。
// 落库后清空内存缓冲；任一写入失败返回首个错误（不影响其余写入尝试）。
func (c *Collector) Flush(ctx context.Context) error {
	if c == nil {
		return nil
	}

	c.mu.Lock()
	apiRows := make([]domain.APIStatDaily, 0, len(c.api))
	for _, r := range c.api {
		apiRows = append(apiRows, *r)
	}
	toolRows := make([]domain.ToolStatDaily, 0, len(c.tool))
	for _, r := range c.tool {
		toolRows = append(toolRows, *r)
	}
	resRows := make([]domain.ResourceStatDaily, 0, len(c.res))
	for _, r := range c.res {
		resRows = append(resRows, *r)
	}
	searchRows := make([]domain.SearchStatDaily, 0, len(c.search))
	for _, r := range c.search {
		searchRows = append(searchRows, *r)
	}
	c.api = make(map[apiKey]*domain.APIStatDaily)
	c.tool = make(map[toolKey]*domain.ToolStatDaily)
	c.res = make(map[resourceKey]*domain.ResourceStatDaily)
	c.search = make(map[string]*domain.SearchStatDaily)
	shouldPrune := c.shouldPruneLocked()
	c.mu.Unlock()

	var firstErr error
	if c.store != nil {
		for _, err := range []error{
			c.store.UpsertAPIStats(ctx, apiRows),
			c.store.UpsertToolStats(ctx, toolRows),
			c.store.UpsertResourceStats(ctx, resRows),
			c.store.UpsertSearchStats(ctx, searchRows),
		} {
			if err != nil && firstErr == nil {
				firstErr = err
			}
		}
		if shouldPrune {
			n, err := c.store.PruneBefore(ctx, c.retentionCutoff())
			if err != nil {
				c.log.Warn("使用统计清理失败", "err", err)
			} else if n > 0 {
				c.log.Info("使用统计超期数据已清理", "rows", n, "retention_days", c.retentionDays)
			}
		}
	}
	return firstErr
}

// retentionCutoff 清理阈值：删除 stat_date < cutoff 的行，保留最近 retentionDays 天（含当天）。
func (c *Collector) retentionCutoff() time.Time {
	if c.retentionDays <= 0 {
		return time.Time{}
	}
	return c.today().AddDate(0, 0, -(c.retentionDays - 1))
}

// shouldPruneLocked 判断本次 flush 是否需要执行清理（调用方须持锁）。
func (c *Collector) shouldPruneLocked() bool {
	if c.retentionDays <= 0 || c.store == nil {
		return false
	}
	if !c.lastPrune.IsZero() && c.now().Sub(c.lastPrune) < c.pruneInterval {
		return false
	}
	c.lastPrune = c.now()
	return true
}

// Start 启动后台落库与清理循环。重复调用无副作用。
// ctx 取消即停止；另可调用 Stop 主动停止并做最终落库。
func (c *Collector) Start(ctx context.Context) {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return
	}
	c.running = true
	c.stop = make(chan struct{})
	c.done = make(chan struct{})
	c.mu.Unlock()

	go func() {
		ticker := time.NewTicker(c.flushInterval)
		defer ticker.Stop()
		defer close(c.done)
		for {
			select {
			case <-ctx.Done():
				return
			case <-c.stop:
				return
			case <-ticker.C:
				c.flushWithTimeout()
			}
		}
	}()
}

// Stop 停止后台循环并做最终落库（用于优雅关闭）。未启动时为无操作。
func (c *Collector) Stop() {
	if c == nil {
		return
	}
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return
	}
	c.running = false
	close(c.stop)
	c.mu.Unlock()
	<-c.done
	c.flushWithTimeout()
}

// flushWithTimeout 带超时地落库，避免关闭流程被数据库卡死。
func (c *Collector) flushWithTimeout() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := c.Flush(ctx); err != nil {
		c.log.Warn("使用统计落库失败", "err", err)
	}
}

// PercentileFromBuckets 用固定分桶近似分位数：返回累计占比首次达到 p 的桶上界。
// total 为样本总数；分桶不足时返回最大上界（保守估计）。total <= 0 返回 0。
func PercentileFromBuckets(buckets []int64, bounds []int64, total int64, p float64) int64 {
	if total <= 0 || len(bounds) == 0 {
		return 0
	}
	target := int64(float64(total)*p + 0.999999) // 向上取整
	if target < 1 {
		target = 1
	}
	var acc int64
	for i, b := range bounds {
		if i < len(buckets) {
			acc += buckets[i]
		}
		if acc >= target {
			return b
		}
	}
	// 样本落在溢出桶（均大于最大上界）时返回最大上界。
	return bounds[len(bounds)-1]
}
