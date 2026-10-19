package submission_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"mckit/backend/internal/domain"
	db "mckit/backend/internal/repository"
	"mckit/backend/internal/service/resource"
	"mckit/backend/internal/service/submission"
)

// submissionSchema 本包集成测试专用 schema（与 cmd/api 集成测试的 public 隔离）。
const submissionSchema = "submission_it"

func withSearchPath(dsn, schema string) string {
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "search_path=" + schema
}

func setupSubmissionDB(t *testing.T) *gorm.DB {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("未设置 TEST_DATABASE_URL，跳过数据库集成测试")
	}
	ctx := context.Background()

	adminDB, err := db.Open(url)
	if err != nil {
		t.Fatalf("连接测试库失败: %v", err)
	}
	if err := adminDB.Exec("DROP SCHEMA IF EXISTS " + submissionSchema + " CASCADE").Error; err != nil {
		t.Fatalf("清理测试 schema 失败: %v", err)
	}
	if err := adminDB.Exec("CREATE SCHEMA " + submissionSchema).Error; err != nil {
		t.Fatalf("创建测试 schema 失败: %v", err)
	}
	if sqlDB, err := adminDB.DB(); err == nil {
		_ = sqlDB.Close()
	}

	gdb, err := db.Open(withSearchPath(url, submissionSchema))
	if err != nil {
		t.Fatalf("连接测试 schema 失败: %v", err)
	}
	if _, err := db.Migrate(ctx, gdb, "../../../migrations"); err != nil {
		t.Fatalf("执行迁移失败: %v", err)
	}
	return gdb
}

// seedAccount 建一个普通用户账号（投稿者外键来源）。
func seedAccount(t *testing.T, gdb *gorm.DB, name string) uuid.UUID {
	t.Helper()
	acc := &domain.Account{
		Username:     name,
		PasswordHash: "x",
		Email:        name + "@example.com",
		Role:         domain.RoleUser,
		IsActive:     true,
	}
	if err := gdb.Create(acc).Error; err != nil {
		t.Fatalf("建账号失败: %v", err)
	}
	return acc.ID
}

// validInput 一份最小合法投稿。
func validInput(accountID uuid.UUID) *submission.CreateInput {
	return &submission.CreateInput{
		AccountID:      accountID,
		SubmitterName:  "alice",
		Name:           "JEI 物品管理器",
		Summary:        "查看合成配方",
		ResourceType:   domain.ResourceTypeMod,
		SourceURL:      "https://example.com/jei",
		GameEdition:    "java",
		LicenseVersion: domain.SubmissionLicenseVersion,
	}
}

