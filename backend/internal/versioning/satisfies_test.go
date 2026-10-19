package versioning

import "testing"

// TestSatisfies 覆盖 TODO §9A：药水/酿造资料按游戏版本筛选的精确语义。
func TestSatisfies(t *testing.T) {
	cases := []struct {
		name     string
		versions []string
		target   string
		want     bool
	}{
		// 「自某版本起」（开上界）声明
		{"1.20 命中 1.8+", []string{"1.8+"}, "1.20", true},
		{"1.8 命中 1.8+", []string{"1.8+"}, "1.8", true},
		{"1.8.9 命中 1.8+", []string{"1.8+"}, "1.8.9", true},
		{"1.21.1 命中 1.8+", []string{"1.8+"}, "1.21.1", true},
		{"1.7.10 不命中 1.8+", []string{"1.8+"}, "1.7.10", false},
		{"1.20 不命中 1.21+", []string{"1.21+"}, "1.20", false},
		{"1.21 命中 1.21+", []string{"1.21+"}, "1.21", true},
		{"1.0 命中 1.0+", []string{"1.0+"}, "1.0", true},
		{"1.20 命中 1.0+", []string{"1.0+"}, "1.20", true},

		// 闭区间声明
		{"区间内命中", []string{"1.16-1.17"}, "1.16", true},
		{"区间上界命中", []string{"1.16-1.17"}, "1.17", true},
		{"区间外不命中", []string{"1.16-1.17"}, "1.18", false},

		// 单版本声明：同主版本线即命中
		{"同主版本线命中", []string{"1.20.1"}, "1.20", true},
		{"单版本完全一致命中", []string{"1.20.1"}, "1.20.1", true},
		{"不同主版本不命中", []string{"1.20.1"}, "1.21", false},

		// 通配声明
		{"all 声明命中任意具体版本", []string{"all"}, "1.20", true},
		{"all 目标仅命中 all 声明", []string{"all"}, "all", true},
		{"all 目标不命中具体声明", []string{"1.20"}, "all", false},

		// 边界与脏数据
		{"空声明不命中", []string{}, "1.20", false},
		{"空目标不命中", []string{"1.8+"}, "", false},
		{"无法解析的目标不命中", []string{"1.8+"}, "最新版", false},
		{"非法声明被跳过", []string{"坏版本", "1.8+"}, "1.20", true},
		{"多项声明任一命中即通过", []string{"1.8-1.12", "1.20+"}, "1.20", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Satisfies(tc.versions, tc.target); got != tc.want {
				t.Errorf("Satisfies(%v, %q) = %v，期望 %v", tc.versions, tc.target, got, tc.want)
			}
		})
	}
}
