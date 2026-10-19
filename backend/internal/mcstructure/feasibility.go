package structure

// 结构群系可行性校验。
//
// 背景：Position/PositionsInArea 给出的是「理论候选坐标」——由结构 set 的随机偏移决定，
// 与实际生成无关。真实游戏中，结构还必须落在允许的生物群系内，否则该候选点不会生成结构
// （村庄、海底神殿、林地府邸等对群系有硬性要求）。
//
// 本文件基于 Minecraft Wiki 的结构群系约束，对候选点做一次可行性判定：
//   - valid   ：候选点群系满足要求；
//   - shifted ：候选点不满足，但邻近区块存在可行群系（实际结构可能略微偏移）；
//   - invalid ：邻近搜索范围内均无可行群系，该候选点基本不会生成；
//   - unknown ：无群系约束（要塞/矿井/试炼厅/下界/末地等）或未提供采样器。
//
// 说明：这是对真实生成算法的近似，用于把「理论候选」升级为「群系可行/不符」的参考信号，
// 不保证与实际游戏逐格一致。

// BiomeSampler 抽象生物群系采样，避免与具体引擎（mcbiome）强耦合，便于测试。
type BiomeSampler interface {
	// BiomeAt 返回方块坐标 (blockX, blockZ) 处的地表群系标识。
	BiomeAt(blockX, blockZ int) string
}

// BiomeAtFunc 将函数适配为 BiomeSampler。
type BiomeAtFunc func(blockX, blockZ int) string

// BiomeAt 实现 BiomeSampler。
func (f BiomeAtFunc) BiomeAt(blockX, blockZ int) string { return f(blockX, blockZ) }

// FeasibilityStatus 群系可行性状态。
type FeasibilityStatus string

const (
	// FeasibleValid 候选点群系满足结构要求。
	FeasibleValid FeasibilityStatus = "valid"
	// FeasibleShifted 候选点不满足，但邻近区块存在可行群系。
	FeasibleShifted FeasibilityStatus = "shifted"
	// FeasibleInvalid 邻近范围内均无可行群系。
	FeasibleInvalid FeasibilityStatus = "invalid"
	// FeasibleUnknown 无群系约束或未提供采样器。
	FeasibleUnknown FeasibilityStatus = "unknown"
)

// FeasibilityResult 单个候选点的群系可行性结果。
type FeasibilityResult struct {
	Status FeasibilityStatus `json:"status"`
	// Biome 候选点自身的群系标识。
	Biome string `json:"biome,omitempty"`
	// ShiftX/ShiftZ 命中邻近可行群系时的偏移（方块）；仅 shifted 有意义。
	ShiftX int `json:"shift_x,omitempty"`
	ShiftZ int `json:"shift_z,omitempty"`
}

// biomeRule 单个结构的群系约束。
type biomeRule struct {
	// allowed 允许的群系标识集合（词表与 mcbiome/biomeNames 一致，采用 cubiomes 命名）。
	allowed map[string]bool
	// searchChunks 邻近搜索半径（区块）；<=0 时取默认 2。
	searchChunks int
}

// defaultSearchChunks 默认邻近搜索半径（区块）。
const defaultSearchChunks = 2

