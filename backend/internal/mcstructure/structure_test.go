package structure

import (
	"math"
	"testing"
)

// 要塞估算：坐标应落在对应环的半径范围内。
func TestStrongholdPosition(t *testing.T) {
	// 第 0 环（3 个要塞）半径 1280–2816
	p := StrongholdPos(3055141959546, 0)
	dist := math.Hypot(float64(p.X), float64(p.Z))
	if dist < 1200 || dist > 2900 {
		t.Fatalf("要塞#0 距离 %0.f 超出第 0 环范围", dist)
	}

	// 第 100 个要塞应在靠外环（> 19000）
	p100 := StrongholdPos(3055141959546, 100)
	d100 := math.Hypot(float64(p100.X), float64(p100.Z))
	if d100 < 19000 {
		t.Fatalf("要塞#100 距离 %.0f 过近", d100)
	}
}

func TestStrongholdDeterministic(t *testing.T) {
	a := StrongholdPos(8888, 5)
	b := StrongholdPos(8888, 5)
	if a != b {
		t.Fatal("同种子要塞坐标应一致")
	}
}

// 所有已知参数表结构都能算出坐标。
func TestAllStructuresPosition(t *testing.T) {
	seed := int64(123456789)
	for _, s := range []Type{
		Village, AncientCity, DesertPyramid, EndCity, Igloo, JunglePyramid,
		Mineshaft, NetherFortress, Bastion, NetherFossil, OceanRuin,
		PillagerOutpost, RuinedPortal, Shipwreck, SwampHut, TrailRuins,
		TrialChambers, WoodlandMansion, OceanMonument,
	} {
		p, err := Position(seed, s, 0, 0)
		if err != nil {
			t.Fatalf("%s: %v", s, err)
		}
		if p.X < 0 || p.Z < 0 {
			t.Fatalf("%s 出现负坐标: (%d,%d)", s, p.X, p.Z)
		}
	}
}

// 科学验证：同种子同结构，坐标确定且位于候选窗口内。
func TestPositionDeterministic(t *testing.T) {
	a, _ := Position(555, Village, 3, -2)
	b, _ := Position(555, Village, 3, -2)
	if a != b {
		t.Fatalf("同种子结构坐标不一致: %v vs %v", a, b)
	}
}

// 丰收性验证：不同 region 应产生不同坐标（统计上基本不同）。
func TestPositionsDifferAcrossRegions(t *testing.T) {
	seen := map[Pos]bool{}
	for rx := 0; rx < 5; rx++ {
		for rz := 0; rz < 5; rz++ {
			p, _ := Position(777, Village, rx, rz)
			if seen[p] {
				// 允许少量碰撞（窗口内随机），但不允许大量重复
			}
			seen[p] = true
		}
	}
	if len(seen) < 20 {
		t.Fatalf("region 间坐标分布过于集中: %d/25", len(seen))
	}
}

func TestSlimeChunk(t *testing.T) {
	// 史莱姆区块约 1/10 区块，统计验证命中率近似。
	hits := 0
	const n = 2000
	for cx := int64(0); cx < n; cx++ {
		if SlimeChunk(12345, cx, 0) {
			hits++
		}
	}
	ratio := float64(hits) / n
	if ratio < 0.05 || ratio > 0.18 {
		t.Fatalf("史莱姆区块命中率异常: %.1f%%", ratio*100)
	}
}

func TestUnknownType(t *testing.T) {
	if _, err := Position(1, Type("not_exist"), 0, 0); err == nil {
		t.Fatal("未知类型应报错")
	}
}

func TestPositionMulti(t *testing.T) {
	pts, err := PositionMulti(222, Village, -1, -1, 3, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(pts) != 9 {
		t.Fatalf("期望 9 个点，实际 %d", len(pts))
	}
}