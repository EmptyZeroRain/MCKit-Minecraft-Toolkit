package content

import (
	"errors"
	"strings"
	"testing"

	"mckit/backend/internal/domain"
)

// TestValidateTool 覆盖需求 §5.2 / §13.1：工具创建参数校验。
func TestValidateTool(t *testing.T) {
	valid := func() *ToolInput {
		return &ToolInput{
			Slug:     "coordinate",
			Name:     "坐标换算",
			Summary:  "主世界与下界坐标换算",
			Category: domain.ToolCategoryCalculation,
		}
	}
	cases := []struct {
		name    string
		mutate  func(*ToolInput)
		wantErr bool
	}{
		{"合法", func(*ToolInput) {}, false},
		{"缺少 slug", func(in *ToolInput) { in.Slug = " " }, true},
		{"缺少 name", func(in *ToolInput) { in.Name = "" }, true},
		{"缺少 summary", func(in *ToolInput) { in.Summary = "" }, true},
		{"非法分类", func(in *ToolInput) { in.Category = "nope" }, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := valid()
			tc.mutate(in)
			err := validateTool(in)
			if tc.wantErr && err == nil {
				t.Fatal("期望校验失败")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("期望通过，实际 %v", err)
			}
			if tc.wantErr && tc.name == "非法分类" && !errors.Is(err, ErrBadEnum) {
				t.Fatalf("非法分类应返回 ErrBadEnum，实际 %v", err)
			}
		})
	}
}

// TestToolCategoryAndStatus 工具分类与状态白名单。
func TestToolCategoryAndStatus(t *testing.T) {
	for _, c := range []string{
		domain.ToolCategoryQuery, domain.ToolCategoryCalculation,
		domain.ToolCategoryGeneration, domain.ToolCategoryEditing,
		domain.ToolCategoryAuxiliary,
	} {
		if !validToolCategory(c) {
			t.Errorf("分类 %q 应为合法", c)
		}
	}
	if validToolCategory("query") {
		t.Error("英文旧枚举不应再合法（迁移后为中文）")
	}
	if validToolStatus(domain.StatusRemoved) {
		t.Error("removed 不应由工具状态白名单允许")
	}
	if !validToolStatus(domain.StatusHidden) {
		t.Error("hidden 应为合法工具状态")
	}
}

// TestWikiStatus 百科状态白名单。
func TestWikiStatus(t *testing.T) {
	if !validWikiStatus(domain.StatusDraft) || !validWikiStatus(domain.StatusPublished) {
		t.Fatal("draft/published 应为合法百科状态")
	}
	if validWikiStatus(domain.StatusRemoved) {
		t.Fatal("removed 不应由白名单允许")
	}
}

// TestValidateFeedback 覆盖需求 §5.10 / §13.1：反馈长度与枚举校验。
func TestValidateFeedback(t *testing.T) {
	cases := []struct {
		name    string
		fb      *domain.Feedback
		wantErr bool
	}{
		{"合法", &domain.Feedback{FeedbackType: "bug", Title: "标题", Content: "内容"}, false},
		{"缺少标题", &domain.Feedback{FeedbackType: "bug", Title: " ", Content: "内容"}, true},
		{"缺少内容", &domain.Feedback{FeedbackType: "bug", Title: "标题", Content: ""}, true},
		{"非法类型", &domain.Feedback{FeedbackType: "spam", Title: "标题", Content: "内容"}, true},
		{"标题超长", &domain.Feedback{FeedbackType: "bug", Title: strings.Repeat("字", 201), Content: "内容"}, true},
		{"内容超长", &domain.Feedback{FeedbackType: "bug", Title: "标题", Content: strings.Repeat("字", 5001)}, true},
		{"标题边界 200", &domain.Feedback{FeedbackType: "other", Title: strings.Repeat("字", 200), Content: "内容"}, false},
		{"内容边界 5000", &domain.Feedback{FeedbackType: "other", Title: "标题", Content: strings.Repeat("字", 5000)}, false},
		{"资源失效类型", &domain.Feedback{FeedbackType: "resource_broken", Title: "标题", Content: "内容"}, false},
		{"侵权类型", &domain.Feedback{FeedbackType: "infringement", Title: "标题", Content: "内容"}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateFeedback(tc.fb)
			if tc.wantErr && err == nil {
				t.Fatal("期望校验失败")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("期望校验通过，实际 %v", err)
			}
			if tc.name == "非法类型" && !errors.Is(err, ErrBadEnum) {
				t.Fatalf("非法类型应返回 ErrBadEnum，实际 %v", err)
			}
		})
	}
}

// TestSlugify 分类/标签 slug 归一化。
func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Hello World": "hello-world",
		"  Trim Me  ": "trim-me",
		"中文 name":     "name", // 非 ASCII 折叠为连字符并去首尾
		"a---b":       "a---b",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q)=%q，期望 %q", in, got, want)
		}
	}
}

