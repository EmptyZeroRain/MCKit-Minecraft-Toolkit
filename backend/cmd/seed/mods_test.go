package main

import (
	"encoding/json"
	"os"
	"testing"

	"mckit/backend/internal/domain"
)

// TestBuildModNormalizesAndValidates 覆盖 TODO §16：Mod 导入行的归一化与校验。
func TestBuildModNormalizesAndValidates(t *testing.T) {
	m, err := buildMod(modRow{
		Slug:         "  sodium ",
		NameZh:       " Sodium 渲染优化 ",
		Categories:   []string{"optimization"},
		Loaders:      []string{"fabric"},
		MCVersions:   []string{"1.20-1.21"},
		SourceURL:    " https://github.com/CaffeineMC/sodium ",
		License:      "LGPL-3.0",
		ReviewStatus: "published",
	})
	if err != nil {
		t.Fatalf("合法行不应报错: %v", err)
	}
	if m.Slug != "sodium" || m.NameZh != "Sodium 渲染优化" {
		t.Errorf("字段应被裁剪空格: %+v", m)
	}
	if m.GameEdition != "java" {
		t.Errorf("game_edition 缺省应为 java，实际 %q", m.GameEdition)
	}
	if m.SourceURL != "https://github.com/CaffeineMC/sodium" {
		t.Errorf("source_url 应被裁剪: %q", m.SourceURL)
	}
}

// TestBuildModRejectsBadRows 覆盖 Mod 必填与枚举校验。
func TestBuildModRejectsBadRows(t *testing.T) {
	base := func() modRow {
		return modRow{
			Slug: "x", NameZh: "X",
			Categories: []string{"optimization"}, Loaders: []string{"fabric"},
			MCVersions: []string{"1.20-1.21"}, SourceURL: "u",
		}
	}
	cases := []struct {
		name   string
		mutate func(r *modRow)
	}{
		{"缺少 slug", func(r *modRow) { r.Slug = "" }},
		{"缺少 name_zh", func(r *modRow) { r.NameZh = "" }},
		{"category 非法", func(r *modRow) { r.Categories = []string{"bogus"} }},
		{"loader 非法", func(r *modRow) { r.Loaders = []string{"liteloader"} }},
		{"版本非法", func(r *modRow) { r.MCVersions = []string{"not-a-version"} }},
		{"缺少 source_url", func(r *modRow) { r.SourceURL = "" }},
		{"review_status 非法", func(r *modRow) { r.ReviewStatus = "unknown" }},
		{"game_edition 非法", func(r *modRow) { r.GameEdition = "switch" }},
	}
	for _, tc := range cases {
		row := base()
		tc.mutate(&row)
		if _, err := buildMod(row); err == nil {
			t.Errorf("%s：应被拒绝", tc.name)
		}
	}
}

// TestBuildModVersionValidates 覆盖发布版本校验（含下载链接授权红线）。
func TestBuildModVersionValidates(t *testing.T) {
	v, err := buildModVersion(modVersionRow{
		Version:     "0.5.11",
		ReleaseType: "release",
		MCVersions:  []string{"1.20.1"},
		Loaders:     []string{"fabric"},
		DownloadURL: "https://modrinth.com/mod/sodium",
	}, "LGPL-3.0", "")
	if err != nil {
		t.Fatalf("合法版本不应报错: %v", err)
	}
	if v.ReleaseType != domain.ModReleaseTypeRelease {
		t.Errorf("release_type 未正确保留: %+v", v)
	}
	// release_type 缺省为 release。
	if def, err := buildModVersion(modVersionRow{Version: "1.0"}, "", ""); err != nil || def.ReleaseType != domain.ModReleaseTypeRelease {
		t.Errorf("release_type 缺省应为 release: %v %+v", err, def)
	}
	// 无授权说明时不允许给下载链接（不未经许可镜像分发）。
	if _, err := buildModVersion(modVersionRow{Version: "1.0", DownloadURL: "https://example.com/x.jar"}, "", ""); err == nil {
		t.Error("download_url 非空且无授权说明时应被拒绝")
	}
	// 有 license_note 说明时允许下载链接。
	if _, err := buildModVersion(modVersionRow{Version: "1.0", DownloadURL: "https://example.com/x.jar"}, "", "许可信息见官方仓库。"); err != nil {
		t.Errorf("有 license_note 时应允许下载链接: %v", err)
	}
	bad := []modVersionRow{
		{Version: ""},
		{Version: "1.0", ReleaseType: "bogus"},
		{Version: "1.0", MCVersions: []string{"not-a-version"}},
		{Version: "1.0", Loaders: []string{"bogus"}},
	}
	for i, row := range bad {
		if _, err := buildModVersion(row, "MIT", ""); err == nil {
			t.Errorf("非法版本第 %d 行应被拒绝: %+v", i+1, row)
		}
	}
}

