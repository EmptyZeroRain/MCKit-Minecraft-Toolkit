package structure

import "testing"

// knownSeedPositions 已知种子对照基线（todo §10 第 7 项）。
//
// 期望值由本实现当前算法（Java Random 48 位 LCG 复刻，与 cubiomes 同源公式）
// 在固定输入下产出，用于锁定行为、防止后续改动造成坐标漂移。
//
// **重要限制**：这些数值尚未与官方游戏或 cubiomes 输出逐格比对，因此只能作为
// 「防回归基线」，不能作为「与游戏生成一致」的证据。如发现与实测不符，应优先
// 核对数据来源并更新本表。
var knownSeedPositions = []struct {
	seed   int64
	typ    Type
	rx, rz int
	wantX  int
	wantZ  int
}{
	{123456789, Village, 0, 0, 352, 80},
	{123456789, DesertPyramid, 2, -3, 1312, -1360},
	{123456789, OceanMonument, -1, 1, -112, 688},
	{0, WoodlandMansion, 1, 1, 1600, 2016},
	{-1, TrialChambers, 3, 4, 1696, 2272},
	{3055141959546, Village, 5, 6, 2896, 3456},
	{42, BuriedTreasure, 10, -7, 160, -112},
	{42, NetherFortress, 4, 4, 1968, 2080},
}

func TestKnownSeedPositionBaseline(t *testing.T) {
	for _, c := range knownSeedPositions {
		p, err := Position(c.seed, c.typ, c.rx, c.rz)
		if err != nil {
			t.Fatalf("%s region(%d,%d): %v", c.typ, c.rx, c.rz, err)
		}
		if p.X != c.wantX || p.Z != c.wantZ {
			t.Errorf("seed=%d %s region(%d,%d) = (%d,%d)，基线为 (%d,%d)（算法行为可能已漂移）",
				c.seed, c.typ, c.rx, c.rz, p.X, p.Z, c.wantX, c.wantZ)
		}
	}
}

// TestKnownSeedSlimeChunkBaseline 史莱姆区块固定种子基线。
func TestKnownSeedSlimeChunkBaseline(t *testing.T) {
	cases := []struct {
		seed   int64
		cx, cz int64
		want   bool
	}{
		{12345, 0, 0, false},
		{12345, 1, 0, false},
		{12345, -1, 0, true},
		{12345, 100, 0, false},
		{12345, -250, 0, false},
	}
	for _, c := range cases {
		if got := SlimeChunk(c.seed, c.cx, c.cz); got != c.want {
			t.Errorf("SlimeChunk(%d,%d,%d) = %v，基线为 %v", c.seed, c.cx, c.cz, got, c.want)
		}
	}
}

// TestBuriedTreasureZeroOffset spacing=1、separation=0 时偏移恒为 0，候选点等于 region 区块。
func TestBuriedTreasureZeroOffset(t *testing.T) {
	for _, r := range [][2]int{{0, 0}, {10, -7}, {-3, 5}} {
		p, err := Position(2024, BuriedTreasure, r[0], r[1])
		if err != nil {
			t.Fatal(err)
		}
		if p.X != r[0]*16 || p.Z != r[1]*16 {
			t.Fatalf("region(%d,%d) 期望 (%d,%d)，实际 (%d,%d)",
				r[0], r[1], r[0]*16, r[1]*16, p.X, p.Z)
		}
	}
}

// TestStructureDimensions 维度归属表。
func TestStructureDimensions(t *testing.T) {
	cases := map[Type]Dimension{
		NetherFortress: NetherDimension,
		Bastion:        NetherDimension,
		NetherFossil:   NetherDimension,
		EndCity:        EndDimension,
		Village:        OverworldDimension,
		BuriedTreasure: OverworldDimension,
	}
	for typ, want := range cases {
		if got := DimensionOf(typ); got != want {
			t.Errorf("%s 维度应为 %s，实际 %s", typ, want, got)
		}
	}
}

// TestStructureProbability 频率门槛表（wiki Structure set frequency）。
func TestStructureProbability(t *testing.T) {
	cases := map[Type]float64{
		BuriedTreasure:  0.01,
		Mineshaft:       0.004,
		PillagerOutpost: 0.2,
		Village:         1.0, // 未登记 → 1.0
	}
	for typ, want := range cases {
		if got := ProbabilityOf(typ); got != want {
			t.Errorf("%s 频率应为 %v，实际 %v", typ, want, got)
		}
	}
}

// TestDescribe 描述表覆盖全部已知结构且字段自洽。
func TestDescribe(t *testing.T) {
	all := DescribeAll()
	if len(all) != len(ListKnown()) {
		t.Fatalf("描述数量 %d 与已知结构数量 %d 不一致", len(all), len(ListKnown()))
	}
	for _, d := range all {
		if !IsKnown(d.Type) {
			t.Errorf("描述包含未知类型: %s", d.Type)
		}
		if d.Spacing <= 0 {
			t.Errorf("%s spacing 应 > 0", d.Type)
		}
		if d.Dimension == "" {
			t.Errorf("%s 维度不应为空", d.Type)
		}
		if d.Probability <= 0 || d.Probability > 1 {
			t.Errorf("%s 频率应在 (0,1]，实际 %v", d.Type, d.Probability)
		}
		if d.SinceVersion == "" {
			t.Errorf("%s 起始版本不应为空", d.Type)
		}
	}
	if _, ok := Describe(Type("not_exist")); ok {
		t.Error("未知类型应返回 false")
	}
}
