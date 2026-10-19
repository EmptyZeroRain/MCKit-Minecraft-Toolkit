package main

import (
	"encoding/json"
	"os"
	"testing"

	"mckit/backend/internal/domain"
)

// TestBuildMobNormalizesAndValidates 覆盖 TODO §9C：生物导入行的归一化与必填校验。
func TestBuildMobNormalizesAndValidates(t *testing.T) {
	health := 10.0
	mob, err := buildMob(mobRow{
		Slug:              "  cow ",
		NameZh:            " 牛 ",
		Category:          "passive",
		Health:            &health,
		Dimensions:        []string{"overworld", "nether"},
		SourceURL:         " https://minecraft.wiki/w/Cow ",
		SupportedVersions: []string{"1.0+"},
	})
	if err != nil {
		t.Fatalf("合法行不应报错: %v", err)
	}
	if mob.Slug != "cow" || mob.NameZh != "牛" {
		t.Errorf("字段应被裁剪空格: %+v", mob)
	}
	if mob.Category != domain.MobCategoryPassive {
		t.Errorf("category 应为 passive，实际 %q", mob.Category)
	}
	if mob.Health == nil || *mob.Health != 10 {
		t.Errorf("health 应为 10，实际 %v", mob.Health)
	}
	if len(mob.Dimensions) != 2 {
		t.Errorf("dimensions 应保留 2 项，实际 %v", mob.Dimensions)
	}
	if mob.GameEdition != "java" {
		t.Errorf("game_edition 缺省应为 java，实际 %q", mob.GameEdition)
	}
}

// TestBuildMobRejectsBadRows 覆盖必填与枚举校验。
func TestBuildMobRejectsBadRows(t *testing.T) {
	neg := -1.0
	cases := []struct {
		name string
		row  mobRow
	}{
		{"缺少 slug", mobRow{NameZh: "牛", Category: "passive", SourceURL: "u", SupportedVersions: []string{"1.0+"}}},
		{"缺少 name_zh", mobRow{Slug: "cow", Category: "passive", SourceURL: "u", SupportedVersions: []string{"1.0+"}}},
		{"category 非法", mobRow{Slug: "cow", NameZh: "牛", Category: "bogus", SourceURL: "u", SupportedVersions: []string{"1.0+"}}},
		{"dimension 非法", mobRow{Slug: "cow", NameZh: "牛", Category: "passive", Dimensions: []string{"moon"}, SourceURL: "u", SupportedVersions: []string{"1.0+"}}},
		{"health 为负", mobRow{Slug: "cow", NameZh: "牛", Category: "passive", Health: &neg, SourceURL: "u", SupportedVersions: []string{"1.0+"}}},
		{"缺少 source_url", mobRow{Slug: "cow", NameZh: "牛", Category: "passive", SupportedVersions: []string{"1.0+"}}},
		{"缺少 supported_versions", mobRow{Slug: "cow", NameZh: "牛", Category: "passive", SourceURL: "u"}},
		{"版本非法", mobRow{Slug: "cow", NameZh: "牛", Category: "passive", SourceURL: "u", SupportedVersions: []string{"not-a-version"}}},
		{"game_edition 非法", mobRow{Slug: "cow", NameZh: "牛", Category: "passive", SourceURL: "u", SupportedVersions: []string{"1.0+"}, GameEdition: "switch"}},
	}
	for _, tc := range cases {
		if _, err := buildMob(tc.row); err == nil {
			t.Errorf("%s：应被拒绝", tc.name)
		}
	}
}

