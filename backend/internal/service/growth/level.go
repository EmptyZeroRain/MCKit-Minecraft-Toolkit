package growth

import (
	"sort"

	"mckit/backend/internal/domain"
)

// 访客等级与活跃门槛。
//
// 新用户默认 Lv.0「访客」；累计满 7 个活跃日（每天最多记一次）后，
// 才会进入 Lv.1 及以上的经验等级体系。活跃日由成长事件按业务时区聚合，
// 因此刷新页面、查看资料等只读操作不会增加活跃天数。
const (
	GuestLevel              = 0
	GuestActiveDaysRequired = 7
	GuestLevelTitle         = "访客"

	// DefaultMaxLevel 默认等级上限；等级按经验曲线自动生成，不再逐级建表。
	DefaultMaxLevel = 9999
	// DefaultLevelStepXP 默认每级递增值：Lv.N 门槛 = step * N * (N-1) / 2。
	DefaultLevelStepXP int64 = 100
	// MaxLevelStepXP 防止配置过大导致阈值溢出 int64。
	MaxLevelStepXP int64 = 1_000_000_000
)

// LevelCurve 等级曲线参数。
type LevelCurve struct {
	MaxLevel int
	StepXP   int64
}

// DefaultLevelCurve 返回默认等级曲线。
func DefaultLevelCurve() LevelCurve {
	return LevelCurve{MaxLevel: DefaultMaxLevel, StepXP: DefaultLevelStepXP}
}

// NormalizeLevelCurve 把数据库配置收敛到合法范围。
func NormalizeLevelCurve(cfg *domain.GrowthLevelConfig) LevelCurve {
	out := DefaultLevelCurve()
	if cfg == nil {
		return out
	}
	if cfg.MaxLevel > 0 && cfg.MaxLevel <= DefaultMaxLevel {
		out.MaxLevel = cfg.MaxLevel
	}
	if cfg.StepXP > 0 && cfg.StepXP <= MaxLevelStepXP {
		out.StepXP = cfg.StepXP
	}
	return out
}

// ValidateLevelCurve 校验后台提交的等级曲线。
func ValidateLevelCurve(maxLevel int, stepXP int64) error {
	if maxLevel < 1 || maxLevel > DefaultMaxLevel {
		return ErrLevelThreshold
	}
	if stepXP <= 0 || stepXP > MaxLevelStepXP {
		return ErrLevelThreshold
	}
	return nil
}

// LevelView 等级展示视图。
type LevelView struct {
	Level           int    `json:"level"`
	Title           string `json:"level_title"`
	NextThreshold   *int64 `json:"next_level_xp"`
	XPToNext        int64  `json:"xp_to_next_level"`
	ProgressPercent int    `json:"level_progress_percent"`
	IsMax           bool   `json:"is_max_level"`
	// Overridden 表示当前展示等级由后台人工直接覆盖，经验变化不会自动改级。
	Overridden bool `json:"is_level_overridden"`
	// ActiveDays 已累计活跃天数（封顶 GuestActiveDaysRequired）。
	ActiveDays int `json:"active_days"`
	// GuestActiveDays 脱离访客阶段所需的活跃天数。
	GuestActiveDays int `json:"guest_active_days"`
}

// LevelResolver 依据等级曲线与称号规则计算展示等级。
type LevelResolver struct {
	Curve  LevelCurve
	Titles []domain.GrowthLevelTitle
}

// NewLevelResolver 创建等级解析器；titles 只保留启用规则。
func NewLevelResolver(cfg *domain.GrowthLevelConfig, titles []domain.GrowthLevelTitle) LevelResolver {
	out := LevelResolver{Curve: NormalizeLevelCurve(cfg)}
	for _, t := range titles {
		if !t.Enabled {
			continue
		}
		if t.MinLevel < 0 || t.MaxLevel < t.MinLevel || t.MaxLevel > out.Curve.MaxLevel {
			continue
		}
		out.Titles = append(out.Titles, t)
	}
	sort.SliceStable(out.Titles, func(i, j int) bool {
		if out.Titles[i].MinLevel != out.Titles[j].MinLevel {
			return out.Titles[i].MinLevel < out.Titles[j].MinLevel
		}
		return out.Titles[i].MaxLevel < out.Titles[j].MaxLevel
	})
	return out
}

// LevelThreshold 返回达到指定等级所需的累计经验。
func LevelThreshold(level int, curve LevelCurve) int64 {
	if level <= 1 {
		return 0
	}
	if level > curve.MaxLevel {
		level = curve.MaxLevel
	}
	return curve.StepXP * int64(level) * int64(level-1) / 2
}

