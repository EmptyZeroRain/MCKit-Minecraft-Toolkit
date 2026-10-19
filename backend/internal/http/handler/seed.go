package handler

import (
	"encoding/json"
	"net/http"

	"mckit/backend/internal/http/response"
	"mckit/backend/internal/mcbiome"
	mcstructure "mckit/backend/internal/mcstructure"
)

// seedStructureMap 前端结构名 → mcstructure 类型（理论候选坐标）。
// 仅纳入主世界结构用于地图标注；要塞与史莱姆区块另行计算。
// 结构可用性按所选版本过滤（见 AvailableIn）。
var seedStructureMap = []struct {
	front string
	mc    mcstructure.Type
}{
	{"village", mcstructure.Village},
	{"ancient_city", mcstructure.AncientCity},
	{"mansion", mcstructure.WoodlandMansion},
	{"monument", mcstructure.OceanMonument},
	{"outpost", mcstructure.PillagerOutpost},
	{"ruined_portal", mcstructure.RuinedPortal},
	{"end_city", mcstructure.EndCity},
	{"desert_pyramid", mcstructure.DesertPyramid},
	{"jungle_pyramid", mcstructure.JunglePyramid},
	{"igloo", mcstructure.Igloo},
	{"swamp_hut", mcstructure.SwampHut},
	{"ocean_ruin", mcstructure.OceanRuin},
	{"shipwreck", mcstructure.Shipwreck},
	{"trail_ruins", mcstructure.TrailRuins},
	{"trial_chambers", mcstructure.TrialChambers},
	// 埋藏的宝藏 spacing=1（每区块一个候选）且有 1% 频率门槛，逐个标注会
	// 在地图上产生大量噪声，故不纳入地图聚合；请在「结构理论候选坐标」中查询。
}

// biomeSample 单区块生物群系采样（供地图渲染群系底色）。
type biomeSample struct {
	Cx    int    `json:"cx"`
	Cz    int    `json:"cz"`
	Biome string `json:"biome"`
}

// newBiomeSampler 按版本区间构造主世界群系采样器。
// 仅 1.18+ 使用多噪声群系引擎；其余版本返回 nil（不做群系校验）。
func newBiomeSampler(seed int64, ver mcstructure.Version) mcstructure.BiomeSampler {
	if ver != mcstructure.Ver1_18Plus {
		return nil
	}
	return mcbiome.NewSampler(seed, mcbiome.VersionFromString(string(ver)))
}

// structurePoint 由理论候选坐标构造带群系可行性标注的结构点。
func structurePoint(sampler mcstructure.BiomeSampler, front string, t mcstructure.Type, p mcstructure.Pos) point {
	pt := point{Type: front, X: p.X, Z: p.Z}
	res := mcstructure.CheckFeasibility(sampler, t, p)
	switch res.Status {
	case mcstructure.FeasibleValid:
		pt.Label, pt.Biome = "群系可行", res.Biome
		pt.Feasible = boolPtr(true)
	case mcstructure.FeasibleShifted:
		pt.Label, pt.Biome = "群系可行（邻近偏移）", res.Biome
		pt.ShiftX, pt.ShiftZ = res.ShiftX, res.ShiftZ
		pt.Feasible = boolPtr(true)
	case mcstructure.FeasibleInvalid:
		pt.Label, pt.Biome = "群系不符", res.Biome
		pt.Feasible = boolPtr(false)
	default:
		pt.Label = "理论候选位置"
	}
	return pt
}

// boolPtr 返回布尔值指针（用于 JSON 中的三态字段）。
func boolPtr(b bool) *bool { return &b }

// point 种子分析中的结构/区块点。
type point struct {
	Type     string `json:"type"`
	X        int    `json:"x"`
	Z        int    `json:"z"`
	Label    string `json:"label,omitempty"`
	Biome    string `json:"biome,omitempty"`
	Feasible *bool  `json:"feasible,omitempty"`
	ShiftX   int    `json:"shift_x,omitempty"`
	ShiftZ   int    `json:"shift_z,omitempty"`
}