// TestBuildMobDropValidates 覆盖掉落行校验（引用 / 数量 / 概率）。
func TestBuildMobDropValidates(t *testing.T) {
	known := map[string]bool{"cow": true}

	ok, err := buildMobDrop(dropRow{
		DropKey: "cow:raw_beef", MobSlug: "cow",
		ItemID: "minecraft:raw_beef", ItemNameZh: "生牛肉", CountMin: 1, CountMax: 3, Chance: 1,
	}, known)
	if err != nil {
		t.Fatalf("合法掉落不应报错: %v", err)
	}
	if ok.CountMax != 3 || ok.Chance != 1 {
		t.Errorf("数量 / 概率未正确保留: %+v", ok)
	}

	// 未给出 chance 时默认「必定掉落」。
	def, err := buildMobDrop(dropRow{
		DropKey: "cow:leather", MobSlug: "cow",
		ItemID: "minecraft:leather", ItemNameZh: "皮革", CountMin: 0, CountMax: 2,
	}, known)
	if err != nil || def.Chance != 1 {
		t.Fatalf("缺省 chance 应为 1，实际 %v / err=%v", def.Chance, err)
	}

	bad := []struct {
		name string
		row  dropRow
	}{
		{"缺少 drop_key", dropRow{MobSlug: "cow", ItemID: "i", ItemNameZh: "n", CountMin: 1}},
		{"mob 悬空", dropRow{DropKey: "k", MobSlug: "ghost", ItemID: "i", ItemNameZh: "n", CountMin: 1}},
		{"缺少 item_id", dropRow{DropKey: "k", MobSlug: "cow", ItemNameZh: "n", CountMin: 1}},
		{"count_max < count_min", dropRow{DropKey: "k", MobSlug: "cow", ItemID: "i", ItemNameZh: "n", CountMin: 3, CountMax: 1}},
		{"count_min 为负", dropRow{DropKey: "k", MobSlug: "cow", ItemID: "i", ItemNameZh: "n", CountMin: -1}},
		{"chance 超界", dropRow{DropKey: "k", MobSlug: "cow", ItemID: "i", ItemNameZh: "n", CountMin: 1, Chance: 1.5}},
	}
	for _, tc := range bad {
		if _, err := buildMobDrop(tc.row, known); err == nil {
			t.Errorf("%s：应被拒绝", tc.name)
		}
	}
}

// TestSeedFileMobsAreValid 校验种子文件：全部行合法、slug / drop_key 不重复、
// 掉落引用可解析、维度与类别合法，且覆盖被动 / 中立 / 敌对 / Boss 四类。
func TestSeedFileMobsAreValid(t *testing.T) {
	raw, err := os.ReadFile("../../seeds/mobs_demo.json")
	if err != nil {
		t.Fatalf("读取种子文件失败: %v", err)
	}
	var file mobSeedFile
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatalf("种子文件 JSON 解析失败: %v", err)
	}
	if len(file.Mobs) == 0 || len(file.Drops) == 0 {
		t.Fatalf("种子文件不应为空：mobs=%d drops=%d", len(file.Mobs), len(file.Drops))
	}

	known := make(map[string]bool, len(file.Mobs))
	categories := map[string]int{}
	for i, row := range file.Mobs {
		mob, err := buildMob(row)
		if err != nil {
			t.Fatalf("生物第 %d 行（%s）非法: %v", i+1, row.Slug, err)
		}
		if known[mob.Slug] {
			t.Fatalf("生物 slug 重复: %s", mob.Slug)
		}
		known[mob.Slug] = true
		categories[mob.Category]++
	}
	for _, want := range []string{
		domain.MobCategoryPassive, domain.MobCategoryNeutral,
		domain.MobCategoryHostile, domain.MobCategoryBoss,
	} {
		if categories[want] == 0 {
			t.Errorf("种子数据应覆盖类别 %q", want)
		}
	}

	seenKeys := make(map[string]bool, len(file.Drops))
	for i, row := range file.Drops {
		drop, err := buildMobDrop(row, known)
		if err != nil {
			t.Fatalf("掉落第 %d 行（%s）非法: %v", i+1, row.DropKey, err)
		}
		if seenKeys[drop.DropKey] {
			t.Fatalf("drop_key 重复: %s", drop.DropKey)
		}
		seenKeys[drop.DropKey] = true
	}
}
