package blockids

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sample = `说明文字第一行，没有 tab。
默认	该方块具有ID相同物品形式。
图标	方块	命名空间ID	物品形式
金合欢木按钮	acacia_button	ID相同
甜菜根	beetroots	ID不同（beetroot）
墙上的金合欢木告示牌	acacia_wall_sign	物品的放置方式变种（acacia_sign）
插上黑色蜡烛的蛋糕	black_candle_cake	叠加型方块（cake）
空气	air	等同于空物品槽
火	fire	没有物品形式
未知格式条目	weird_form	某种奇怪的描述
	acacia_wood	ID相同
物品	ID
白色坐垫[新增：JE 26.3]	white_cushion
凋灵刷怪蛋	wither_spawn_egg
`

func TestSaveLoadJSON(t *testing.T) {
	entries := Parse([]byte(sample))
	path := filepath.Join(t.TempDir(), "item.json")

	if err := SaveJSON(entries, path, "物品id.md"); err != nil {
		t.Fatalf("SaveJSON 失败: %v", err)
	}

	// 落盘文件应为合法 JSON 且包含元信息。
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"generatedAt"`, `"total"`, `"entries"`, "white_cushion"} {
		if !strings.Contains(string(raw), key) {
			t.Errorf("item.json 应包含 %s", key)
		}
	}

	// 往返一致。
	back, err := LoadJSON(path)
	if err != nil {
		t.Fatalf("LoadJSON 失败: %v", err)
	}
	if len(back) != len(entries) {
		t.Fatalf("往返后条目数不符: %d vs %d", len(back), len(entries))
	}
	for i := range entries {
		if back[i] != entries[i] {
			t.Errorf("第 %d 条往返不一致: %+v vs %+v", i, back[i], entries[i])
		}
	}
}

func TestParse(t *testing.T) {
	entries := Parse([]byte(sample))

	// 方块条目 8 条 + 物品条目 2 条；说明行与两个表头均被跳过。
	if len(entries) != 10 {
		t.Fatalf("应解析出 10 条，实际 %d: %+v", len(entries), entries)
	}

	byID := map[string]Entry{}
	for _, e := range entries {
		byID[e.ID] = e
	}

	if e := byID["acacia_button"]; e.Kind != KindSame || e.Name != "金合欢木按钮" {
		t.Errorf("acacia_button 分类应为 same: %+v", e)
	}
	if e := byID["beetroots"]; e.Kind != KindDifferent || e.RefID != "beetroot" {
		t.Errorf("beetroots 应为 different 并提取引用: %+v", e)
	}
	if e := byID["acacia_wall_sign"]; e.Kind != KindVariant || e.RefID != "acacia_sign" {
		t.Errorf("acacia_wall_sign 应为 variant: %+v", e)
	}
	if e := byID["black_candle_cake"]; e.Kind != KindOverlay || e.RefID != "cake" {
		t.Errorf("black_candle_cake 应为 overlay: %+v", e)
	}
	if e := byID["air"]; e.Kind != KindSpecial {
		t.Errorf("air 应为 special: %+v", e)
	}
	if e := byID["fire"]; e.Kind != KindNone {
		t.Errorf("fire 应为 none: %+v", e)
	}
	if e := byID["acacia_wood"]; e.Name != "" || e.Kind != KindSame {
		t.Errorf("空中文名行应正常解析: %+v", e)
	}
	if e := byID["weird_form"]; e.Kind != KindOther || e.RefID != "" {
		t.Errorf("无法识别的物品形式应归入 other: %+v", e)
	}
	if e := byID["white_cushion"]; e.Kind != KindItem || e.Form != "" || !strings.Contains(e.Name, "[新增：JE 26.3]") {
		t.Errorf("物品子表条目应为 item: %+v", e)
	}
	if e := byID["wither_spawn_egg"]; e.Kind != KindItem {
		t.Errorf("刷怪蛋应为 item: %+v", e)
	}
}

func TestClassify(t *testing.T) {
	cases := map[string]string{
		"ID相同":                     KindSame,
		"ID不同（lava_bucket）":        KindDifferent,
		"没有物品形式":                   KindNone,
		"等同于空物品槽":                  KindSpecial,
		"叠加型方块（cauldron）":          KindOverlay,
		"物品的生长阶段变种（pumpkin_seeds）": KindVariant,
		"": KindOther,
	}
	for form, want := range cases {
		if got := classify(form); got != want {
			t.Errorf("classify(%q) 期望 %s，实际 %s", form, want, got)
		}
	}
}
