package handler

import (
	"testing"

	mcstructure "mckit/backend/internal/mcstructure"
)

// TestNewBiomeSamplerVersionGate 仅 1.18+ 构造群系采样器。
func TestNewBiomeSamplerVersionGate(t *testing.T) {
	if s := newBiomeSampler(1, mcstructure.Ver1_18Plus); s == nil {
		t.Fatal("1.18+ 应返回采样器")
	}
	for _, v := range []mcstructure.Version{mcstructure.Ver1_8_1_12, mcstructure.Ver1_13_1_15, mcstructure.Ver1_16_1_17} {
		if s := newBiomeSampler(1, v); s != nil {
			t.Fatalf("%s 不应返回采样器", v)
		}
	}
}

// TestStructurePointFeasible 候选点落在允许群系时标注 feasible=true。
func TestStructurePointFeasible(t *testing.T) {
	sampler := mcstructure.BiomeAtFunc(func(x, z int) string { return "plains" })
	pt := structurePoint(sampler, "village", mcstructure.Village, mcstructure.Pos{X: 0, Z: 0})
	if pt.Feasible == nil || !*pt.Feasible {
		t.Fatalf("村庄在平原应 feasible=true，实际 %+v", pt)
	}
	if pt.Biome != "plains" || pt.Label != "群系可行" {
		t.Fatalf("群系/标签不符: %+v", pt)
	}
}

// TestStructurePointInvalid 候选点及邻近均非允许群系时标注 feasible=false。
func TestStructurePointInvalid(t *testing.T) {
	sampler := mcstructure.BiomeAtFunc(func(x, z int) string { return "ocean" })
	pt := structurePoint(sampler, "village", mcstructure.Village, mcstructure.Pos{X: 0, Z: 0})
	if pt.Feasible == nil || *pt.Feasible {
		t.Fatalf("村庄在海洋应 feasible=false，实际 %+v", pt)
	}
	if pt.Biome != "ocean" {
		t.Fatalf("群系应为 ocean，实际 %q", pt.Biome)
	}
}

// TestStructurePointUnknown 无采样器时不做判定（feasible 缺省）。
func TestStructurePointUnknown(t *testing.T) {
	pt := structurePoint(nil, "village", mcstructure.Village, mcstructure.Pos{})
	if pt.Feasible != nil || pt.Biome != "" {
		t.Fatalf("无采样器应为未知，实际 %+v", pt)
	}
	if pt.Label != "理论候选位置" {
		t.Fatalf("标签应为理论候选位置，实际 %q", pt.Label)
	}
}

// TestNewStructurePosDTO 结构坐标 DTO 的可行性标注。
func TestNewStructurePosDTO(t *testing.T) {
	// 候选点海洋、正东 2 区块平原 → feasible=true 且带偏移。
	sampler := mcstructure.BiomeAtFunc(func(x, z int) string {
		if x >= 32 && x < 48 && z >= 0 && z < 16 {
			return "plains"
		}
		return "ocean"
	})
	dto := newStructurePosDTO(sampler, mcstructure.Village, mcstructure.Pos{X: 0, Z: 0})
	if dto.Feasible == nil || !*dto.Feasible {
		t.Fatalf("邻近存在平原应 feasible=true，实际 %+v", dto)
	}
	if dto.ShiftX != 32 || dto.ShiftZ != 0 {
		t.Fatalf("偏移应为 (32,0)，实际 (%d,%d)", dto.ShiftX, dto.ShiftZ)
	}
	if dto.Biome != "ocean" {
		t.Fatalf("候选点群系应为 ocean，实际 %q", dto.Biome)
	}

	// 无规则结构（矿井）不带可行性字段。
	plain := mcstructure.BiomeAtFunc(func(x, z int) string { return "plains" })
	if got := newStructurePosDTO(plain, mcstructure.Mineshaft, mcstructure.Pos{}); got.Feasible != nil || got.Biome != "" {
		t.Fatalf("矿井不应带可行性字段，实际 %+v", got)
	}
}
