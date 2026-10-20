package contentfilter

import "testing"

func TestNormalizeText(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"  外挂  ", "外挂"},
		{"BAD   Word", "bad word"},
		{"多\t行\n空白", "多 行 空白"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := normalizeText(tc.in); got != tc.want {
			t.Fatalf("normalizeText(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeInput(t *testing.T) {
	if _, _, err := normalizeInput("   ", ""); err == nil {
		t.Fatal("空白关键字应被拒绝")
	}
	if _, _, err := normalizeInput("正常", string(make([]rune, 256))); err == nil {
		t.Fatal("超长备注应被拒绝")
	}
	keyword, normalized, err := normalizeInput("  Bad   Word  ", "备注")
	if err != nil {
		t.Fatalf("合法输入不应报错: %v", err)
	}
	if keyword != "Bad   Word" || normalized != "bad word" {
		t.Fatalf("归一化结果异常: keyword=%q normalized=%q", keyword, normalized)
	}
}
