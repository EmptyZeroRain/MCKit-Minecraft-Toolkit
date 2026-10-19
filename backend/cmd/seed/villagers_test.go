package main

import (
	"encoding/json"
	"os"
	"testing"

	"mckit/backend/internal/domain"
)

// TestBuildVillagerProfessionNormalizesAndValidates 覆盖 TODO §9B：职业导入行的归一化与必填校验。
func TestBuildVillagerProfessionNormalizesAndValidates(t *testing.T) {
	prof, err := buildVillagerProfession(villagerProfessionRow{
		Slug:              "  armorer ",
		NameZh:            " 盔甲匠 ",
		Kind:              "normal",
		WorkstationItemID: " minecraft:blast_furnace ",
		WorkstationNameZh: " 高炉 ",
		SourceURL:         " https://minecraft.wiki/w/Trading ",
		SupportedVersions: []string{"1.14+"},
	})
	if err != nil {
		t.Fatalf("合法行不应报错: %v", err)
	}
	if prof.Slug != "armorer" || prof.NameZh != "盔甲匠" || prof.WorkstationNameZh != "高炉" {
		t.Errorf("字段应被裁剪空格: %+v", prof)
	}
	if prof.Kind != domain.VillagerKindNormal {
		t.Errorf("kind 应保留 normal，实际 %q", prof.Kind)
	}
	if prof.GameEdition != "java" {
		t.Errorf("game_edition 缺省应为 java，实际 %q", prof.GameEdition)
	}
}

// TestBuildVillagerProfessionRejectsBadRows 覆盖 §9「校验资料来源和版本适用范围」。
func TestBuildVillagerProfessionRejectsBadRows(t *testing.T) {
	base := func() villagerProfessionRow {
		return villagerProfessionRow{
			Slug:              "armorer",
			NameZh:            "盔甲匠",
			Kind:              "normal",
			SourceURL:         "https://minecraft.wiki/w/Trading",
			SupportedVersions: []string{"1.14+"},
		}
	}
	cases := []struct {
		name   string
		mutate func(*villagerProfessionRow)
	}{
		{"缺少 slug", func(r *villagerProfessionRow) { r.Slug = " " }},
		{"缺少 name_zh", func(r *villagerProfessionRow) { r.NameZh = "" }},
		{"非法 kind", func(r *villagerProfessionRow) { r.Kind = "pirate" }},
		{"缺少 source_url", func(r *villagerProfessionRow) { r.SourceURL = "" }},
		{"缺少 supported_versions", func(r *villagerProfessionRow) { r.SupportedVersions = nil }},
		{"非法版本声明", func(r *villagerProfessionRow) { r.SupportedVersions = []string{"v1"} }},
		{"非法版本归属", func(r *villagerProfessionRow) { r.GameEdition = "switch" }},
		{"有工作站但缺中文名", func(r *villagerProfessionRow) { r.WorkstationItemID = "minecraft:blast_furnace" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			row := base()
			tc.mutate(&row)
			if _, err := buildVillagerProfession(row); err == nil {
				t.Fatal("期望校验失败，实际通过")
			}
		})
	}
}

// TestBuildVillagerTradeLevelValidates 覆盖职业等级的边界校验。
func TestBuildVillagerTradeLevelValidates(t *testing.T) {
	valid := villagerLevelRow{Level: 3, Slug: "journeyman", NameZh: "老手", RequiredExperience: 70}
	if _, err := buildVillagerTradeLevel(valid); err != nil {
		t.Fatalf("合法等级不应报错: %v", err)
	}
	bad := []villagerLevelRow{
		{Level: 0, Slug: "novice", NameZh: "新手"},
		{Level: 6, Slug: "grandmaster", NameZh: "宗师"},
		{Level: 1, Slug: "", NameZh: "新手"},
		{Level: 1, Slug: "novice", NameZh: ""},
		{Level: 1, Slug: "novice", NameZh: "新手", RequiredExperience: -1},
	}
	for i, row := range bad {
		if _, err := buildVillagerTradeLevel(row); err == nil {
			t.Errorf("第 %d 个非法等级应校验失败", i+1)
		}
	}
}

