// Package structure 实现 Minecraft 结构的理论坐标计算（Java 版）。
// 算法依据社区逆向工程成果（cubiomes）与 Minecraft Wiki Structure set 数据：
//   - 候选窗口 w = spacing - separation
//   - 每轴偏移由 Java Random 基于 S = W + 341873128712·Rx + 132897987541·Rz + salt 产生
//   - uniform 结构 offset = nextInt(w)；非 uniform 结构 offset = (nextInt(w)+nextInt(w))/2
//   - 候选区块 cx = spacing·Rx + offset，坐标 = 区块×16
//
// 注意：此实现给出"理论候选坐标"，实际生成还依赖生物群系可行性检查；
// 对于村庄、海底神殿等要求特定群系的结构，最终落点可能偏移或不存在。
package structure

import (
	"math"
	"sort"

	"mckit/backend/internal/mcrand"
)

// Type 结构类型。
type Type string

// 结构类型常量（依据 wiki Structure set 默认表与用户需求清单）。
const (
	AncientCity     Type = "ancient_city"
	BuriedTreasure  Type = "buried_treasure"
	DesertPyramid   Type = "desert_pyramid"
	EndCity         Type = "end_city"
	Igloo           Type = "igloo"
	JunglePyramid   Type = "jungle_pyramid"
	Mineshaft       Type = "mineshaft"
	NetherFortress  Type = "nether_fortress"
	Bastion         Type = "bastion_remnant"
	NetherFossil    Type = "nether_fossil"
	OceanRuin       Type = "ocean_ruin"
	PillagerOutpost Type = "pillager_outpost"
	RuinedPortal    Type = "ruined_portal"
	Shipwreck       Type = "shipwreck"
	SwampHut        Type = "swamp_hut"
	TrailRuins      Type = "trail_ruins"
	TrialChambers   Type = "trial_chambers"
	Village         Type = "village"
	WoodlandMansion Type = "woodland_mansion"
	OceanMonument   Type = "ocean_monument"
	// 特殊：要塞（环形分布）、史莱姆区块（独立算法）、末地传送门（岛屿附近）
)

// Dimension 结构所在维度。候选坐标为该维度内的方块坐标。
type Dimension string

const (
	// OverworldDimension 主世界。
	OverworldDimension Dimension = "overworld"
	// NetherDimension 下界。
	NetherDimension Dimension = "nether"
	// EndDimension 末地。
	EndDimension Dimension = "end"
)

// StructureSet 单个结构的生成参数（数据来源 wiki Structure set）。
type StructureSet struct {
	Spacing      int       // spacing（平均间距，区块）
	Separation   int       // separation（最小间隔，区块）
	Salt         int64     // salt（随机盐）
	Uniform      bool      // true=单次 nextInt；false=三角分布（两次 nextInt 取平均）
	Dimension    Dimension // 所在维度；空值视为主世界
	// Probability 生成尝试频率上限（wiki Structure set frequency 字段），
	// 0 表示未登记（按 1.0 处理）。实际是否生成还取决于生物群系与排除区，
	// 本实现只输出频率供前端提示，不在候选坐标中模拟该概率门槛。
	Probability float64
}

// sets 结构参数表。数据来源：
// https://minecraft.wiki/w/Structure_set (Java Edition 默认 structure set)
var sets = map[Type]StructureSet{
	AncientCity:   {Spacing: 24, Separation: 8, Salt: 20083232},
	BuriedTreasure: {
		Spacing: 1, Separation: 0, Salt: 0, Uniform: true,
		// 1% 概率门槛（wiki: buried_treasures frequency 注「Probability of 1%」）
		Probability: 0.01,
	},
	DesertPyramid:   {Spacing: 32, Separation: 8, Salt: 14357617},
	EndCity:         {Spacing: 20, Separation: 11, Salt: 10387313, Dimension: EndDimension},
	Igloo:           {Spacing: 32, Separation: 8, Salt: 14357618},
	JunglePyramid:   {Spacing: 32, Separation: 8, Salt: 14357619},
	Mineshaft:       {Spacing: 1, Separation: 0, Salt: 0, Uniform: true, Probability: 0.004}, // 0.4% 概率
	NetherFortress:  {Spacing: 27, Separation: 4, Salt: 30084232, Uniform: true, Dimension: NetherDimension},
	Bastion:         {Spacing: 27, Separation: 4, Salt: 30084232, Uniform: true, Dimension: NetherDimension},
	NetherFossil:    {Spacing: 2, Separation: 1, Salt: 14357921, Dimension: NetherDimension},
	OceanRuin:       {Spacing: 20, Separation: 8, Salt: 14357621},
	PillagerOutpost: {Spacing: 32, Separation: 8, Salt: 165745296, Probability: 0.2}, // 20% 概率 + 村庄排除区
	RuinedPortal:    {Spacing: 40, Separation: 15, Salt: 34222645},
	Shipwreck:       {Spacing: 24, Separation: 4, Salt: 165745295},
	SwampHut:        {Spacing: 32, Separation: 8, Salt: 14357620},
	TrailRuins:      {Spacing: 34, Separation: 8, Salt: 83469867},
	TrialChambers:   {Spacing: 34, Separation: 12, Salt: 94251327},
	Village:         {Spacing: 34, Separation: 8, Salt: 10387312},
	WoodlandMansion: {Spacing: 80, Separation: 20, Salt: 10387319},
	OceanMonument:   {Spacing: 32, Separation: 5, Salt: 10387313, Uniform: true},
}

