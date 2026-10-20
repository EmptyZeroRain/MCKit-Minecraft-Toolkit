package community

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"mckit/backend/internal/accountid"
	"mckit/backend/internal/domain"
	db "mckit/backend/internal/repository"

	"gorm.io/gorm"
)

// communitySchema 社区集成测试专用 schema，避免与其他并行测试包互相清库。
const communitySchema = "community_it"

func withSearchPath(dsn, schema string) string {
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "search_path=" + schema
}

func setupCommunityDB(t *testing.T) *gorm.DB {
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
	if err := adminDB.Exec("DROP SCHEMA IF EXISTS " + communitySchema + " CASCADE").Error; err != nil {
		t.Fatalf("清理测试 schema 失败: %v", err)
	}
	if err := adminDB.Exec("CREATE SCHEMA " + communitySchema).Error; err != nil {
		t.Fatalf("创建测试 schema 失败: %v", err)
	}
	if sqlDB, err := adminDB.DB(); err == nil {
		_ = sqlDB.Close()
	}

	gdb, err := db.Open(withSearchPath(url, communitySchema))
	if err != nil {
		t.Fatalf("连接测试 schema 失败: %v", err)
	}
	if _, err := db.Migrate(ctx, gdb, "../../../migrations"); err != nil {
		t.Fatalf("执行迁移失败: %v", err)
	}
	return gdb
}

func createTestUser(t *testing.T, gdb *gorm.DB, username string) *domain.User {
	t.Helper()
	userNo, err := accountid.Generate()
	if err != nil {
		t.Fatalf("生成 user_no 失败: %v", err)
	}
	u := &domain.User{
		Username: username,
		UserNo:   userNo,
		Role:     domain.RoleUser,
		IsActive: true,
	}
	if err := gdb.Create(u).Error; err != nil {
		t.Fatalf("创建测试用户失败: %v", err)
	}
	return u
}

// TestCommunityFlowIntegration 覆盖发帖 / 列表 / 详情 / 留言 / 点赞 / 隐藏 / 删除主流程。
func TestCommunityFlowIntegration(t *testing.T) {
	gdb := setupCommunityDB(t)
	ctx := context.Background()
	svc := NewService(gdb)

	author := createTestUser(t, gdb, "it-community-author")
	other := createTestUser(t, gdb, "it-community-other")

	post, err := svc.CreatePost(ctx, author.ID.String(), "chat", "  集成测试帖子  ", "  正文内容  ")
	if err != nil {
		t.Fatalf("发帖失败: %v", err)
	}
	if post.Title != "集成测试帖子" || post.Body != "正文内容" {
		t.Fatalf("标题 / 正文未按预期裁剪: %+v", post)
	}
	if post.Author.UserNo != author.UserNo {
		t.Fatalf("作者 user_no 应为 %q，实际 %q", author.UserNo, post.Author.UserNo)
	}

	items, total, err := svc.ListPosts(ctx, ListFilter{Page: 1, PageSize: 20})
	if err != nil || total != 1 || len(items) != 1 {
		t.Fatalf("公开列表异常: total=%d len=%d err=%v", total, len(items), err)
	}
	if items[0].Liked {
		t.Fatal("匿名查看不应标记已点赞")
	}

	comment, err := svc.CreateComment(ctx, other.ID.String(), post.ID, "  第一条留言  ")
	if err != nil {
		t.Fatalf("留言失败: %v", err)
	}
	if comment.Body != "第一条留言" {
		t.Fatalf("留言未裁剪: %q", comment.Body)
	}
	comments, commentTotal, err := svc.ListComments(ctx, post.ID, 1, 20, false)
	if err != nil || commentTotal != 1 || len(comments) != 1 {
		t.Fatalf("留言列表异常: total=%d len=%d err=%v", commentTotal, len(comments), err)
	}

	liked, count, err := svc.ToggleLike(ctx, other.ID.String(), post.ID)
	if err != nil || !liked || count != 1 {
		t.Fatalf("首次点赞异常: liked=%v count=%d err=%v", liked, count, err)
	}
	liked, count, err = svc.ToggleLike(ctx, other.ID.String(), post.ID)
	if err != nil || liked || count != 0 {
		t.Fatalf("取消点赞异常: liked=%v count=%d err=%v", liked, count, err)
	}

	// 非作者不能删除帖子，也不能删除他人留言。
	if err := svc.DeletePost(ctx, other.ID.String(), post.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("非作者删帖应被拒绝，实际 %v", err)
	}
	third := createTestUser(t, gdb, "it-community-third")
	if err := svc.DeleteComment(ctx, third.ID.String(), comment.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("非留言作者 / 非帖子作者删留言应被拒绝，实际 %v", err)
	}
	// 帖子作者可以清理自己帖子下的留言。
	if err := svc.DeleteComment(ctx, author.ID.String(), comment.ID); err != nil {
		t.Fatalf("帖子作者删除留言失败: %v", err)
	}

	if err := svc.SetPostStatus(ctx, post.ID, domain.CommunityStatusHidden); err != nil {
		t.Fatalf("隐藏帖子失败: %v", err)
	}
	if _, err := svc.GetPost(ctx, post.ID, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("隐藏帖子公开详情应 404，实际 %v", err)
	}
	if _, _, err := svc.ListComments(ctx, post.ID, 1, 20, false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("隐藏帖子公开留言应 404，实际 %v", err)
	}
	adminItems, adminTotal, err := svc.AdminListPosts(ctx, AdminListFilter{Status: domain.CommunityStatusHidden})
	if err != nil || adminTotal != 1 || len(adminItems) != 1 {
		t.Fatalf("后台隐藏列表异常: total=%d len=%d err=%v", adminTotal, len(adminItems), err)
	}

	if err := svc.AdminDeletePost(ctx, post.ID); err != nil {
		t.Fatalf("后台删除帖子失败: %v", err)
	}
	if _, err := svc.GetPost(ctx, post.ID, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("删除后详情应 404，实际 %v", err)
	}
}
