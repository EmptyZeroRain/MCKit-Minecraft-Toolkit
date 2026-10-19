package resource

import (
	"encoding/json"
	"strings"
	"testing"

	"mckit/backend/internal/domain"
)

// TestNormalizeDependencies 覆盖 TODO §6：依赖 / 前置 Mod / 已知冲突的规范化与校验。
func TestNormalizeDependencies(t *testing.T) {
	cases := []struct {
		name     string
		raw      string
		wantErr  bool
		wantType string
	}{
		{"空值归一为空数组", ``, false, ""},
		{"null 归一为空数组", `null`, false, ""},
		{"合法依赖", `[{"name":"Fabric API","type":"dependency","resource_url":"https://modrinth.com/mod/fabric-api","required":true}]`, false, domain.DependencyTypeDependency},
		{"type 缺省补 dependency", `[{"name":"Forge"}]`, false, domain.DependencyTypeDependency},
		{"前置 Mod", `[{"name":"Forge","type":"prerequisite"}]`, false, domain.DependencyTypePrerequisite},
		{"已知冲突", `[{"name":"OptiFine","type":"conflict"}]`, false, domain.DependencyTypeConflict},
		{"缺少 name", `[{"type":"dependency"}]`, true, ""},
		{"name 全空白", `[{"name":"   "}]`, true, ""},
		{"非法 type", `[{"name":"X","type":"suggests"}]`, true, ""},
		{"非 http 链接", `[{"name":"X","resource_url":"ftp://a/b"}]`, true, ""},
		{"非数组", `{"name":"X"}`, true, ""},
		{"元素为字符串", `["X"]`, true, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := NormalizeDependencies(json.RawMessage(tc.raw))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("期望校验失败，实际通过：%s", string(out))
				}
				return
			}
			if err != nil {
				t.Fatalf("期望校验通过，实际: %v", err)
			}
			var deps []Dependency
			if err := json.Unmarshal(out, &deps); err != nil {
				t.Fatalf("输出不是合法 JSON 数组: %v", err)
			}
			if tc.wantType != "" {
				if len(deps) != 1 || deps[0].Type != tc.wantType {
					t.Fatalf("type 期望 %q，实际 %+v", tc.wantType, deps)
				}
			}
		})
	}
}

// TestNormalizeDependenciesLimit 依赖条数超限应被拒绝，避免超大 JSON。
func TestNormalizeDependenciesLimit(t *testing.T) {
	items := make([]Dependency, 0, maxDependencies+1)
	for i := 0; i <= maxDependencies; i++ {
		items = append(items, Dependency{Name: "dep"})
	}
	raw, _ := json.Marshal(items)
	if _, err := NormalizeDependencies(raw); err == nil {
		t.Fatal("超过上限应校验失败")
	}
}

// TestParseDependencies 脏数据按「无依赖」降级，且缺省 type 补齐为 dependency。
func TestParseDependencies(t *testing.T) {
	if got := ParseDependencies(json.RawMessage(`{bad json`)); got != nil {
		t.Fatalf("脏数据应返回 nil，实际 %+v", got)
	}
	got := ParseDependencies(json.RawMessage(`[{"name":"A"},{"name":"B","type":"conflict"}]`))
	if len(got) != 2 || got[0].Type != domain.DependencyTypeDependency || got[1].Type != domain.DependencyTypeConflict {
		t.Fatalf("缺省 type 应补齐为 dependency，实际 %+v", got)
	}
}

// TestNormalizeLoaderTypes 覆盖 TODO §6：加载器枚举校验与去重。
func TestNormalizeLoaderTypes(t *testing.T) {
	got, err := NormalizeLoaderTypes([]string{" forge ", "forge", "fabric", ""})
	if err != nil {
		t.Fatalf("合法输入不应报错: %v", err)
	}
	if len(got) != 2 || got[0] != domain.LoaderForge || got[1] != domain.LoaderFabric {
		t.Fatalf("期望去重后 [forge fabric]，实际 %v", got)
	}

	if _, err := NormalizeLoaderTypes([]string{"bukkit"}); err == nil {
		t.Fatal("未知加载器应校验失败")
	}
}

// TestNormalizeGameVersions 复用 versioning 规则：排序、去重、非法格式拦截。
func TestNormalizeGameVersions(t *testing.T) {
	got, err := NormalizeGameVersions([]string{"1.21.1", "1.20.1", " 1.21 "})
	if err != nil {
		t.Fatalf("合法版本不应报错: %v", err)
	}
	want := []string{"1.20.1", "1.21", "1.21.1"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("期望升序 %v，实际 %v", want, got)
	}

	if _, err := NormalizeGameVersions([]string{"1.20.1", "1.20.1"}); err == nil {
		t.Fatal("重复版本应校验失败")
	}
	if _, err := NormalizeGameVersions([]string{"not-a-version"}); err == nil {
		t.Fatal("非法版本应校验失败")
	}
}

