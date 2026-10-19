package versioning

import (
	"strings"
	"testing"
)

// TestAllVersionsSpec 覆盖「所有版本」通配声明（TODO §6-C）。
func TestAllVersionsSpec(t *testing.T) {
	for _, raw := range []string{"all", "ALL", " all ", "*"} {
		spec, err := ParseSpec(raw)
		if err != nil {
			t.Fatalf("%q 应可解析: %v", raw, err)
		}
		if spec.Kind != KindAll {
			t.Fatalf("%q 应解析为 KindAll，实际 %q", raw, spec.Kind)
		}
		if spec.Raw != AllKeyword {
			t.Fatalf("%q 应归一化为 %q，实际 %q", raw, AllKeyword, spec.Raw)
		}
	}

	if _, err := ParseSpec("al"); err == nil {
		t.Fatal("非通配写法不应被接受")
	}
}

// TestCanonicalSpec 归一化：仅 all / * 变体，其余保持原文。
func TestCanonicalSpec(t *testing.T) {
	cases := map[string]string{
		"*":       AllKeyword,
		"all":     AllKeyword,
		"ALL":     AllKeyword,
		"  all  ": AllKeyword,
		"1.21.1":  "1.21.1",
		"1.18+":   "1.18+",
	}
	for in, want := range cases {
		if got := CanonicalSpec(in); got != want {
			t.Errorf("CanonicalSpec(%q) 期望 %q，实际 %q", in, want, got)
		}
	}
}

// TestIsAll 通配识别。
func TestIsAll(t *testing.T) {
	if !IsAll([]string{"1.20.1", "*"}) {
		t.Fatal("包含 * 应识别为通配")
	}
	if !IsAll([]string{AllKeyword}) {
		t.Fatal("all 应识别为通配")
	}
	if IsAll([]string{"1.20.1", "1.21"}) {
		t.Fatal("无通配时不应识别为通配")
	}
	if IsAll(nil) {
		t.Fatal("空列表不应识别为通配")
	}
}

// TestValidateAllExclusive 「所有版本」必须独占，与具体版本并列视为不合理。
func TestValidateAllExclusive(t *testing.T) {
	if errs := Validate([]string{AllKeyword}); len(errs) != 0 {
		t.Fatalf("单独声明 all 应合法，实际 %v", errs)
	}
	if errs := Validate([]string{"*", "1.20.1"}); len(errs) == 0 {
		t.Fatal("all 与具体版本并列应报错")
	}
	if errs := Validate([]string{"*", AllKeyword}); len(errs) == 0 {
		t.Fatal("all 与 * 互为重复，应报错")
	}
}

// TestValidateAllDuplicate all 重复出现应被判定为重复项。
func TestValidateAllDuplicate(t *testing.T) {
	errs := Validate([]string{AllKeyword, AllKeyword})
	if len(errs) == 0 {
		t.Fatal("all 重复应报错")
	}
	if !strings.Contains(errs[0].Error(), "重复") {
		t.Fatalf("应提示重复，实际 %v", errs[0])
	}
}

// TestSortAllFirst 「所有版本」最宽泛，排序应在最前，且输出归一化为 all。
func TestSortAllFirst(t *testing.T) {
	got := Sort([]string{"1.21.1", "*", "1.20.1"})
	want := []string{AllKeyword, "1.20.1", "1.21.1"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("期望 %v，实际 %v", want, got)
	}
}

// TestCompareSpecAll 通配在声明比较中最靠前。
func TestCompareSpecAll(t *testing.T) {
	all, _ := ParseSpec(AllKeyword)
	single, _ := ParseSpec("1.20.1")
	if CompareSpec(all, single) != -1 {
		t.Fatal("all 应排在单版本之前")
	}
	if CompareSpec(single, all) != 1 {
		t.Fatal("单版本应排在 all 之后")
	}
	if CompareSpec(all, all) != 0 {
		t.Fatal("同为 all 应相等")
	}
}