// TestBuildModDependencyValidates 覆盖依赖 / 冲突校验。
func TestBuildModDependencyValidates(t *testing.T) {
	if _, err := buildModDependency(modDependencyRow{
		DepKind: domain.ModDependencyRequired, TargetModSlug: "fabric-api",
	}); err != nil {
		t.Fatalf("合法依赖不应报错: %v", err)
	}
	// 未收录对方 Mod 时回退 target_mod_name。
	if _, err := buildModDependency(modDependencyRow{
		DepKind: domain.ModDependencyRequired, TargetModName: "Sophisticated Core",
	}); err != nil {
		t.Fatalf("回退 target_mod_name 不应报错: %v", err)
	}
	bad := []struct {
		name string
		row  modDependencyRow
	}{
		{"dep_kind 非法", modDependencyRow{DepKind: "bogus", TargetModSlug: "x"}},
		{"缺少目标", modDependencyRow{DepKind: domain.ModDependencyRequired}},
		{"incompatible 缺 note", modDependencyRow{DepKind: domain.ModDependencyIncompatible, TargetModName: "OptiFine"}},
	}
	for _, tc := range bad {
		if _, err := buildModDependency(tc.row); err == nil {
			t.Errorf("%s：应被拒绝", tc.name)
		}
	}
	// incompatible 带 note 合法。
	if _, err := buildModDependency(modDependencyRow{
		DepKind: domain.ModDependencyIncompatible, TargetModName: "OptiFine", Note: "实现冲突。",
	}); err != nil {
		t.Errorf("incompatible 带 note 不应报错: %v", err)
	}
}

// TestBuildModpackValidates 覆盖整合包校验。
func TestBuildModpackValidates(t *testing.T) {
	if _, err := buildModpack(modpackRow{
		Slug: "pack", NameZh: "整合包", MCVersion: "1.20.1", Loader: "fabric",
		SourceURL: "u", ReviewStatus: "published",
	}); err != nil {
		t.Fatalf("合法整合包不应报错: %v", err)
	}
	bad := []struct {
		name string
		row  modpackRow
	}{
		{"缺少 slug", modpackRow{NameZh: "X", SourceURL: "u"}},
		{"缺少 source_url", modpackRow{Slug: "x", NameZh: "X"}},
		{"loader 非法", modpackRow{Slug: "x", NameZh: "X", SourceURL: "u", Loader: "bogus"}},
		{"mc_version 非法", modpackRow{Slug: "x", NameZh: "X", SourceURL: "u", MCVersion: "not-a-version"}},
		{"review_status 非法", modpackRow{Slug: "x", NameZh: "X", SourceURL: "u", ReviewStatus: "unknown"}},
	}
	for _, tc := range bad {
		if _, err := buildModpack(tc.row); err == nil {
			t.Errorf("%s：应被拒绝", tc.name)
		}
	}
}