// TestBuildVillagerTradeValidates 覆盖 TODO §9B：交易的悬空引用、分组/等级约束与成本项校验。
func TestBuildVillagerTradeValidates(t *testing.T) {
	known := map[string]bool{"armorer": true, "wandering_trader": true}

	valid := villagerTradeRow{
		TradeKey:          "armorer_l1_coal",
		ProfessionSlug:    "armorer",
		Level:             1,
		TradeGroup:        domain.VillagerTradeGroupLevel,
		CostItems:         []domain.VillagerCostItem{{ItemID: "minecraft:coal", NameZh: "煤炭", CountMin: 15}},
		ResultItemID:      "minecraft:emerald",
		ResultItemNameZh:  "绿宝石",
		SupportedVersions: []string{"1.14+"},
	}
	trade, err := buildVillagerTrade(valid, known)
	if err != nil {
		t.Fatalf("合法行不应报错: %v", err)
	}
	if trade.ResultCount != 1 || trade.PriceMultiplier != 0.05 {
		t.Errorf("缺省 result_count/price_multiplier 应归一化，实际 %+v", trade)
	}
	if trade.SourceURL != villagerTradingSource {
		t.Errorf("source_url 应回退为交易总览页，实际 %q", trade.SourceURL)
	}
	// count_max 缺省时等于 count_min。
	if trade.CostItems[0].CountMax != 15 {
		t.Errorf("count_max 缺省应等于 count_min，实际 %d", trade.CostItems[0].CountMax)
	}

	bad := []struct {
		name string
		row  villagerTradeRow
	}{
		{"缺少 trade_key", func() villagerTradeRow { r := valid; r.TradeKey = ""; return r }()},
		{"职业未收录", func() villagerTradeRow { r := valid; r.ProfessionSlug = "nope"; return r }()},
		{"非法分组", func() villagerTradeRow { r := valid; r.TradeGroup = "gift"; return r }()},
		{"等级分组缺等级", func() villagerTradeRow { r := valid; r.Level = 0; return r }()},
		{"非等级分组带了等级", func() villagerTradeRow {
			r := valid
			r.TradeGroup = domain.VillagerTradeGroupPurchase
			return r
		}()},
		{"成本为空", func() villagerTradeRow { r := valid; r.CostItems = nil; return r }()},
		{"成本数量为 0", func() villagerTradeRow {
			r := valid
			r.CostItems = []domain.VillagerCostItem{{ItemID: "minecraft:coal", NameZh: "煤炭", CountMin: 0}}
			return r
		}()},
		{"成本数量区间倒挂", func() villagerTradeRow {
			r := valid
			r.CostItems = []domain.VillagerCostItem{{ItemID: "minecraft:coal", NameZh: "煤炭", CountMin: 10, CountMax: 5}}
			return r
		}()},
		{"结果物品缺失", func() villagerTradeRow { r := valid; r.ResultItemID = " "; return r }()},
		{"缺少版本", func() villagerTradeRow { r := valid; r.SupportedVersions = nil; return r }()},
		{"负价格系数", func() villagerTradeRow {
			v := -1.0
			r := valid
			r.PriceMultiplier = &v
			return r
		}()},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := buildVillagerTrade(tc.row, known); err == nil {
				t.Fatal("期望校验失败，实际通过")
			}
		})
	}
}

