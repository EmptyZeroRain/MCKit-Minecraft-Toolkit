package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"mckit/backend/internal/domain"
)

// strPtr 返回 string 指针，便于构造可空的药水 slug。
func strPtr(v string) *string { return &v }

// TestBuildPotionNormalizesAndValidates 覆盖 TODO §9A：药水导入行的归一化与必填校验。
func TestBuildPotionNormalizesAndValidates(t *testing.T) {
	potion, err := buildPotion(potionRow{
		Slug:              "  potion-of-healing ",
		NameZh:            " 治疗药水 ",
		SourceURL:         " https://minecraft.wiki/w/Potion_of_Healing ",
		SupportedVersions: []string{"1.0+"},
		Effects: []domain.PotionEffect{
			{Variant: "normal", EffectID: "minecraft:instant_health", NameZh: "瞬间治疗", Amplifier: 1},
		},
	})
	if err != nil {
		t.Fatalf("合法行不应报错: %v", err)
	}
	if potion.Slug != "potion-of-healing" || potion.NameZh != "治疗药水" {
		t.Errorf("字段应被裁剪空格: %+v", potion)
	}
	if potion.GameEdition != "both" {
		t.Errorf("game_edition 缺省应为 both，实际 %q", potion.GameEdition)
	}
	if !potion.IsBrewable {
		t.Error("is_brewable 缺省应为 true")
	}
	if potion.IsExtendable || potion.IsEnhanceable {
		t.Error("is_extendable / is_enhanceable 缺省应为 false")
	}
	if len(potion.Effects) != 1 || potion.Effects[0].Variant != domain.PotionVariantNormal {
		t.Errorf("效果条目应保留，实际 %+v", potion.Effects)
	}
}

// TestBuildPotionRequiresSourceAndVersions 覆盖 §9「校验资料来源和版本适用范围」。
func TestBuildPotionRequiresSourceAndVersions(t *testing.T) {
	base := func() potionRow {
		return potionRow{
			Slug:              "potion-of-healing",
			NameZh:            "治疗药水",
			SourceURL:         "https://minecraft.wiki/w/Potion_of_Healing",
			SupportedVersions: []string{"1.0+"},
		}
	}

	cases := []struct {
		name   string
		mutate func(*potionRow)
	}{
		{"缺少 slug", func(r *potionRow) { r.Slug = "  " }},
		{"缺少 name_zh", func(r *potionRow) { r.NameZh = "" }},
		{"缺少 source_url", func(r *potionRow) { r.SourceURL = "" }},
		{"缺少 supported_versions", func(r *potionRow) { r.SupportedVersions = nil }},
		{"非法版本声明", func(r *potionRow) { r.SupportedVersions = []string{"v1"} }},
		{"非法类别", func(r *potionRow) { r.Category = "magic" }},
		{"非法版本归属", func(r *potionRow) { r.GameEdition = "switch" }},
		{"效果等级非法", func(r *potionRow) {
			r.Effects = []domain.PotionEffect{{EffectID: "minecraft:speed", NameZh: "迅捷", Amplifier: 0}}
		}},
		{"效果缺少 effect_id", func(r *potionRow) {
			r.Effects = []domain.PotionEffect{{NameZh: "迅捷", Amplifier: 1}}
		}},
		{"效果变体非法", func(r *potionRow) {
			r.Effects = []domain.PotionEffect{{Variant: "huge", EffectID: "minecraft:speed", NameZh: "迅捷", Amplifier: 1}}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			row := base()
			tc.mutate(&row)
			if _, err := buildPotion(row); err == nil {
				t.Fatal("期望校验失败，实际通过")
			}
		})
	}
}

// TestBuildBrewingRecipeValidatesRefs 覆盖 TODO §9A：酿造步骤的悬空引用与类型约束。
func TestBuildBrewingRecipeValidatesRefs(t *testing.T) {
	known := map[string]bool{"awkward-potion": true, "potion-of-healing": true}

	valid := brewingRow{
		RecipeKey:         "effect:potion-of-healing/minecraft:glistering_melon_slice",
		StepKind:          domain.BrewStepEffect,
		InputPotionSlug:   strPtr("awkward-potion"),
		IngredientItemID:  "minecraft:glistering_melon_slice",
		IngredientNameZh:  "闪烁的西瓜片",
		ResultPotionSlug:  strPtr("potion-of-healing"),
		SupportedVersions: []string{"1.0+"},
	}
	step, err := buildBrewingRecipe(valid, known)
	if err != nil {
		t.Fatalf("合法行不应报错: %v", err)
	}
	// 未提供 source_url 时应回退为酿造总览页（全部步骤的唯一来源）。
	if step.SourceURL != brewingOverviewSource {
		t.Errorf("source_url 应回退为酿造总览页，实际 %q", step.SourceURL)
	}
	if step.GameEdition != "both" {
		t.Errorf("game_edition 缺省应为 both，实际 %q", step.GameEdition)
	}

	bad := []struct {
		name string
		row  brewingRow
	}{
		{"缺少 recipe_key", func() brewingRow { r := valid; r.RecipeKey = ""; return r }()},
		{"非法 step_kind", func() brewingRow { r := valid; r.StepKind = "brew"; return r }()},
		{"缺少原料", func() brewingRow { r := valid; r.IngredientItemID = " "; return r }()},
		{"缺少原料中文名", func() brewingRow { r := valid; r.IngredientNameZh = ""; return r }()},
		{"缺少版本", func() brewingRow { r := valid; r.SupportedVersions = nil; return r }()},
		{"输入药水未收录", func() brewingRow { r := valid; r.InputPotionSlug = strPtr("nope"); return r }()},
		{"结果药水未收录", func() brewingRow { r := valid; r.ResultPotionSlug = strPtr("nope"); return r }()},
		{"效果步骤缺少结果药水", func() brewingRow { r := valid; r.ResultPotionSlug = nil; return r }()},
		{"通用规则不应有结果药水", func() brewingRow {
			r := valid
			r.StepKind = domain.BrewStepModifier
			return r
		}()},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := buildBrewingRecipe(tc.row, known); err == nil {
				t.Fatal("期望校验失败，实际通过")
			}
		})
	}
}

