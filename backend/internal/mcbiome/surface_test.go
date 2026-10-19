package mcbiome

import "testing"

// TestSurfaceNoCaveBiomes 地表采样不应出现洞穴群系。
// 这是对「固定高度采样导致高地形误判为洞穴群系」的回归保护：
// 地表地图使用 SAMPLE_NO_DEPTH 语义（跳过深度项）。
func TestSurfaceNoCaveBiomes(t *testing.T) {
	caves := map[string]bool{"dripstone_caves": true, "lush_caves": true, "deep_dark": true}
	for _, seed := range []int64{12345, -987654321, 0, 42} {
		s := NewSampler(seed, v1_18)
		for x := -2048; x <= 2048; x += 128 {
			for z := -2048; z <= 2048; z += 128 {
				name := s.BiomeAt(x, z)
				if caves[name] {
					t.Fatalf("seed=%d 地表 (%d,%d) 误判为洞穴群系 %s", seed, x, z, name)
				}
			}
		}
	}
}

// TestSurfaceDistributionSane 地表采样应覆盖多种群系且以海洋/陆地为主。
func TestSurfaceDistributionSane(t *testing.T) {
	s := NewSampler(12345, v1_18)
	seen := map[string]int{}
	for x := -2048; x <= 2048; x += 128 {
		for z := -2048; z <= 2048; z += 128 {
			seen[s.BiomeAt(x, z)]++
		}
	}
	if len(seen) < 10 {
		t.Fatalf("地表群系种类过少: %v", seen)
	}
	t.Logf("地表群系种类=%d", len(seen))
}