// TestNormalizeGameVersionsAll 覆盖 TODO §6-C：「所有版本」通配的归一化与独占约束。
func TestNormalizeGameVersionsAll(t *testing.T) {
	got, err := NormalizeGameVersions([]string{" * "})
	if err != nil {
		t.Fatalf("通配声明应合法: %v", err)
	}
	if len(got) != 1 || got[0] != "all" {
		t.Fatalf("期望归一化为 [all]，实际 %v", got)
	}

	if _, err := NormalizeGameVersions([]string{"all", "1.20.1"}); err == nil {
		t.Fatal("通配与具体版本并列应校验失败")
	}
}

// TestNormalizeGameEdition 游戏版本类型校验（空值默认 java）。
func TestNormalizeGameEdition(t *testing.T) {
	if got, err := NormalizeGameEdition(""); err != nil || got != domain.GameEditionJava {
		t.Fatalf("空值应默认 java，实际 %q err=%v", got, err)
	}
	if got, err := NormalizeGameEdition("bedrock"); err != nil || got != domain.GameEditionBedrock {
		t.Fatalf("bedrock 应通过，实际 %q err=%v", got, err)
	}
	if _, err := NormalizeGameEdition("console"); err == nil {
		t.Fatal("未知版本类型应校验失败")
	}
}

// TestNormalizeReleases 覆盖 TODO §6：发布版本校验与排序。
func TestNormalizeReleases(t *testing.T) {
	base := func() []ReleaseInput {
		return []ReleaseInput{
			{
				VersionLabel: "15.2.0.51",
				GameVersions: []string{"1.20.1"},
				LoaderTypes:  []string{"forge"},
				SourceURL:    "https://example.com/jei/15",
				ReleasedAt:   "2023-11-05",
			},
		}
	}

	t.Run("合法并通过排序", func(t *testing.T) {
		items := []ReleaseInput{
			{VersionLabel: "old", SourceURL: "https://e.com/1", ReleasedAt: "2022-01-01"},
			{VersionLabel: "unknown", SourceURL: "https://e.com/2"},
			{VersionLabel: "new", SourceURL: "https://e.com/3", ReleasedAt: "2024-01-01", IsLatest: true},
		}
		got, err := NormalizeReleases(items)
		if err != nil {
			t.Fatalf("合法输入不应报错: %v", err)
		}
		want := "new,old,unknown"
		labels := make([]string, 0, len(got))
		for _, it := range got {
			labels = append(labels, it.VersionLabel)
		}
		if strings.Join(labels, ",") != want {
			t.Fatalf("排序期望 %s，实际 %s", want, strings.Join(labels, ","))
		}
	})

	cases := []struct {
		name    string
		mutate  func([]ReleaseInput) []ReleaseInput
		wantErr bool
	}{
		{"缺少 version_label", func(in []ReleaseInput) []ReleaseInput {
			in[0].VersionLabel = "  "
			return in
		}, true},
		{"重复 version_label", func(in []ReleaseInput) []ReleaseInput {
			return append(in, in[0])
		}, true},
		{"缺少 source_url", func(in []ReleaseInput) []ReleaseInput {
			in[0].SourceURL = ""
			return in
		}, true},
		{"source_url 非 http", func(in []ReleaseInput) []ReleaseInput {
			in[0].SourceURL = "javascript:alert(1)"
			return in
		}, true},
		{"非法游戏版本", func(in []ReleaseInput) []ReleaseInput {
			in[0].GameVersions = []string{"版本一"}
			return in
		}, true},
		{"非法加载器", func(in []ReleaseInput) []ReleaseInput {
			in[0].LoaderTypes = []string{"bukkit"}
			return in
		}, true},
		{"非法发布时间", func(in []ReleaseInput) []ReleaseInput {
			in[0].ReleasedAt = "2023/11/05"
			return in
		}, true},
		{"多条 is_latest", func(in []ReleaseInput) []ReleaseInput {
			in[0].IsLatest = true
			return append(in, ReleaseInput{VersionLabel: "b", SourceURL: "https://e.com/2", IsLatest: true})
		}, true},
		{"合法", func(in []ReleaseInput) []ReleaseInput { return in }, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NormalizeReleases(tc.mutate(base()))
			if tc.wantErr && err == nil {
				t.Fatal("期望校验失败，实际通过")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("期望校验通过，实际: %v", err)
			}
		})
	}
}

// TestNormalizeReleasesLimit 发布版本条数超限应被拒绝。
func TestNormalizeReleasesLimit(t *testing.T) {
	items := make([]ReleaseInput, 0, maxReleases+1)
	for i := 0; i <= maxReleases; i++ {
		items = append(items, ReleaseInput{VersionLabel: "v", SourceURL: "https://e.com"})
	}
	if _, err := NormalizeReleases(items); err == nil {
		t.Fatal("超过上限应校验失败")
	}
}

// TestParseReleaseDate 发布时间同时接受 RFC3339 与 YYYY-MM-DD。
func TestParseReleaseDate(t *testing.T) {
	for _, in := range []string{"2024-09-12", "2024-09-12T08:30:00Z", "2024-09-12T08:30:00+08:00"} {
		if _, err := ParseReleaseDate(in); err != nil {
			t.Errorf("%q 应可解析: %v", in, err)
		}
	}
	for _, in := range []string{"", "2024/09/12", "9月12日"} {
		if _, err := ParseReleaseDate(in); err == nil {
			t.Errorf("%q 不应解析成功", in)
		}
	}
}