// SeedAnalyze POST /api/v1/seed/analyze
// 种子分析：返回世界种子、史莱姆区块、结构理论候选坐标与（可选）生物群系采样。
//
// 版本（version）：影响结构的可用性（如 ancient_city 仅 1.19+、trial_chambers 仅 1.21+）。
// 生物群系（include_biomes）：按 cubiomes 移植的 1.18+ 多噪声引擎采样，默认关闭。
func (h *MCHandler) SeedAnalyze(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Seed              string `json:"seed"`
		Version           string `json:"version"`
		Radius            int    `json:"radius"`
		IncludeBiomes     *bool  `json:"include_biomes"`
		IncludeStructures *bool  `json:"include_structures"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	seed, err := mcstructure.ParseSeed(req.Seed)
	if err != nil {
		response.Validation(w, r, []response.ErrorDetail{{Field: "seed", Message: err.Error()}})
		return
	}

	// 区块半径，限制规模防滥用（最大 32，即 65×65 区块）。
	radius := req.Radius
	if radius <= 0 {
		radius = 8
	}
	if radius > 32 {
		radius = 32
	}

	includeStructures := true
	if req.IncludeStructures != nil {
		includeStructures = *req.IncludeStructures
	}
	includeBiomes := false
	if req.IncludeBiomes != nil {
		includeBiomes = *req.IncludeBiomes
	}

	ver := mcstructure.NormalizeVersion(req.Version)

	structures := make([]point, 0)

	// 群系采样器仅在需要时构造：地图底色（includeBiomes）或 1.18+ 结构群系校验。
	var sampler mcstructure.BiomeSampler
	if includeBiomes || (includeStructures && ver == mcstructure.Ver1_18Plus) {
		sampler = newBiomeSampler(seed, ver)
	}

	if includeStructures {
		// 史莱姆区块（本地精确算法，坐标取区块中心）。
		for cx := -radius; cx <= radius; cx++ {
			for cz := -radius; cz <= radius; cz++ {
				if mcstructure.SlimeChunk(seed, int64(cx), int64(cz)) {
					structures = append(structures, point{Type: "slime_chunk", X: cx*16 + 8, Z: cz*16 + 8})
				}
			}
		}

		// 结构理论候选坐标；1.18+ 附群系可行性校验。
		// 按所选版本过滤：仅返回该版本已生成的结构。
		halfBlock := radius * 16
		for _, m := range seedStructureMap {
			if !mcstructure.AvailableIn(m.mc, ver) {
				continue
			}
			positions, err := mcstructure.PositionsInArea(seed, m.mc, halfBlock)
			if err != nil {
				continue
			}
			for _, p := range positions {
				structures = append(structures, structurePoint(sampler, m.front, m.mc, p))
			}
		}

		// 要塞（环形估算）。
		for i := 0; i < mcstructure.StrongholdCount; i++ {
			p := mcstructure.StrongholdPos(seed, i)
			if abs(p.X) <= halfBlock && abs(p.Z) <= halfBlock {
				structures = append(structures, point{Type: "stronghold", X: p.X, Z: p.Z, Label: "环内估算"})
			}
		}
	}

	resp := map[string]any{
		"world_seed": mcstructure.FormatSeed(seed),
		"radius":     radius,
		"version":    string(ver),
		"structures": structures,
		"note": "结构坐标为理论候选/估算值；1.18+ 已附群系可行性（feasible）标注，" +
			"invalid 表示邻近范围无合适群系；version 仅影响结构可用性",
	}

	if includeBiomes {
		// 生物群系采样：以区块中心（方块坐标）按 1.18+ 多噪声引擎求值。
		bs, ok := sampler.(*mcbiome.Sampler)
		if !ok {
			bs = mcbiome.NewSampler(seed, mcbiome.VersionFromString(req.Version))
		}
		biomes := make([]biomeSample, 0, (2*radius+1)*(2*radius+1))
		for cz := -radius; cz <= radius; cz++ {
			for cx := -radius; cx <= radius; cx++ {
				biomes = append(biomes, biomeSample{
					Cx:    cx,
					Cz:    cz,
					Biome: bs.BiomeAt(cx*16+8, cz*16+8),
				})
			}
		}
		resp["biomes"] = biomes
		resp["note"] = "结构坐标为理论候选/估算值；1.18+ 已附群系可行性（feasible）标注；" +
			"生物群系为 1.18+ 多噪声引擎在海平面附近采样（cubiomes 移植），version 仅影响结构可用性"
	}

	response.OK(w, resp)
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