// Pos 结构坐标（方块）。
type Pos struct {
	X int `json:"x"`
	Z int `json:"z"`
}

// Config 结构查询配置。
type Config struct {
	Seed      int64  `json:"seed"`
	Type      Type   `json:"type"`
	RegionX   int    `json:"region_x"`
	RegionZ   int    `json:"region_z"`
	ModifierX int64  `json:"-"`
	ModifierZ int64  `json:"-"`
}

// IsKnown 判断结构类型是否有参数表。
func IsKnown(t Type) bool {
	_, ok := sets[t]
	return ok
}

// ListKnown 返回所有已知结构类型（按类型名升序，保证返回顺序稳定）。
func ListKnown() []Type {
	types := make([]Type, 0, len(sets))
	for t := range sets {
		types = append(types, t)
	}
	sort.Slice(types, func(i, j int) bool { return types[i] < types[j] })
	return types
}

// Descriptor 结构的完整参数描述（供 API 展示版本、维度与约束）。
type Descriptor struct {
	Type         Type      `json:"type"`
	Spacing      int       `json:"spacing"`
	Separation   int       `json:"separation"`
	Salt         int64     `json:"salt"`
	Uniform      bool      `json:"uniform"`
	Dimension    Dimension `json:"dimension"`
	Probability  float64   `json:"probability"`
	HasBiomeRule bool      `json:"has_biome_rule"`
	SinceVersion Version   `json:"since_version"`
}

// DimensionOf 返回结构所在维度（未登记视为主世界）。
func DimensionOf(t Type) Dimension {
	if cfg, ok := sets[t]; ok && cfg.Dimension != "" {
		return cfg.Dimension
	}
	return OverworldDimension
}

// ProbabilityOf 返回结构生成尝试频率上限；未登记返回 1.0。
func ProbabilityOf(t Type) float64 {
	if cfg, ok := sets[t]; ok && cfg.Probability > 0 {
		return cfg.Probability
	}
	return 1.0
}

// PlacementSupported 判断该版本区间的结构分布算法是否与当前实现一致。
//
// 背景：当前的 random_spread 分布（spacing/separation + uniform/三角偏移）
// 自 1.13 起启用；1.12 及更早使用另一套结构分布算法，本实现套用 1.13+ 公式
// 得到的候选坐标不能作为依据（未与游戏对照核实）。
func PlacementSupported(v Version) bool {
	return v.rank() >= Ver1_13_1_15.rank()
}

// Describe 返回结构参数描述；未知结构返回 false。
func Describe(t Type) (Descriptor, bool) {
	cfg, ok := sets[t]
	if !ok {
		return Descriptor{}, false
	}
	return Descriptor{
		Type:         t,
		Spacing:      cfg.Spacing,
		Separation:   cfg.Separation,
		Salt:         cfg.Salt,
		Uniform:      cfg.Uniform,
		Dimension:    DimensionOf(t),
		Probability:  ProbabilityOf(t),
		HasBiomeRule: HasBiomeRule(t),
		SinceVersion: SinceVersion(t),
	}, true
}

// DescribeAll 返回所有已知结构的参数描述（按类型名升序，顺序稳定）。
func DescribeAll() []Descriptor {
	out := make([]Descriptor, 0, len(sets))
	for _, t := range ListKnown() {
		d, _ := Describe(t)
		out = append(out, d)
	}
	return out
}

// Position 计算指定 region 网格内的结构理论候选坐标（方块坐标）。
// region 坐标是结构网格单元索引（非区块索引）。
func Position(seed int64, t Type, regionX, regionZ int) (Pos, error) {
	cfg, ok := sets[t]
	if !ok {
		return Pos{}, errUnknown(t)
	}
	return positionFor(seed, cfg, regionX, regionZ), nil
}

// positionFor 核心算法。
func positionFor(seed int64, cfg StructureSet, regionX, regionZ int) Pos {
	window := cfg.Spacing - cfg.Separation

	// Random 种子：S = W + 341873128712·Rx + 132897987541·Rz + σ
	randomSeed := seed + int64(regionX)*341873128712 + int64(regionZ)*132897987541 + cfg.Salt
	r := mcrand.New(randomSeed)

	var offX, offZ int
	if cfg.Uniform {
		offX = r.NextInt(window)
		offZ = r.NextInt(window)
	} else {
		// 三角分布：两次 nextInt 取平均，偏向窗口中部
		offX = (r.NextInt(window) + r.NextInt(window)) / 2
		offZ = (r.NextInt(window) + r.NextInt(window)) / 2
	}

	// 候选区块坐标
	chunkX := cfg.Spacing*regionX + offX
	chunkZ := cfg.Spacing*regionZ + offZ

	return Pos{X: chunkX * 16, Z: chunkZ * 16}
}

