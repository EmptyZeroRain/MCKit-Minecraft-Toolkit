package versioning

import (
	"reflect"
	"testing"
)

// TestParse 覆盖各类版本号的解析（表驱动）。
func TestParse(t *testing.T) {
	cases := []struct {
		raw   string
		ok    bool
		kind  Kind
		major int
		minor int
		patch int
		stage string
	}{
		{"1.21", true, KindRelease, 1, 21, 0, ""},
		{"1.21.1", true, KindRelease, 1, 21, 1, ""},
		{"1.20", true, KindRelease, 1, 20, 0, ""},
		{"1.21.0", true, KindRelease, 1, 21, 0, ""},
		{"1.21-pre1", true, KindPre, 1, 21, 0, "pre"},
		{"1.21.1-rc2", true, KindRC, 1, 21, 1, "rc"},
		{"24w14a", true, KindSnapshot, 0, 0, 0, ""},
		{"", false, "", 0, 0, 0, ""},
		{"最新版", false, "", 0, 0, 0, ""},
		{"1.21.1.2", false, "", 0, 0, 0, ""},
		{"24w14", false, "", 0, 0, 0, ""},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			v, err := Parse(tc.raw)
			if tc.ok != (err == nil) {
				t.Fatalf("Parse(%q) err=%v，期望 ok=%v", tc.raw, err, tc.ok)
			}
			if !tc.ok {
				return
			}
			if v.Kind != tc.kind || v.Major != tc.major || v.Minor != tc.minor || v.Patch != tc.patch || v.Stage != tc.stage {
				t.Fatalf("Parse(%q)=%+v，与期望不符", tc.raw, v)
			}
		})
	}
}

// TestCompareOrdering 覆盖排序语义：补丁 > 正式 > 候选 > 预发布；快照恒小于正式版。
func TestCompareOrdering(t *testing.T) {
	ordered := []string{
		"1.20",
		"1.21-pre1",
		"1.21-pre2",
		"1.21-rc1",
		"1.21",
		"1.21.1-pre1",
		"1.21.1",
	}
	for i := 0; i < len(ordered)-1; i++ {
		a, err := Parse(ordered[i])
		if err != nil {
			t.Fatalf("解析 %q 失败: %v", ordered[i], err)
		}
		b, err := Parse(ordered[i+1])
		if err != nil {
			t.Fatalf("解析 %q 失败: %v", ordered[i+1], err)
		}
		if Compare(a, b) >= 0 {
			t.Errorf("期望 %s < %s", ordered[i], ordered[i+1])
		}
	}

	// 同族自反
	a, _ := Parse("1.21.1")
	if Compare(a, a) != 0 {
		t.Error("同版本比较应为 0")
	}
}

// TestValidate 覆盖格式非法与重复检测。
func TestValidate(t *testing.T) {
	if errs := Validate([]string{"1.21", "1.21.1", "24w14a"}); len(errs) != 0 {
		t.Fatalf("合法列表不应报错: %v", errs)
	}
	if errs := Validate(nil); len(errs) != 0 {
		t.Fatalf("空列表不报错: %v", errs)
	}
	// 范围声明（TODO §2「适用版本范围」）
	if errs := Validate([]string{"1.8-1.12", "1.13-1.15", "1.16-1.17", "1.18+"}); len(errs) != 0 {
		t.Fatalf("合法范围不应报错: %v", errs)
	}

	errs := Validate([]string{"1.21", "1.21", "x"})
	if len(errs) != 2 {
		t.Fatalf("期望 2 个问题（重复 + 非法），实际 %d：%v", len(errs), errs)
	}
}

// TestParseSpec 覆盖单版本与范围声明解析，含「起点大于终点」的不合理范围。
func TestParseSpec(t *testing.T) {
	s, err := ParseSpec("1.21")
	if err != nil || s.Kind == KindRange {
		t.Fatalf("单版本应解析为单版本: %+v, %v", s, err)
	}

	s, err = ParseSpec("1.18+")
	if err != nil {
		t.Fatalf("开上界范围应可解析: %v", err)
	}
	if s.Kind != KindRange || !s.OpenMax {
		t.Errorf("1.18+ 应解析为开上界范围: %+v", s)
	}

	s, err = ParseSpec("1.16-1.17")
	if err != nil {
		t.Fatalf("闭区间应可解析: %v", err)
	}
	if s.Kind != KindRange || s.OpenMax || Compare(s.Min, s.Max) > 0 {
		t.Errorf("1.16-1.17 解析异常: %+v", s)
	}

	// 预发布含连字符，必须优先按单版本解析
	if s, err := ParseSpec("1.21-pre1"); err != nil || s.Kind == KindRange {
		t.Errorf("1.21-pre1 应解析为单版本: %+v, %v", s, err)
	}

	// 不合理范围必须报错
	if _, err := ParseSpec("1.17-1.16"); err == nil {
		t.Error("起点大于终点应报错")
	}
	if _, err := ParseSpec("v1-v2"); err == nil {
		t.Error("非法范围应报错")
	}
}

// TestSort 覆盖排序：正式版升序、非法项在末位且不丢失。
func TestSort(t *testing.T) {
	got := Sort([]string{"1.21.1", "24w14a", "1.21", "脏数据", "1.20"})
	want := []string{"1.20", "1.21", "1.21.1", "24w14a", "脏数据"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Sort=%v，期望 %v", got, want)
	}

	// 范围声明按下界升序
	got = Sort([]string{"1.18+", "1.13-1.15", "1.16-1.17", "1.8-1.12"})
	want = []string{"1.8-1.12", "1.13-1.15", "1.16-1.17", "1.18+"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("范围排序=%v，期望 %v", got, want)
	}
}
