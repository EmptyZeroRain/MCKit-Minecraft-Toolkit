package structure

import "testing"

// TestAvailableInNewStructures 新增结构的版本可用性。
func TestAvailableInNewStructures(t *testing.T) {
	// 埋藏的宝藏 1.13 加入。
	if AvailableIn(BuriedTreasure, Ver1_8_1_12) {
		t.Error("埋藏的宝藏不应在 1.8-1.12 生成")
	}
	if !AvailableIn(BuriedTreasure, Ver1_13_1_15) {
		t.Error("埋藏的宝藏应在 1.13-1.15 生成")
	}
	// 试炼密室 1.21 才加入，1.18+ 区间视为可用（最细粒度限制）。
	if AvailableIn(TrialChambers, Ver1_16_1_17) {
		t.Error("试炼密室不应在 1.16-1.17 生成")
	}
}

// TestSinceVersionDefault 未登记结构返回最早版本区间。
func TestSinceVersionDefault(t *testing.T) {
	if got := SinceVersion(Type("not_exist")); got != Ver1_8_1_12 {
		t.Errorf("未登记结构起始版本应为 %s，实际 %s", Ver1_8_1_12, got)
	}
	if got := SinceVersion(Village); got != Ver1_8_1_12 {
		t.Errorf("村庄起始版本应为 %s，实际 %s", Ver1_8_1_12, got)
	}
}

// TestPlacementSupported 结构分布算法仅在 1.13+ 与当前实现一致。
func TestPlacementSupported(t *testing.T) {
	if PlacementSupported(Ver1_8_1_12) {
		t.Error("1.8-1.12 不应标记为算法一致")
	}
	for _, v := range []Version{Ver1_13_1_15, Ver1_16_1_17, Ver1_18Plus} {
		if !PlacementSupported(v) {
			t.Errorf("%s 应标记为算法一致", v)
		}
	}
}

// TestCapabilities 支持矩阵覆盖全部版本区间，且关键限制符合预期。
func TestCapabilities(t *testing.T) {
	all := AllVersions()
	caps := Capabilities()
	if len(caps) == 0 {
		t.Fatal("能力清单不应为空")
	}
	seen := map[string]bool{}
	for _, c := range caps {
		if c.ID == "" || c.Name == "" || c.Note == "" {
			t.Errorf("能力 %+v 字段不完整", c)
		}
		if seen[c.ID] {
			t.Errorf("能力 id 重复: %s", c.ID)
		}
		seen[c.ID] = true
		if len(c.Supported) != len(all) {
			t.Fatalf("能力 %s 支持矩阵应有 %d 项，实际 %d", c.ID, len(all), len(c.Supported))
		}
		for i, s := range c.Supported {
			if s.Version != all[i] {
				t.Errorf("能力 %s 第 %d 项版本应为 %s，实际 %s", c.ID, i, all[i], s.Version)
			}
		}
	}

	// 群系查询仅 1.18+ 支持。
	biome := featureByID(t, caps, "biome_query")
	for i, s := range biome.Supported {
		want := all[i] == Ver1_18Plus
		if s.Supported != want {
			t.Errorf("biome_query 在 %s 支持状态应为 %v，实际 %v", s.Version, want, s.Supported)
		}
	}

	// 结构分布在 1.8-1.12 不支持。
	structure := featureByID(t, caps, "structures")
	if structure.Supported[0].Supported {
		t.Error("structures 在 1.8-1.12 应标记为不支持")
	}
	if !structure.Supported[len(structure.Supported)-1].Supported {
		t.Error("structures 在 1.18+ 应标记为支持")
	}
}

func featureByID(t *testing.T, caps []FeatureCapability, id string) FeatureCapability {
	t.Helper()
	for _, c := range caps {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("未找到能力 %s", id)
	return FeatureCapability{}
}