func TestCreateAndValidation(t *testing.T) {
	gdb := setupSubmissionDB(t)
	ctx := context.Background()
	svc := submission.NewService(gdb, resource.NewService(gdb))
	acc := seedAccount(t, gdb, "alice")

	// 合法提交 → pending。
	sub, err := svc.Create(ctx, validInput(acc))
	if err != nil {
		t.Fatalf("提交失败: %v", err)
	}
	if sub.Status != domain.SubmissionStatusPending {
		t.Errorf("状态应为 pending，实际 %s", sub.Status)
	}
	if sub.ID == uuid.Nil {
		t.Error("应生成投稿 ID")
	}
	if sub.GameEdition != "java" {
		t.Errorf("游戏版本应归一化为 java，实际 %s", sub.GameEdition)
	}

	// 同账号 + 同来源链接 → 重复（部分唯一索引兜底）。
	if _, err := svc.Create(ctx, validInput(acc)); !errors.Is(err, submission.ErrDuplicate) {
		t.Errorf("重复投稿应返回 ErrDuplicate，实际 %v", err)
	}

	// 不同来源链接可再次提交。
	in2 := validInput(acc)
	in2.SourceURL = "https://example.com/other"
	if _, err := svc.Create(ctx, in2); err != nil {
		t.Errorf("不同来源链接应可提交: %v", err)
	}

	// 字段校验：错误字段应带 FieldError.Field。
	cases := []struct {
		name  string
		mut   func(*submission.CreateInput)
		field string
	}{
		{"名称必填", func(in *submission.CreateInput) { in.Name = "  " }, "name"},
		{"简介必填", func(in *submission.CreateInput) { in.Summary = "" }, "summary"},
		{"类型非法", func(in *submission.CreateInput) { in.ResourceType = "bogus" }, "resource_type"},
		{"来源缺失", func(in *submission.CreateInput) { in.SourceURL = "" }, "source_url"},
		{"来源非 http", func(in *submission.CreateInput) { in.SourceURL = "ftp://x/y" }, "source_url"},
		{"来源超长", func(in *submission.CreateInput) {
			in.SourceURL = "https://example.com/" + strings.Repeat("a", 2100)
		}, "source_url"},
		{"游戏版本非法", func(in *submission.CreateInput) { in.GameEdition = "switch" }, "game_edition"},
		{"未接受授权条款", func(in *submission.CreateInput) { in.LicenseVersion = "" }, "license_version"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := validInput(acc)
			in.SourceURL = "https://example.com/" + tc.name
			tc.mut(in)
			_, err := svc.Create(ctx, in)
			var fe *submission.FieldError
			if !errors.As(err, &fe) {
				t.Fatalf("应为 FieldError，实际 %v", err)
			}
			if fe.Field != tc.field {
				t.Errorf("字段应为 %s，实际 %s", tc.field, fe.Field)
			}
		})
	}
}

func TestReviewApproveCreatesResource(t *testing.T) {
	gdb := setupSubmissionDB(t)
	ctx := context.Background()
	resSvc := resource.NewService(gdb)
	svc := submission.NewService(gdb, resSvc)
	acc := seedAccount(t, gdb, "bob")

	in := validInput(acc)
	in.LoaderTypes = []string{"fabric"}
	in.SupportedVersions = []string{"1.20.1"}
	sub, err := svc.Create(ctx, in)
	if err != nil {
		t.Fatalf("提交失败: %v", err)
	}
	reviewer := seedAccount(t, gdb, "reviewer")

	out, res, err := svc.Review(ctx, submission.ReviewInput{
		ID:         sub.ID,
		Status:     domain.SubmissionStatusApproved,
		Reviewer:   "admin",
		ReviewerID: reviewer,
		Note:       "字段完整",
	})
	if err != nil {
		t.Fatalf("审核通过失败: %v", err)
	}
	if out.Status != domain.SubmissionStatusApproved {
		t.Errorf("状态应为 approved，实际 %s", out.Status)
	}
	if res == nil {
		t.Fatal("应生成资源")
	}
	// 默认生成草稿，需人工核验后发布。
	if res.Status != domain.StatusDraft {
		t.Errorf("默认应生成草稿，实际 %s", res.Status)
	}
	if out.ResourceID == nil || *out.ResourceID != res.ID {
		t.Error("投稿应回填 resource_id")
	}
	// 字段透传 + slug 由名称生成。
	if res.Name != in.Name || res.SourceURL != in.SourceURL {
		t.Error("资源应透传投稿字段")
	}
	if res.Slug == "" || strings.Contains(res.Slug, " ") {
		t.Errorf("slug 应自动生成且无空格，实际 %q", res.Slug)
	}
	// 生成的资源可被资源服务按 ID 读回（证明走了正式创建链路）。
	got, err := resSvc.GetByID(ctx, res.ID.String())
	if err != nil || got == nil {
		t.Fatalf("生成的资源应可读回: %v", err)
	}

	// 重复审核 → 拒绝。
	if _, _, err := svc.Review(ctx, submission.ReviewInput{
		ID: sub.ID, Status: domain.SubmissionStatusApproved, Reviewer: "admin",
	}); !errors.Is(err, submission.ErrAlreadyReviewed) {
		t.Errorf("重复审核应返回 ErrAlreadyReviewed，实际 %v", err)
	}
}

