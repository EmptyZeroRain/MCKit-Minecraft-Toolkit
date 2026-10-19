package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"mckit/backend/internal/domain"
)

// ptr 返回 int 指针，便于构造可选属性。
func ptr(v int) *int { return &v }

// TestBuildItemNormalizesDefaults 覆盖：缺省类别 / 版本 / 别名时的归一化行为。
func TestBuildItemNormalizesDefaults(t *testing.T) {
	item, err := buildItem(itemRow{ItemID: "minecraft:stone", NameZh: "石头"})
	if err != nil {
		t.Fatalf("合法行不应报错: %v", err)
	}
	if item.ItemCategory != domain.ItemCategoryOther {
		t.Errorf("类别缺省应为 other，实际 %q", item.ItemCategory)
	}
	if item.GameEdition != "java" {
		t.Errorf("版本缺省应为 java，实际 %q", item.GameEdition)
	}
	if item.Aliases == nil || len(item.Aliases) != 0 {
		t.Errorf("别名缺省应为空切片（非 nil），实际 %v", item.Aliases)
	}
	if item.SupportedVersions == nil || len(item.SupportedVersions) != 0 {
		t.Errorf("适用版本缺省应为空切片（非 nil），实际 %v", item.SupportedVersions)
	}
	if item.LastVerifiedAt != nil {
		t.Errorf("未提供校验时间时应为 nil，实际 %v", item.LastVerifiedAt)
	}
}

