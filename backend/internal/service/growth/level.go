package growth

import (
	"mckit/backend/internal/domain"
)

// LevelView 等级展示视图。
type LevelView struct {
	Level           int    `json:"level"`
	Title           string `json:"level_title"`
	NextThreshold   *int64 `json:"next_level_xp"`
	XPToNext        int64  `json:"xp_to_next_level"`
	ProgressPercent int    `json:"level_progress_percent"`
	IsMax           bool   `json:"is_max_level"`
}

// enabledLevels 过滤并返回启用的等级（阈值升序）。
func enabledLevels(levels []domain.GrowthLevel) []domain.GrowthLevel {
	out := make([]domain.GrowthLevel, 0, len(levels))
	for _, l := range levels {
		if l.Enabled {
			out = append(out, l)
		}
	}
	return out
}

// computeLevel 依据当前有效经验与等级阈值计算等级（jingyan.md §5）。
//
// 规则：取所有 threshold_xp <= xp 的最高等级；达到最高等级后进度显示为满级；
// 冲正导致经验下降时由同一函数重新计算（是否降级不由前端决定）。
func computeLevel(levels []domain.GrowthLevel, xp int64) LevelView {
	enabled := enabledLevels(levels)
	if len(enabled) == 0 {
		// 阈值表缺失时的兜底：始终至少 Lv.1。
		return LevelView{Level: 1, Title: "Lv.1", IsMax: true, ProgressPercent: 100}
	}
	// 阈值严格递增（数据库保证唯一且由后台校验递增）。
	cur := enabled[0]
	idx := 0
	for i, l := range enabled {
		if l.ThresholdXP <= xp {
			cur = l
			idx = i
		}
	}
	view := LevelView{Level: cur.Level, Title: cur.Title}
	if idx >= len(enabled)-1 {
		view.IsMax = true
		view.ProgressPercent = 100
		return view
	}
	next := enabled[idx+1]
	span := next.ThresholdXP - cur.ThresholdXP
	view.NextThreshold = &next.ThresholdXP
	view.XPToNext = next.ThresholdXP - xp
	if view.XPToNext < 0 {
		view.XPToNext = 0
	}
	if span > 0 {
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

// computeLevelWithOffset 在经验计算等级上叠加后台人工等级偏移。
//
// 偏移只改变展示等级，不改变经验；后续经验变化仍由 computeLevel 计算基础等级，
// 再叠加同一偏移，因此人工调整不会被下一次升级覆盖。等级始终限制在有效等级范围内。
func computeLevelWithOffset(levels []domain.GrowthLevel, xp int64, offset int) LevelView {
	enabled := enabledLevels(levels)
	if len(enabled) == 0 {
		return LevelView{Level: 1, Title: "Lv.1", IsMax: true, ProgressPercent: 100}
	}
	if offset == 0 {
		return computeLevel(levels, xp)
	}

	base := computeLevel(levels, xp)
	target := base.Level + offset
	minLevel, maxLevel := enabled[0].Level, enabled[len(enabled)-1].Level
	if target < minLevel {
		target = minLevel
	}
	if target > maxLevel {
		target = maxLevel
	}

	cur := enabled[0]
	idx := 0
	for i, l := range enabled {
		if l.Level <= target {
			cur = l
			idx = i
		}
	}
	view := LevelView{Level: cur.Level, Title: cur.Title}
	if idx >= len(enabled)-1 {
		view.IsMax = true
		view.ProgressPercent = 100
		return view
	}
	next := enabled[idx+1]
	span := next.ThresholdXP - cur.ThresholdXP
	view.NextThreshold = &next.ThresholdXP
	view.XPToNext = next.ThresholdXP - xp
	if view.XPToNext < 0 {
		view.XPToNext = 0
	}
	if span > 0 {
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

// effectiveLevel 返回叠加人工偏移后的有效等级（用于回写 user_growth.level）。
func effectiveLevel(levels []domain.GrowthLevel, xp int64, offset int) int {
	return computeLevelWithOffset(levels, xp, offset).Level
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
