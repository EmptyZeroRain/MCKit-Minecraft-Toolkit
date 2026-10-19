package resource

import (
	"testing"

	"mckit/backend/internal/domain"
	"mckit/backend/internal/repository/types"
)

// TestBuildCompatibility 覆盖 TODO §6：仅在有可靠数据依据时给出兼容性结论，
// 否则明确「未验证」，绝不把缺失数据当成「兼容」。
func TestBuildCompatibility(t *testing.T) {
	cases := []struct {
		name          string
		res           domain.Resource
		releases      []domain.ResourceRelease
		wantVerified  bool
		wantVersions  string
		wantLoaders   string
		wantSources   string
		wantHintEmpty bool
	}{
		{
			name:          "资源与发布均无数据 → 未验证",
			res:           domain.Resource{},
			wantVerified:  false,
			wantHintEmpty: false,
		},
		{
			name: "仅资源级字段有数据",
			res: domain.Resource{
				SupportedVersions: types.JSONSlice{"1.20.1"},
				LoaderTypes:       types.JSONSlice{"forge"},
			},
			wantVerified:  true,
			wantVersions:  "1.20.1",
			wantLoaders:   "forge",
			wantSources:   "resource",
			wantHintEmpty: true,
		},
		{
			name: "仅发布版本有数据",
			releases: []domain.ResourceRelease{
				{GameVersions: types.JSONSlice{"1.21.1"}, LoaderTypes: types.JSONSlice{"neoforge"}},
			},
			wantVerified:  true,
			wantVersions:  "1.21.1",
			wantLoaders:   "neoforge",
			wantSources:   "releases",
			wantHintEmpty: true,
		},
		{
			name: "资源 + 发布合并去重并排序",
			res: domain.Resource{
				SupportedVersions: types.JSONSlice{"1.20.1", "1.21.1"},
				LoaderTypes:       types.JSONSlice{"forge"},
			},
			releases: []domain.ResourceRelease{
				{GameVersions: types.JSONSlice{"1.20.1"}, LoaderTypes: types.JSONSlice{"neoforge", "forge"}},
			},
			wantVerified:  true,
			wantVersions:  "1.20.1,1.21.1",
			wantLoaders:   "forge,neoforge",
			wantSources:   "resource,releases",
			wantHintEmpty: true,
		},
		{
			name: "加载器按枚举顺序排列",
			res: domain.Resource{
				LoaderTypes: types.JSONSlice{"fabric", "forge", "quilt"},
			},
			wantVerified:  true,
			wantLoaders:   "forge,fabric,quilt",
			wantSources:   "resource",
			wantHintEmpty: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := BuildCompatibility(&tc.res, tc.releases)
			if got.Verified != tc.wantVerified {
				t.Fatalf("Verified 期望 %v，实际 %v", tc.wantVerified, got.Verified)
			}
			if join(got.GameVersions) != tc.wantVersions {
				t.Fatalf("GameVersions 期望 %q，实际 %q", tc.wantVersions, join(got.GameVersions))
			}
			if join(got.LoaderTypes) != tc.wantLoaders {
				t.Fatalf("LoaderTypes 期望 %q，实际 %q", tc.wantLoaders, join(got.LoaderTypes))
			}
			if join(got.Sources) != tc.wantSources {
				t.Fatalf("Sources 期望 %q，实际 %q", tc.wantSources, join(got.Sources))
			}
			if tc.wantHintEmpty && got.UnverifiedHint != "" {
				t.Fatalf("已验证时不应返回提示，实际 %q", got.UnverifiedHint)
			}
			if !tc.wantHintEmpty && got.UnverifiedHint == "" {
				t.Fatal("未验证时必须给出明确提示，避免被误读为兼容")
			}
		})
	}
}

// TestBuildCompatibilityKeepsNote 人工核验备注始终透传，便于解释结论例外。
func TestBuildCompatibilityKeepsNote(t *testing.T) {
	res := domain.Resource{CompatibilityNote: "1.21 起以 NeoForge 为准"}
	got := BuildCompatibility(&res, nil)
	if got.Note != res.CompatibilityNote {
		t.Fatalf("备注应透传，实际 %q", got.Note)
	}
}

// TestBuildCompatibilityNilSafeSlices 未验证时切片必须为非 nil 空切片，
// 否则 JSON 会序列化出 null，破坏前端契约。
func TestBuildCompatibilityNilSafeSlices(t *testing.T) {
	got := BuildCompatibility(&domain.Resource{}, nil)
	if got.GameVersions == nil || got.LoaderTypes == nil || got.Sources == nil {
		t.Fatalf("切片应为非 nil 空切片，实际 versions=%v loaders=%v sources=%v",
			got.GameVersions, got.LoaderTypes, got.Sources)
	}
	// 仅资源级字段时 Sources 有值，但 versions/loaders 仍需稳定
	got = BuildCompatibility(&domain.Resource{LoaderTypes: types.JSONSlice{"forge"}}, nil)
	if got.GameVersions == nil {
		t.Fatal("无版本数据时 GameVersions 应为非 nil 空切片")
	}
}

// TestBuildCompatibilityAllVersions 覆盖 TODO §6-C：声明「所有版本」时视为有数据依据，
// 并置 all_versions 供前端展开完整版本清单。
func TestBuildCompatibilityAllVersions(t *testing.T) {
	res := domain.Resource{SupportedVersions: types.JSONSlice{"all"}}
	got := BuildCompatibility(&res, nil)
	if !got.Verified {
		t.Fatal("声明「所有版本」应视为已验证")
	}
	if !got.AllVersions {
		t.Fatal("应置 all_versions=true")
	}
	if len(got.GameVersions) != 1 || got.GameVersions[0] != "all" {
		t.Fatalf("GameVersions 期望 [all]，实际 %v", got.GameVersions)
	}

	// 发布版本声明通配同样生效
	got = BuildCompatibility(&domain.Resource{}, []domain.ResourceRelease{
		{GameVersions: types.JSONSlice{"*"}},
	})
	if !got.AllVersions || !got.Verified {
		t.Fatalf("发布版本声明通配应生效，实际 %+v", got)
	}
	if got.GameVersions[0] != "all" {
		t.Fatalf("应归一化为 all，实际 %v", got.GameVersions)
	}

	// 未声明通配时为 false
	if BuildCompatibility(&domain.Resource{SupportedVersions: types.JSONSlice{"1.20.1"}}, nil).AllVersions {
		t.Fatal("未声明通配时 all_versions 应为 false")
	}
}

func join(in []string) string {
	out := ""
	for i, v := range in {
		if i > 0 {
			out += ","
		}
		out += v
	}
	return out
}