// TestBuildBrewingRecipeAllowsNullPotionSlugs 覆盖：空字符串应归一化为 nil（而非悬空引用）。
func TestBuildBrewingRecipeAllowsNullPotionSlugs(t *testing.T) {
	step, err := buildBrewingRecipe(brewingRow{
		RecipeKey:         "base:water-bottle/minecraft:nether_wart",
		StepKind:          domain.BrewStepBase,
		InputPotionSlug:   strPtr("   "),
		IngredientItemID:  "minecraft:nether_wart",
		IngredientNameZh:  "地狱疣",
		ResultPotionSlug:  strPtr("awkward-potion"),
		SupportedVersions: []string{"1.0+"},
	}, map[string]bool{"awkward-potion": true})
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if step.InputPotionSlug != nil {
		t.Errorf("空白输入药水应归一化为 nil，实际 %v", *step.InputPotionSlug)
	}
}

// TestSeedFilePotionsAreValid 直接校验随仓库发布的种子数据文件：
// 任何一行非法（缺来源、缺版本、悬空引用、重复键）都会让本测试失败。
func TestSeedFilePotionsAreValid(t *testing.T) {
	raw, err := os.ReadFile("../../seeds/potions_demo.json")
	if err != nil {
		t.Fatalf("读取种子文件失败: %v", err)
	}
	var file potionSeedFile
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatalf("种子文件 JSON 解析失败: %v", err)
	}
	if len(file.Potions) == 0 || len(file.BrewingRecipes) == 0 {
		t.Fatalf("种子文件不应为空：potions=%d brewing=%d", len(file.Potions), len(file.BrewingRecipes))
	}

	known := make(map[string]bool, len(file.Potions))
	for i, row := range file.Potions {
		potion, err := buildPotion(row)
		if err != nil {
			t.Fatalf("药水第 %d 行（%s）非法: %v", i+1, row.Slug, err)
		}
		if known[potion.Slug] {
			t.Fatalf("药水 slug 重复: %s", potion.Slug)
		}
		known[potion.Slug] = true
	}

	seen := make(map[string]bool, len(file.BrewingRecipes))
	for i, row := range file.BrewingRecipes {
		step, err := buildBrewingRecipe(row, known)
		if err != nil {
			t.Fatalf("酿造第 %d 行（%s）非法: %v", i+1, row.RecipeKey, err)
		}
		if seen[step.RecipeKey] {
			t.Fatalf("酿造步骤 recipe_key 重复: %s", step.RecipeKey)
		}
		seen[step.RecipeKey] = true
	}
}

// TestSeedFilePotionsCoverVariantSemantics 抽样校验关键数据的语义正确性，
// 避免种子文件被无意改坏（如治疗药水丢掉增强变体、版本声明写成通配）。
func TestSeedFilePotionsCoverVariantSemantics(t *testing.T) {
	raw, err := os.ReadFile("../../seeds/potions_demo.json")
	if err != nil {
		t.Fatalf("读取种子文件失败: %v", err)
	}
	var file potionSeedFile
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatalf("JSON 解析失败: %v", err)
	}

	bySlug := make(map[string]domain.Potion, len(file.Potions))
	for _, row := range file.Potions {
		potion, err := buildPotion(row)
		if err != nil {
			t.Fatalf("药水 %s 非法: %v", row.Slug, err)
		}
		bySlug[potion.Slug] = *potion
	}

	healing, ok := bySlug["potion-of-healing"]
	if !ok {
		t.Fatal("缺少治疗药水")
	}
	if healing.IsExtendable {
		t.Error("治疗药水为瞬间效果，不应可延长")
	}
	if len(healing.Effects) != 2 {
		t.Errorf("治疗药水应有普通与增强两条效果，实际 %d", len(healing.Effects))
	}

	turtle, ok := bySlug["potion-of-the-turtle_master"]
	if !ok {
		t.Fatal("缺少神龟药水")
	}
	if len(turtle.Effects) != 6 {
		t.Errorf("神龟药水每个变体含两种效果，应为 6 条，实际 %d", len(turtle.Effects))
	}

	oozing, ok := bySlug["potion-of-oozing"]
	if !ok {
		t.Fatal("缺少渗浆药水")
	}
	if got := strings.Join(oozing.SupportedVersions, "|"); got != "1.21+" {
		t.Errorf("渗浆药水版本声明应为 1.21+，实际 %v", oozing.SupportedVersions)
	}

	for slug, p := range bySlug {
		if len(p.SupportedVersions) == 0 {
			t.Errorf("%s 缺少适用版本", slug)
		}
		if p.SourceURL == "" {
			t.Errorf("%s 缺少来源 URL", slug)
		}
	}
}