// TestContentNormalizePage 内容服务分页边界。
func TestContentNormalizePage(t *testing.T) {
	if p, s := normalizePage(0, 0); p != 1 || s != 20 {
		t.Fatalf("默认应为 (1,20)，实际 (%d,%d)", p, s)
	}
	if _, s := normalizePage(1, 999); s != 50 {
		t.Fatalf("page_size 上限应为 50，实际 %d", s)
	}
}

// TestValidateFeedbackSubmission 覆盖 TODO §17：投稿类型、关联资料、授权条款与恶意链接校验。
func TestValidateFeedbackSubmission(t *testing.T) {
	correction := func(mutate func(*domain.Feedback)) *domain.Feedback {
		fb := &domain.Feedback{
			FeedbackType:   domain.FeedbackTypeDataCorrection,
			Title:          "牛的生命值有误",
			Content:        "应为 10 而非 20。",
			TargetKind:     domain.FeedbackTargetMob,
			TargetRef:      "cow",
			LicenseVersion: domain.FeedbackLicenseVersion,
		}
		if mutate != nil {
			mutate(fb)
		}
		return fb
	}

	cases := []struct {
		name    string
		fb      *domain.Feedback
		wantErr bool
	}{
		{"资料纠错合法", correction(nil), false},
		{"资源补充合法", &domain.Feedback{
			FeedbackType:   domain.FeedbackTypeResourceSupplement,
			Title:          "补充来源",
			Content:        "官方仓库地址。",
			TargetKind:     domain.FeedbackTargetResource,
			TargetRef:      "some-resource",
			LicenseVersion: domain.FeedbackLicenseVersion,
		}, false},
		{"纠错未接受条款", correction(func(fb *domain.Feedback) { fb.LicenseVersion = "" }), true},
		{"纠错条款版本过期", correction(func(fb *domain.Feedback) { fb.LicenseVersion = "2020-01-01" }), true},
		{"举报无需条款", &domain.Feedback{FeedbackType: domain.FeedbackTypeBug, Title: "标题", Content: "内容"}, false},
		{"关联类型非法", correction(func(fb *domain.Feedback) { fb.TargetKind = "spaceship" }), true},
		{"关联类型缺省补 other", &domain.Feedback{FeedbackType: domain.FeedbackTypeBug, Title: "标题", Content: "内容"}, false},
		{"关联标识超长", correction(func(fb *domain.Feedback) { fb.TargetRef = strings.Repeat("a", 201) }), true},
		{"链接为脚本伪协议", correction(func(fb *domain.Feedback) { fb.RelatedURL = "javascript:alert(1)" }), true},
		{"链接指向本机", correction(func(fb *domain.Feedback) { fb.RelatedURL = "http://localhost:8080/x" }), true},
		{"链接指向内网 IP", correction(func(fb *domain.Feedback) { fb.RelatedURL = "http://192.168.1.10/x" }), true},
		{"链接指向环回 IP", correction(func(fb *domain.Feedback) { fb.RelatedURL = "http://127.0.0.1/x" }), true},
		{"链接为合法外链", correction(func(fb *domain.Feedback) { fb.RelatedURL = "https://minecraft.wiki/w/Cow" }), false},
		{"链接为空可接受", correction(func(fb *domain.Feedback) { fb.RelatedURL = "  " }), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateFeedback(tc.fb)
			if tc.wantErr && err == nil {
				t.Fatal("期望校验失败")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("期望校验通过，实际 %v", err)
			}
		})
	}
}

// TestFeedbackDedupHashStable 覆盖 TODO §17 反垃圾：摘要对空白 / 大小写不敏感，对内容变化敏感。
func TestFeedbackDedupHashStable(t *testing.T) {
	base := &domain.Feedback{
		FeedbackType: domain.FeedbackTypeDataCorrection,
		TargetKind:   domain.FeedbackTargetMob,
		TargetRef:    "cow",
		Title:        "牛的生命值有误",
		Content:      "应为 10 而非 20。",
	}
	same := &domain.Feedback{
		FeedbackType: " DATA_CORRECTION ",
		TargetKind:   "Mob",
		TargetRef:    "cow",
		Title:        "  牛的生命值有误 ",
		Content:      "应为 10\n 而非 20。",
	}
	if feedbackDedupHash(base) != feedbackDedupHash(same) {
		t.Fatal("归一化后摘要应一致")
	}
	different := *base
	different.Content = "应为 12 而非 20。"
	if feedbackDedupHash(base) == feedbackDedupHash(&different) {
		t.Fatal("内容不同摘要应不同")
	}
	if len(feedbackDedupHash(base)) != 64 {
		t.Fatalf("摘要应为 64 位十六进制，实际长度 %d", len(feedbackDedupHash(base)))
	}
}

// TestFeedbackStatusWhitelist 覆盖 TODO §17：审核状态白名单。
func TestFeedbackStatusWhitelist(t *testing.T) {
	for _, s := range []string{"new", "processing", "resolved", "ignored"} {
		if !domain.IsValidFeedbackStatus(s) {
			t.Fatalf("%q 应为合法状态", s)
		}
	}
	for _, s := range []string{"", "approved", "pending", "spam"} {
		if domain.IsValidFeedbackStatus(s) {
			t.Fatalf("%q 不应为合法状态", s)
		}
	}
}
