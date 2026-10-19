package mcbiome

import "strings"

// SurfaceQuartY 名义地表采样高度（quart）：方块 y≈60。
// 注意：地表地图采样会跳过深度项（sampleNoDepth），此时该值不参与计算。
const SurfaceQuartY = 15

// Sampler 持有一次种子初始化后的气候噪声，可重复采样多个坐标。
type Sampler struct {
	bn *biomeNoise
}

// NewSampler 初始化指定种子与版本的生物群系采样器。
func NewSampler(seed int64, v version) *Sampler {
	return &Sampler{bn: newBiomeNoise(seed, v)}
}

// BiomeAt 返回方块坐标 (blockX, blockZ) 处的地表生物群系标识。
//
// 坐标为方块坐标，内部按 1:4 取 quart 坐标。
// 采用 cubiomes SAMPLE_NO_DEPTH 语义（跳过深度项）：取地表群系，
// 避免固定高度采样在高地形/深海处误判为洞穴群系。
func (s *Sampler) BiomeAt(blockX, blockZ int) string {
	if s == nil || s.bn == nil {
		return ""
	}
	id := s.bn.sampleBiome(blockX>>2, SurfaceQuartY, blockZ>>2, sampleNoDepth)
	return BiomeName(id)
}

// BiomeName 返回 cubiomes/MC 生物群系标识；未知返回 "unknown"。
func BiomeName(id int) string {
	if n, ok := biomeNames[id]; ok {
		return n
	}
	return "unknown"
}

// biomeNames 生物群系 id → 标识（取自 cubiomes biomes.h 枚举，保持引擎自身词表）。
var biomeNames = map[int]string{
	0:   "ocean",
	1:   "plains",
	2:   "desert",
	3:   "mountains",
	4:   "forest",
	5:   "taiga",
	6:   "swamp",
	7:   "river",
	8:   "nether_wastes",
	9:   "the_end",
	10:  "frozen_ocean",
	11:  "frozen_river",
	12:  "snowy_tundra",
	13:  "snowy_mountains",
	14:  "mushroom_fields",
	15:  "mushroom_field_shore",
	16:  "beach",
	17:  "desert_hills",
	18:  "wooded_hills",
	19:  "taiga_hills",
	20:  "mountain_edge",
	21:  "jungle",
	22:  "jungle_hills",
	23:  "jungle_edge",
	24:  "deep_ocean",
	25:  "stone_shore",
	26:  "snowy_beach",
	27:  "birch_forest",
	28:  "birch_forest_hills",
	29:  "dark_forest",
	30:  "snowy_taiga",
	31:  "snowy_taiga_hills",
	32:  "giant_tree_taiga",
	33:  "giant_tree_taiga_hills",
	34:  "wooded_mountains",
	35:  "savanna",
	36:  "savanna_plateau",
	37:  "badlands",
	38:  "wooded_badlands_plateau",
	39:  "badlands_plateau",
	40:  "small_end_islands",
	41:  "end_midlands",
	42:  "end_highlands",
	43:  "end_barrens",
	44:  "warm_ocean",
	45:  "lukewarm_ocean",
	46:  "cold_ocean",
	47:  "deep_warm_ocean",
	48:  "deep_lukewarm_ocean",
	49:  "deep_cold_ocean",
	50:  "deep_frozen_ocean",
	51:  "seasonal_forest",
	52:  "rainforest",
	53:  "shrubland",
	127: "the_void",
	129: "sunflower_plains",
	130: "desert_lakes",
	131: "gravelly_mountains",
	132: "flower_forest",
	133: "taiga_mountains",
	134: "swamp_hills",
	140: "ice_spikes",
	149: "modified_jungle",
	151: "modified_jungle_edge",
	155: "tall_birch_forest",
	156: "tall_birch_hills",
	157: "dark_forest_hills",
	158: "snowy_taiga_mountains",
	160: "giant_spruce_taiga",
	161: "giant_spruce_taiga_hills",
	162: "modified_gravelly_mountains",
	163: "shattered_savanna",
	164: "shattered_savanna_plateau",
	165: "eroded_badlands",
	166: "modified_wooded_badlands_plateau",
	167: "modified_badlands_plateau",
	168: "bamboo_jungle",
	169: "bamboo_jungle_hills",
	170: "soul_sand_valley",
	171: "crimson_forest",
	172: "warped_forest",
	173: "basalt_deltas",
	174: "dripstone_caves",
	175: "lush_caves",
	177: "meadow",
	178: "grove",
	179: "snowy_slopes",
	180: "jagged_peaks",
	181: "frozen_peaks",
	182: "stony_peaks",
	183: "deep_dark",
	184: "mangrove_swamp",
	185: "cherry_grove",
	186: "pale_garden",
}

// VersionFromString 将前端版本区间或具体版本解析为内部决策树版本。
//
// 说明：前端提供的 "1.18+" 是粗粒度区间，无法区分 1.18 与 1.21 的细微差异，
// 这里统一按最新支持版本（1.21）的决策树采样；也可传入具体版本（如 "1.19.4"）。
func VersionFromString(s string) version {
	v := strings.ToLower(strings.TrimSpace(s))
	switch {
	case v == "" || v == "1.18+" || v == "latest":
		return v1_21
	case strings.HasPrefix(v, "1.18"):
		return v1_18
	case strings.HasPrefix(v, "1.19.2"), strings.HasPrefix(v, "1.19.3"):
		return v1_19_2
	case strings.HasPrefix(v, "1.19"):
		return v1_19_4
	case strings.HasPrefix(v, "1.20"):
		return v1_20_6
	default:
		return v1_21
	}
}