// TestBuildModpackModValidates 覆盖整合包清单项校验。
func TestBuildModpackModValidates(t *testing.T) {
	if _, err := buildModpackMod(modpackModRow{ModSlug: "sodium"}); err != nil {
		t.Errorf("mod_slug 合法不应报错: %v", err)
	}
	if _, err := buildModpackMod(modpackModRow{ModName: "Curios API"}); err != nil {
		t.Errorf("mod_name 回退不应报错: %v", err)
	}
	if _, err := buildModpackMod(modpackModRow{}); err == nil {
		t.Error("既无 mod_slug 又无 mod_name 应被拒绝")
	}
}

// TestSeedFileModsIsValid 校验种子文件：全部行合法、slug 不重复、
// Mod ≥ 14 且每个 ≥ 2 个版本、整合包 ≥ 6 且每个 ≥ 6 项清单、类别与加载器覆盖充分。
func TestSeedFileModsIsValid(t *testing.T) {
	raw, err := os.ReadFile("../../seeds/mods_demo.json")
	if err != nil {
		t.Fatalf("读取种子文件失败: %v", err)
	}
	var file modsSeedFile
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatalf("种子文件 JSON 解析失败: %v", err)
	}
	if len(file.Mods) < 14 {
		t.Fatalf("Mod 数量应 ≥ 14，实际 %d", len(file.Mods))
	}
	if len(file.Modpacks) < 6 {
		t.Fatalf("整合包数量应 ≥ 6，实际 %d", len(file.Modpacks))
	}

	seen := map[string]bool{}
	categories := map[string]int{}
	loaders := map[string]int{}
	for i, row := range file.Mods {
		m, err := buildMod(row)
		if err != nil {
			t.Fatalf("Mod 第 %d 行（%s）非法: %v", i+1, row.Slug, err)
		}
		if seen[m.Slug] {
			t.Fatalf("Mod slug 重复: %s", m.Slug)
		}
		seen[m.Slug] = true
		if len(row.Versions) < 2 {
			t.Errorf("Mod %s 的发布版本应 ≥ 2，实际 %d", m.Slug, len(row.Versions))
		}
		for j, vrow := range row.Versions {
			if _, err := buildModVersion(vrow, m.License, m.LicenseNote); err != nil {
				t.Fatalf("Mod %s 版本第 %d 行非法: %v", m.Slug, j+1, err)
			}
		}
		for j, drow := range row.Dependencies {
			if _, err := buildModDependency(drow); err != nil {
				t.Fatalf("Mod %s 依赖第 %d 行非法: %v", m.Slug, j+1, err)
			}
		}
		for _, c := range m.Categories {
			categories[c]++
		}
		for _, l := range m.Loaders {
			loaders[l]++
		}
	}
	for _, want := range []string{
		domain.ModCategoryLibrary,
		domain.ModCategoryUI,
		domain.ModCategoryOptimization,
		domain.ModCategoryTechnology,
		domain.ModCategoryStorage,
		domain.ModCategoryWorld,
	} {
		if categories[want] == 0 {
			t.Errorf("种子数据应覆盖类别 %q", want)
		}
	}
	for _, want := range []string{domain.ModLoaderFabric, domain.ModLoaderForge, domain.ModLoaderNeoForge} {
		if loaders[want] == 0 {
			t.Errorf("种子数据应覆盖加载器 %q", want)
		}
	}

	seenPacks := map[string]bool{}
	for i, row := range file.Modpacks {
		pack, err := buildModpack(row)
		if err != nil {
			t.Fatalf("整合包第 %d 行（%s）非法: %v", i+1, row.Slug, err)
		}
		if seenPacks[pack.Slug] {
			t.Fatalf("整合包 slug 重复: %s", pack.Slug)
		}
		seenPacks[pack.Slug] = true
		if len(row.Mods) < 6 {
			t.Errorf("整合包 %s 的 Mod 清单应 ≥ 6 项，实际 %d", pack.Slug, len(row.Mods))
		}
		for j, mr := range row.Mods {
			if _, err := buildModpackMod(mr); err != nil {
				t.Fatalf("整合包 %s 清单第 %d 行非法: %v", pack.Slug, j+1, err)
			}
		}
	}
}
