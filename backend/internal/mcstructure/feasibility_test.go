package structure

import "testing"

// fakeSampler 由「区块坐标 → 群系」映射构造的测试采样器。
// 未命中时回退 defaultBiome。
type fakeSampler struct {
	byChunk      map[[2]int]string
	defaultBiome string
}

func (f fakeSampler) BiomeAt(blockX, blockZ int) string {
	// 对齐到区块（向下取整，正确处理负数）。
	cx := floorDiv(blockX, 16)
	cz := floorDiv(blockZ, 16)
	if b, ok := f.byChunk[[2]int{cx, cz}]; ok {
		return b
	}
	return f.defaultBiome
}

func TestCheckFeasibilityValid(t *testing.T) {
	s := fakeSampler{byChunk: map[[2]int]string{{0, 0}: "plains"}}
	got := CheckFeasibility(s, Village, Pos{X: 8, Z: 8})
	if got.Status != FeasibleValid || got.Biome != "plains" {
		t.Fatalf("期望 valid/plains，实际 %+v", got)
	}
}

func TestCheckFeasibilityInvalid(t *testing.T) {
	s := fakeSampler{defaultBiome: "ocean"}
	got := CheckFeasibility(s, Village, Pos{X: 8, Z: 8})
	if got.Status != FeasibleInvalid {
		t.Fatalf("期望 invalid，实际 %+v", got)
	}
	if got.Biome != "ocean" {
		t.Fatalf("期望候选点群系 ocean，实际 %q", got.Biome)
	}
}

func TestCheckFeasibilityShifted(t *testing.T) {
	// 候选点在 (0,0) 为海洋，正东 2 个区块处为平原 → shifted。
	s := fakeSampler{
		defaultBiome: "ocean",
		byChunk:      map[[2]int]string{{2, 0}: "plains"},
	}
	got := CheckFeasibility(s, Village, Pos{X: 0, Z: 0})
	if got.Status != FeasibleShifted {
		t.Fatalf("期望 shifted，实际 %+v", got)
	}
	if got.ShiftX != 32 || got.ShiftZ != 0 {
		t.Fatalf("期望偏移 (32,0)，实际 (%d,%d)", got.ShiftX, got.ShiftZ)
	}
}

func TestCheckFeasibilityUnknown(t *testing.T) {
	// 无规则的结构（矿井）与 nil 采样器都返回 unknown。
	s := fakeSampler{defaultBiome: "plains"}
	if got := CheckFeasibility(s, Mineshaft, Pos{}); got.Status != FeasibleUnknown {
		t.Fatalf("矿井应无规则，实际 %+v", got)
	}
	if got := CheckFeasibility(nil, Village, Pos{}); got.Status != FeasibleUnknown {
		t.Fatalf("nil 采样器应为 unknown，实际 %+v", got)
	}
}

func TestCheckFeasibilityAncientCity(t *testing.T) {
	s := fakeSampler{byChunk: map[[2]int]string{{0, 0}: "deep_dark"}}
	if got := CheckFeasibility(s, AncientCity, Pos{}); got.Status != FeasibleValid {
		t.Fatalf("远古城市在 deep_dark 应为 valid，实际 %+v", got)
	}
}

func TestHasBiomeRule(t *testing.T) {
	if !HasBiomeRule(Village) || !HasBiomeRule(OceanMonument) || !HasBiomeRule(BuriedTreasure) {
		t.Fatal("村庄/海底神殿/埋藏的宝藏应有群系规则")
	}
	if HasBiomeRule(Mineshaft) || HasBiomeRule(TrialChambers) || HasBiomeRule(EndCity) {
		t.Fatal("矿井/试炼厅/末地城不应有地表群系规则")
	}
}

// TestCheckFeasibilityBuriedTreasure 埋藏的宝藏只生成于海滩类群系。
func TestCheckFeasibilityBuriedTreasure(t *testing.T) {
	beach := fakeSampler{defaultBiome: "beach"}
	if got := CheckFeasibility(beach, BuriedTreasure, Pos{}); got.Status != FeasibleValid {
		t.Fatalf("海滩应为 valid，实际 %+v", got)
	}
	plain := fakeSampler{defaultBiome: "plains"}
	if got := CheckFeasibility(plain, BuriedTreasure, Pos{}); got.Status != FeasibleInvalid {
		t.Fatalf("平原应为 invalid，实际 %+v", got)
	}
}