// biomeRules 结构群系约束表。
//
// 数据依据 Minecraft Wiki 各结构的「Generating biomes」；采用 cubiomes 群系命名
// （如 snowy_plains 记为 snowy_tundra、old_growth_* 记为 giant_*）。
// 未登记的结构视为无群系约束（unknown）。
var biomeRules = map[Type]biomeRule{
	// 村庄：平原/沙漠/热带草原/针叶林/雪原等。
	Village: {allowed: set(
		"plains", "sunflower_plains", "meadow", "desert",
		"savanna", "savanna_plateau", "shattered_savanna",
		"taiga", "snowy_tundra", "snowy_taiga",
	)},
	// 海底神殿：任意海洋群系。
	OceanMonument: {allowed: set(
		"ocean", "deep_ocean", "warm_ocean", "lukewarm_ocean",
		"cold_ocean", "frozen_ocean",
		"deep_warm_ocean", "deep_lukewarm_ocean", "deep_cold_ocean", "deep_frozen_ocean",
	)},
	// 林地府邸：黑森林。
	WoodlandMansion: {allowed: set("dark_forest", "dark_forest_hills")},
	// 掠夺者前哨站：平原/沙漠/热带草原/针叶林/雪原及部分山地。
	PillagerOutpost: {allowed: set(
		"plains", "sunflower_plains", "desert",
		"savanna", "savanna_plateau", "shattered_savanna",
		"taiga", "snowy_tundra", "snowy_taiga",
		"meadow", "cherry_grove", "snowy_slopes", "grove",
	)},
	// 远古城市：深邃洞穴。
	AncientCity: {allowed: set("deep_dark")},
	// 沙漠神殿：沙漠。
	DesertPyramid: {allowed: set("desert")},
	// 丛林神庙：丛林类。
	JunglePyramid: {allowed: set(
		"jungle", "jungle_hills", "modified_jungle", "bamboo_jungle",
	)},
	// 雪屋：雪原/积雪针叶林。
	Igloo: {allowed: set("snowy_tundra", "snowy_taiga")},
	// 沼泽小屋：沼泽。
	SwampHut: {allowed: set("swamp", "swamp_hills")},
	// 海底废墟：任意海洋群系。
	OceanRuin: {allowed: set(
		"ocean", "deep_ocean", "warm_ocean", "lukewarm_ocean",
		"cold_ocean", "frozen_ocean",
		"deep_warm_ocean", "deep_lukewarm_ocean", "deep_cold_ocean", "deep_frozen_ocean",
	)},
	// 沉船：海洋与海滩。
	Shipwreck: {allowed: set(
		"ocean", "deep_ocean", "warm_ocean", "lukewarm_ocean",
		"cold_ocean", "frozen_ocean",
		"deep_warm_ocean", "deep_lukewarm_ocean", "deep_cold_ocean", "deep_frozen_ocean",
		"beach", "snowy_beach", "stone_shore",
	)},
	// 埋藏的宝藏：海滩类（wiki: beach / snowy beach / stone shore）。
	BuriedTreasure: {allowed: set("beach", "snowy_beach", "stone_shore")},
	// 古迹废墟：针叶林/白桦/丛林等。
	TrailRuins: {allowed: set(
		"taiga", "snowy_taiga",
		"giant_tree_taiga", "giant_tree_taiga_hills",
		"giant_spruce_taiga", "giant_spruce_taiga_hills",
		"birch_forest", "tall_birch_forest",
		"jungle", "bamboo_jungle",
	)},
	// 废弃传送门、矿井、试炼厅、要塞、下界/末地结构：无地表群系约束。
}

// set 由标识列表构造集合。
func set(names ...string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

// HasBiomeRule 判断结构是否有地表群系约束。
func HasBiomeRule(t Type) bool {
	_, ok := biomeRules[t]
	return ok
}

// CheckFeasibility 在候选坐标处做群系可行性校验。
// sampler 为 nil 或无对应规则时返回 FeasibleUnknown。
func CheckFeasibility(sampler BiomeSampler, t Type, pos Pos) FeasibilityResult {
	rule, ok := biomeRules[t]
	if !ok || sampler == nil {
		return FeasibilityResult{Status: FeasibleUnknown}
	}

	biome := sampler.BiomeAt(pos.X, pos.Z)
	if rule.allowed[biome] {
		return FeasibilityResult{Status: FeasibleValid, Biome: biome}
	}

	// 邻近搜索：由近及远，按区块步进（16 方块）。
	radius := rule.searchChunks
	if radius <= 0 {
		radius = defaultSearchChunks
	}
	bestX, bestZ, bestDist := 0, 0, -1
	for dcx := -radius; dcx <= radius; dcx++ {
		for dcz := -radius; dcz <= radius; dcz++ {
			if dcx == 0 && dcz == 0 {
				continue
			}
			ox, oz := dcx*16, dcz*16
			if !rule.allowed[sampler.BiomeAt(pos.X+ox, pos.Z+oz)] {
				continue
			}
			d := dcx*dcx + dcz*dcz
			if bestDist < 0 || d < bestDist {
				bestDist, bestX, bestZ = d, ox, oz
			}
		}
	}
	if bestDist >= 0 {
		return FeasibilityResult{
			Status: FeasibleShifted, Biome: biome, ShiftX: bestX, ShiftZ: bestZ,
		}
	}
	return FeasibilityResult{Status: FeasibleInvalid, Biome: biome}
}
