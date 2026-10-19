package main

import (
	"strings"
	"testing"

	"mckit/backend/internal/domain"
)

// TestBuildResourceNormalizesAndValidates 覆盖 TODO §6：资源导入行的校验与规范化。
func TestBuildResourceNormalizesAndValidates(t *testing.T) {
	valid := func() resourceRow {
		return resourceRow{
			Slug:              "jei",
			Name:              "JEI",
			Summary:           "物品配方查看",
			ResourceType:      domain.ResourceTypeMod,
			SourceURL:         "https://example.com/jei",
			Status:            domain.StatusPublished,
			SupportedVersions: []string{"1.21.1", "1.20.1"},
			LoaderTypes:       []string{"forge", "forge", "neoforge"},
			Dependencies:      []byte(`[{"name":"Forge","type":"prerequisite"}]`),
		}
	}

	res, releases, replace, err := buildResource(valid())
	if err != nil {
		t.Fatalf("合法行不应报错: %v", err)
	}
	if res.Slug != "jei" || res.Status != domain.StatusPublished {
		t.Fatalf("基础字段应保留，实际 %+v", res)
	}
	if res.GameEdition != domain.GameEditionJava {
		t.Errorf("game_edition 缺省应为 java，实际 %q", res.GameEdition)
	}
	if got := strings.Join(res.SupportedVersions, ","); got != "1.20.1,1.21.1" {
		t.Errorf("supported_versions 应升序去空，实际 %q", got)
	}
	if got := strings.Join(res.LoaderTypes, ","); got != "forge,neoforge" {
		t.Errorf("loader_types 应去重保序，实际 %q", got)
	}
	if !strings.Contains(string(res.Dependencies), `"type":"prerequisite"`) {
		t.Errorf("依赖 type 应被规范化保留，实际 %s", string(res.Dependencies))
	}
	if replace || len(releases) != 0 {
		t.Errorf("未提供 releases 字段时不应替换发布版本，实际 replace=%v len=%d", replace, len(releases))
	}
}

// TestBuildResourceReleasesPointerSemantics releases 指针区分「未提供」与「显式清空」。
func TestBuildResourceReleasesPointerSemantics(t *testing.T) {
	base := func() resourceRow {
		return resourceRow{
			Slug:         "jei",
			Name:         "JEI",
			Summary:      "查看配方",
			ResourceType: domain.ResourceTypeMod,
			SourceURL:    "https://example.com/jei",
			Status:       domain.StatusDraft,
		}
	}

	empty := []releaseRow{}
	row := base()
	row.Releases = &empty
	if _, releases, replace, err := buildResource(row); err != nil || !replace || len(releases) != 0 {
		t.Fatalf("显式空数组应表示清空，实际 replace=%v len=%d err=%v", replace, len(releases), err)
	}

	one := []releaseRow{{VersionLabel: "1.0", SourceURL: "https://example.com/dl"}}
	row = base()
	row.Releases = &one
	_, releases, replace, err := buildResource(row)
	if err != nil || !replace || len(releases) != 1 {
		t.Fatalf("提供发布版本应替换，实际 replace=%v len=%d err=%v", replace, len(releases), err)
	}
}

// TestBuildResourceRejectsInvalidRows 非法行应给出可定位的错误。
func TestBuildResourceRejectsInvalidRows(t *testing.T) {
	valid := func() resourceRow {
		return resourceRow{
			Slug:         "jei",
			Name:         "JEI",
			Summary:      "查看配方",
			ResourceType: domain.ResourceTypeMod,
			SourceURL:    "https://example.com/jei",
			Status:       domain.StatusDraft,
		}
	}

	cases := []struct {
		name   string
		mutate func(*resourceRow)
	}{
		{"缺少 slug", func(r *resourceRow) { r.Slug = " " }},
		{"缺少 name", func(r *resourceRow) { r.Name = "" }},
		{"缺少 summary", func(r *resourceRow) { r.Summary = "" }},
		{"缺少 source_url", func(r *resourceRow) { r.SourceURL = "" }},
		{"source_url 非 http", func(r *resourceRow) { r.SourceURL = "ftp://a/b" }},
		{"非法资源类型", func(r *resourceRow) { r.ResourceType = "unknown" }},
		{"非法状态", func(r *resourceRow) { r.Status = domain.StatusRemoved }},
		{"非法加载器", func(r *resourceRow) { r.LoaderTypes = []string{"bukkit"} }},
		{"非法版本", func(r *resourceRow) { r.SupportedVersions = []string{"v1"} }},
		{"非法依赖", func(r *resourceRow) { r.Dependencies = []byte(`[{"name":""}]`) }},
		{"非法发布版本", func(r *resourceRow) {
			bad := []releaseRow{{VersionLabel: "", SourceURL: "https://e.com/1"}}
			r.Releases = &bad
		}},
		{"发布版本缺少来源", func(r *resourceRow) {
			bad := []releaseRow{{VersionLabel: "1.0"}}
			r.Releases = &bad
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			row := valid()
			tc.mutate(&row)
			if _, _, _, err := buildResource(row); err == nil {
				t.Fatal("期望校验失败，实际通过")
			}
		})
	}
}

// TestBuildResourceDefaultsStatus status 缺省为 draft，避免误发布未核验资源。
func TestBuildResourceDefaultsStatus(t *testing.T) {
	row := resourceRow{
		Slug:         "x",
		Name:         "X",
		Summary:      "摘要",
		ResourceType: domain.ResourceTypeModpack,
		SourceURL:    "https://example.com/x",
	}
	res, _, _, err := buildResource(row)
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if res.Status != domain.StatusDraft {
		t.Fatalf("status 缺省应为 draft，实际 %q", res.Status)
	}
}
