package mcbiome

import "testing"

// TestTreesLoaded 校验内嵌决策树解析结果的规模与结构。
func TestTreesLoaded(t *testing.T) {
	treesOnce.Do(loadTrees)
	if treesErr != nil {
		t.Fatalf("决策树加载失败: %v", treesErr)
	}
	bt := trees["btree18"]
	if bt == nil {
		t.Fatal("btree18 未加载")
	}
	if bt.order != 10 {
		t.Fatalf("btree18 order=%d，期望 10", bt.order)
	}
	want := []uint32{1111, 111, 11, 1, 0}
	if len(bt.steps) != len(want) {
		t.Fatalf("steps 长度=%d，期望 %d", len(bt.steps), len(want))
	}
	for i := range want {
		if bt.steps[i] != want[i] {
			t.Fatalf("steps[%d]=%d，期望 %d", i, bt.steps[i], want[i])
		}
	}
	if len(bt.param)%2 != 0 {
		t.Fatalf("param 长度应为偶数，实际 %d", len(bt.param))
	}
	if bt.length != uint32(len(bt.nodes)) {
		t.Fatalf("length=%d 与 nodes=%d 不一致", bt.length, len(bt.nodes))
	}
	t.Logf("btree18: nodes=%d paramPairs=%d", len(bt.nodes), len(bt.param)/2)
}

// TestSamplerDeterministic 同种子同坐标结果稳定。
func TestSamplerDeterministic(t *testing.T) {
	a := NewSampler(123456789, v1_18)
	b := NewSampler(123456789, v1_18)
	for _, c := range [][2]int{{0, 0}, {100, -200}, {1234, 5678}, {-9999, 42}} {
		if got, want := a.BiomeAt(c[0], c[1]), b.BiomeAt(c[0], c[1]); got != want {
			t.Fatalf("(%d,%d) 结果不稳定: %s vs %s", c[0], c[1], got, want)
		}
	}
}

// TestSamplerNamesKnown 采样结果必须落在已知群系词表内（无 unknown）。
func TestSamplerNamesKnown(t *testing.T) {
	s := NewSampler(42, v1_21)
	seen := map[string]int{}
	for x := -2000; x <= 2000; x += 250 {
		for z := -2000; z <= 2000; z += 250 {
			n := s.BiomeAt(x, z)
			if n == "" || n == "unknown" {
				t.Fatalf("(%d,%d) 返回未知群系: %q", x, z, n)
			}
			seen[n]++
		}
	}
	if len(seen) < 3 {
		t.Fatalf("采样群系种类过少: %v", seen)
	}
	t.Logf("采样群系分布: %v", seen)
}

// TestDifferentSeedsDiffer 不同种子应产生不同群系分布。
func TestDifferentSeedsDiffer(t *testing.T) {
	s1 := NewSampler(1, v1_18)
	s2 := NewSampler(2, v1_18)
	same := 0
	total := 0
	for x := -1000; x <= 1000; x += 200 {
		for z := -1000; z <= 1000; z += 200 {
			total++
			if s1.BiomeAt(x, z) == s2.BiomeAt(x, z) {
				same++
			}
		}
	}
	if same == total {
		t.Fatal("不同种子的群系分布完全相同，疑似未使用种子")
	}
}

// TestVersionFromString 版本字符串解析。
func TestVersionFromString(t *testing.T) {
	cases := map[string]version{
		"":        v1_21,
		"1.18+":   v1_21,
		"1.18":    v1_18,
		"1.19.2":  v1_19_2,
		"1.19.4":  v1_19_4,
		"1.20.6":  v1_20_6,
		"1.21":    v1_21,
		"unknown": v1_21,
	}
	for in, want := range cases {
		if got := VersionFromString(in); got != want {
			t.Errorf("VersionFromString(%q)=%d，期望 %d", in, got, want)
		}
	}
}
