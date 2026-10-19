package resource

import (
	"errors"
	"testing"

	"mckit/backend/internal/domain"
)

// TestValidateCreate 覆盖需求 §6.3 / §13.1：资源创建参数校验。
func TestValidateCreate(t *testing.T) {
	valid := func() *CreateInput {
		return &CreateInput{
			Slug:         "jei",
			Name:         "JEI",
			Summary:      "物品配方查询",
			ResourceType: domain.ResourceTypeMod,
			SourceURL:    "https://example.com/jei",
			Status:       domain.StatusPublished,
		}
	}

	cases := []struct {
		name    string
		mutate  func(*CreateInput)
		wantErr bool
	}{
		{"合法输入", func(*CreateInput) {}, false},
		{"缺少 slug", func(in *CreateInput) { in.Slug = "  " }, true},
		{"缺少 name", func(in *CreateInput) { in.Name = "" }, true},
		{"缺少 summary", func(in *CreateInput) { in.Summary = "" }, true},
		{"缺少 source_url", func(in *CreateInput) { in.SourceURL = "" }, true},
		{"非法资源类型", func(in *CreateInput) { in.ResourceType = "unknown" }, true},
		{"非法状态", func(in *CreateInput) { in.Status = "removed" }, true},
		{"状态为空非法", func(in *CreateInput) { in.Status = "" }, true},
		{"非 http 协议", func(in *CreateInput) { in.SourceURL = "ftp://example.com/x" }, true},
		{"javascript 协议", func(in *CreateInput) { in.SourceURL = "javascript:alert(1)" }, true},
		{"https 合法", func(in *CreateInput) { in.SourceURL = "https://example.com/x" }, false},
		{"hidden 允许", func(in *CreateInput) { in.Status = domain.StatusHidden }, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := valid()
			tc.mutate(in)
			err := validateCreate(in)
			if tc.wantErr && err == nil {
				t.Fatalf("期望校验失败，实际通过")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("期望校验通过，实际: %v", err)
			}
		})
	}
}

// TestValidStatus 校验资源状态白名单（removed 只能由删除接口写入）。
func TestValidStatus(t *testing.T) {
	cases := map[string]bool{
		domain.StatusDraft:     true,
		domain.StatusPublished: true,
		domain.StatusHidden:    true,
		domain.StatusRemoved:   false,
		"":                     false,
		"bogus":                false,
	}
	for status, want := range cases {
		if got := validStatus(status, true); got != want {
			t.Errorf("validStatus(%q, true)=%v，期望 %v", status, got, want)
		}
	}
}

// TestNormalizePage 覆盖需求 §8.1：分页边界（默认 20，最大 50）。
func TestNormalizePage(t *testing.T) {
	cases := []struct {
		page, size       int
		wantPage, wantSz int
	}{
		{0, 0, 1, 20},
		{-5, -1, 1, 20},
		{2, 100, 2, 50},
		{3, 20, 3, 20},
		{1, 1, 1, 1},
	}
	for _, tc := range cases {
		p, s := normalizePage(tc.page, tc.size)
		if p != tc.wantPage || s != tc.wantSz {
			t.Errorf("normalizePage(%d,%d)=(%d,%d)，期望 (%d,%d)",
				tc.page, tc.size, p, s, tc.wantPage, tc.wantSz)
		}
	}
}

// TestParseUUID 非 UUID 字符串安全降级为 nil，不 panic。
func TestParseUUID(t *testing.T) {
	if parseUUID("not-a-uuid") != nil {
		t.Fatal("非法 UUID 应返回 nil")
	}
	if parseUUID("") != nil {
		t.Fatal("空串应返回 nil")
	}
	if parseUUID("00000000-0000-0000-0000-000000000001") == nil {
		t.Fatal("合法 UUID 应解析成功")
	}
}

// TestErrSlugTakenSentinel 确认哨兵错误可被 errors.Is 命中（handler 依赖）。
func TestErrSlugTakenSentinel(t *testing.T) {
	if !errors.Is(ErrSlugTaken, ErrSlugTaken) {
		t.Fatal("ErrSlugTaken 哨兵应可用 errors.Is 判定")
	}
}
