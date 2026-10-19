package searchstats

import "testing"

// TestNormalize 覆盖 TODO §5 隐私设计：只记录「有检索意义」的归一化关键词。
func TestNormalize(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"去首尾空白并转小写", "  Stone  ", "stone"},
		{"折叠内部空白与换行", "iron\t\n  ingot", "iron ingot"},
		{"保留中文", " 钻石镐 ", "钻石镐"},
		{"空输入丢弃", "   ", ""},
		{"纯符号丢弃", "!!!@@@", ""},
		{"纯 emoji 丢弃", "🍎🍌", ""},
		{"混合符号保留（含字母）", "C++", "c++"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Normalize(tc.raw); got != tc.want {
				t.Fatalf("Normalize(%q)=%q，期望 %q", tc.raw, got, tc.want)
			}
		})
	}
}

// TestNormalizeTooLong 超长关键词直接丢弃，避免写入异常数据。
func TestNormalizeTooLong(t *testing.T) {
	long := make([]rune, MaxTermLength+1)
	for i := range long {
		long[i] = 'a'
	}
	if got := Normalize(string(long)); got != "" {
		t.Fatalf("超长关键词应被丢弃，实际 %q", got)
	}
	// 恰好等于上限应保留。
	if got := Normalize(string(long[:MaxTermLength])); got == "" {
		t.Fatal("等于长度上限的关键词应保留")
	}
}

// TestEscapeLike 用户输入的通配符必须被转义，避免改变 LIKE 语义。
func TestEscapeLike(t *testing.T) {
	cases := map[string]string{
		"100%":       `100\%`,
		"a_b":        `a\_b`,
		`back\slash`: `back\\slash`,
		"plain":      "plain",
	}
	for in, want := range cases {
		if got := escapeLike(in); got != want {
			t.Fatalf("escapeLike(%q)=%q，期望 %q", in, got, want)
		}
	}
}
