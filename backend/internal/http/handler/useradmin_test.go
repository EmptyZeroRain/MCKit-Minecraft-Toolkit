package handler

import "testing"

// TestCSVSafe 验证 CSV 公式注入防护：以危险字符开头的值被前置单引号转义，
// 其余值原样返回（含空串）。
func TestCSVSafe(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"普通用户名", "player_01", "player_01"},
		{"空串", "", ""},
		{"等号开头", "=1+1", "'=1+1"},
		{"加号开头", "+SUM(A1)", "'+SUM(A1)"},
		{"减号开头", "-2+3", "'-2+3"},
		{"at 开头", "@cmd", "'@cmd"},
		{"制表符开头", "\tpayload", "'\tpayload"},
		{"回车开头", "\rpayload", "'\rpayload"},
		{"等号在中间", "a=b", "a=b"},
		{"中文开头", "玩家", "玩家"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := csvSafe(c.in); got != c.want {
				t.Fatalf("csvSafe(%q) = %q，期望 %q", c.in, got, c.want)
			}
		})
	}
}