// TestSeedFileVillagersAreValid 直接校验随仓库发布的种子数据文件：
// 任何一行非法（缺来源、缺版本、悬空职业引用、重复键）都会让本测试失败。
func TestSeedFileVillagersAreValid(t *testing.T) {
	raw, err := os.ReadFile("../../seeds/villagers_demo.json")
	if err != nil {
		t.Fatalf("读取种子文件失败: %v", err)
	}
	var file villagerSeedFile
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatalf("种子文件 JSON 解析失败: %v", err)
	}
	if len(file.Professions) == 0 || len(file.Levels) == 0 || len(file.Trades) == 0 {
		t.Fatalf("种子文件不应为空：professions=%d levels=%d trades=%d",
			len(file.Professions), len(file.Levels), len(file.Trades))
	}

	known := make(map[string]bool, len(file.Professions))
	for i, row := range file.Professions {
		prof, err := buildVillagerProfession(row)
		if err != nil {
			t.Fatalf("职业第 %d 行（%s）非法: %v", i+1, row.Slug, err)
		}
		if known[prof.Slug] {
			t.Fatalf("职业 slug 重复: %s", prof.Slug)
		}
		known[prof.Slug] = true
	}

	seenLevels := make(map[int]bool, len(file.Levels))
	for i, row := range file.Levels {
		if _, err := buildVillagerTradeLevel(row); err != nil {
			t.Fatalf("等级第 %d 行非法: %v", i+1, err)
		}
		if seenLevels[row.Level] {
			t.Fatalf("等级重复: %d", row.Level)
		}
		seenLevels[row.Level] = true
	}

	seenTrades := make(map[string]bool, len(file.Trades))
	for i, row := range file.Trades {
		trade, err := buildVillagerTrade(row, known)
		if err != nil {
			t.Fatalf("交易第 %d 行（%s）非法: %v", i+1, row.TradeKey, err)
		}
		if seenTrades[trade.TradeKey] {
			t.Fatalf("交易 trade_key 重复: %s", trade.TradeKey)
		}
		seenTrades[trade.TradeKey] = true
	}
}

// TestSeedFileVillagerTradesCoverSemantics 抽样校验关键数据的语义正确性，
// 避免种子文件被无意改坏（如大师交易丢等级、流浪商人被误标等级、附魔书版本漂移）。
func TestSeedFileVillagerTradesCoverSemantics(t *testing.T) {
	raw, err := os.ReadFile("../../seeds/villagers_demo.json")
	if err != nil {
		t.Fatalf("读取种子文件失败: %v", err)
	}
	var file villagerSeedFile
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatalf("JSON 解析失败: %v", err)
	}
	known := map[string]bool{}
	for _, row := range file.Professions {
		known[row.Slug] = true
	}
	byKey := make(map[string]domain.VillagerTrade, len(file.Trades))
	for _, row := range file.Trades {
		trade, err := buildVillagerTrade(row, known)
		if err != nil {
			t.Fatalf("交易 %s 非法: %v", row.TradeKey, err)
		}
		byKey[trade.TradeKey] = *trade
	}

	// 大师级盔甲匠出售附魔钻石胸甲，价格为浮动区间。
	chest, ok := byKey["armorer_l5_diamond_chestplate"]
	if !ok {
		t.Fatal("缺少大师级钻石胸甲交易")
	}
	if chest.Level != 5 || chest.CostItems[0].CountMin != 21 || chest.CostItems[0].CountMax != 35 {
		t.Errorf("钻石胸甲交易等级/价格区间不符: %+v", chest)
	}

	// 流浪商人交易不应带职业等级（level=0），分组为 purchase/special/ordinary。
	invis, ok := byKey["wt_special_invisibility"]
	if !ok {
		t.Fatal("缺少流浪商人隐身药水交易")
	}
	if invis.Level != 0 || invis.TradeGroup != domain.VillagerTradeGroupSpecial {
		t.Errorf("流浪商人交易应 level=0 且分组 special: %+v", invis)
	}

	// 图书管理员的附魔书交易覆盖 5–64 绿宝石价格区间。
	book, ok := byKey["librarian_l1_enchanted_book"]
	if !ok {
		t.Fatal("缺少图书管理员附魔书交易")
	}
	if book.CostItems[0].CountMin != 5 || book.CostItems[0].CountMax != 64 {
		t.Errorf("附魔书价格区间应为 5–64: %+v", book.CostItems[0])
	}

	// 试炼密室地图为 1.21+，与其余 1.14+ 交易区分。
	trial, ok := byKey["cartographer_l3_trial_map"]
	if !ok {
		t.Fatal("缺少试炼密室地图交易")
	}
	if len(trial.SupportedVersions) != 1 || trial.SupportedVersions[0] != "1.21+" {
		t.Errorf("试炼密室地图版本应为 1.21+，实际 %v", trial.SupportedVersions)
	}
}
