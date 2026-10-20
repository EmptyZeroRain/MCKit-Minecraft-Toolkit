package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"mckit/backend/internal/accountid"
	"mckit/backend/internal/domain"
	"mckit/backend/internal/http/middleware"
)

// TestCommunityAPIIntegration 覆盖社区 HTTP 契约：
// 公开读匿名可用、写操作需要普通用户 + CSRF、后台审核需要 content.community。
func TestCommunityAPIIntegration(t *testing.T) {
	gdb, router := integrationSetup(t)
	resetData(t, gdb)

	const (
		username = "it-community-user"
		password = "community-pass-123"
	)
	rec := do(t, router, http.MethodPost, "/api/v1/user/register",
		fmt.Sprintf(`{"username":%q,"nickname":"社区用户","email":"it-community@example.com","password":%q,"captcha_id":"it","captcha_answer":"135790"}`, username, password),
		nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("注册期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	var registerResp struct {
		Data struct {
			UserNo string `json:"user_no"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &registerResp); err != nil {
		t.Fatalf("解析注册响应失败: %v", err)
	}
	if !accountid.Valid(registerResp.Data.UserNo) {
		t.Fatalf("注册响应 user_no 不合法: %q", registerResp.Data.UserNo)
	}

	// 前台登录，拿到统一会话 Cookie 与 CSRF token。
	rec = do(t, router, http.MethodPost, "/api/v1/user/login",
		fmt.Sprintf(`{"identifier":%q,"password":%q}`, username, password), nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("前台登录期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	var loginResp struct {
		Data struct {
			UserNo    string `json:"user_no"`
			CSRFToken string `json:"csrf_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &loginResp); err != nil {
		t.Fatalf("解析登录响应失败: %v", err)
	}
	if loginResp.Data.UserNo != registerResp.Data.UserNo {
		t.Fatalf("登录响应 user_no 与注册不一致: %q != %q", loginResp.Data.UserNo, registerResp.Data.UserNo)
	}
	var userCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == middleware.SessionCookieName() {
			userCookie = c
		}
	}
	if userCookie == nil {
		t.Fatal("登录响应缺少会话 Cookie")
	}
	userCSRF := loginResp.Data.CSRFToken

	// 匿名列表：空结果，公开可访问。
	rec = do(t, router, http.MethodPost, "/api/v1/community/posts", `{}`, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("匿名帖子列表期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 发帖：缺少 CSRF 被拒绝，带 CSRF 成功。
	createBody := `{"category":"chat","title":"HTTP 集成测试帖","body":"正文内容"}`
	if rec = do(t, router, http.MethodPost, "/api/v1/community/posts/create", createBody, userCookie, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("缺少 CSRF 发帖期望 403，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	rec = do(t, router, http.MethodPost, "/api/v1/community/posts/create", createBody, userCookie, userCSRF)
	if rec.Code != http.StatusCreated {
		t.Fatalf("发帖期望 201，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	var postResp struct {
		Data struct {
			ID     string `json:"id"`
			Author struct {
				UserNo string `json:"user_no"`
				ID     string `json:"id"`
			} `json:"author"`
			Liked bool `json:"liked"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &postResp); err != nil {
		t.Fatalf("解析发帖响应失败: %v", err)
	}
	if postResp.Data.ID == "" || postResp.Data.Author.UserNo != registerResp.Data.UserNo {
		t.Fatalf("发帖响应字段不符: %+v", postResp.Data)
	}
	if postResp.Data.Author.ID != "" {
		t.Fatalf("社区作者不应暴露内部 UUID: %s", rec.Body.String())
	}

	// 关键字屏蔽：启用规则后，帖子标题与留言正文命中均被服务端拒绝。
	if err := gdb.Create(&domain.ContentKeyword{
		Keyword: "违禁词", Normalized: "违禁词", Enabled: true, Note: "集成测试",
	}).Error; err != nil {
		t.Fatalf("创建关键字规则失败: %v", err)
	}
	rec = do(t, router, http.MethodPost, "/api/v1/community/posts/create",
		`{"category":"chat","title":"违禁词测试","body":"正常正文"}`, userCookie, userCSRF)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("命中关键字发帖期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	rec = do(t, router, http.MethodPost, "/api/v1/community/comments/create",
		fmt.Sprintf(`{"post_id":%q,"body":"这是违禁词内容"}`, postResp.Data.ID), userCookie, userCSRF)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("命中关键字留言期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 匿名详情公开可读，且 liked 为 false。
	rec = do(t, router, http.MethodPost, "/api/v1/community/posts/detail",
		fmt.Sprintf(`{"id":%q}`, postResp.Data.ID), nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("匿名详情期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	var detailResp struct {
		Data struct {
			Liked bool `json:"liked"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &detailResp); err != nil {
		t.Fatalf("解析详情响应失败: %v", err)
	}
	if detailResp.Data.Liked {
		t.Fatal("匿名查看不应返回 liked=true")
	}

	// 点赞需要登录 + CSRF，切换后计数实时变化。
	if rec = do(t, router, http.MethodPost, "/api/v1/community/posts/like",
		fmt.Sprintf(`{"post_id":%q}`, postResp.Data.ID), nil, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("匿名点赞期望 401，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	rec = do(t, router, http.MethodPost, "/api/v1/community/posts/like",
		fmt.Sprintf(`{"post_id":%q}`, postResp.Data.ID), userCookie, userCSRF)
	if rec.Code != http.StatusOK {
		t.Fatalf("点赞期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	var likeResp struct {
		Data struct {
			Liked     bool  `json:"liked"`
			LikeCount int64 `json:"like_count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &likeResp); err != nil {
		t.Fatalf("解析点赞响应失败: %v", err)
	}
	if !likeResp.Data.Liked || likeResp.Data.LikeCount != 1 {
		t.Fatalf("点赞结果不符: %+v", likeResp.Data)
	}

	// 留言：成功后可公开读取。
	rec = do(t, router, http.MethodPost, "/api/v1/community/comments/create",
		fmt.Sprintf(`{"post_id":%q,"body":"第一条留言"}`, postResp.Data.ID), userCookie, userCSRF)
	if rec.Code != http.StatusCreated {
		t.Fatalf("留言期望 201，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	rec = do(t, router, http.MethodPost, "/api/v1/community/comments",
		fmt.Sprintf(`{"post_id":%q}`, postResp.Data.ID), nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("公开留言列表期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	var commentsResp struct {
		Data []struct {
			ID     string `json:"id"`
			Body   string `json:"body"`
			Author struct {
				UserNo string `json:"user_no"`
			} `json:"author"`
		} `json:"data"`
		Meta struct {
			Total int64 `json:"total"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &commentsResp); err != nil {
		t.Fatalf("解析留言列表失败: %v", err)
	}
	if commentsResp.Meta.Total != 1 || len(commentsResp.Data) != 1 || commentsResp.Data[0].Body != "第一条留言" {
		t.Fatalf("留言列表不符: total=%d data=%+v", commentsResp.Meta.Total, commentsResp.Data)
	}
	if commentsResp.Data[0].Author.UserNo != registerResp.Data.UserNo {
		t.Fatalf("留言作者 user_no 不符: %q", commentsResp.Data[0].Author.UserNo)
	}
	commentID := commentsResp.Data[0].ID
	if commentID == "" {
		t.Fatal("留言响应缺少 id")
	}

	// 普通用户访问后台审核接口 → 403。
	if rec = do(t, router, http.MethodPost, "/api/v1/admin/community/posts/list", `{}`, userCookie, userCSRF); rec.Code != http.StatusForbidden {
		t.Fatalf("普通用户访问后台社区期望 403，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 后台隐藏留言：公开列表不再返回，但后台仍可查看。
	adminCookie, adminCSRF := loginAs(t, router, itAdminUser)
	rec = do(t, router, http.MethodPost, "/api/v1/admin/community/comments/all", `{}`, adminCookie, adminCSRF)
	if rec.Code != http.StatusOK ||
		!strings.Contains(rec.Body.String(), `"post_title":"HTTP 集成测试帖"`) ||
		!strings.Contains(rec.Body.String(), `"total":1`) {
		t.Fatalf("后台全站评论列表不符，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	rec = do(t, router, http.MethodPost, "/api/v1/admin/content-health/keywords/list", `{}`, adminCookie, adminCSRF)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"keyword":"违禁词"`) {
		t.Fatalf("后台关键字列表不符，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	rec = do(t, router, http.MethodPost, "/api/v1/admin/community/comments/status",
		fmt.Sprintf(`{"id":%q,"status":"hidden"}`, commentID), adminCookie, adminCSRF)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("后台隐藏留言期望 204，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	rec = do(t, router, http.MethodPost, "/api/v1/community/comments",
		fmt.Sprintf(`{"post_id":%q}`, postResp.Data.ID), nil, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"total":0`) {
		t.Fatalf("隐藏留言后公开列表应为空，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	rec = do(t, router, http.MethodPost, "/api/v1/admin/community/comments/list",
		fmt.Sprintf(`{"post_id":%q}`, postResp.Data.ID), adminCookie, adminCSRF)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"total":1`) {
		t.Fatalf("后台应仍能看到隐藏留言，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 后台隐藏帖子：公开详情与公开留言均 404，后台仍可列出。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/community/posts/status",
		fmt.Sprintf(`{"id":%q,"status":"hidden"}`, postResp.Data.ID), adminCookie, adminCSRF)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("后台隐藏期望 204，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	if rec = do(t, router, http.MethodPost, "/api/v1/community/posts/detail",
		fmt.Sprintf(`{"id":%q}`, postResp.Data.ID), nil, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("隐藏后详情期望 404，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	if rec = do(t, router, http.MethodPost, "/api/v1/community/comments",
		fmt.Sprintf(`{"post_id":%q}`, postResp.Data.ID), nil, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("隐藏后公开留言期望 404，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	rec = do(t, router, http.MethodPost, "/api/v1/admin/community/posts/list", `{"status":"hidden"}`, adminCookie, adminCSRF)
	if rec.Code != http.StatusOK {
		t.Fatalf("后台隐藏列表期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 后台删除帖子：审计留痕 + 幂等删除返回 404。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/community/posts/delete",
		fmt.Sprintf(`{"id":%q}`, postResp.Data.ID), adminCookie, adminCSRF)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("后台删除期望 204，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	var auditCount int64
	if err := gdb.Table("audit_logs").Where("action = ?", "community.post.delete").Count(&auditCount).Error; err != nil {
		t.Fatalf("查询审计日志失败: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("后台删除应写入 1 条审计日志，实际 %d", auditCount)
	}
	if rec = do(t, router, http.MethodPost, "/api/v1/admin/community/posts/delete",
		fmt.Sprintf(`{"id":%q}`, postResp.Data.ID), adminCookie, adminCSRF); rec.Code != http.StatusNotFound {
		t.Fatalf("重复删除期望 404，实际 %d（%s）", rec.Code, rec.Body.String())
	}
}
