package growth

import "mckit/backend/internal/domain"

// LevelView 等级计算结果（只读展示，不构成权限）。
type LevelView struct {
	Level           int    `json:"level"`
	Title           string `json:"level_title"`
	NextThreshold   *int64 `json:"next_level_xp"`
	XPToNext        int64  `json:"xp_to_next_level"`
	ProgressPercent int    `json:"level_progress_percent"`
	IsMax           bool   `json:"is_max_level"`
}

// computeLevel 依据当前有效经验与等级阈值计算等级（jingyan.md §5）。
//
// 规则：取所有 threshold_xp <= xp 的最高等级；达到最高等级后进度显示为满级；
// 冲正导致经验下降时由同一函数重新计算（是否降级不由前端决定）。
func computeLevel(levels []domain.GrowthLevel, xp int64) LevelView {
	enabled := make([]domain.GrowthLevel, 0, len(levels))
	for _, l := range levels {
		if l.Enabled {
			enabled = append(enabled, l)
		}
	}
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
