package main

import (
	"encoding/json"
	"os"
	"testing"

	"mckit/backend/internal/domain"
)

// TestBuildRedstoneComponentNormalizesAndValidates 覆盖 TODO §15：元件导入行的归一化与校验。
func TestBuildRedstoneComponentNormalizesAndValidates(t *testing.T) {
	c, err := buildRedstoneComponent(redstoneComponentRow{
		Slug:              "  redstone_dust ",
		NameZh:            " 红石粉 ",
		Category:          "transmission",
		SupportedVersions: []string{"1.0+"},
		SourceURL:         " https://minecraft.wiki/w/Redstone_Dust ",
	})
	if err != nil {
		t.Fatalf("合法行不应报错: %v", err)
	}
	if c.Slug != "redstone_dust" || c.NameZh != "红石粉" {
		t.Errorf("字段应被裁剪空格: %+v", c)
	}
	if c.Category != domain.RedstoneComponentCategoryTransmission {
		t.Errorf("category 应为 transmission，实际 %q", c.Category)
	}
	if c.GameEdition != "java" {
		t.Errorf("game_edition 缺省应为 java，实际 %q", c.GameEdition)
	}
	if c.SourceURL != "https://minecraft.wiki/w/Redstone_Dust" {
		t.Errorf("source_url 应被裁剪: %q", c.SourceURL)
	}
}

// TestBuildRedstoneComponentRejectsBadRows 覆盖元件必填与枚举校验。
func TestBuildRedstoneComponentRejectsBadRows(t *testing.T) {
	cases := []struct {
		name string
		row  redstoneComponentRow
	}{
		{"缺少 slug", redstoneComponentRow{NameZh: "红石粉", Category: "transmission", SupportedVersions: []string{"1.0+"}}},
		{"缺少 name_zh", redstoneComponentRow{Slug: "redstone_dust", Category: "transmission", SupportedVersions: []string{"1.0+"}}},
		{"category 非法", redstoneComponentRow{Slug: "x", NameZh: "X", Category: "bogus", SupportedVersions: []string{"1.0+"}}},
		{"版本非法", redstoneComponentRow{Slug: "x", NameZh: "X", Category: "source", SupportedVersions: []string{"not-a-version"}}},
		{"game_edition 非法", redstoneComponentRow{Slug: "x", NameZh: "X", Category: "source", SupportedVersions: []string{"1.0+"}, GameEdition: "switch"}},
	}
	for _, tc := range cases {
		if _, err := buildRedstoneComponent(tc.row); err == nil {
			t.Errorf("%s：应被拒绝", tc.name)
		}
	}
}

// TestBuildRedstoneDesignValidates 覆盖设计行校验（必填 / 枚举 / 材料 / 授权红线）。
func TestBuildRedstoneDesignValidates(t *testing.T) {
	ok, err := buildRedstoneDesign(redstoneDesignRow{
		Slug: "clock_circuit", NameZh: "时钟电路",
		Purpose: "logic", Difficulty: "easy",
		SupportedVersions: []string{"1.0+"},
		SourceURL:         "https://minecraft.wiki/w/Redstone_circuit",
		ReviewStatus:      "published",
		Materials: []redstoneMaterialRow{
			{ItemID: "minecraft:redstone", NameZh: "红石粉", CountMin: 4, CountMax: 16},
			{NameZh: "中继器", CountMin: 1},
		},
	})
	if err != nil {
		t.Fatalf("合法行不应报错: %v", err)
	}
	if ok.Purpose != domain.RedstoneDesignPurposeLogic || ok.Difficulty != domain.RedstoneDesignDifficultyEasy {
		t.Errorf("枚举未正确保留: %+v", ok)
	}
	// count_max 缺省等于 count_min。
	if ok.Materials[1].CountMax != 1 {
		t.Errorf("count_max 缺省应等于 count_min，实际 %d", ok.Materials[1].CountMax)
	}

	bad := []struct {
		name string
		row  redstoneDesignRow
	}{
		{"缺少 slug", redstoneDesignRow{NameZh: "X", Purpose: "farm", Difficulty: "easy", SupportedVersions: []string{"1.0+"}, SourceURL: "u"}},
		{"purpose 非法", redstoneDesignRow{Slug: "x", NameZh: "X", Purpose: "bogus", Difficulty: "easy", SupportedVersions: []string{"1.0+"}, SourceURL: "u"}},
		{"difficulty 非法", redstoneDesignRow{Slug: "x", NameZh: "X", Purpose: "farm", Difficulty: "extreme", SupportedVersions: []string{"1.0+"}, SourceURL: "u"}},
		{"review_status 非法", redstoneDesignRow{Slug: "x", NameZh: "X", Purpose: "farm", Difficulty: "easy", SupportedVersions: []string{"1.0+"}, SourceURL: "u", ReviewStatus: "unknown"}},
		{"缺少 source_url", redstoneDesignRow{Slug: "x", NameZh: "X", Purpose: "farm", Difficulty: "easy", SupportedVersions: []string{"1.0+"}}},
		{"缺少 supported_versions", redstoneDesignRow{Slug: "x", NameZh: "X", Purpose: "farm", Difficulty: "easy", SourceURL: "u"}},
		{"材料数量非法", redstoneDesignRow{Slug: "x", NameZh: "X", Purpose: "farm", Difficulty: "easy", SupportedVersions: []string{"1.0+"}, SourceURL: "u", Materials: []redstoneMaterialRow{{ItemID: "i", NameZh: "n", CountMin: 3, CountMax: 1}}}},
		{"材料数量为 0", redstoneDesignRow{Slug: "x", NameZh: "X", Purpose: "farm", Difficulty: "easy", SupportedVersions: []string{"1.0+"}, SourceURL: "u", Materials: []redstoneMaterialRow{{ItemID: "i", NameZh: "n", CountMin: 0}}}},
		{"材料缺少名称", redstoneDesignRow{Slug: "x", NameZh: "X", Purpose: "farm", Difficulty: "easy", SupportedVersions: []string{"1.0+"}, SourceURL: "u", Materials: []redstoneMaterialRow{{CountMin: 1}}}},
		{"未授权却带下载链接", redstoneDesignRow{Slug: "x", NameZh: "X", Purpose: "farm", Difficulty: "easy", SupportedVersions: []string{"1.0+"}, SourceURL: "u", WorldDownloadURL: "https://example.com/world.zip"}},
	}
	for _, tc := range bad {
		if _, err := buildRedstoneDesign(tc.row); err == nil {
			t.Errorf("%s：应被拒绝", tc.name)
		}
	}

	// 带合法许可时允许给出下载链接。
	if _, err := buildRedstoneDesign(redstoneDesignRow{
		Slug: "x", NameZh: "X", Purpose: "farm", Difficulty: "easy",
		SupportedVersions: []string{"1.0+"}, SourceURL: "u",
		WorldDownloadURL: "https://example.com/world.zip", License: "CC BY-SA 4.0",
	}); err != nil {
		t.Errorf("有合法授权时不应拒绝下载链接: %v", err)
	}
}