// levelForXP 返回当前经验对应的最高等级（1..MaxLevel）。
func levelForXP(xp int64, curve LevelCurve) int {
	if xp <= 0 {
		return 1
	}
	if LevelThreshold(curve.MaxLevel, curve) <= xp {
		return curve.MaxLevel
	}
	lo, hi := 1, curve.MaxLevel
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if LevelThreshold(mid, curve) <= xp {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo
}

// Title 返回指定等级的称号。
//
// 优先级：指定等级称号（min=max）> 包含该等级的区间称号 > 兜底 Lv.N。
// 区间在数据库与服务层均要求不重叠；这里仍按“区间更窄优先”稳定选择，
// 避免历史脏数据造成随机结果。
func (r LevelResolver) Title(level int) string {
	var exact *domain.GrowthLevelTitle
	var rangeTitle *domain.GrowthLevelTitle
	for i := range r.Titles {
		t := &r.Titles[i]
		if t.MinLevel == level && t.MaxLevel == level {
			exact = t
			break
		}
		if t.MinLevel < t.MaxLevel && t.MinLevel <= level && t.MaxLevel >= level {
			if rangeTitle == nil {
				rangeTitle = t
				continue
			}
			curWidth := rangeTitle.MaxLevel - rangeTitle.MinLevel
			newWidth := t.MaxLevel - t.MinLevel
			if newWidth < curWidth ||
				(newWidth == curWidth && t.UpdatedAt.After(rangeTitle.UpdatedAt)) {
				rangeTitle = t
			}
		}
	}
	if exact != nil {
		return exact.Title
	}
	if rangeTitle != nil {
		return rangeTitle.Title
	}
	if level == GuestLevel {
		return GuestLevelTitle
	}
	return "Lv." + itoa(level)
}

// View 计算最终展示等级：人工覆盖优先于经验 / 活跃天数。
func (r LevelResolver) View(xp int64, override *int, activeDays int) LevelView {
	if override != nil {
		level := clampLevel(*override, r.Curve.MaxLevel)
		view := LevelView{
			Level: level, Title: r.Title(level), Overridden: true,
			ActiveDays: activeDays, GuestActiveDays: GuestActiveDaysRequired,
		}
		if level == GuestLevel {
			view.ProgressPercent = guestActiveProgress(activeDays)
		} else {
			// 覆盖等级不参与经验自动升级，进度条固定显示满格。
			view.ProgressPercent = 100
		}
		view.IsMax = level >= r.Curve.MaxLevel
		return view
	}

	if activeDays < GuestActiveDaysRequired {
		return LevelView{
			Level: GuestLevel, Title: r.Title(GuestLevel),
			ProgressPercent: guestActiveProgress(activeDays),
			ActiveDays:      activeDays,
			GuestActiveDays: GuestActiveDaysRequired,
		}
	}

	level := levelForXP(xp, r.Curve)
	view := LevelView{
		Level: level, Title: r.Title(level),
		ActiveDays: activeDays, GuestActiveDays: GuestActiveDaysRequired,
	}
	if level >= r.Curve.MaxLevel {
		view.IsMax = true
		view.ProgressPercent = 100
		return view
	}

	curThreshold := LevelThreshold(level, r.Curve)
	nextThreshold := LevelThreshold(level+1, r.Curve)
	view.NextThreshold = &nextThreshold
	view.XPToNext = nextThreshold - xp
	if view.XPToNext < 0 {
		view.XPToNext = 0
	}
	span := nextThreshold - curThreshold
	if span > 0 {
		view.ProgressPercent = int((xp - curThreshold) * 100 / span)
		if view.ProgressPercent < 0 {
			view.ProgressPercent = 0
		}
		if view.ProgressPercent > 100 {
			view.ProgressPercent = 100
		}
	}
	return view
}

// Level 返回最终展示等级（用于回写 user_growth.level）。
func (r LevelResolver) Level(xp int64, override *int, activeDays int) int {
	return r.View(xp, override, activeDays).Level
}

// clampLevel 把等级限制在 0..maxLevel。
func clampLevel(level, maxLevel int) int {
	if level < GuestLevel {
		return GuestLevel
	}
	if level > maxLevel {
		return maxLevel
	}
	return level
}

// guestActiveProgress 把活跃天数换算成 0-100 的访客阶段进度。
func guestActiveProgress(activeDays int) int {
	if activeDays <= 0 {
		return 0
	}
	if activeDays >= GuestActiveDaysRequired {
		return 100
	}
	return activeDays * 100 / GuestActiveDaysRequired
}