// PositionMulti 计算一段 region 范围内的所有候选坐标（步进为 spacing）。
func PositionMulti(seed int64, t Type, startX, startZ, countX, countZ int) ([]Pos, error) {
	cfg, ok := sets[t]
	if !ok {
		return nil, errUnknown(t)
	}
	var out []Pos
	for rx := startX; rx < startX+countX; rx++ {
		for rz := startZ; rz < startZ+countZ; rz++ {
			out = append(out, positionFor(seed, cfg, rx, rz))
		}
	}
	return out, nil
}

// PositionsInArea 返回以原点为中心、半径 halfBlock（方块）正方形区域内
// 该结构的全部理论候选坐标。用于种子分析等按区域聚合的场景。
func PositionsInArea(seed int64, t Type, halfBlock int) ([]Pos, error) {
	cfg, ok := sets[t]
	if !ok {
		return nil, errUnknown(t)
	}
	if halfBlock < 0 {
		halfBlock = -halfBlock
	}
	cell := cfg.Spacing * 16
	if cell <= 0 {
		cell = 16
	}
	minR := floorDiv(-halfBlock, cell) - 1
	maxR := floorDiv(halfBlock, cell) + 1

	var out []Pos
	for rx := minR; rx <= maxR; rx++ {
		for rz := minR; rz <= maxR; rz++ {
			p := positionFor(seed, cfg, rx, rz)
			if absInt(p.X) <= halfBlock && absInt(p.Z) <= halfBlock {
				out = append(out, p)
			}
		}
	}
	return out, nil
}

// floorDiv 向下取整的整数除法（正确处理负数）。
func floorDiv(a, b int) int {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

// absInt 整数绝对值。
func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// strongholdRing 要塞环参数（数据来源：Minecraft Wiki Stronghold Generation）。
type strongholdRing struct {
	Count int
	MinR  float64
	MaxR  float64
}

// 8 环共 128 个要塞；环内数量为三角数（最外环封顶 9）。
var strongholdRings = []strongholdRing{
	{Count: 3, MinR: 1280, MaxR: 2816},
	{Count: 6, MinR: 4352, MaxR: 5888},
	{Count: 10, MinR: 7424, MaxR: 8960},
	{Count: 15, MinR: 10496, MaxR: 12032},
	{Count: 21, MinR: 13568, MaxR: 15104},
	{Count: 28, MinR: 16640, MaxR: 18176},
	{Count: 36, MinR: 19712, MaxR: 21248},
	{Count: 9, MinR: 22784, MaxR: 24320},
}

// StrongholdCount 要塞总数。
const StrongholdCount = 128

// StrongholdPos 估算第 index 个要塞（0 起）的理论坐标。
//
// 说明：要塞位置为分布环内的估算（误差可达约 ±112 格），
// 精确落点需要完整生物群系可行性检查（与 cubiomes 的 estimated position 语义一致）。
// 坐标与游戏实际值可能有偏差，仅供定位参考。
func StrongholdPos(seed int64, index int) Pos {
	if index < 0 {
		index = 0
	}
	if index >= StrongholdCount {
		index = StrongholdCount - 1
	}

	// 定位所在环与环内序号
	ring := 0
	remaining := index
	for ring < len(strongholdRings) {
		if remaining < strongholdRings[ring].Count {
			break
		}
		remaining -= strongholdRings[ring].Count
		ring++
	}
	if ring >= len(strongholdRings) {
		ring = len(strongholdRings) - 1
		remaining = strongholdRings[ring].Count - 1
	}
	info := strongholdRings[ring]

	// 用世界种子驱动的随机序列生成角度扰动与半径。
	r := mcrand.New(seed + int64(index))

	// 环内等角分布 + 随机扰动（同一环内要塞大致等角）。
	baseAngle := 2 * math.Pi * float64(remaining) / float64(info.Count)
	step := 2 * math.Pi / float64(info.Count)
	jitter := (r.NextDouble() - 0.5) * step
	angle := baseAngle + jitter

	radius := info.MinR + r.NextDouble()*(info.MaxR-info.MinR)

	x := int(math.Round(math.Cos(angle) * radius))
	z := int(math.Round(math.Sin(angle) * radius))
	return Pos{X: x, Z: z}
}

// SlimeChunk 判断指定区块是否为史莱姆区块（Java 版）。
// 算法：Random(seed + 341873128712·cx + 132897987541·cz).nextInt(10) == 0
func SlimeChunk(seed int64, chunkX, chunkZ int64) bool {
	randomSeed := seed + 341873128712*chunkX + 132897987541*chunkZ
	r := mcrand.New(randomSeed)
	return r.NextInt(10) == 0
}

// errUnknown 未知结构类型错误。
type errUnknown string

func (e errUnknown) Error() string { return "未知结构类型: " + string(e) }