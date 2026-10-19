package structure

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"time"
)

// FindRequest 种子查找请求。
type FindRequest struct {
	// Structure 目标结构类型。
	Structure Type
	// CenterX/CenterZ 期望结构出现的中心坐标（方块）。
	CenterX int
	CenterZ int
	// Radius 允许的偏差半径（方块）。
	Radius int
	// Limit 最多返回的种子数量。
	Limit int
	// MaxAttempts 最大尝试次数（防止无限搜索）。
	MaxAttempts int
}

// FoundSeed 命中的种子及结构坐标。
type FoundSeed struct {
	Seed int64 `json:"seed"`
	X    int   `json:"x"`
	Z    int   `json:"z"`
	Dist float64 `json:"distance"`
}

// Find 搜索满足"指定结构出现在目标坐标附近"的随机种子。
//
// 实现说明：这是概率性搜索（随机采样种子并检查理论坐标）。
// 由于结构坐标由种子的低 48 位决定，随机采样在合理尝试次数内可命中。
// 真实生成仍需生物群系检查，这里基于理论坐标筛选。
func Find(ctx context.Context, req FindRequest) ([]FoundSeed, error) {
	if !IsKnown(req.Structure) {
		return nil, errUnknown(req.Structure)
	}
	if req.Radius <= 0 {
		req.Radius = 256
	}
	if req.Limit <= 0 || req.Limit > 50 {
		req.Limit = 10
	}
	if req.MaxAttempts <= 0 || req.MaxAttempts > 5_000_000 {
		req.MaxAttempts = 1_000_000
	}

	// 目标所在 region 网格（粗定位，取结构窗口内的 region）
	cfg := sets[req.Structure]
	targetRegionX := req.CenterX / (cfg.Spacing * 16)
	targetRegionZ := req.CenterZ / (cfg.Spacing * 16)

	var out []FoundSeed
	// 每次搜索使用不同的随机起点，避免返回相同结果。
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	for i := 0; i < req.MaxAttempts && len(out) < req.Limit; i++ {
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		default:
		}

		seed := rng.Int63()
		// 检查目标 region 及邻近 region，取该种子下最近的匹配点
		bestDist := math.Inf(1)
		var bestPos Pos
		matched := false
		for dx := -1; dx <= 1; dx++ {
			for dz := -1; dz <= 1; dz++ {
				p := positionFor(seed, cfg, targetRegionX+dx, targetRegionZ+dz)
				d := math.Hypot(float64(p.X-req.CenterX), float64(p.Z-req.CenterZ))
				if d <= float64(req.Radius) && d < bestDist {
					bestDist = d
					bestPos = p
					matched = true
				}
			}
		}
		if matched {
			out = append(out, FoundSeed{Seed: seed, X: bestPos.X, Z: bestPos.Z, Dist: bestDist})
			if len(out) >= req.Limit {
				return out, nil
			}
		}
	}
	if len(out) == 0 {
		return nil, errors.New("在尝试次数内未找到匹配种子，请增大半径或尝试次数")
	}
	return out, nil
}