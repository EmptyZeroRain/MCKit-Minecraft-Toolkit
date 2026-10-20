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
)

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

// enabledLevels 过滤并返回启用的等级（阈值升序、同阈值按等级号升序）。
//
// Lv.0 与 Lv.1 都是 0 XP：同阈值时等级号更大的 Lv.1 排在后面，
// 保证满 7 个活跃日后选择 Lv.1 而不是停留 Lv.0。
func enabledLevels(levels []domain.GrowthLevel) []domain.GrowthLevel {
	out := make([]domain.GrowthLevel, 0, len(levels))
	for _, l := range levels {
		if l.Enabled {
			out = append(out, l)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ThresholdXP != out[j].ThresholdXP {
			return out[i].ThresholdXP < out[j].ThresholdXP
		}
		return out[i].Level < out[j].Level
	})
	return out
}

// computeLevel 依据当前有效经验、活跃天数与等级阈值自动计算等级（jingyan.md §5）。
//
// 规则：
//   - 活跃天数不足 7 天时固定为 Lv.0「访客」，经验继续累积但不展示更高等级；
//   - 满 7 个活跃日后，取所有 threshold_xp <= xp 的最高等级；
//   - 达到最高等级后进度显示为满级；
//   - 冲正导致经验下降时由同一函数重新计算（是否降级不由前端决定）。
func computeLevel(levels []domain.GrowthLevel, xp int64, activeDays int) LevelView {
	enabled := enabledLevels(levels)
	if len(enabled) == 0 {
		// 阈值表缺失时的兜底：保持访客状态，避免把新用户误判为高等级。
		return LevelView{
			Level: GuestLevel, Title: GuestLevelTitle, IsMax: true,
			ActiveDays: activeDays, GuestActiveDays: GuestActiveDaysRequired,
		}
	}

	if activeDays < GuestActiveDaysRequired {
		for _, l := range enabled {
			if l.Level == GuestLevel {
				return LevelView{
					Level:           l.Level,
					Title:           l.Title,
					ProgressPercent: guestActiveProgress(activeDays),
					ActiveDays:      activeDays,
					GuestActiveDays: GuestActiveDaysRequired,
				}
			}
		}
		// 等级表缺少 Lv.0 时的兜底：仍按访客阶段展示最低等级与活跃日进度，
		// 避免活跃天数门槛被意外绕过。
		lowest := enabled[0]
		return LevelView{
			Level:           lowest.Level,
			Title:           lowest.Title,
			ProgressPercent: guestActiveProgress(activeDays),
			ActiveDays:      activeDays,
			GuestActiveDays: GuestActiveDaysRequired,
		}
	}

	cur := enabled[0]
	idx := 0
	for i, l := range enabled {
		if l.ThresholdXP <= xp {
			cur = l
			idx = i
		}
	}
	view := LevelView{
		Level: cur.Level, Title: cur.Title,
		ActiveDays: activeDays, GuestActiveDays: GuestActiveDaysRequired,
	}
	if cur.Level == GuestLevel {
		view.ProgressPercent = guestActiveProgress(activeDays)
	}
	if idx >= len(enabled)-1 {
		view.IsMax = true
		if cur.Level != GuestLevel {
			view.ProgressPercent = 100
		}
		return view
	}
	next := enabled[idx+1]
	span := next.ThresholdXP - cur.ThresholdXP
	view.NextThreshold = &next.ThresholdXP
	view.XPToNext = next.ThresholdXP - xp
	if view.XPToNext < 0 {
		view.XPToNext = 0
	}
	if cur.Level == GuestLevel {
		// Lv.0 的进度由活跃天数决定，而不是经验阈值。
		view.XPToNext = 0
		view.ProgressPercent = guestActiveProgress(activeDays)
	} else if span > 0 {
		view.ProgressPercent = int((xp - cur.ThresholdXP) * 100 / span)
		if view.ProgressPercent < 0 {
			view.ProgressPercent = 0
		}
		if view.ProgressPercent > 100 {
			view.ProgressPercent = 100
		}
	}
	return view
}

// computeLevelWithOverride 计算最终展示等级：人工覆盖优先于经验 / 活跃天数。
//
// 覆盖是固定值：一旦设置，后续经验变化不会改变展示等级，直到下一次人工调整。
// 覆盖值仍会被限制在当前启用等级范围内；等级定义删除 / 停用后取不超过覆盖值的
// 最高启用等级。覆盖期间不展示“距下一级”的经验进度。
func computeLevelWithOverride(levels []domain.GrowthLevel, xp int64, override *int, activeDays int) LevelView {
	if override == nil {
		return computeLevel(levels, xp, activeDays)
	}
	enabled := enabledLevels(levels)
	if len(enabled) == 0 {
		return LevelView{
			Level: GuestLevel, Title: GuestLevelTitle, IsMax: true, Overridden: true,
			ActiveDays: activeDays, GuestActiveDays: GuestActiveDaysRequired,
		}
	}

	target := *override
	if target < enabled[0].Level {
		target = enabled[0].Level
	}
	if target > enabled[len(enabled)-1].Level {
		target = enabled[len(enabled)-1].Level
	}

	cur := enabled[0]
	idx := 0
	for i, l := range enabled {
		if l.Level <= target {
			cur = l
			idx = i
		}
	}
	view := LevelView{
		Level: cur.Level, Title: cur.Title, Overridden: true,
		ActiveDays: activeDays, GuestActiveDays: GuestActiveDaysRequired,
	}
	if cur.Level == GuestLevel {
		view.ProgressPercent = guestActiveProgress(activeDays)
	} else {
		// 覆盖等级不参与经验自动升级，进度条固定显示满格，
		// 具体“人工覆盖”状态由 Overridden 字段告知前端。
		view.ProgressPercent = 100
	}
	if idx >= len(enabled)-1 {
		view.IsMax = true
	}
	return view
}

// effectiveLevel 返回最终展示等级（用于回写 user_growth.level）。
func effectiveLevel(levels []domain.GrowthLevel, xp int64, override *int, activeDays int) int {
	return computeLevelWithOverride(levels, xp, override, activeDays).Level
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

// levelTitle 返回指定等级对应的称号；找不到时回退为 Lv.N。
func levelTitle(levels []domain.GrowthLevel, level int) string {
	for _, l := range levels {
		if l.Level == level && l.Enabled {
			return l.Title
		}
	}
	return "Lv." + itoa(level)
}
