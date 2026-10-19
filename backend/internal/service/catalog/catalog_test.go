package catalog

import (
	"reflect"
	"testing"

	"mckit/backend/internal/domain"
	"mckit/backend/internal/repository/types"
)

// TestRecipeItems 汇总引用物品：结果物品 + 材料，去重且保持出现顺序（TODO §3）。
func TestRecipeItems(t *testing.T) {
	recipes := []domain.Recipe{
		{
			ResultItemID: "minecraft:iron_ingot",
			Ingredients:  types.JSONB(`[{"item_id":"minecraft:raw_iron","count":1}]`),
		},
		{
			ResultItemID: "minecraft:iron_ingot", // 与上一条结果重复，应只保留一次
			Ingredients: types.JSONB(`[{"item_id":"minecraft:coal","count":1},
				{"item_id":"minecraft:raw_iron","count":2}]`),
		},
	}

	got := RecipeItems(recipes)
	want := []string{
		"minecraft:iron_ingot",
		"minecraft:raw_iron",
		"minecraft:coal",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("RecipeItems=%v，期望 %v", got, want)
	}
}

// TestRecipeRefItems 单配方引用物品：用于把批量解析结果按配方切分。
func TestRecipeRefItems(t *testing.T) {
	r := domain.Recipe{
		ResultItemID: "minecraft:bread",
		Ingredients:  types.JSONB(`[{"item_id":"minecraft:wheat","count":3}]`),
	}
	want := []string{"minecraft:bread", "minecraft:wheat"}
	if got := RecipeRefItems(r); !reflect.DeepEqual(got, want) {
		t.Fatalf("RecipeRefItems=%v，期望 %v", got, want)
	}
}

// TestIngredientItemIDs 脏数据不应导致 panic：非法 JSON / 缺字段 / 空字段按「无材料」处理。
func TestIngredientItemIDs(t *testing.T) {
	cases := []struct {
		name string
		raw  types.JSONB
		want []string
	}{
		{"空值", nil, nil},
		{"非法 JSON", types.JSONB(`{oops`), nil},
		{"对象而非数组", types.JSONB(`{"item_id":"x"}`), nil},
		{"缺字段", types.JSONB(`[{"count":1}]`), nil},
		{"空 item_id 被跳过", types.JSONB(`[{"item_id":""},{"item_id":"a"}]`), []string{"a"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ingredientItemIDs(tc.raw)
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ingredientItemIDs=%v，期望 %v", got, tc.want)
			}
		})
	}
}

// TestRecipeItemsNoRefs 无引用的配方应返回空列表而非 nil（便于 JSON 序列化为 []）。
func TestRecipeItemsNoRefs(t *testing.T) {
	got := RecipeItems([]domain.Recipe{{}})
	if got == nil || len(got) != 0 {
		t.Fatalf("无引用配方应返回非 nil 空切片，实际 %v", got)
	}
}
