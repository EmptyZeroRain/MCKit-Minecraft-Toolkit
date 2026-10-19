package domain

import "time"

// 使用统计（TODO §13）按天聚合。
//
// 隐私设计：本组表只保存可聚合的最小信息（日期、公开 slug、路由模板、状态码计数、
// 延迟分桶），**不含** IP、会话、用户标识、User-Agent、query、请求体或逐次时间序列。

// ToolStatDaily 工具访问 / 使用 / 失败（日聚合，按工具 slug）。
type ToolStatDaily struct {
	StatDate time.Time `gorm:"column:stat_date;type:date;primaryKey" json:"stat_date"`
	ToolSlug string    `gorm:"column:tool_slug;size:180;primaryKey" json:"tool_slug"`
	Visits   int64     `gorm:"column:visits;not null;default:0" json:"visits"`
	Uses     int64     `gorm:"column:uses;not null;default:0" json:"uses"`
	Failures int64     `gorm:"column:failures;not null;default:0" json:"failures"`
}

// TableName 指定表名。
func (ToolStatDaily) TableName() string { return "tool_stats_daily" }

// ResourceStatDaily 资源详情访问量（日聚合，按资源 slug）。
type ResourceStatDaily struct {
	StatDate     time.Time `gorm:"column:stat_date;type:date;primaryKey" json:"stat_date"`
	ResourceSlug string    `gorm:"column:resource_slug;size:180;primaryKey" json:"resource_slug"`
	Views        int64     `gorm:"column:views;not null;default:0" json:"views"`
}

// TableName 指定表名。
func (ResourceStatDaily) TableName() string { return "resource_stats_daily" }

// APIStatDaily 关键 API 指标（日聚合，按路由模板 + 方法）。
//
// errors 为 5xx；client_errors 为 4xx（不含 429）；rate_limited 为 429，
// 不与 client_errors 重复计数。延迟以固定分桶记录（见 metrics.LatencyBucketBounds）。
type APIStatDaily struct {
	StatDate     time.Time `gorm:"column:stat_date;type:date;primaryKey" json:"stat_date"`
	Route        string    `gorm:"column:route;size:200;primaryKey" json:"route"`
	Method       string    `gorm:"column:method;size:10;primaryKey" json:"method"`
	Total        int64     `gorm:"column:total;not null;default:0" json:"total"`
	Errors       int64     `gorm:"column:errors;not null;default:0" json:"errors"`
	ClientErrors int64     `gorm:"column:client_errors;not null;default:0" json:"client_errors"`
	RateLimited  int64     `gorm:"column:rate_limited;not null;default:0" json:"rate_limited"`
	LatencyMsSum int64     `gorm:"column:latency_ms_sum;not null;default:0" json:"latency_ms_sum"`
	LatencyMsMax int64     `gorm:"column:latency_ms_max;not null;default:0" json:"latency_ms_max"`
	Le50         int64     `gorm:"column:latency_le_50;not null;default:0" json:"latency_le_50"`
	Le200        int64     `gorm:"column:latency_le_200;not null;default:0" json:"latency_le_200"`
	Le500        int64     `gorm:"column:latency_le_500;not null;default:0" json:"latency_le_500"`
	Le1000       int64     `gorm:"column:latency_le_1000;not null;default:0" json:"latency_le_1000"`
	Le3000       int64     `gorm:"column:latency_le_3000;not null;default:0" json:"latency_le_3000"`
	Gt3000       int64     `gorm:"column:latency_gt_3000;not null;default:0" json:"latency_gt_3000"`
}

// TableName 指定表名。
func (APIStatDaily) TableName() string { return "api_stats_daily" }

// SearchStatDaily 搜索总量（日聚合）。词级明细见 SearchTerm。
type SearchStatDaily struct {
	StatDate   time.Time `gorm:"column:stat_date;type:date;primaryKey" json:"stat_date"`
	Queries    int64     `gorm:"column:queries;not null;default:0" json:"queries"`
	ZeroResult int64     `gorm:"column:zero_result;not null;default:0" json:"zero_result"`
	Failed     int64     `gorm:"column:failed;not null;default:0" json:"failed"`
}

// TableName 指定表名。
func (SearchStatDaily) TableName() string { return "search_stats_daily" }
