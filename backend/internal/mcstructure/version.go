package structure

// Version 前端提供的游戏版本区间（与 frontend/src/types/seed.ts 的 SEED_VERSIONS 对齐）。
type Version string

// 版本区间常量（由旧到新）。
const (
	Ver1_8_1_12  Version = "1.8-1.12"
	Ver1_13_1_15 Version = "1.13-1.15"
	Ver1_16_1_17 Version = "1.16-1.17"
	Ver1_18Plus  Version = "1.18+"
)

// rank 版本区间的序（越大越新）；未知返回最新，保证向后兼容。
func (v Version) rank() int {
	switch v {
	case Ver1_8_1_12:
		return 0
	case Ver1_13_1_15:
		return 1
	case Ver1_16_1_17:
		return 2
	case Ver1_18Plus:
		return 3
	default:
		return 3
	}
}

// NormalizeVersion 将任意输入归一化为已知版本区间；空值默认最新。
func NormalizeVersion(s string) Version {
	switch Version(s) {
	case Ver1_8_1_12, Ver1_13_1_15, Ver1_16_1_17, Ver1_18Plus:
		return Version(s)
	default:
		return Ver1_18Plus
	}
}

// since 记录每种结构“开始生成”的最低版本区间。
// 依据 Minecraft Wiki 各结构首次加入版本（按前端粗粒度区间归类）。
var since = map[Type]Version{
	// 远古结构（1.12 及更早）
	Mineshaft:       Ver1_8_1_12,
	DesertPyramid:   Ver1_8_1_12,
	JunglePyramid:   Ver1_8_1_12,
	Igloo:           Ver1_8_1_12,
	Village:         Ver1_8_1_12,
	OceanMonument:   Ver1_8_1_12,
	SwampHut:        Ver1_8_1_12,
	NetherFortress:  Ver1_8_1_12,
	WoodlandMansion: Ver1_8_1_12, // 1.11
	EndCity:         Ver1_8_1_12, // 1.9
	// 1.13
	OceanRuin:      Ver1_13_1_15,
	Shipwreck:      Ver1_13_1_15,
	BuriedTreasure: Ver1_13_1_15, // 1.13 加入
	// 1.14
	PillagerOutpost: Ver1_13_1_15,
	// 1.16
	RuinedPortal: Ver1_16_1_17,
	Bastion:      Ver1_16_1_17,
	NetherFossil: Ver1_16_1_17,
	// 1.19+
	AncientCity:   Ver1_18Plus,
	TrailRuins:    Ver1_18Plus, // 1.20
	TrialChambers: Ver1_18Plus, // 1.21
}

// SinceVersion 返回结构「开始生成」的最低版本区间；未登记返回最早区间。
func SinceVersion(t Type) Version {
	if s, ok := since[t]; ok {
		return s
	}
	return Ver1_8_1_12
}

// AvailableIn 判断结构是否在指定版本区间生成。
// 未登记的结构视为全版本可用（保守：宁可多列，不误删）。
func AvailableIn(t Type, v Version) bool {
	s, ok := since[t]
	if !ok {
		return true
	}
	return v.rank() >= s.rank()
}