// TestSeedFileRedstoneIsValid 校验种子文件：全部行合法、slug 不重复、
// 覆盖五类元件与六类设计用途，且所有已发布设计均有来源与许可。
func TestSeedFileRedstoneIsValid(t *testing.T) {
	raw, err := os.ReadFile("../../seeds/redstone_demo.json")
	if err != nil {
		t.Fatalf("读取种子文件失败: %v", err)
	}
	var file redstoneSeedFile
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatalf("种子文件 JSON 解析失败: %v", err)
	}
	if len(file.Components) == 0 || len(file.Designs) == 0 {
		t.Fatalf("种子文件不应为空：components=%d designs=%d", len(file.Components), len(file.Designs))
	}

	seenComponents := map[string]bool{}
	componentCategories := map[string]int{}
	for i, row := range file.Components {
		c, err := buildRedstoneComponent(row)
		if err != nil {
			t.Fatalf("元件第 %d 行（%s）非法: %v", i+1, row.Slug, err)
		}
		if seenComponents[c.Slug] {
			t.Fatalf("元件 slug 重复: %s", c.Slug)
		}
		seenComponents[c.Slug] = true
		componentCategories[c.Category]++
	}
	for _, want := range []string{
		domain.RedstoneComponentCategorySource,
		domain.RedstoneComponentCategoryTransmission,
		domain.RedstoneComponentCategoryLogic,
		domain.RedstoneComponentCategoryActuator,
		domain.RedstoneComponentCategorySensor,
	} {
		if componentCategories[want] == 0 {
			t.Errorf("种子数据应覆盖元件类别 %q", want)
		}
	}

	seenDesigns := map[string]bool{}
	designPurposes := map[string]int{}
	for i, row := range file.Designs {
		d, err := buildRedstoneDesign(row)
		if err != nil {
			t.Fatalf("设计第 %d 行（%s）非法: %v", i+1, row.Slug, err)
		}
		if seenDesigns[d.Slug] {
			t.Fatalf("设计 slug 重复: %s", d.Slug)
		}
		seenDesigns[d.Slug] = true
		designPurposes[d.Purpose]++
		if d.ReviewStatus == domain.RedstoneReviewPublished && d.SourceURL == "" {
			t.Errorf("已发布设计 %s 缺少 source_url", d.Slug)
		}
	}
	for _, want := range []string{
		domain.RedstoneDesignPurposeFarm,
		domain.RedstoneDesignPurposeSorter,
		domain.RedstoneDesignPurposeTransport,
		domain.RedstoneDesignPurposeDoor,
		domain.RedstoneDesignPurposeLogic,
		domain.RedstoneDesignPurposeUtility,
	} {
		if designPurposes[want] == 0 {
			t.Errorf("种子数据应覆盖设计用途 %q", want)
		}
	}
}
