package structure

// 世界分析工具的版本支持范围梳理（todo §10 第 1、8 项）。
//
// 这里集中声明「每项能力在各版本区间是否可用」，供 API 与前端显式提示，
// 避免把只在部分版本成立的结果当成通用结论。所有「不支持 / 近似」的判断
// 都要求有公开依据；无依据的差异一律以前置说明标注「未核实」。

// FeatureSupport 单个功能在某一版本区间的支持情况。
type FeatureSupport struct {
	Version   Version `json:"version"`
	Supported bool    `json:"supported"`
	// Note 该版本区间下的额外说明（如算法差异、精度限制）。
	Note string `json:"note,omitempty"`
}

// FeatureCapability 单个工具能力的版本支持矩阵。
type FeatureCapability struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Supported 由旧到新覆盖全部版本区间（顺序与 AllVersions 一致）。
	Supported []FeatureSupport `json:"supported"`
	// Note 该能力的整体说明（数据依据与已知限制）。
	Note string `json:"note"`
}

// AllVersions 返回由旧到新的全部版本区间（与前端 SEED_VERSIONS 对齐）。
func AllVersions() []Version {
	return []Version{Ver1_8_1_12, Ver1_13_1_15, Ver1_16_1_17, Ver1_18Plus}
}

// buildSupport 按版本区间构造支持矩阵。decide 返回该区间的支持状态与说明。
func buildSupport(decide func(Version) (bool, string)) []FeatureSupport {
	versions := AllVersions()
	out := make([]FeatureSupport, 0, len(versions))
	for _, v := range versions {
		ok, note := decide(v)
		out = append(out, FeatureSupport{Version: v, Supported: ok, Note: note})
	}
	return out
}

// Capabilities 返回世界分析工具各能力的版本支持矩阵。
//
// 依据：
//   - 结构分布自 1.13 起使用 random_spread（spacing/separation + 偏移），1.12
//     及更早算法不同，故 1.12 及更早标为不支持（见 PlacementSupported）。
//   - 群系查询依赖 mcbiome 的 1.18+ 多噪声引擎移植，仅 1.18+ 可用。
//   - 史莱姆区块与要塞估算算法跨版本稳定；史莱姆区块不适用于基岩版。
//   - 种子搜索基于理论候选坐标的概率性搜索，非游戏内精确生成。
func Capabilities() []FeatureCapability {
	return []FeatureCapability{
		{
			ID:   "structures",
			Name: "结构理论候选坐标",
			Supported: buildSupport(func(v Version) (bool, string) {
				if !PlacementSupported(v) {
					return false, "1.12 及更早使用不同的结构分布算法，本实现结果不可作为依据（未核实）"
				}
				return true, "random_spread 分布；仍需按结构类型核对可用版本（见结构清单）"
			}),
			Note: "候选坐标由结构 set 的随机偏移决定；实际生成还要求落在允许的生物群系内，且部分结构有频率门槛（如掠夺者前哨站 20%、埋藏的宝藏 1%、废弃矿井 0.4%）。",
		},
		{
			ID:   "biome_query",
			Name: "生物群系查询",
			Supported: buildSupport(func(v Version) (bool, string) {
				if v != Ver1_18Plus {
					return false, "服务端为 1.18+ 多噪声引擎移植，其他版本暂不支持"
				}
				return true, "1.18+ 多噪声引擎，海平面附近地表采样"
			}),
			Note: "群系标识采用 cubiomes 词表；1.18+ 为多噪声决策树，1.17 及更早为旧算法，未实现。",
		},
		{
			ID:   "slime_chunks",
			Name: "史莱姆区块",
			Supported: buildSupport(func(v Version) (bool, string) {
				return true, "算法在 Java 版各版本一致"
			}),
			Note: "算法：Random(seed + 341873128712·cx + 132897987541·cz).nextInt(10)==0；仅 Java 版，基岩版算法不同。",
		},
		{
			ID:   "strongholds",
			Name: "要塞估算",
			Supported: buildSupport(func(v Version) (bool, string) {
				if v == Ver1_8_1_12 {
					return true, "1.8 仅 3 个要塞（3 环），1.9 起为 8 环共 128 个；本估算按 1.9+ 布局，1.8 结果仅供参考"
				}
				return true, "8 环共 128 个要塞，环内估算误差约 ±112 格"
			}),
			Note: "要塞采用 concentric_rings 分布（distance=32、count=128、spread=3）；精确落点需在 112 格方形范围做群系优选，本实现为环内估算。",
		},
		{
			ID:   "seed_find",
			Name: "种子搜索",
			Supported: buildSupport(func(v Version) (bool, string) {
				return true, "基于理论候选坐标的概率性搜索"
			}),
			Note: "随机采样种子并检查理论候选坐标，未做生物群系可行性筛选，命中结果仍需进游戏核实。",
		},
	}
}
