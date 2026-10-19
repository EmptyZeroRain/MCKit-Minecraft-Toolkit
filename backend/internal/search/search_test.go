package search

import "testing"

// TestRelevance 覆盖 TODO §5「优化中文名、英文名、命名空间 ID 和别名的搜索权重」。
// 权重梯度：标题 > 命名空间 ID > 别名 > slug > 摘要 > 兜底。
func TestRelevance(t *testing.T) {
	cases := []struct {
		name      string
		q         string
		in        rankInput
		wantScore int
	}{
		{"标题精确", "stone", rankInput{Title: "stone"}, 100},
		{"大小写不敏感精确", "Stone", rankInput{Title: "stone"}, 100},
		{"标题前缀", "sto", rankInput{Title: "stone"}, 80},
		{"标题包含", "ton", rankInput{Title: "stone"}, 60},
		{"命名空间 ID 精确", "minecraft:stone", rankInput{Title: "石头", Identifier: "minecraft:stone"}, 95},
		{"命名空间 ID 包含", "stone", rankInput{Title: "石头", Identifier: "minecraft:stone"}, 70},
		{"别名精确", "石头", rankInput{Title: "钻石", Aliases: []string{"石头"}}, 75},
		{"英文名作为别名精确", "Stone", rankInput{Title: "石头", Aliases: []string{"stone"}}, 75},
		{"别名包含", "diamond", rankInput{Title: "钻石镐", Aliases: []string{"diamond hoe"}}, 55},
		{"slug 命中", "st", rankInput{Title: "aaaa", Slug: "stone-slug"}, 45},
		{"摘要命中", "采矿", rankInput{Title: "镐子", Summary: "用于采矿"}, 20},
		{"兜底", "zzz", rankInput{Title: "stone", Summary: "desc"}, 5},
		{"空关键词", "  ", rankInput{Title: "stone"}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := relevance(tc.q, tc.in); got != tc.wantScore {
				t.Fatalf("relevance=%d，期望 %d", got, tc.wantScore)
			}
		})
	}
}

// TestRankItems 相关度降序，同分按标题升序（TODO §5 排序回归）。
func TestRankItems(t *testing.T) {
	items := []ResultItem{
		{Title: "b", score: 50},
		{Title: "a", score: 50},
		{Title: "c", score: 90},
		{Title: "d", score: 10},
	}
	rankItems(items)
	want := []string{"c", "a", "b", "d"}
	for i, w := range want {
		if items[i].Title != w {
			t.Fatalf("排序后第 %d 位应为 %s，实际 %s（全序：%v）", i, w, items[i].Title, titles(items))
		}
	}
}

// titles 提取标题，便于断言输出。
func titles(items []ResultItem) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.Title)
	}
	return out
}

// TestPageSlice 分页切片：正常页、越界页、末页截断（TODO §5 分页回归）。
func TestPageSlice(t *testing.T) {
	items := []ResultItem{{Title: "1"}, {Title: "2"}, {Title: "3"}, {Title: "4"}, {Title: "5"}}

	got := pageSlice(items, 1, 2)
	if len(got) != 2 || got[0].Title != "1" || got[1].Title != "2" {
		t.Fatalf("第 1 页应为 [1 2]，实际 %v", titles(got))
	}
	if got = pageSlice(items, 3, 2); len(got) != 1 || got[0].Title != "5" {
		t.Fatalf("第 3 页应截断为 [5]，实际 %v", titles(got))
	}
	if got = pageSlice(items, 9, 2); got == nil || len(got) != 0 {
		t.Fatalf("越界页应返回非 nil 空切片，实际 %v", got)
	}
	if got = pageSlice(items, 2, 2); got[0].Title != "3" || got[1].Title != "4" {
		t.Fatalf("第 2 页应为 [3 4]，实际 %v", titles(got))
	}
}

// TestSearchNormalizePage 覆盖需求 §8.1：page_size 最大 50。
func TestSearchNormalizePage(t *testing.T) {
	if p, s := normalizePage(0, 0); p != 1 || s != 20 {
		t.Fatalf("默认应为 (1,20)，实际 (%d,%d)", p, s)
	}
	if _, s := normalizePage(1, 100); s != 50 {
		t.Fatalf("page_size 上限应为 50，实际 %d", s)
	}
	if p, s := normalizePage(-3, -9); p != 1 || s != 20 {
		t.Fatalf("负数应回退默认，实际 (%d,%d)", p, s)
	}
}

// TestSearchEmptyQuery 空关键词不触库：nil db 也不应 panic。
func TestSearchEmptyQuery(t *testing.T) {
	svc := NewService(nil)
	res, err := svc.Search(nil, Filter{Q: "   "})
	if err != nil {
		t.Fatalf("空关键词不应报错: %v", err)
	}
	if res.Total != 0 || len(res.Items) != 0 {
		t.Fatalf("空关键词应返回空结果，实际 total=%d items=%d", res.Total, len(res.Items))
	}
	if res.Page != 1 || res.PageSize != 20 {
		t.Fatalf("空关键词也应返回规范化分页，实际 (%d,%d)", res.Page, res.PageSize)
	}
}

// TestFullTextMatch 组合条件：tsvector 命中优先参数在前，随后是 ILIKE 兜底参数。
func TestFullTextMatch(t *testing.T) {
	got := fullTextMatch("a ILIKE ? OR b ILIKE ?")
	want := "search_vector @@ plainto_tsquery('simple', ?) OR (a ILIKE ? OR b ILIKE ?)"
	if got != want {
		t.Fatalf("fullTextMatch 片段不符:\n got=%q\nwant=%q", got, want)
	}
}