// TestBuildItemTrimsAndParses 覆盖：字段裁剪与 RFC3339 时间解析。
func TestBuildItemTrimsAndParses(t *testing.T) {
	item, err := buildItem(itemRow{
		ItemID:         "  minecraft:diamond_pickaxe ",
		NameZh:         " 钻石镐 ",
		NameEn:         " Diamond Pickaxe ",
		ItemCategory:   domain.ItemCategoryTool,
		Durability:     ptr(1561),
		StackSize:      ptr(1),
		LastVerifiedAt: "2026-01-15T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("合法行不应报错: %v", err)
	}
	if item.ItemID != "minecraft:diamond_pickaxe" || item.NameZh != "钻石镐" || item.NameEn != "Diamond Pickaxe" {
		t.Errorf("字段应被裁剪空格: %+v", item)
	}
	if item.Durability == nil || *item.Durability != 1561 {
		t.Errorf("耐久度应被保留，实际 %v", item.Durability)
	}
	want := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	if item.LastVerifiedAt == nil || !item.LastVerifiedAt.Equal(want) {
		t.Errorf("校验时间应解析为 %v，实际 %v", want, item.LastVerifiedAt)
	}
}

// TestBuildItemRejectsInvalidRows 覆盖：非法数据必须被拒绝，避免写入脏数据。
func TestBuildItemRejectsInvalidRows(t *testing.T) {
	cases := []struct {
		name string
		row  itemRow
	}{
		{"缺少 item_id", itemRow{NameZh: "石头"}},
		{"缺少 name_zh", itemRow{ItemID: "minecraft:stone"}},
		{"非法类别", itemRow{ItemID: "minecraft:stone", NameZh: "石头", ItemCategory: "magic"}},
		{"负耐久度", itemRow{ItemID: "minecraft:stone", NameZh: "石头", Durability: ptr(-1)}},
		{"负堆叠上限", itemRow{ItemID: "minecraft:stone", NameZh: "石头", StackSize: ptr(-2)}},
		{"非法时间格式", itemRow{ItemID: "minecraft:stone", NameZh: "石头", LastVerifiedAt: "2026/01/15"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := buildItem(tc.row); err == nil {
				t.Fatalf("期望报错，实际通过: %+v", tc.row)
			}
		})
	}
}

// TestValidItemCategories 覆盖：类别白名单与后端 domain 常量保持一致。
func TestValidItemCategories(t *testing.T) {
	for _, c := range domain.ItemCategories {
		if !domain.IsValidItemCategory(c) {
			t.Errorf("合法类别 %q 被误判", c)
		}
	}
	for _, c := range []string{"", "magic", "BLOCK", "方块"} {
		if domain.IsValidItemCategory(c) {
			t.Errorf("非法类别 %q 被误判为合法", c)
		}
	}
}

// TestBuildItemValidatesAndSortsVersions 覆盖：适用版本格式校验 + 归一化排序（TODO §2）。
func TestBuildItemValidatesAndSortsVersions(t *testing.T) {
	item, err := buildItem(itemRow{
		ItemID:            "minecraft:stone",
		NameZh:            "石头",
		SupportedVersions: []string{"1.21.1", "1.20", "1.21"},
	})
	if err != nil {
		t.Fatalf("合法版本列表不应报错: %v", err)
	}
	want := []string{"1.20", "1.21", "1.21.1"}
	if len(item.SupportedVersions) != len(want) {
		t.Fatalf("版本数量不符: %v", item.SupportedVersions)
	}
	for i := range want {
		if item.SupportedVersions[i] != want[i] {
			t.Fatalf("版本未按升序归一化: %v", item.SupportedVersions)
		}
	}

	// 非法版本号与重复项都必须被拒绝。
	for _, bad := range [][]string{{"1.21.1.2"}, {"最新版"}, {"1.21", "1.21"}} {
		if _, err := buildItem(itemRow{ItemID: "minecraft:stone", NameZh: "石头", SupportedVersions: bad}); err == nil {
			t.Errorf("非法版本列表应被拒绝: %v", bad)
		}
	}
}

// TestValidateVersionsAggregatesErrors 覆盖：校验错误聚合（而非只报第一条）。
func TestValidateVersionsAggregatesErrors(t *testing.T) {
	err := validateVersions("game_versions", []string{"1.21", "坏版本", "1.21"})
	if err == nil {
		t.Fatal("期望校验失败")
	}
	msg := err.Error()
	for _, frag := range []string{"game_versions", "坏版本", "重复"} {
		if !strings.Contains(msg, frag) {
			t.Errorf("错误信息缺少片段 %q：%s", frag, msg)
		}
	}
	if err := validateVersions("game_versions", []string{"1.21", "24w14a"}); err != nil {
		t.Errorf("合法版本不应报错: %v", err)
	}
}

// TestBuildImportJob 覆盖：导入统计到任务记录的映射与错误摘要截断（TODO §4）。
func TestBuildImportJob(t *testing.T) {
	now := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	msgs := make([]string, 0, maxErrorSummary+5)
	for i := 0; i < maxErrorSummary+5; i++ {
		msgs = append(msgs, fmt.Sprintf("第 %d 行：非法", i+1))
	}
	s := &summary{total: 60, created: 50, updated: 2, skipped: 3, failed: 5, messages: msgs}

	job := buildImportJob("items", "seeds/items_demo.json", true, s, "tester", now)

	if job.DataType != "items" || job.SourceFile != "seeds/items_demo.json" || !job.DryRun {
		t.Errorf("任务基础字段异常: %+v", job)
	}
	if job.TotalCount != 60 || job.CreatedCount != 50 || job.UpdatedCount != 2 || job.SkippedCount != 3 || job.FailedCount != 5 {
		t.Errorf("任务统计字段异常: %+v", job)
	}
	if job.Operator != "tester" || job.FinishedAt == nil || !job.FinishedAt.Equal(now) {
		t.Errorf("执行者 / 完成时间异常: %+v", job)
	}
	if len(job.ErrorSummary) != maxErrorSummary+1 {
		t.Fatalf("错误摘要应截断为 %d 条（含省略说明），实际 %d", maxErrorSummary+1, len(job.ErrorSummary))
	}
	if !strings.Contains(job.ErrorSummary[len(job.ErrorSummary)-1], "另有 5 条未展示") {
		t.Errorf("缺少截断说明: %v", job.ErrorSummary[len(job.ErrorSummary)-1])
	}
}