func TestReviewApprovePublishAndSlugCollision(t *testing.T) {
	gdb := setupSubmissionDB(t)
	ctx := context.Background()
	resSvc := resource.NewService(gdb)
	svc := submission.NewService(gdb, resSvc)
	acc := seedAccount(t, gdb, "carol")

	// 第一条：通过并直接发布。
	sub1, err := svc.Create(ctx, validInput(acc))
	if err != nil {
		t.Fatalf("提交失败: %v", err)
	}
	reviewer := seedAccount(t, gdb, "reviewer2")
	_, res1, err := svc.Review(ctx, submission.ReviewInput{
		ID: sub1.ID, Status: domain.SubmissionStatusApproved, Reviewer: "admin",
		ReviewerID: reviewer, Publish: true,
	})
	if err != nil {
		t.Fatalf("审核失败: %v", err)
	}
	if res1.Status != domain.StatusPublished {
		t.Errorf("publish=true 应生成已发布资源，实际 %s", res1.Status)
	}

	// 第二条同名资源（不同来源链接）：slug 冲突应自动追加序号。
	in2 := validInput(acc)
	in2.SourceURL = "https://example.com/jei-2"
	sub2, err := svc.Create(ctx, in2)
	if err != nil {
		t.Fatalf("第二次提交失败: %v", err)
	}
	_, res2, err := svc.Review(ctx, submission.ReviewInput{
		ID: sub2.ID, Status: domain.SubmissionStatusApproved, Reviewer: "admin",
	})
	if err != nil {
		t.Fatalf("第二次审核失败: %v", err)
	}
	if res2.Slug == res1.Slug {
		t.Errorf("slug 应自动去重，两次均为 %q", res1.Slug)
	}
}

func TestReviewRejectAndList(t *testing.T) {
	gdb := setupSubmissionDB(t)
	ctx := context.Background()
	svc := submission.NewService(gdb, resource.NewService(gdb))
	acc := seedAccount(t, gdb, "dave")

	sub, err := svc.Create(ctx, validInput(acc))
	if err != nil {
		t.Fatalf("提交失败: %v", err)
	}
	out, res, err := svc.Review(ctx, submission.ReviewInput{
		ID: sub.ID, Status: domain.SubmissionStatusRejected, Reviewer: "admin", Note: "来源不可靠",
	})
	if err != nil {
		t.Fatalf("驳回失败: %v", err)
	}
	if res != nil {
		t.Error("驳回不应生成资源")
	}
	if out.Status != domain.SubmissionStatusRejected || out.ReviewNote != "来源不可靠" {
		t.Errorf("驳回留痕不正确: %+v", out)
	}
	if out.ReviewedAt == nil {
		t.Error("应记录审核时间")
	}

	// 我的投稿：1 条。
	items, total, err := svc.ListMine(ctx, acc, 1, 20)
	if err != nil || total != 1 || len(items) != 1 {
		t.Fatalf("我的投稿应为 1 条，实际 total=%d len=%d err=%v", total, len(items), err)
	}

	// 后台按状态筛选。
	rejected, total, err := svc.List(ctx, submission.Filter{Status: domain.SubmissionStatusRejected, Page: 1, PageSize: 20})
	if err != nil || total != 1 || len(rejected) != 1 {
		t.Fatalf("驳回列表应为 1 条，实际 total=%d len=%d err=%v", total, len(rejected), err)
	}
	if _, _, err := svc.List(ctx, submission.Filter{Status: "bogus"}); !errors.Is(err, submission.ErrBadStatus) {
		t.Errorf("非法状态筛选应返回 ErrBadStatus，实际 %v", err)
	}

	// 非法审核状态。
	if _, _, err := svc.Review(ctx, submission.ReviewInput{ID: sub.ID, Status: "pending"}); !errors.Is(err, submission.ErrBadStatus) {
		t.Errorf("非法审核状态应返回 ErrBadStatus，实际 %v", err)
	}

	// 不存在。
	if _, err := svc.GetByID(ctx, uuid.New()); !errors.Is(err, submission.ErrNotFound) {
		t.Errorf("不存在应返回 ErrNotFound，实际 %v", err)
	}
}

