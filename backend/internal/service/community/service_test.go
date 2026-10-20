package community

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// TestNormalizePage 覆盖分页默认值与上限。
func TestNormalizePage(t *testing.T) {
	tests := []struct {
		page, pageSize         int
		wantPage, wantPageSize int
	}{
		{page: 0, pageSize: 0, wantPage: 1, wantPageSize: 20},
		{page: -1, pageSize: -5, wantPage: 1, wantPageSize: 20},
		{page: 2, pageSize: 10, wantPage: 2, wantPageSize: 10},
		{page: 1, pageSize: MaxPageSize + 1, wantPage: 1, wantPageSize: 20},
	}
	for _, tt := range tests {
		gotPage, gotSize := normalizePage(tt.page, tt.pageSize)
		if gotPage != tt.wantPage || gotSize != tt.wantPageSize {
			t.Fatalf("normalizePage(%d, %d) = (%d, %d), want (%d, %d)",
				tt.page, tt.pageSize, gotPage, gotSize, tt.wantPage, tt.wantPageSize)
		}
	}
}

// TestViewerID 验证非法 / 匿名查看者不会影响 SQL 参数。
func TestViewerID(t *testing.T) {
	if got := viewerID(""); got != uuid.Nil {
		t.Fatalf("空串应映射为 uuid.Nil，实际 %v", got)
	}
	if got := viewerID("not-a-uuid"); got != uuid.Nil {
		t.Fatalf("非法 UUID 应映射为 uuid.Nil，实际 %v", got)
	}
	id := uuid.New()
	if got := viewerID(" " + id.String() + " "); got != id {
		t.Fatalf("合法 UUID 应原样返回，实际 %v", got)
	}
}

// TestCreatePostValidation 校验在触达数据库前完成的输入约束。
func TestCreatePostValidation(t *testing.T) {
	svc := &Service{}
	uid := uuid.NewString()
	tests := []struct {
		name     string
		category string
		title    string
		body     string
		want     error
	}{
		{name: "非法分类", category: "unknown", title: "标题", body: "正文", want: ErrBadCategory},
		{name: "空标题", category: "chat", title: "   ", body: "正文", want: ErrBadTitle},
		{name: "标题超长", category: "chat", title: strings.Repeat("长", MaxTitleLength+1), body: "正文", want: ErrBadTitle},
		{name: "空正文", category: "chat", title: "标题", body: " ", want: ErrBadBody},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.CreatePost(context.Background(), uid, tt.category, tt.title, tt.body)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

// TestCreateCommentValidation 覆盖留言长度与非法帖子 ID。
func TestCreateCommentValidation(t *testing.T) {
	svc := &Service{}
	uid := uuid.NewString()
	if _, err := svc.CreateComment(context.Background(), uid, "bad-id", "留言"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("非法帖子 ID 应返回 ErrNotFound，实际 %v", err)
	}
	if _, err := svc.CreateComment(context.Background(), uid, uuid.NewString(), "  "); !errors.Is(err, ErrBadComment) {
		t.Fatalf("空留言应返回 ErrBadComment，实际 %v", err)
	}
}
