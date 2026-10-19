package growth

import (
	"strings"
	"time"
)

// DefaultTimezone 站点业务时区默认值（jingyan.md §4.2）。
const DefaultTimezone = "Asia/Shanghai"

// businessDate 按站点业务时区计算业务日期（YYYY-MM-DD）。
// 每日任务按该日期形成 (user_id, task_code, task_date) 独立记录，无需定时清空。
func businessDate(t time.Time, loc *time.Location) string {
	if loc == nil {
		loc = time.UTC
	}
	return t.In(loc).Format("2006-01-02")
}

// dayRange 返回业务日期对应的 UTC 时间区间 [start, end)，用于按天统计经验流水。
func dayRange(date string, loc *time.Location) (time.Time, time.Time, error) {
	if loc == nil {
		loc = time.UTC
	}
	start, err := time.ParseInLocation("2006-01-02", date, loc)
	if err != nil {
		return time.Time{}, time.Time{}, ErrBadInput
	}
	return start.UTC(), start.AddDate(0, 0, 1).UTC(), nil
}

// normalizeTimezone 解析时区名；非法时回退默认时区。
func normalizeTimezone(name string) *time.Location {
	name = strings.TrimSpace(name)
	if name == "" {
		name = DefaultTimezone
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		if loc, err = time.LoadLocation(DefaultTimezone); err != nil {
			return time.UTC
		}
	}
	return loc
}