// TestBatchReviewPartialSuccess 批量审核逐条执行：已审核的记录失败、其余成功，
// 返回逐条结果而不是整批回滚（运营需要知道哪几条没生效）。
func TestBatchReviewPartialSuccess(t *testing.T) {
	gdb := setupSubmissionDB(t)
	ctx := context.Background()
	svc := submission.NewService(gdb, resource.NewService(gdb))
	acc := seedAccount(t, gdb, "batch-user")

	// 三条投稿。
	ids := make([]uuid.UUID, 0, 3)
	for i := 0; i < 3; i++ {
		in := validInput(acc)
		in.SourceURL = "https://example.com/batch-" + string(rune('a'+i))
		sub, err := svc.Create(ctx, in)
		if err != nil {
			t.Fatalf("创建投稿失败: %v", err)
		}
		ids = append(ids, sub.ID)
	}

	// 先把第一条单独审核掉，制造「已被审核」的部分失败。
	if _, _, err := svc.Review(ctx, submission.ReviewInput{
		ID: ids[0], Status: domain.SubmissionStatusApproved, Reviewer: "reviewer",
	}); err != nil {
		t.Fatalf("预审核失败: %v", err)
	}

	approved := 0
	result, err := svc.BatchReview(ctx, submission.BatchReviewInput{
		IDs:      ids,
		Status:   domain.SubmissionStatusApproved,
		Reviewer: "reviewer",
		OnApproved: func(sub *domain.ResourceSubmission, res *domain.Resource) {
			approved++
		},
	})
	if err != nil {
		t.Fatalf("批量审核失败: %v", err)
	}
	if result.Requested != 3 || result.Succeeded != 2 || result.Failed != 1 {
		t.Fatalf("汇总异常: %+v", result)
	}
	if approved != 2 {
		t.Fatalf("通过回调次数 = %d, want 2", approved)
	}
	for _, item := range result.Items {
		if item.ID == ids[0].String() {
			if item.OK || item.Error == "" {
				t.Fatalf("已审核记录应失败并带原因: %+v", item)
			}
		}
	}

	// 批量驳回剩余不存在的情况：ID 重复应去重。
	dup := []uuid.UUID{ids[1], ids[1], uuid.Nil}
	res2, err := svc.BatchReview(ctx, submission.BatchReviewInput{
		IDs: dup, Status: domain.SubmissionStatusRejected, Reviewer: "reviewer",
	})
	if err != nil {
		t.Fatalf("批量驳回失败: %v", err)
	}
	if res2.Requested != 1 {
		t.Fatalf("去重后应为 1 条，实际 %d", res2.Requested)
	}
}

// TestBatchReviewValidation 批量审核的入参边界：空列表 / 超上限 / 非法状态。
func TestBatchReviewValidation(t *testing.T) {
	gdb := setupSubmissionDB(t)
	ctx := context.Background()
	svc := submission.NewService(gdb, resource.NewService(gdb))

	if _, err := svc.BatchReview(ctx, submission.BatchReviewInput{
		Status: domain.SubmissionStatusApproved,
	}); !errors.Is(err, submission.ErrNoIDs) {
		t.Fatalf("空列表应返回 ErrNoIDs，实际 %v", err)
	}

	tooMany := make([]uuid.UUID, domain.MaxBatchReview+1)
	for i := range tooMany {
		tooMany[i] = uuid.New()
	}
	if _, err := svc.BatchReview(ctx, submission.BatchReviewInput{
		IDs: tooMany, Status: domain.SubmissionStatusApproved,
	}); !errors.Is(err, submission.ErrTooManyIDs) {
		t.Fatalf("超上限应返回 ErrTooManyIDs，实际 %v", err)
	}

	if _, err := svc.BatchReview(ctx, submission.BatchReviewInput{
		IDs: []uuid.UUID{uuid.New()}, Status: "pending",
	}); !errors.Is(err, submission.ErrBadStatus) {
		t.Fatalf("非法状态应返回 ErrBadStatus，实际 %v", err)
	}
}
