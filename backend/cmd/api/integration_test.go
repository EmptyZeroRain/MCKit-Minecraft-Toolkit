package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"mckit/backend/internal/auth"
	"mckit/backend/internal/config"
	"mckit/backend/internal/domain"
	"mckit/backend/internal/http/middleware"
	"mckit/backend/internal/http/response"
	db "mckit/backend/internal/repository"
	"mckit/backend/internal/repository/types"
	"mckit/backend/internal/service/metrics"
	"mckit/backend/internal/service/searchstats"
)

// 集成测试使用独立的测试库（TEST_DATABASE_URL）。
// 未设置时自动跳过，保证 `go test ./...` 在无数据库环境下仍可运行。
//
//	TEST_DATABASE_URL='postgres://user:pass@127.0.0.1:5432/mckit_test?sslmode=disable' go test ./cmd/api
const (
	itAdminUser  = "it-admin"
	itEditorUser = "it-editor"
	itPassword   = "integration-pass-123"
)

var (
	itOnce      sync.Once
	itDB        *gorm.DB
	itRouter    http.Handler
	itCollector *metrics.Collector
	itErr       error
)

// integrationSetup 初始化测试库、执行迁移并构建完整路由（仅执行一次）。
func integrationSetup(t *testing.T) (*gorm.DB, http.Handler) {
	t.Helper()
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("未设置 TEST_DATABASE_URL，跳过数据库集成测试")
	}
	itOnce.Do(func() {
		gdb, err := db.Open(os.Getenv("TEST_DATABASE_URL"))
		if err != nil {
			itErr = err
			return
		}
		// 从干净 schema 重建，确保迁移可重复执行。
		if err := gdb.Exec("DROP SCHEMA public CASCADE; CREATE SCHEMA public;").Error; err != nil {
			itErr = err
			return
		}
		if _, err := db.Migrate(context.Background(), gdb, "../../migrations"); err != nil {
			itErr = err
			return
		}
		if err := seedAdmins(gdb); err != nil {
			itErr = err
			return
		}

		cfgInstance = &config.Config{
			AppEnv:               "test",
			AppBaseURL:           "http://localhost:5173",
			SessionCookieName:    "mc_session",
			SessionSecure:        false,
			LogLevel:             "error",
			RateLimitRPS:         1000,
			RateLimitBurst:       1000,
			LinkCheckTimeout:     3 * time.Second,
			LinkCheckMaxRedirect: 3,
			MCMetaCacheDir:       "cache",
			MCMetaCacheTTL:       time.Hour,
			BlockIDFile:          "data/物品id.md",
			MCSRCEnabled:         true,
			MCSRCCacheDir:        filepath.Join(os.TempDir(), "mckit-mcsrc-it"),
			MCSRCJarMaxMB:        64,
			MCSRCIndexMaxClasses: 1000,
			// 关闭自动准备：匿名 / 普通用户的 /prepare 一律 429，
			// 既验证额度边界，也确保集成测试绝不访问 Mojang 上游。
			MCSRCAutoPrepareMax: 0,
			// 图形验证码：启用并固定校验码，避免集成测试依赖图片识别。
			CaptchaEnabled:   true,
			CaptchaTTL:       5 * time.Minute,
			CaptchaFixedCode: "135790",
			// 双因子认证：集成测试用固定密钥，验证绑定 / 登录二次校验全链路。
			MFAEncryptionKey: base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")),
			MFAIssuer:        "MC 工具站测试",
		}
		middleware.SetCookieName(cfgInstance.SessionCookieName)

		// 使用统计采集器（TODO §13）：集成测试直接写入测试库。
		itCollector = metrics.New(metrics.NewGormStore(gdb), metrics.Options{
			FlushInterval: time.Hour,
			RetentionDays: 180,
			Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		})

		r := chi.NewRouter()
		r.Use(middleware.RequestID)
		r.Use(middleware.Recover)
		r.Use(middleware.Metrics(itCollector))
		api := chi.NewRouter()
		registerRoutes(context.Background(), api, gdb, itCollector)
		api.NotFound(func(w http.ResponseWriter, req *http.Request) { response.NotFound(w, req) })
		api.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) {
			response.Error(w, req, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "请求方法不允许", nil)
		})
		r.Mount("/api/v1", api)

		itDB, itRouter = gdb, r
	})
	if itErr != nil {
		t.Fatalf("集成测试环境初始化失败: %v", itErr)
	}
	return itDB, itRouter
}

func seedAdmins(gdb *gorm.DB) error {
	hash, err := auth.HashPassword(itPassword)
	if err != nil {
		return err
	}
	for _, a := range []domain.Admin{
		{Username: itAdminUser, PasswordHash: hash, Role: domain.RoleAdmin, IsActive: true},
		{Username: itEditorUser, PasswordHash: hash, Role: domain.RoleEditor, IsActive: true},
	} {
		if err := gdb.Create(&a).Error; err != nil {
			return err
		}
	}
	return nil
}

// resetData 清理业务数据，保留后台账号。
//
// 统一账号（迁移 0022）后 admins/users 合并为 accounts，不能整表 TRUNCATE
// （会删掉 seedAdmins 建的后台账号），改为只删除 role = 'user' 的普通用户；
// 其个人数据（收藏 / 最近使用 / 常用种子）由外键 ON DELETE CASCADE 一并清理。
func resetData(t *testing.T, gdb *gorm.DB) {
	t.Helper()
	// 统计采集器（itCollector）是包级单例，内存缓冲会跨用例累积：
	// 先落库、再随下面的 TRUNCATE 一起清掉，避免上一用例的统计事件
	// 被后续用例的 Flush 带进本次断言（曾导致 TestUsageStatsAndAnalytics 偶发失败）。
	if itCollector != nil {
		if err := itCollector.Flush(context.Background()); err != nil {
			t.Fatalf("清理统计缓冲失败: %v", err)
		}
	}
	err := gdb.Exec(`TRUNCATE resources, tools, wiki_entries, items, recipes, commands,
		feedback, resource_submissions, categories, tags, audit_logs, data_import_jobs, search_terms, dev_docs,
		tool_stats_daily, resource_stats_daily, api_stats_daily, search_stats_daily,
		growth_events, experience_ledger, user_growth, user_daily_tasks, user_achievements,
		achievement_progress, growth_audit_logs
		RESTART IDENTITY CASCADE`).Error
	if err != nil {
		t.Fatalf("清理测试数据失败: %v", err)
	}
	if err := gdb.Exec("DELETE FROM accounts WHERE role = ?", domain.RoleUser).Error; err != nil {
		t.Fatalf("清理普通用户失败: %v", err)
	}
}

// do 发起请求，可选携带会话 Cookie 与 CSRF token。
func do(t *testing.T, router http.Handler, method, path, body string, cookie *http.Cookie, csrf string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// loginAs 登录并返回会话 Cookie 与 CSRF token。
func loginAs(t *testing.T, router http.Handler, username string) (*http.Cookie, string) {
	t.Helper()
	body := fmt.Sprintf(`{"username":%q,"password":%q}`, username, itPassword)
	rec := do(t, router, http.MethodPost, "/api/v1/admin/auth/login", body, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("登录 %s 失败：%d %s", username, rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			CSRFToken string `json:"csrf_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析登录响应失败: %v", err)
	}
	var cookie *http.Cookie
	// 统一账号（迁移 0022）：后台与前台共用一个会话 Cookie，名字以配置为准。
	for _, c := range rec.Result().Cookies() {
		if c.Name == middleware.SessionCookieName() {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("登录响应缺少会话 Cookie")
	}
	return cookie, resp.Data.CSRFToken
}

func decodeData(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var resp struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应失败: %v（body=%s）", err, rec.Body.String())
	}
	return resp.Data
}

// TestAdminAPIRequiresAuth 覆盖 §13.2：未登录不能访问任何管理写接口。
func TestAdminAPIRequiresAuth(t *testing.T) {
	_, router := integrationSetup(t)
	resetData(t, itDB)

	for _, tc := range []struct {
		method, path, body string
	}{
		{http.MethodPost, "/api/v1/admin/resources/list", `{}`},
		{http.MethodPost, "/api/v1/admin/resources/create", `{"name":"x"}`},
		{http.MethodPost, "/api/v1/admin/tools/list", `{}`},
		{http.MethodPost, "/api/v1/admin/audit-logs/list", `{}`},
	} {
		rec := do(t, router, tc.method, tc.path, tc.body, nil, "")
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s 未登录期望 401，实际 %d", tc.method, tc.path, rec.Code)
		}
	}
}

// TestEditorCannotDoAdminOnly 覆盖 §13.2：editor 不能执行 admin 专属操作。
func TestEditorCannotDoAdminOnly(t *testing.T) {
	_, router := integrationSetup(t)
	resetData(t, itDB)

	cookie, csrf := loginAs(t, router, itEditorUser)
	rec := do(t, router, http.MethodPost, "/api/v1/admin/audit-logs/list", `{}`, cookie, csrf)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("editor 访问审计日志期望 403，实际 %d（%s）", rec.Code, rec.Body.String())
	}
}

// TestCSRFRequired 覆盖 §13.2：Cookie 会话写操作缺少 CSRF token 被拒绝。
func TestCSRFRequired(t *testing.T) {
	_, router := integrationSetup(t)
	resetData(t, itDB)

	cookie, _ := loginAs(t, router, itAdminUser)
	rec := do(t, router, http.MethodPost, "/api/v1/admin/resources/create",
		`{"slug":"csrf-x","name":"x","summary":"y","resource_type":"mod","source_url":"https://example.com","status":"draft"}`,
		cookie, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("缺少 CSRF token 期望 403，实际 %d", rec.Code)
	}
}

// TestResourceLifecycleAndPublicVisibility 覆盖 §13.2：草稿/下架对公开不可见。
func TestResourceLifecycleAndPublicVisibility(t *testing.T) {
	_, router := integrationSetup(t)
	resetData(t, itDB)

	cookie, csrf := loginAs(t, router, itAdminUser)

	// 创建草稿
	createBody := `{"slug":"jei-it","name":"JEI 测试","summary":"摘要","resource_type":"mod",
		"source_url":"https://example.com/jei","supported_versions":["1.21.1"],"loader_types":["fabric"],
		"status":"draft"}`
	rec := do(t, router, http.MethodPost, "/api/v1/admin/resources/create", createBody, cookie, csrf)
	if rec.Code != http.StatusCreated {
		t.Fatalf("创建资源期望 201，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	data := decodeData(t, rec)
	id, _ := data["id"].(string)
	if id == "" {
		t.Fatal("创建响应应包含 id")
	}

	// 公开列表不应包含草稿
	rec = do(t, router, http.MethodPost, "/api/v1/resources", `{}`, nil, "")
	if strings.Contains(rec.Body.String(), "jei-it") {
		t.Fatalf("公开列表不应返回草稿：%s", rec.Body.String())
	}
	// 公开详情应 404
	rec = do(t, router, http.MethodPost, "/api/v1/resources/jei-it", `{}`, nil, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("草稿详情期望 404，实际 %d", rec.Code)
	}

	// 发布
	rec = do(t, router, http.MethodPost, "/api/v1/admin/resources/update",
		fmt.Sprintf(`{"id":%q,"status":"published"}`, id), cookie, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("发布期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 公开列表/详情可见
	rec = do(t, router, http.MethodPost, "/api/v1/resources", `{"q":"JEI"}`, nil, "")
	if !strings.Contains(rec.Body.String(), "jei-it") {
		t.Fatalf("发布后公开列表应包含：%s", rec.Body.String())
	}
	rec = do(t, router, http.MethodPost, "/api/v1/resources/jei-it", `{}`, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("已发布详情期望 200，实际 %d", rec.Code)
	}

	// 下架（软删除）
	rec = do(t, router, http.MethodPost, "/api/v1/admin/resources/delete",
		fmt.Sprintf(`{"id":%q}`, id), cookie, csrf)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("删除期望 204，实际 %d", rec.Code)
	}
	rec = do(t, router, http.MethodPost, "/api/v1/resources", `{"q":"JEI"}`, nil, "")
	if strings.Contains(rec.Body.String(), "jei-it") {
		t.Fatalf("下架后公开列表不应包含：%s", rec.Body.String())
	}
}

// TestResourceSlugConflict 覆盖 §13.1：slug 唯一性与冲突处理。
func TestResourceSlugConflict(t *testing.T) {
	_, router := integrationSetup(t)
	resetData(t, itDB)

	cookie, csrf := loginAs(t, router, itAdminUser)
	body := `{"slug":"dup-it","name":"A","summary":"s","resource_type":"mod",
		"source_url":"https://example.com/a","status":"draft"}`

	if rec := do(t, router, http.MethodPost, "/api/v1/admin/resources/create", body, cookie, csrf); rec.Code != http.StatusCreated {
		t.Fatalf("首次创建期望 201，实际 %d", rec.Code)
	}
	rec := do(t, router, http.MethodPost, "/api/v1/admin/resources/create", body, cookie, csrf)
	if rec.Code != http.StatusConflict {
		t.Fatalf("重复 slug 期望 409，实际 %d（%s）", rec.Code, rec.Body.String())
	}
}

// TestUpdateRejectsInvalidEnum 覆盖 §6.3：更新时校验枚举，不落入数据库 500。
func TestUpdateRejectsInvalidEnum(t *testing.T) {
	_, router := integrationSetup(t)
	resetData(t, itDB)

	cookie, csrf := loginAs(t, router, itAdminUser)
	body := `{"slug":"enum-it","name":"A","summary":"s","resource_type":"mod",
		"source_url":"https://example.com/a","status":"draft"}`
	rec := do(t, router, http.MethodPost, "/api/v1/admin/resources/create", body, cookie, csrf)
	id, _ := decodeData(t, rec)["id"].(string)

	rec = do(t, router, http.MethodPost, "/api/v1/admin/resources/update",
		fmt.Sprintf(`{"id":%q,"status":"bogus"}`, id), cookie, csrf)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("非法状态期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}
}

// TestSearchPagination 覆盖 §13.2：搜索结果正确分页且排除草稿。
func TestSearchPagination(t *testing.T) {
	gdb, router := integrationSetup(t)
	resetData(t, itDB)

	for i := 0; i < 5; i++ {
		gdb.Create(&domain.Tool{
			Slug: fmt.Sprintf("search-tool-%d", i), Name: fmt.Sprintf("分页工具 %d", i),
			Summary: "测试分页", Category: domain.ToolCategoryQuery, Status: domain.StatusPublished,
		})
	}
	// 草稿不应出现在搜索结果
	gdb.Create(&domain.Tool{
		Slug: "search-draft", Name: "分页工具草稿", Summary: "测试分页",
		Category: domain.ToolCategoryQuery, Status: domain.StatusDraft,
	})

	rec := do(t, router, http.MethodPost, "/api/v1/search",
		`{"q":"分页","type":"tool","page":1,"page_size":2}`, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("搜索期望 200，实际 %d", rec.Code)
	}
	var resp struct {
		Data []map[string]any `json:"data"`
		Meta struct {
			Total      int64 `json:"total"`
			Page       int   `json:"page"`
			PageSize   int   `json:"page_size"`
			TotalPages int   `json:"total_pages"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析搜索响应失败: %v", err)
	}
	if resp.Meta.Total != 5 {
		t.Fatalf("total 应为 5（不含草稿），实际 %d", resp.Meta.Total)
	}
	if resp.Meta.TotalPages != 3 {
		t.Fatalf("total_pages 应为 3，实际 %d", resp.Meta.TotalPages)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("第 1 页应返回 2 条，实际 %d", len(resp.Data))
	}
	firstPageIDs := make(map[string]bool)
	for _, it := range resp.Data {
		firstPageIDs[fmt.Sprint(it["id"])] = true
	}

	rec = do(t, router, http.MethodPost, "/api/v1/search",
		`{"q":"分页","type":"tool","page":2,"page_size":2}`, nil, "")
	var resp2 struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp2)
	if len(resp2.Data) != 2 {
		t.Fatalf("第 2 页应返回 2 条，实际 %d", len(resp2.Data))
	}
	for _, it := range resp2.Data {
		if firstPageIDs[fmt.Sprint(it["id"])] {
			t.Fatal("第 2 页不应与第 1 页重复")
		}
	}
}

// TestSearchDevDocs 覆盖 §21：开发文档已接入全站搜索。
// 命中标题/英文标题/slug/摘要/正文/标签；仅返回已发布条目，且 url 指向 /dev/{slug}。
func TestSearchDevDocs(t *testing.T) {
	gdb, router := integrationSetup(t)
	resetData(t, itDB)

	gdb.Create(&domain.DevDoc{
		Slug: "it-neoforge-events", TitleZh: "NeoForge 事件系统", TitleEn: "NeoForge Events",
		Loader: "neoforge", Category: "events", Summary: "生命周期与事件总线",
		BodyMd: "使用 EventBus 订阅 PlayerTickEvent。",
		Tags:   types.JSONSlice{"入门"}, Status: domain.StatusPublished,
	})
	// 正文命中：关键词只出现在 body_md 中。
	gdb.Create(&domain.DevDoc{
		Slug: "it-fabric-mixins", TitleZh: "Fabric 注入", TitleEn: "Fabric Mixins",
		Loader: "fabric", Category: "mapping", Summary: "字节码注入",
		BodyMd: "通过 MixinTargetMarker 描述目标。",
		Status: domain.StatusPublished,
	})
	// 草稿不应出现在搜索结果中。
	gdb.Create(&domain.DevDoc{
		Slug: "it-draft", TitleZh: "NeoForge 事件系统草稿", Loader: "neoforge",
		Category: "events", Summary: "生命周期与事件总线", Status: domain.StatusDraft,
	})

	type searchResp struct {
		Data []struct {
			Type  string `json:"type"`
			Slug  string `json:"slug"`
			Title string `json:"title"`
			URL   string `json:"url"`
		} `json:"data"`
		Meta struct {
			Total int64 `json:"total"`
		} `json:"meta"`
	}

	// 1) 显式 type=devdoc：中文标题命中，草稿被排除。
	rec := do(t, router, http.MethodPost, "/api/v1/search", `{"q":"事件","type":"devdoc"}`, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("搜索期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	var got searchResp
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("解析搜索响应失败: %v", err)
	}
	if got.Meta.Total != 1 || len(got.Data) != 1 {
		t.Fatalf("期望命中 1 条已发布开发文档，实际 total=%d len=%d", got.Meta.Total, len(got.Data))
	}
	if got.Data[0].Type != "devdoc" || got.Data[0].URL != "/dev/it-neoforge-events" {
		t.Fatalf("结果类型/URL 不符：type=%s url=%s", got.Data[0].Type, got.Data[0].URL)
	}

	// 2) 正文命中：关键词只存在于 body_md。
	rec = do(t, router, http.MethodPost, "/api/v1/search", `{"q":"MixinTargetMarker","type":"devdoc"}`, nil, "")
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got.Data) != 1 || got.Data[0].Slug != "it-fabric-mixins" {
		t.Fatalf("正文命中失败：%+v", got.Data)
	}

	// 3) 不传 type：开发文档应并入全站结果。
	rec = do(t, router, http.MethodPost, "/api/v1/search", `{"q":"事件"}`, nil, "")
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	found := false
	for _, it := range got.Data {
		if it.Type == "devdoc" {
			found = true
		}
	}
	if !found {
		t.Fatalf("全站搜索未包含开发文档：%+v", got.Data)
	}

	// 4) type=item：开发文档不应出现在物品结果中。
	rec = do(t, router, http.MethodPost, "/api/v1/search", `{"q":"事件","type":"item"}`, nil, "")
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	for _, it := range got.Data {
		if it.Type == "devdoc" {
			t.Fatal("type=item 的结果不应包含开发文档")
		}
	}
}

// TestLinkCheckBlocksSSRF 覆盖 §13.2：非法/内网 URL 无法通过链接检查，且只更新状态。
// TestAdminUsersList 覆盖后台「普通用户管理」（admin 专属）：
// 列表分页 + 个人数据计数 + 状态 / 关键词筛选，且 editor 无权访问。
func TestAdminUsersList(t *testing.T) {
	gdb, router := integrationSetup(t)
	resetData(t, itDB)

	hash, err := auth.HashPassword("user-pass-123456")
	if err != nil {
		t.Fatalf("哈希用户密码失败: %v", err)
	}
	// 统一账号后普通用户必须显式带 role = 'user'（否则会被判为后台账号）。
	alice := domain.User{Username: "alice", PasswordHash: hash, Role: domain.RoleUser, IsActive: true}
	bob := domain.User{Username: "bob", PasswordHash: hash, Role: domain.RoleUser, IsActive: true}
	if err := gdb.Create(&alice).Error; err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}
	if err := gdb.Create(&bob).Error; err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}
	// 注意：users.is_active 带 default true，GORM Create 会忽略零值 false，
	// 因此「禁用」必须走 Update（与后台启用/禁用的真实路径一致）。
	if err := gdb.Model(&domain.User{}).Where("id = ?", bob.ID).
		Update("is_active", false).Error; err != nil {
		t.Fatalf("禁用用户失败: %v", err)
	}

	// alice：2 收藏 / 1 最近使用 / 1 常用种子；bob：全 0。
	for _, f := range []domain.UserFavorite{
		{UserID: alice.ID, Kind: "tool", ObjectID: "redstone", Label: "红石资料", URL: "/tools/redstone"},
		{UserID: alice.ID, Kind: "item", ObjectID: "minecraft:stone", Label: "石头", URL: "/items/minecraft:stone"},
	} {
		if err := gdb.Create(&f).Error; err != nil {
			t.Fatalf("创建收藏失败: %v", err)
		}
	}
	if err := gdb.Create(&domain.UserRecent{UserID: alice.ID, Kind: "tool", ObjectID: "redstone", Label: "红石资料", URL: "/tools/redstone"}).Error; err != nil {
		t.Fatalf("创建最近使用失败: %v", err)
	}
	if err := gdb.Create(&domain.UserSeed{UserID: alice.ID, Name: "测试种子", Seed: "123456789"}).Error; err != nil {
		t.Fatalf("创建常用种子失败: %v", err)
	}

	type userRow struct {
		Username      string `json:"username"`
		IsActive      bool   `json:"is_active"`
		FavoriteCount int64  `json:"favorite_count"`
		RecentCount   int64  `json:"recent_count"`
		SeedCount     int64  `json:"seed_count"`
	}
	type listResp struct {
		Data []userRow `json:"data"`
		Meta struct {
			Total int64 `json:"total"`
		} `json:"meta"`
	}

	// 1) editor 访问被拒（403）。
	editorCookie, editorCSRF := loginAs(t, router, itEditorUser)
	rec := do(t, router, http.MethodPost, "/api/v1/admin/users/list", `{}`, editorCookie, editorCSRF)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("editor 期望 403，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 2) admin 访问成功，计数正确。
	cookie, csrf := loginAs(t, router, itAdminUser)
	rec = do(t, router, http.MethodPost, "/api/v1/admin/users/list", `{}`, cookie, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	var got listResp
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if got.Meta.Total != 2 || len(got.Data) != 2 {
		t.Fatalf("期望 2 名用户，实际 total=%d len=%d", got.Meta.Total, len(got.Data))
	}
	byName := map[string]userRow{}
	for _, u := range got.Data {
		byName[u.Username] = u
	}
	a := byName["alice"]
	if a.FavoriteCount != 2 || a.RecentCount != 1 || a.SeedCount != 1 {
		t.Fatalf("alice 计数不符：%+v", a)
	}
	if b := byName["bob"]; b.FavoriteCount != 0 || b.RecentCount != 0 || b.SeedCount != 0 || b.IsActive {
		t.Fatalf("bob 数据不符：%+v", b)
	}

	// 3) 状态筛选：仅禁用。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/users/list", `{"status":"disabled"}`, cookie, csrf)
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Meta.Total != 1 || len(got.Data) != 1 || got.Data[0].Username != "bob" {
		t.Fatalf("禁用筛选不符：%+v", got)
	}

	// 4) 关键词筛选：用户名模糊匹配。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/users/list", `{"q":"ali"}`, cookie, csrf)
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Meta.Total != 1 || got.Data[0].Username != "alice" {
		t.Fatalf("关键词筛选不符：%+v", got)
	}
}

// TestAdminUsersManagement 覆盖后台「普通用户管理」的写操作：
// 创建、启用 / 禁用、重置密码、详情、导出、删除，以及「绝不误伤后台账号」的边界。
func TestAdminUsersManagement(t *testing.T) {
	gdb, router := integrationSetup(t)
	resetData(t, itDB)

	const userPass = "user-pass-123456"
	hash, err := auth.HashPassword(userPass)
	if err != nil {
		t.Fatalf("哈希用户密码失败: %v", err)
	}
	alice := domain.User{Username: "alice-mgmt", PasswordHash: hash, Role: domain.RoleUser, IsActive: true}
	if err := gdb.Create(&alice).Error; err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}
	if err := gdb.Create(&domain.UserFavorite{
		UserID: alice.ID, Kind: "tool", ObjectID: "redstone", Label: "红石资料", URL: "/tools/redstone",
	}).Error; err != nil {
		t.Fatalf("创建收藏失败: %v", err)
	}
	if err := gdb.Create(&domain.UserSeed{UserID: alice.ID, Name: "种子", Seed: "42"}).Error; err != nil {
		t.Fatalf("创建种子失败: %v", err)
	}

	cookie, csrf := loginAs(t, router, itAdminUser)

	// 1) 未登录 / 无权限：401 / 403。
	editorCookie, editorCSRF := loginAs(t, router, itEditorUser)
	rec := do(t, router, http.MethodPost, "/api/v1/admin/users/update",
		fmt.Sprintf(`{"id":%q,"is_active":false}`, alice.ID), editorCookie, editorCSRF)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("editor 更新期望 403，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	rec = do(t, router, http.MethodPost, "/api/v1/admin/users/update",
		fmt.Sprintf(`{"id":%q,"is_active":false}`, alice.ID), nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("匿名更新期望 401，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 2) 参数校验：缺 is_active → 422；缺 id → 422。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/users/update",
		fmt.Sprintf(`{"id":%q}`, alice.ID), cookie, csrf)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("缺 is_active 期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	rec = do(t, router, http.MethodPost, "/api/v1/admin/users/update", `{"is_active":false}`, cookie, csrf)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("缺 id 期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 3) 禁用：200 且落库为 false。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/users/update",
		fmt.Sprintf(`{"id":%q,"is_active":false}`, alice.ID), cookie, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("禁用期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	var acc domain.Account
	if err := gdb.Where("id = ?", alice.ID).First(&acc).Error; err != nil {
		t.Fatalf("查询账号失败: %v", err)
	}
	if acc.IsActive {
		t.Fatal("禁用后账号仍为启用状态")
	}
	// 再启用：200 且落库为 true。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/users/update",
		fmt.Sprintf(`{"id":%q,"is_active":true}`, alice.ID), cookie, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("启用期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 4) 重置密码：弱密码 422；合法密码 204，且新密码可登录。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/users/reset-password",
		fmt.Sprintf(`{"id":%q,"new_password":"short"}`, alice.ID), cookie, csrf)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("弱密码期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	const newPass = "new-user-pass-9876"
	rec = do(t, router, http.MethodPost, "/api/v1/admin/users/reset-password",
		fmt.Sprintf(`{"id":%q,"new_password":%q}`, alice.ID, newPass), cookie, csrf)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("重置密码期望 204，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	rec = do(t, router, http.MethodPost, "/api/v1/user/login",
		fmt.Sprintf(`{"username":"alice-mgmt","password":%q}`, newPass), nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("新密码登录期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	// 旧密码应失效。
	rec = do(t, router, http.MethodPost, "/api/v1/user/login",
		fmt.Sprintf(`{"username":"alice-mgmt","password":%q}`, userPass), nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("旧密码登录期望 401，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 5) 详情：含个人数据明细与计数。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/users/detail",
		fmt.Sprintf(`{"id":%q}`, alice.ID), cookie, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("详情期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	var detailResp struct {
		Data struct {
			Username      string `json:"username"`
			FavoriteCount int64  `json:"favorite_count"`
			SeedCount     int64  `json:"seed_count"`
			Favorites     []struct {
				ObjectID string `json:"object_id"`
			} `json:"favorites"`
			Seeds []struct {
				Seed string `json:"seed"`
			} `json:"seeds"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &detailResp); err != nil {
		t.Fatalf("解析详情失败: %v", err)
	}
	if detailResp.Data.Username != "alice-mgmt" || detailResp.Data.FavoriteCount != 1 || detailResp.Data.SeedCount != 1 {
		t.Fatalf("详情数据不符：%+v", detailResp.Data)
	}
	if len(detailResp.Data.Favorites) != 1 || detailResp.Data.Favorites[0].ObjectID != "redstone" {
		t.Fatalf("收藏明细不符：%+v", detailResp.Data.Favorites)
	}

	// 6) 导出：返回 CSV 附件，含表头与数据行。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/users/export", `{}`, cookie, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("导出期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Fatalf("导出 Content-Type 不符：%q", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "用户名") || !strings.Contains(body, "alice-mgmt") {
		t.Fatalf("导出内容不符：%q", body)
	}

	// 7) 边界：用普通用户管理接口操作后台账号，必须 404 且不改变其状态。
	var admin domain.Account
	if err := gdb.Where("username = ?", itAdminUser).First(&admin).Error; err != nil {
		t.Fatalf("查询后台账号失败: %v", err)
	}
	rec = do(t, router, http.MethodPost, "/api/v1/admin/users/update",
		fmt.Sprintf(`{"id":%q,"is_active":false}`, admin.ID), cookie, csrf)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("操作后台账号期望 404，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	var adminRecheck domain.Account
	if err := gdb.Where("id = ?", admin.ID).First(&adminRecheck).Error; err != nil {
		t.Fatalf("复查后台账号失败: %v", err)
	}
	if !adminRecheck.IsActive {
		t.Fatal("后台账号不应被普通用户管理接口停用")
	}
	rec = do(t, router, http.MethodPost, "/api/v1/admin/users/delete",
		fmt.Sprintf(`{"id":%q}`, admin.ID), cookie, csrf)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("删除后台账号期望 404，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 8) 删除普通用户：204，账号与个人数据一并清除。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/users/delete",
		fmt.Sprintf(`{"id":%q}`, alice.ID), cookie, csrf)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("删除期望 204，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	var cnt int64
	if err := gdb.Model(&domain.Account{}).Where("id = ?", alice.ID).Count(&cnt).Error; err != nil {
		t.Fatalf("复查账号失败: %v", err)
	}
	if cnt != 0 {
		t.Fatal("删除后账号仍存在")
	}
	if err := gdb.Model(&domain.UserFavorite{}).Where("user_id = ?", alice.ID).Count(&cnt).Error; err != nil {
		t.Fatalf("复查收藏失败: %v", err)
	}
	if cnt != 0 {
		t.Fatal("删除后个人数据未级联清除")
	}
	rec = do(t, router, http.MethodPost, "/api/v1/admin/users/detail",
		fmt.Sprintf(`{"id":%q}`, alice.ID), cookie, csrf)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("删除后详情期望 404，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 9) 非法 id 视为不存在（404），不得因 UUID 解析失败返回 500。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/users/detail", `{"id":"not-a-uuid"}`, cookie, csrf)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("非法 id 期望 404，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	rec = do(t, router, http.MethodPost, "/api/v1/admin/users/delete", `{"id":"not-a-uuid"}`, cookie, csrf)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("非法 id 删除期望 404，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 10) 创建普通用户：非法用户名 / 弱密码 422，与后台账号重名 409，
	//     成功 201 且 role 固定为 user、可直接前台登录、不能登后台。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/users/create",
		`{"username":"1bad","password":"good-pass-123"}`, cookie, csrf)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("非法用户名期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	rec = do(t, router, http.MethodPost, "/api/v1/admin/users/create",
		`{"username":"carol-mgmt","password":"short"}`, cookie, csrf)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("弱密码期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	// accounts 全局唯一：与已存在的后台账号重名 → 409。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/users/create",
		fmt.Sprintf(`{"username":%q,"password":"good-pass-123"}`, itAdminUser), cookie, csrf)
	if rec.Code != http.StatusConflict {
		t.Fatalf("重名期望 409，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	const carolPass = "carol-pass-123456"
	rec = do(t, router, http.MethodPost, "/api/v1/admin/users/create",
		fmt.Sprintf(`{"username":"carol-mgmt","password":%q}`, carolPass), cookie, csrf)
	if rec.Code != http.StatusCreated {
		t.Fatalf("创建期望 201，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), carolPass) {
		t.Fatalf("创建响应不应包含明文密码: %s", rec.Body.String())
	}
	var carol domain.Account
	if err := gdb.Where("username = ?", "carol-mgmt").First(&carol).Error; err != nil {
		t.Fatalf("查询新建用户失败: %v", err)
	}
	if carol.Role != domain.RoleUser || !carol.IsActive {
		t.Fatalf("新建用户角色/状态不符：role=%s active=%v", carol.Role, carol.IsActive)
	}
	// 新建用户可直接从前台登录。
	rec = do(t, router, http.MethodPost, "/api/v1/user/login",
		fmt.Sprintf(`{"username":"carol-mgmt","password":%q}`, carolPass), nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("新建用户前台登录期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	// 但不能从后台入口登录（role = user）。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/auth/login",
		fmt.Sprintf(`{"username":"carol-mgmt","password":%q}`, carolPass), nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("普通用户登后台期望 401，实际 %d（%s）", rec.Code, rec.Body.String())
	}
}

// TestUnifiedAccountBoundaries 覆盖迁移 0022「统一账号体系」的边界：
//   - admins / users 合并为 accounts，靠 role 区分身份，注册固定 role = 'user'；
//   - 双登录入口互不串号：普通用户不能登后台，后台账号不能登前台；
//   - 会话 Cookie 合并为同一个，但接口仍按角色隔离（401 / 403）。
func TestUnifiedAccountBoundaries(t *testing.T) {
	gdb, router := integrationSetup(t)
	resetData(t, itDB)

	const userPass = "user-pass-123456"

	// 1) 注册普通用户：固定 role = 'user' 且默认启用。
	rec := do(t, router, http.MethodPost, "/api/v1/user/register",
		fmt.Sprintf(`{"username":"it-normal","nickname":"集成用户","email":"it-normal@example.com","password":%q,"captcha_id":"it","captcha_answer":"135790"}`, userPass), nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("注册期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	var acc domain.Account
	if err := gdb.Where("username = ?", "it-normal").First(&acc).Error; err != nil {
		t.Fatalf("查询注册账号失败: %v", err)
	}
	if acc.Role != domain.RoleUser || !acc.IsActive {
		t.Fatalf("注册账号角色/状态不符：role=%q active=%v", acc.Role, acc.IsActive)
	}
	if acc.Email != "it-normal@example.com" {
		t.Fatalf("注册账号邮箱不符：%q", acc.Email)
	}
	if acc.Nickname != "集成用户" {
		t.Fatalf("注册账号昵称不符：%q", acc.Nickname)
	}

	// 2) 普通用户不能从后台入口登录（401，不泄露账号是否存在）。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/auth/login",
		fmt.Sprintf(`{"username":"it-normal","password":%q}`, userPass), nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("普通用户登后台期望 401，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 3) 后台账号不能从前台入口登录。
	rec = do(t, router, http.MethodPost, "/api/v1/user/login",
		fmt.Sprintf(`{"username":%q,"password":%q}`, itAdminUser, itPassword), nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("后台账号登前台期望 401，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 4) 普通用户登录前台：会话 Cookie 与后台同名（统一会话）。
	rec = do(t, router, http.MethodPost, "/api/v1/user/login",
		fmt.Sprintf(`{"username":"it-normal","password":%q}`, userPass), nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("前台登录期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	var userCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == middleware.SessionCookieName() {
			userCookie = c
		}
	}
	if userCookie == nil {
		t.Fatal("前台登录缺少统一会话 Cookie")
	}
	var loginResp struct {
		Data struct {
			CSRFToken string `json:"csrf_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &loginResp); err != nil {
		t.Fatalf("解析前台登录响应失败: %v", err)
	}
	userCSRF := loginResp.Data.CSRFToken

	// 5) 普通用户会话访问后台自助接口 → 401（前端据此视为未登录后台）。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/auth/me", `{}`, userCookie, userCSRF)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("普通用户访问 /admin/auth/me 期望 401，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 6) 普通用户会话访问后台业务接口 → 403（role=user 不含任何权限点）。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/users/list", `{}`, userCookie, userCSRF)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("普通用户访问后台接口期望 403，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 7) 后台会话访问普通用户接口 → 401（RequireUserRole 兜底）。
	adminCookie, adminCSRF := loginAs(t, router, itAdminUser)
	rec = do(t, router, http.MethodPost, "/api/v1/user/me", `{}`, adminCookie, adminCSRF)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("后台会话访问 /user/me 期望 401，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 8) 后台「普通用户管理」只返回普通用户，后台账号不混入。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/users/list", `{}`, adminCookie, adminCSRF)
	if rec.Code != http.StatusOK {
		t.Fatalf("普通用户列表期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	var userList struct {
		Data []struct {
			Username string `json:"username"`
		} `json:"data"`
		Meta struct {
			Total int64 `json:"total"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &userList); err != nil {
		t.Fatalf("解析用户列表失败: %v", err)
	}
	if userList.Meta.Total != 1 || len(userList.Data) != 1 || userList.Data[0].Username != "it-normal" {
		t.Fatalf("普通用户列表应只含 it-normal：%+v", userList)
	}

	// 9) 后台「管理员账号管理」不返回普通用户，且拒绝创建 role = 'user'。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/admins/list", `{}`, adminCookie, adminCSRF)
	if rec.Code != http.StatusOK {
		t.Fatalf("管理员列表期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	var adminList struct {
		Data []struct {
			Username string `json:"username"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &adminList); err != nil {
		t.Fatalf("解析管理员列表失败: %v", err)
	}
	for _, a := range adminList.Data {
		if a.Username == "it-normal" {
			t.Fatal("普通用户不应出现在管理员列表")
		}
	}
	rec = do(t, router, http.MethodPost, "/api/v1/admin/admins/create",
		`{"username":"it-should-fail","password":"another-strong-pass-1","role":"user"}`,
		adminCookie, adminCSRF)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("创建 role=user 期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}
}

// TestRoleManagement 覆盖后台「角色与权限底座」：
// 权限点目录、角色 CRUD 与守卫（内置不可删、被引用不可删、admin 必须全量权限），
// 以及自定义角色对路由访问的真实约束（RequirePermission）。
func TestRoleManagement(t *testing.T) {
	gdb, router := integrationSetup(t)
	resetData(t, itDB)

	adminCookie, adminCSRF := loginAs(t, router, itAdminUser)
	editorCookie, editorCSRF := loginAs(t, router, itEditorUser)

	// 1) editor 没有 roles.manage → 403。
	rec := do(t, router, http.MethodPost, "/api/v1/admin/roles/list", `{}`, editorCookie, editorCSRF)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("editor 访问角色列表期望 403，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 2) admin 列出内置角色，且 admin 角色持有全量权限。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/roles/list", `{}`, adminCookie, adminCSRF)
	if rec.Code != http.StatusOK {
		t.Fatalf("角色列表期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	var listResp struct {
		Data []domain.Role `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("解析角色列表失败: %v", err)
	}
	byName := map[string]domain.Role{}
	for _, r := range listResp.Data {
		byName[r.Name] = r
	}
	for _, want := range []string{domain.RoleAdmin, domain.RoleEditor, domain.RoleUser} {
		if _, ok := byName[want]; !ok {
			t.Fatalf("缺少内置角色 %s：%+v", want, byName)
		}
	}
	if len(byName[domain.RoleAdmin].Permissions) != len(domain.AllPermissions()) {
		t.Fatalf("内置 admin 应持有全量权限，实际 %v", byName[domain.RoleAdmin].Permissions)
	}

	// 3) 权限点目录：分组非空且全部为合法权限点。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/roles/permissions", `{}`, adminCookie, adminCSRF)
	if rec.Code != http.StatusOK {
		t.Fatalf("权限点目录期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	var permResp struct {
		Data []domain.PermissionGroup `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &permResp); err != nil {
		t.Fatalf("解析权限点目录失败: %v", err)
	}
	if len(permResp.Data) == 0 {
		t.Fatal("权限点目录不应为空")
	}
	total := 0
	for _, g := range permResp.Data {
		for _, p := range g.Permissions {
			total++
			if !domain.IsValidPermission(p.Key) {
				t.Fatalf("目录包含非法权限点 %s", p.Key)
			}
		}
	}
	if total != len(domain.AllPermissions()) {
		t.Fatalf("目录权限点数量 %d 与 AllPermissions %d 不一致", total, len(domain.AllPermissions()))
	}

	// 4) 新建自定义角色（仅工具管理）。
	const roleName = "it-toolkeeper"
	const keeperUser = "it-toolkeeper-user"
	cleanupRole := func() {
		gdb.Exec("DELETE FROM accounts WHERE username = ?", keeperUser)
		gdb.Exec("DELETE FROM roles WHERE name = ?", roleName)
	}
	cleanupRole()
	t.Cleanup(cleanupRole)

	rec = do(t, router, http.MethodPost, "/api/v1/admin/roles/create",
		`{"name":"`+roleName+`","label":"工具维护","description":"仅可管理工具","permissions":["content.tools"]}`,
		adminCookie, adminCSRF)
	if rec.Code != http.StatusCreated {
		t.Fatalf("创建角色期望 201，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 5) 非法权限点 → 422。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/roles/create",
		`{"name":"it-bad-perm","label":"非法","permissions":["nope.nope"]}`, adminCookie, adminCSRF)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("非法权限点期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 6) 内置角色不可删除 → 422。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/roles/delete",
		`{"name":"editor"}`, adminCookie, adminCSRF)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("删除内置角色期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 7) 内置 admin 角色不可被削权 → 422。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/roles/update",
		`{"name":"admin","permissions":["content.tools"]}`, adminCookie, adminCSRF)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("削权内置 admin 期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 8) 用自定义角色建管理员账号，验证权限点真实生效。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/admins/create",
		`{"username":"`+keeperUser+`","password":"`+itPassword+`","role":"`+roleName+`"}`,
		adminCookie, adminCSRF)
	if rec.Code != http.StatusCreated {
		t.Fatalf("创建自定义角色账号期望 201，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	keeperCookie, keeperCSRF := loginAs(t, router, keeperUser)

	// 有 content.tools → 工具列表 200。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/tools/list", `{}`, keeperCookie, keeperCSRF)
	if rec.Code != http.StatusOK {
		t.Fatalf("工具列表期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	// 无 content.resources → 403。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/resources/list", `{}`, keeperCookie, keeperCSRF)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("资源列表期望 403，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	// 无 roles.manage → 403。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/roles/list", `{}`, keeperCookie, keeperCSRF)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("角色列表期望 403，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 9) 角色仍被账号引用时不可删除 → 409。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/roles/delete",
		`{"name":"`+roleName+`"}`, adminCookie, adminCSRF)
	if rec.Code != http.StatusConflict {
		t.Fatalf("删除被引用角色期望 409，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 10) 删除账号后可删除角色。
	var keeper domain.Admin
	if err := gdb.Where("username = ?", keeperUser).First(&keeper).Error; err != nil {
		t.Fatalf("查询自定义角色账号失败: %v", err)
	}
	rec = do(t, router, http.MethodPost, "/api/v1/admin/admins/delete",
		`{"id":"`+keeper.ID.String()+`"}`, adminCookie, adminCSRF)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("删除账号期望 204，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	rec = do(t, router, http.MethodPost, "/api/v1/admin/roles/delete",
		`{"name":"`+roleName+`"}`, adminCookie, adminCSRF)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("删除角色期望 204，实际 %d（%s）", rec.Code, rec.Body.String())
	}
}

// TestMcsrcRoutesAuthGating 覆盖「MC 源码库」路由的鉴权边界：
// 只读接口公开（且非法入参不触发上游请求）；触发下载 / 建索引对**所有访客**开放
// （前端选中版本即自动调用），仅由「自动准备额度」约束。
//
// 集成配置里 MCSRCAutoPrepareMax=0（关闭自动准备），因此匿名 / 普通用户一律
// 429 AUTO_PREPARE_DISABLED —— 既验证了额度边界，也保证测试不会真的请求 Mojang 上游。
func TestMcsrcRoutesAuthGating(t *testing.T) {
	_, router := integrationSetup(t)

	// 公开只读：非法版本号 → 422（不会发起上游请求）。
	rec := do(t, router, http.MethodPost, "/api/v1/mcsrc/status", `{"version":"../etc/passwd"}`, nil, "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("非法版本号期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 公开只读：已准备列表 → 200。
	rec = do(t, router, http.MethodPost, "/api/v1/mcsrc/prepared", `{}`, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("已准备列表期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 公开只读：类结构视图 / 继承关系（版本未准备 → 404，不会触发下载）。
	// 注意：断言 404 而非具体错误码——若缓存目录存在历史 jar，
	// 也会因类不存在而返回 404 CLASS_NOT_FOUND，两种情形都是 404。
	rec = do(t, router, http.MethodPost, "/api/v1/mcsrc/classes/detail",
		`{"version":"1.21.4","class":"net.minecraft.server.MinecraftServer"}`, nil, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("未准备版本的类详情期望 404，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	rec = do(t, router, http.MethodPost, "/api/v1/mcsrc/classes/hierarchy",
		`{"version":"1.21.4","class":"net.minecraft.server.MinecraftServer"}`, nil, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("未准备版本的继承关系期望 404，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 公开只读：jar 下载（版本未缓存 → 404，不会触发下载；匿名可访问即证明公开）。
	// 用一个绝不会被缓存的版本号，避免共享临时缓存目录里的历史 jar 造成误判。
	rec = do(t, router, http.MethodPost, "/api/v1/mcsrc/jar", `{"version":"0.0.0-absent"}`, nil, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("未缓存版本的 jar 期望 404，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 公开只读：jar 缺 version → 422（入参校验先于任何 IO）。
	rec = do(t, router, http.MethodPost, "/api/v1/mcsrc/jar", `{}`, nil, "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("jar 缺 version 期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 公开只读：缺 class → 422（入参校验先于任何 IO）。
	rec = do(t, router, http.MethodPost, "/api/v1/mcsrc/classes/detail", `{"version":"1.21.4"}`, nil, "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("缺 class 期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 自动准备：匿名不再 401（OptionalAuth 只识别登录者，绝不拦截）。
	// 额度为 0（关闭自动准备）→ 429 AUTO_PREPARE_DISABLED，且不会发起上游请求。
	// 用绝不会被缓存的版本号，避免共享临时缓存目录里的历史 jar 影响判定。
	rec = do(t, router, http.MethodPost, "/api/v1/mcsrc/prepare", `{"version":"0.0.0-absent"}`, nil, "")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("匿名自动准备期望 429，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "AUTO_PREPARE_DISABLED") {
		t.Fatalf("错误码期望 AUTO_PREPARE_DISABLED，实际 %s", rec.Body.String())
	}

	// 已登录的非管理员（editor，无 ops.mcsrc）同样按额度约束，而非 403。
	editorCookie, editorCSRF := loginAs(t, router, itEditorUser)
	rec = do(t, router, http.MethodPost, "/api/v1/mcsrc/prepare", `{"version":"0.0.0-absent"}`, editorCookie, editorCSRF)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("editor 自动准备期望 429，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 入参校验先于额度判定：缺 version → 422。
	rec = do(t, router, http.MethodPost, "/api/v1/mcsrc/prepare", `{}`, nil, "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("缺 version 期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 映射重命名：公开只读（缺 class → 422，入参校验先于任何查询）。
	rec = do(t, router, http.MethodPost, "/api/v1/mcsrc/classes/mapping", `{"version":"1.21.4"}`, nil, "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("映射重命名缺 class 期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}
}

// TestMcsrcClassMappingLookup 覆盖「映射重命名」接口的数据层语义：
//   - 按版本 + jar 内类名（混淆名 / 可读名 / 中间名，点分与内部名两种写法）反查映射；
//   - 多数据集命中时按映射类型优先级选取（mojang > yarn），结果可预期；
//   - 该版本未导入映射、或没有数据集覆盖该类时返回 available=false（正常状态，非错误）。
func TestMcsrcClassMappingLookup(t *testing.T) {
	gdb, router := integrationSetup(t)

	// 独立的版本号与 set_key，避免与其它用例的数据相互污染。
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	version := "1.21.4-map-it-" + suffix
	mojangSet := domain.MappingSet{SetKey: "it-mojang-" + suffix, MCVersion: version, MappingType: domain.MappingTypeMojang}
	yarnSet := domain.MappingSet{SetKey: "it-yarn-" + suffix, MCVersion: version, MappingType: domain.MappingTypeYarn}
	for _, s := range []*domain.MappingSet{&mojangSet, &yarnSet} {
		if err := gdb.Create(s).Error; err != nil {
			t.Fatalf("创建映射数据集失败: %v", err)
		}
	}
	t.Cleanup(func() {
		gdb.Where("set_key IN ?", []string{mojangSet.SetKey, yarnSet.SetKey}).Delete(&domain.MappingSet{})
	})

	// 两个数据集都覆盖混淆类名 aab：mojang 应优先命中。
	mojangClass := domain.MappingClass{
		SetID: mojangSet.ID, ObfName: "aab", NamedName: "net.minecraft.server.MinecraftServer", MemberCount: 1,
	}
	if err := gdb.Create(&mojangClass).Error; err != nil {
		t.Fatalf("创建类映射失败: %v", err)
	}
	if err := gdb.Create(&domain.MappingMember{
		ClassID: mojangClass.ID, Kind: "method", ObfName: "a", NamedName: "tick",
		DescKey: "()V", ObfDesc: "()V",
	}).Error; err != nil {
		t.Fatalf("创建成员映射失败: %v", err)
	}
	if err := gdb.Create(&domain.MappingClass{
		SetID: yarnSet.ID, ObfName: "aab", NamedName: "net.minecraft.server.MinecraftServerYarn",
	}).Error; err != nil {
		t.Fatalf("创建 yarn 类映射失败: %v", err)
	}
	// 可读名用点分、内部名用 / 分隔，覆盖两种写法。
	if err := gdb.Create(&domain.MappingClass{
		SetID: mojangSet.ID, ObfName: "aac", NamedName: "net.minecraft.world.level.Level",
	}).Error; err != nil {
		t.Fatalf("创建可读名类映射失败: %v", err)
	}
	if err := gdb.Create(&domain.MappingClass{
		SetID: mojangSet.ID, ObfName: "aad", MidName: "net/minecraft/Util",
	}).Error; err != nil {
		t.Fatalf("创建中间名类映射失败: %v", err)
	}

	type mappingData struct {
		Available   bool   `json:"available"`
		MappingType string `json:"mapping_type"`
		MatchedBy   string `json:"matched_by"`
		Class       struct {
			NamedName string `json:"named_name"`
		} `json:"class"`
		Members []struct {
			Kind      string `json:"kind"`
			NamedName string `json:"named_name"`
		} `json:"members"`
	}
	lookup := func(class string) mappingData {
		t.Helper()
		rec := do(t, router, http.MethodPost, "/api/v1/mcsrc/classes/mapping",
			fmt.Sprintf(`{"version":%q,"class":%q}`, version, class), nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("映射查询 %s 期望 200，实际 %d（%s）", class, rec.Code, rec.Body.String())
		}
		var resp struct {
			Data mappingData `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("解析映射响应失败: %v（body=%s）", err, rec.Body.String())
		}
		return resp.Data
	}

	// 1) 混淆名命中，且 mojang 优先于 yarn。
	got := lookup("aab")
	if !got.Available || got.MappingType != domain.MappingTypeMojang || got.MatchedBy != "obf" {
		t.Fatalf("混淆名命中异常：%+v", got)
	}
	if got.Class.NamedName != "net.minecraft.server.MinecraftServer" {
		t.Fatalf("类可读名异常：%q", got.Class.NamedName)
	}
	if len(got.Members) != 1 || got.Members[0].NamedName != "tick" {
		t.Fatalf("成员映射异常：%+v", got.Members)
	}

	// 2) 可读名（点分）命中。
	got = lookup("net.minecraft.world.level.Level")
	if !got.Available || got.MatchedBy != "named" {
		t.Fatalf("可读名命中异常：%+v", got)
	}

	// 3) 中间名以内部名存储、以点分查询命中（分隔符归一化）。
	got = lookup("net.minecraft.Util")
	if !got.Available || got.MatchedBy != "mid" {
		t.Fatalf("中间名命中异常：%+v", got)
	}

	// 4) 已知版本但无覆盖 → available=false。
	if got = lookup("not.covered.Class"); got.Available {
		t.Fatalf("未覆盖类应 available=false，实际 %+v", got)
	}

	// 5) 未导入映射的版本 → available=false。
	rec := do(t, router, http.MethodPost, "/api/v1/mcsrc/classes/mapping",
		`{"version":"1.0.0-map-absent","class":"aab"}`, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("无映射版本期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data mappingData `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析映射响应失败: %v", err)
	}
	if resp.Data.Available {
		t.Fatalf("无映射版本应 available=false，实际 %+v", resp.Data)
	}
}

func TestLinkCheckBlocksSSRF(t *testing.T) {
	gdb, router := integrationSetup(t)
	resetData(t, itDB)

	cookie, csrf := loginAs(t, router, itAdminUser)
	body := `{"slug":"ssrf-it","name":"SSRF","summary":"s","resource_type":"mod",
		"source_url":"http://127.0.0.1/internal","status":"published"}`
	rec := do(t, router, http.MethodPost, "/api/v1/admin/resources/create", body, cookie, csrf)
	if rec.Code != http.StatusCreated {
		t.Fatalf("创建期望 201，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	id, _ := decodeData(t, rec)["id"].(string)

	rec = do(t, router, http.MethodPost, "/api/v1/admin/resources/check-link",
		fmt.Sprintf(`{"id":%q}`, id), cookie, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("链接检查期望 200（失败只更新状态），实际 %d（%s）", rec.Code, rec.Body.String())
	}
	data := decodeData(t, rec)
	if data["link_status"] != domain.LinkStatusBlocked {
		t.Fatalf("内网地址应被标记 blocked，实际 %v", data["link_status"])
	}

	// 状态应已落库，资源未被删除
	var res domain.Resource
	if err := gdb.Where("id = ?", id).First(&res).Error; err != nil {
		t.Fatalf("资源应仍存在: %v", err)
	}
	if res.LinkStatus != domain.LinkStatusBlocked {
		t.Fatalf("数据库 link_status 应为 blocked，实际 %s", res.LinkStatus)
	}
	if res.LastCheckedAt == nil {
		t.Fatal("last_checked_at 应被写入")
	}
	if res.Status != domain.StatusPublished {
		t.Fatalf("链接检查不应改变资源状态，实际 %s", res.Status)
	}
}

// TestFeedbackSubmit 覆盖 §13.2：反馈提交成功、过长字段与非法类型被拒绝。
func TestFeedbackSubmit(t *testing.T) {
	_, router := integrationSetup(t)
	resetData(t, itDB)

	rec := do(t, router, http.MethodPost, "/api/v1/feedback",
		`{"feedback_type":"bug","title":"标题","content":"内容"}`, nil, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("合法反馈期望 201，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	long := strings.Repeat("字", 5001)
	rec = do(t, router, http.MethodPost, "/api/v1/feedback",
		fmt.Sprintf(`{"feedback_type":"bug","title":"标题","content":%q}`, long), nil, "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("超长内容期望 422，实际 %d", rec.Code)
	}

	rec = do(t, router, http.MethodPost, "/api/v1/feedback",
		`{"feedback_type":"spam","title":"标题","content":"内容"}`, nil, "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("非法类型期望 422，实际 %d", rec.Code)
	}
}

// TestFeedbackRateLimit 覆盖 §5.10：反馈接口服务端限流。
func TestFeedbackRateLimit(t *testing.T) {
	_, router := integrationSetup(t)
	resetData(t, itDB)

	body := `{"feedback_type":"other","title":"标题","content":"内容"}`
	var limited bool
	for i := 0; i < 8; i++ {
		rec := do(t, router, http.MethodPost, "/api/v1/feedback", body, nil, "")
		if rec.Code == http.StatusTooManyRequests {
			limited = true
			break
		}
	}
	if !limited {
		t.Fatal("连续提交反馈应触发 429 限流")
	}
}

// TestErrorResponseNoLeak 覆盖 §13.2：错误响应不包含敏感内部信息。
func TestErrorResponseNoLeak(t *testing.T) {
	_, router := integrationSetup(t)
	resetData(t, itDB)

	rec := do(t, router, http.MethodPost, "/api/v1/resources/does-not-exist", `{}`, nil, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("期望 404，实际 %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "request_id") {
		t.Fatalf("错误响应应包含 request_id：%s", body)
	}
	for _, leak := range []string{"goroutine", "sql:", "SELECT ", "password", "stack"} {
		if strings.Contains(body, leak) {
			t.Fatalf("错误响应泄漏内部信息 %q：%s", leak, body)
		}
	}
}

// adminIDByName 查询指定用户名的管理员 ID。
func adminIDByName(t *testing.T, gdb *gorm.DB, username string) string {
	t.Helper()
	var a domain.Admin
	if err := gdb.Where("username = ?", username).First(&a).Error; err != nil {
		t.Fatalf("查询管理员 %s 失败: %v", username, err)
	}
	return a.ID.String()
}

// TestAdminAccountManagement 覆盖 §6.1 / §6.2：管理员账号增删改查与密码重置。
func TestAdminAccountManagement(t *testing.T) {
	_, router := integrationSetup(t)
	resetData(t, itDB)

	cookie, csrf := loginAs(t, router, itAdminUser)

	// 弱密码被拒
	rec := do(t, router, http.MethodPost, "/api/v1/admin/admins/create",
		`{"username":"it-managed","password":"short","role":"editor"}`, cookie, csrf)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("弱密码期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 创建 editor
	rec = do(t, router, http.MethodPost, "/api/v1/admin/admins/create",
		`{"username":"it-managed","password":"managed-pass-123","role":"editor"}`, cookie, csrf)
	if rec.Code != http.StatusCreated {
		t.Fatalf("创建账号期望 201，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	data := decodeData(t, rec)
	id, _ := data["id"].(string)
	if id == "" || data["role"] != domain.RoleEditor {
		t.Fatalf("创建响应异常: %v", data)
	}
	// 响应绝不包含密码或哈希（must_change_password 是状态标记，不是密码，故精确匹配键名）
	if strings.Contains(rec.Body.String(), "password_hash") || strings.Contains(rec.Body.String(), "managed-pass-123") {
		t.Fatalf("创建响应不应包含任何密码信息: %s", rec.Body.String())
	}

	// 重复用户名冲突
	rec = do(t, router, http.MethodPost, "/api/v1/admin/admins/create",
		`{"username":"it-managed","password":"managed-pass-123","role":"editor"}`, cookie, csrf)
	if rec.Code != http.StatusConflict {
		t.Fatalf("重复用户名期望 409，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 列表命中且不泄露哈希
	rec = do(t, router, http.MethodPost, "/api/v1/admin/admins/list", `{"q":"it-managed"}`, cookie, csrf)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "it-managed") {
		t.Fatalf("列表应包含新账号：%d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "password_hash") || strings.Contains(rec.Body.String(), "managed-pass-123") {
		t.Fatalf("列表不应泄露密码信息: %s", rec.Body.String())
	}

	// 停用
	rec = do(t, router, http.MethodPost, "/api/v1/admin/admins/update",
		fmt.Sprintf(`{"id":%q,"is_active":false}`, id), cookie, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("停用期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	if decodeData(t, rec)["is_active"] != false {
		t.Fatal("停用后 is_active 应为 false")
	}

	// 重置密码
	rec = do(t, router, http.MethodPost, "/api/v1/admin/admins/reset-password",
		fmt.Sprintf(`{"id":%q,"new_password":"reset-pass-4567"}`, id), cookie, csrf)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("重置密码期望 204，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 已停用账号无法登录
	rec = do(t, router, http.MethodPost, "/api/v1/admin/auth/login",
		`{"username":"it-managed","password":"reset-pass-4567"}`, nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("停用账号登录期望 401，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 删除
	rec = do(t, router, http.MethodPost, "/api/v1/admin/admins/delete",
		fmt.Sprintf(`{"id":%q}`, id), cookie, csrf)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("删除期望 204，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	rec = do(t, router, http.MethodPost, "/api/v1/admin/admins/list", `{"q":"it-managed"}`, cookie, csrf)
	if strings.Contains(rec.Body.String(), "it-managed") {
		t.Fatalf("删除后列表不应包含该账号：%s", rec.Body.String())
	}
}

// TestEditorCannotManageAdmins 覆盖 §6.2 / §13.2：editor 不能访问账号管理（403）。
func TestEditorCannotManageAdmins(t *testing.T) {
	_, router := integrationSetup(t)
	resetData(t, itDB)

	cookie, csrf := loginAs(t, router, itEditorUser)
	for _, path := range []string{
		"/api/v1/admin/admins/list",
		"/api/v1/admin/admins/create",
		"/api/v1/admin/admins/update",
		"/api/v1/admin/admins/delete",
	} {
		rec := do(t, router, http.MethodPost, path, `{}`, cookie, csrf)
		if rec.Code != http.StatusForbidden {
			t.Errorf("editor 访问 %s 期望 403，实际 %d", path, rec.Code)
		}
	}
}

// TestAdminSelfGuards 覆盖：不能停用/删除自己；不能把最后一名 admin 降级。
func TestAdminSelfGuards(t *testing.T) {
	gdb, router := integrationSetup(t)
	resetData(t, itDB)

	adminID := adminIDByName(t, gdb, itAdminUser)
	cookie, csrf := loginAs(t, router, itAdminUser)

	rec := do(t, router, http.MethodPost, "/api/v1/admin/admins/update",
		fmt.Sprintf(`{"id":%q,"is_active":false}`, adminID), cookie, csrf)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("停用自己期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	rec = do(t, router, http.MethodPost, "/api/v1/admin/admins/delete",
		fmt.Sprintf(`{"id":%q}`, adminID), cookie, csrf)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("删除自己期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 收敛为唯一启用 admin 后，降级自己应被拒绝
	gdb.Exec("UPDATE accounts SET is_active = false WHERE role = 'admin' AND username <> ?", itAdminUser)
	rec = do(t, router, http.MethodPost, "/api/v1/admin/admins/update",
		fmt.Sprintf(`{"id":%q,"role":"editor"}`, adminID), cookie, csrf)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("降级最后一名 admin 期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}
}

// TestItemDetailWithRelatedRecipes 覆盖 TODO §1「统一物品资料库」：
// 物品详情返回扩展属性、生成式获取指令，以及数据驱动的关联配方（产出 / 作为材料）。
func TestItemDetailWithRelatedRecipes(t *testing.T) {
	gdb, router := integrationSetup(t)
	resetData(t, itDB)

	verified := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	stack, durability := 1, 1561
	item := domain.Item{
		ItemID:            "minecraft:diamond_pickaxe",
		NameZh:            "钻石镐",
		NameEn:            "Diamond Pickaxe",
		Aliases:           types.JSONSlice{"钻石镐子"},
		Description:       "顶级镐",
		ItemCategory:      domain.ItemCategoryTool,
		StackSize:         &stack,
		Durability:        &durability,
		ObtainMethod:      "由 3 个钻石与 2 个木棍合成。",
		UsageNotes:        "快速开采石头与矿石。",
		GameEdition:       "java",
		SupportedVersions: types.JSONSlice{"1.21"},
		SourceURL:         "https://minecraft.wiki/w/Diamond_Pickaxe",
		DataVersion:       "demo-2026-01",
		LastVerifiedAt:    &verified,
	}
	if err := gdb.Create(&item).Error; err != nil {
		t.Fatalf("写入物品失败: %v", err)
	}

	// 产出该物品的配方
	if err := gdb.Create(&domain.Recipe{
		RecipeKey: "minecraft:diamond_pickaxe", RecipeType: "shaped", GameVersion: "1.21",
		ResultItemID: "minecraft:diamond_pickaxe", ResultCount: 1,
		Ingredients: types.JSONB(`[{"item_id":"minecraft:diamond","count":3}]`),
	}).Error; err != nil {
		t.Fatalf("写入产出配方失败: %v", err)
	}
	// 以该物品为材料的配方
	if err := gdb.Create(&domain.Recipe{
		RecipeKey: "minecraft:test_uses_pickaxe", RecipeType: "shaped", GameVersion: "1.21",
		ResultItemID: "minecraft:decoration", ResultCount: 1,
		Ingredients: types.JSONB(`[{"item_id":"minecraft:diamond_pickaxe","count":1}]`),
	}).Error; err != nil {
		t.Fatalf("写入材料配方失败: %v", err)
	}
	// 不相关配方：既非产出也非材料，不得出现在关联中
	if err := gdb.Create(&domain.Recipe{
		RecipeKey: "minecraft:unrelated", RecipeType: "shaped", GameVersion: "1.21",
		ResultItemID: "minecraft:stone", ResultCount: 1,
		Ingredients: types.JSONB(`[{"item_id":"minecraft:cobblestone","count":1}]`),
	}).Error; err != nil {
		t.Fatalf("写入无关配方失败: %v", err)
	}

	rec := do(t, router, http.MethodPost, "/api/v1/items/minecraft:diamond_pickaxe", `{}`, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("物品详情期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	data := decodeData(t, rec)

	if data["item_id"] != "minecraft:diamond_pickaxe" || data["name_zh"] != "钻石镐" {
		t.Fatalf("物品基础字段异常: %v", data)
	}
	if data["item_category"] != domain.ItemCategoryTool {
		t.Errorf("类别应为 tool，实际 %v", data["item_category"])
	}
	if data["durability"] != float64(1561) {
		t.Errorf("耐久度应为 1561，实际 %v", data["durability"])
	}
	if data["usage"] != "快速开采石头与矿石。" {
		t.Errorf("用途字段异常: %v", data["usage"])
	}
	if data["last_verified_at"] == nil {
		t.Errorf("最后校验时间不应为空: %v", data["last_verified_at"])
	}
	if data["obtain_command"] != "/give @s minecraft:diamond_pickaxe 1" {
		t.Errorf("获取指令应由 item_id 生成，实际 %v", data["obtain_command"])
	}

	related, _ := data["related_recipes"].(map[string]any)
	produced, _ := related["produced"].([]any)
	usedAs, _ := related["used_as"].([]any)
	if len(produced) != 1 {
		t.Errorf("产出配方应为 1 条，实际 %d（%v）", len(produced), related)
	}
	if len(usedAs) != 1 {
		t.Errorf("作为材料的配方应为 1 条，实际 %d（%v）", len(usedAs), related)
	}
	if strings.Contains(rec.Body.String(), "minecraft:unrelated") {
		t.Errorf("无关配方不应出现在关联中: %s", rec.Body.String())
	}

	// TODO §3：关联物品解析 —— 只链接真实存在的物品，未收录物品不得出现在映射中。
	resolved, _ := related["resolved_items"].(map[string]any)
	if _, ok := resolved["minecraft:diamond_pickaxe"]; !ok {
		t.Errorf("已收录物品应出现在 resolved_items 中: %v", related)
	}
	if _, ok := resolved["minecraft:diamond"]; ok {
		t.Errorf("未收录物品不得出现在 resolved_items 中（避免断链）: %v", related)
	}

	// 未收录物品应 404，且不伪造详情
	rec = do(t, router, http.MethodPost, "/api/v1/items/minecraft:not_exist", `{}`, nil, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("未收录物品期望 404，实际 %d", rec.Code)
	}
}

// TestItemListSearchableByAlias 覆盖 TODO §1「统一检索」：
// 物品列表按别名 / 英文名可检索，并返回新的扩展字段。
func TestItemListSearchableByAlias(t *testing.T) {
	gdb, router := integrationSetup(t)
	resetData(t, itDB)

	if err := gdb.Create(&domain.Item{
		ItemID: "minecraft:stone", NameZh: "石头", NameEn: "Stone",
		Aliases:      types.JSONSlice{"原石"},
		ItemCategory: domain.ItemCategoryBlock,
		GameEdition:  "java",
	}).Error; err != nil {
		t.Fatalf("写入物品失败: %v", err)
	}

	rec := do(t, router, http.MethodPost, "/api/v1/items", `{"q":"原石"}`, nil, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "minecraft:stone") {
		t.Fatalf("按别名应能检索到物品：%d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"item_category":"block"`) {
		t.Fatalf("列表应返回资料类别字段：%s", rec.Body.String())
	}
}

// TestSearchResultIncludesURL 覆盖：搜索结果携带可跳转的详情页 URL（含物品详情页）。
func TestSearchResultIncludesURL(t *testing.T) {
	gdb, router := integrationSetup(t)
	resetData(t, itDB)

	if err := gdb.Create(&domain.Item{
		ItemID: "minecraft:apple", NameZh: "苹果", ItemCategory: domain.ItemCategoryFood,
		GameEdition: "java", Description: "可食用",
	}).Error; err != nil {
		t.Fatalf("写入物品失败: %v", err)
	}

	rec := do(t, router, http.MethodPost, "/api/v1/search", `{"q":"苹果","type":"item"}`, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("搜索期望 200，实际 %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"url":"/items/minecraft:apple"`) {
		t.Fatalf("物品搜索结果应包含详情页 URL：%s", rec.Body.String())
	}
}

// TestChangePasswordInvalidatesSessions 覆盖 §6.1：
// 改密成功后其他设备的会话也立即失效，必须用新密码重新登录。
func TestChangePasswordInvalidatesSessions(t *testing.T) {
	gdb, router := integrationSetup(t)
	resetData(t, itDB)

	deviceA, csrfA := loginAs(t, router, itAdminUser)
	deviceB, _ := loginAs(t, router, itAdminUser)

	rec := do(t, router, http.MethodPost, "/api/v1/admin/auth/change-password",
		fmt.Sprintf(`{"old_password":%q,"new_password":"new-integration-pass-1"}`, itPassword), deviceA, csrfA)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("改密期望 204，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 两个设备的会话都应失效
	for name, c := range map[string]*http.Cookie{"设备A": deviceA, "设备B": deviceB} {
		r := do(t, router, http.MethodPost, "/api/v1/admin/auth/me", `{}`, c, "")
		if r.Code != http.StatusUnauthorized {
			t.Fatalf("改密后 %s 会话应失效（401），实际 %d", name, r.Code)
		}
	}

	// 还原初始密码，避免影响其它用例
	hash, err := auth.HashPassword(itPassword)
	if err != nil {
		t.Fatalf("重建哈希失败: %v", err)
	}
	if err := gdb.Model(&domain.Admin{}).Where("username = ?", itAdminUser).
		Update("password_hash", hash).Error; err != nil {
		t.Fatalf("还原密码失败: %v", err)
	}
}

// checkCount 从概览响应中取出指定检查项的命中数。
func checkCount(t *testing.T, body map[string]any, key string) float64 {
	t.Helper()
	checks, _ := body["checks"].([]any)
	for _, raw := range checks {
		m, _ := raw.(map[string]any)
		if m["key"] == key {
			n, _ := m["count"].(float64)
			return n
		}
	}
	bodyJSON, _ := json.Marshal(body)
	t.Fatalf("概览缺少检查项 %q：%s", key, bodyJSON)
	return 0
}

// TestDataQualityOverview 覆盖 TODO §2 / §4：
// 版本缺失、来源缺失、悬空引用均能被客观统计；非 admin 无权访问。
func TestDataQualityOverview(t *testing.T) {
	gdb, router := integrationSetup(t)
	resetData(t, itDB)

	// 物品：一条完整，一条缺来源与版本
	if err := gdb.Create(&domain.Item{
		ItemID: "minecraft:stone", NameZh: "石头", ItemCategory: domain.ItemCategoryBlock,
		GameEdition: "java", SourceURL: "https://minecraft.wiki/w/Stone",
		SupportedVersions: types.JSONSlice{"1.21"},
	}).Error; err != nil {
		t.Fatalf("写入物品失败: %v", err)
	}
	if err := gdb.Create(&domain.Item{
		ItemID: "minecraft:dirt", NameZh: "泥土", ItemCategory: domain.ItemCategoryBlock,
		GameEdition: "java",
	}).Error; err != nil {
		t.Fatalf("写入物品失败: %v", err)
	}

	// 配方：一条有效，一条结果物品不存在（悬空引用）
	if err := gdb.Create(&domain.Recipe{
		RecipeKey: "minecraft:stone_from_cobblestone", RecipeType: "smelting", GameVersion: "1.21",
		ResultItemID: "minecraft:stone", ResultCount: 1,
		Ingredients:       types.JSONB(`[{"item_id":"minecraft:cobblestone","count":1}]`),
		SupportedVersions: types.JSONSlice{"1.21"},
		SourceURL:         "https://minecraft.wiki/w/Stone",
	}).Error; err != nil {
		t.Fatalf("写入配方失败: %v", err)
	}
	if err := gdb.Create(&domain.Recipe{
		RecipeKey: "minecraft:orphan_recipe", RecipeType: "shaped", GameVersion: "1.21",
		ResultItemID: "minecraft:not_exist", ResultCount: 1,
		Ingredients: types.JSONB(`[]`),
	}).Error; err != nil {
		t.Fatalf("写入悬空配方失败: %v", err)
	}

	// 指令：缺来源
	if err := gdb.Create(&domain.Command{
		Slug: "give", Name: "give", Syntax: "/give <item>", Description: "给予物品",
		GameVersions: types.JSONSlice{"1.21"}, Status: domain.StatusPublished,
	}).Error; err != nil {
		t.Fatalf("写入指令失败: %v", err)
	}

	cookie, csrf := loginAs(t, router, itAdminUser)
	rec := do(t, router, http.MethodPost, "/api/v1/admin/data-quality/overview", `{}`, cookie, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("概览期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	data := decodeData(t, rec)

	if got := checkCount(t, data, "items_missing_source"); got != 1 {
		t.Errorf("items_missing_source 期望 1，实际 %v", got)
	}
	if got := checkCount(t, data, "items_missing_versions"); got != 1 {
		t.Errorf("items_missing_versions 期望 1，实际 %v", got)
	}
	if got := checkCount(t, data, "recipes_orphan_result"); got != 1 {
		t.Errorf("recipes_orphan_result 期望 1，实际 %v", got)
	}
	if got := checkCount(t, data, "recipes_missing_versions"); got != 1 {
		t.Errorf("recipes_missing_versions 期望 1，实际 %v", got)
	}
	if got := checkCount(t, data, "commands_missing_source"); got != 1 {
		t.Errorf("commands_missing_source 期望 1，实际 %v", got)
	}
	if got := checkCount(t, data, "items_total"); got != 2 {
		t.Errorf("items_total 期望 2，实际 %v", got)
	}
	// 存在严重问题（悬空引用）与提醒项
	if n, _ := data["error_checks"].(float64); n < 1 {
		t.Errorf("error_checks 应至少为 1，实际 %v", data["error_checks"])
	}
	if n, _ := data["warning_checks"].(float64); n < 1 {
		t.Errorf("warning_checks 应至少为 1，实际 %v", data["warning_checks"])
	}

	// editor 无权访问管理端数据质量接口
	editorCookie, editorCsrf := loginAs(t, router, itEditorUser)
	rec = do(t, router, http.MethodPost, "/api/v1/admin/data-quality/overview", `{}`, editorCookie, editorCsrf)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("editor 访问数据质量期望 403，实际 %d", rec.Code)
	}

	// 未登录必须 401
	rec = do(t, router, http.MethodPost, "/api/v1/admin/data-quality/overview", `{}`, nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("未登录访问期望 401，实际 %d", rec.Code)
	}
}

// TestDataImportJobsList 覆盖 TODO §4：
// 导入任务可按类型过滤查询，失败摘要原样返回，非 admin 无权访问。
func TestDataImportJobsList(t *testing.T) {
	gdb, router := integrationSetup(t)
	resetData(t, itDB)

	now := time.Now().UTC()
	if err := gdb.Create(&domain.DataImportJob{
		DataType: "items", SourceFile: "seeds/items_demo.json", TotalCount: 12,
		CreatedCount: 11, FailedCount: 1,
		ErrorSummary: types.JSONSlice{"第 4 行：item_category 非法"},
		Operator:     "tester", StartedAt: now, FinishedAt: &now,
	}).Error; err != nil {
		t.Fatalf("写入导入任务失败: %v", err)
	}
	if err := gdb.Create(&domain.DataImportJob{
		DataType: "recipes", SourceFile: "seeds/recipes_demo.json", TotalCount: 5,
		CreatedCount: 5, Operator: "tester", StartedAt: now, FinishedAt: &now,
	}).Error; err != nil {
		t.Fatalf("写入导入任务失败: %v", err)
	}

	cookie, csrf := loginAs(t, router, itAdminUser)
	rec := do(t, router, http.MethodPost, "/api/v1/admin/data-import-jobs/list", `{"page":1,"page_size":10}`, cookie, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("任务列表期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "seeds/items_demo.json") ||
		!strings.Contains(rec.Body.String(), "item_category 非法") {
		t.Fatalf("任务列表缺少导入记录：%s", rec.Body.String())
	}

	// 按类型过滤
	rec = do(t, router, http.MethodPost, "/api/v1/admin/data-import-jobs/list", `{"data_type":"recipes"}`, cookie, csrf)
	body := rec.Body.String()
	if strings.Contains(body, "seeds/items_demo.json") || !strings.Contains(body, "seeds/recipes_demo.json") {
		t.Fatalf("按类型过滤结果异常：%s", body)
	}

	// 非 admin 拒绝
	editorCookie, editorCsrf := loginAs(t, router, itEditorUser)
	rec = do(t, router, http.MethodPost, "/api/v1/admin/data-import-jobs/list", `{}`, editorCookie, editorCsrf)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("editor 访问导入任务期望 403，实际 %d", rec.Code)
	}
}

// TestRecipeListResolvesItems 覆盖 TODO §3「在配方查询结果中关联涉及的物品资料」：
// 只解析真实收录的物品；未收录物品不得生成链接（有效性检查）。
func TestRecipeListResolvesItems(t *testing.T) {
	gdb, router := integrationSetup(t)
	resetData(t, itDB)

	for _, it := range []domain.Item{
		{ItemID: "minecraft:iron_ingot", NameZh: "铁锭", NameEn: "Iron Ingot",
			ItemCategory: domain.ItemCategoryMaterial, GameEdition: "java"},
		{ItemID: "minecraft:raw_iron", NameZh: "粗铁", NameEn: "Raw Iron",
			ItemCategory: domain.ItemCategoryMaterial, GameEdition: "java"},
	} {
		if err := gdb.Create(&it).Error; err != nil {
			t.Fatalf("写入物品失败: %v", err)
		}
	}
	// 材料中含未收录物品 minecraft:coal，它不应出现在解析结果中。
	if err := gdb.Create(&domain.Recipe{
		RecipeKey: "minecraft:iron_ingot_from_blasting", RecipeType: "blasting", GameVersion: "1.21",
		ResultItemID: "minecraft:iron_ingot", ResultCount: 1,
		Ingredients: types.JSONB(`[{"item_id":"minecraft:raw_iron","count":1},{"item_id":"minecraft:coal","count":1}]`),
	}).Error; err != nil {
		t.Fatalf("写入配方失败: %v", err)
	}

	rec := do(t, router, http.MethodPost, "/api/v1/recipes", `{"item_id":"minecraft:iron_ingot"}`, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("配方查询期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data []struct {
			RecipeKey     string                    `json:"recipe_key"`
			ResolvedItems map[string]map[string]any `json:"resolved_items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析配方响应失败: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("应返回 1 条配方，实际 %d", len(resp.Data))
	}
	resolved := resp.Data[0].ResolvedItems
	if ref, ok := resolved["minecraft:iron_ingot"]; !ok || ref["url"] != "/items/minecraft:iron_ingot" {
		t.Fatalf("结果物品应被解析并携带详情页链接: %v", resolved)
	}
	if ref, ok := resolved["minecraft:raw_iron"]; !ok || ref["name_zh"] != "粗铁" {
		t.Fatalf("材料物品应被解析: %v", resolved)
	}
	if _, ok := resolved["minecraft:coal"]; ok {
		t.Fatalf("未收录物品不得出现在 resolved_items 中: %v", resolved)
	}
}

// TestSearchRecordsAnonymousTerms 覆盖 TODO §5：
// 搜索词以匿名聚合形式记录，无结果时给出可尝试的关键词，超长输入被拒绝。
func TestSearchRecordsAnonymousTerms(t *testing.T) {
	gdb, router := integrationSetup(t)
	resetData(t, itDB)

	if err := gdb.Create(&domain.Item{
		ItemID: "minecraft:diamond_pickaxe", NameZh: "钻石镐", ItemCategory: domain.ItemCategoryTool,
		GameEdition: "java",
	}).Error; err != nil {
		t.Fatalf("写入物品失败: %v", err)
	}

	// 1) 无结果搜索：记录并累加 no_result_hits
	for i := 0; i < 2; i++ {
		rec := do(t, router, http.MethodPost, "/api/v1/search", `{"q":"红石","type":"item"}`, nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("搜索期望 200，实际 %d", rec.Code)
		}
	}

	var term domain.SearchTerm
	if err := gdb.Where("term = ?", "红石").First(&term).Error; err != nil {
		t.Fatalf("搜索词应被记录: %v", err)
	}
	if term.Hits != 2 || term.NoResultHits != 2 || term.LastResultCount != 0 {
		t.Fatalf("匿名统计异常: hits=%d no_result=%d last=%d", term.Hits, term.NoResultHits, term.LastResultCount)
	}

	// 2) 扩展词无结果时，应给出包含它的已记录关键词作为建议
	rec := do(t, router, http.MethodPost, "/api/v1/search", `{"q":"红石机器","type":"item"}`, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("搜索期望 200，实际 %d", rec.Code)
	}
	var resp struct {
		Data []any `json:"data"`
		Meta struct {
			Total       int64    `json:"total"`
			Suggestions []string `json:"suggestions"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析搜索响应失败: %v", err)
	}
	if resp.Meta.Total != 0 {
		t.Fatalf("该关键词应无结果，实际 total=%d", resp.Meta.Total)
	}
	found := false
	for _, s := range resp.Meta.Suggestions {
		if s == "红石" {
			found = true
		}
	}
	if !found {
		t.Fatalf("无结果时应建议已记录的相近关键词，实际 %v", resp.Meta.Suggestions)
	}

	// 3) 有结果的关键词：hits 增加但 no_result_hits 不变
	rec = do(t, router, http.MethodPost, "/api/v1/search", `{"q":"Diamond Pickaxe","type":"item"}`, nil, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "minecraft:diamond_pickaxe") {
		t.Fatalf("英文名应可检索到物品：%d %s", rec.Code, rec.Body.String())
	}
	if err := gdb.Where("term = ?", "diamond pickaxe").First(&term).Error; err == nil {
		if term.Hits != 1 || term.NoResultHits != 0 || term.LastResultCount != 1 {
			t.Fatalf("有结果搜索统计异常: hits=%d no_result=%d last=%d",
				term.Hits, term.NoResultHits, term.LastResultCount)
		}
	}

	// 4) 超长关键词拒绝（TODO §5 输入防护）
	tooLong := strings.Repeat("a", 81)
	rec = do(t, router, http.MethodPost, "/api/v1/search",
		fmt.Sprintf(`{"q":%q,"type":"item"}`, tooLong), nil, "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("超长关键词应返回 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}
}

// TestSearchSortByUpdated 覆盖 TODO §5「支持按相关性 / 最近更新排序」。
func TestSearchSortByUpdated(t *testing.T) {
	gdb, router := integrationSetup(t)
	resetData(t, itDB)

	older := time.Now().UTC().Add(-48 * time.Hour)
	newer := time.Now().UTC()
	oldTool := domain.Tool{Slug: "sort-old", Name: "排序工具甲", Summary: "s",
		Category: domain.ToolCategoryQuery, Status: domain.StatusPublished}
	newTool := domain.Tool{Slug: "sort-new", Name: "排序工具乙", Summary: "s",
		Category: domain.ToolCategoryQuery, Status: domain.StatusPublished}
	if err := gdb.Create(&oldTool).Error; err != nil {
		t.Fatalf("写入工具失败: %v", err)
	}
	if err := gdb.Create(&newTool).Error; err != nil {
		t.Fatalf("写入工具失败: %v", err)
	}
	// 显式覆盖 updated_at，避免依赖创建顺序。
	gdb.Model(&domain.Tool{}).Where("id = ?", oldTool.ID).UpdateColumn("updated_at", older)
	gdb.Model(&domain.Tool{}).Where("id = ?", newTool.ID).UpdateColumn("updated_at", newer)

	rec := do(t, router, http.MethodPost, "/api/v1/search",
		`{"q":"排序工具","type":"tool","sort":"updated_desc"}`, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("搜索期望 200，实际 %d", rec.Code)
	}
	var resp struct {
		Data []struct {
			Slug string `json:"slug"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析搜索响应失败: %v", err)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("应返回 2 条，实际 %d", len(resp.Data))
	}
	if resp.Data[0].Slug != "sort-new" {
		t.Fatalf("按更新时间降序时最新工具应排首位，实际 %v", resp.Data)
	}
}

// TestSearchFailureRecording 覆盖 TODO §5「记录……查询失败情况」：
// 查询失败与「无结果」必须分开计数，二者处置方式完全不同。
func TestSearchFailureRecording(t *testing.T) {
	gdb, router := integrationSetup(t)
	resetData(t, itDB)

	stats := searchstats.NewService(gdb)
	ctx := context.Background()
	// 同一个词：先失败两次（带空白，验证归一化），再成功一次（有结果）
	for i := 0; i < 2; i++ {
		if err := stats.RecordFailure(ctx, "  Boom  "); err != nil {
			t.Fatalf("记录失败搜索出错: %v", err)
		}
	}
	if err := stats.Record(ctx, "Boom", 4); err != nil {
		t.Fatalf("记录成功搜索出错: %v", err)
	}

	var term domain.SearchTerm
	if err := gdb.Where("term = ?", "boom").First(&term).Error; err != nil {
		t.Fatalf("搜索词应按归一化结果记录: %v", err)
	}
	if term.Hits != 3 || term.FailedHits != 2 || term.NoResultHits != 0 || term.LastResultCount != 4 {
		t.Fatalf("失败与无结果必须分开计数: hits=%d failed=%d no_result=%d last=%d",
			term.Hits, term.FailedHits, term.NoResultHits, term.LastResultCount)
	}

	cookie, csrf := loginAs(t, router, itAdminUser)
	rec := do(t, router, http.MethodPost, "/api/v1/admin/search-terms/list", `{"kind":"failed"}`, cookie, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("失败词列表期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); !strings.Contains(body, "boom") || !strings.Contains(body, `"failed_hits":2`) {
		t.Fatalf("失败词列表内容异常：%s", body)
	}

	// 只有失败记录的词不应出现在「无结果」统计中
	rec = do(t, router, http.MethodPost, "/api/v1/admin/search-terms/list", `{"kind":"no_result"}`, cookie, csrf)
	if strings.Contains(rec.Body.String(), "boom") {
		t.Fatalf("仅有失败记录的词不应计入无结果统计：%s", rec.Body.String())
	}
}

// TestSearchTermsAdminAccess 覆盖 TODO §5 管理端统计的权限边界。
func TestSearchTermsAdminAccess(t *testing.T) {
	gdb, router := integrationSetup(t)
	resetData(t, itDB)

	if err := gdb.Create(&domain.SearchTerm{Term: "红石", Hits: 3, NoResultHits: 3}).Error; err != nil {
		t.Fatalf("写入搜索词失败: %v", err)
	}
	if err := gdb.Create(&domain.SearchTerm{Term: "钻石", Hits: 9, NoResultHits: 0, LastResultCount: 2}).Error; err != nil {
		t.Fatalf("写入搜索词失败: %v", err)
	}

	// 未登录 401
	rec := do(t, router, http.MethodPost, "/api/v1/admin/search-terms/list", `{}`, nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("未登录期望 401，实际 %d", rec.Code)
	}

	// editor 403
	editorCookie, editorCsrf := loginAs(t, router, itEditorUser)
	rec = do(t, router, http.MethodPost, "/api/v1/admin/search-terms/list", `{}`, editorCookie, editorCsrf)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("editor 期望 403，实际 %d", rec.Code)
	}

	// admin：默认按搜索次数排序
	cookie, csrf := loginAs(t, router, itAdminUser)
	rec = do(t, router, http.MethodPost, "/api/v1/admin/search-terms/list", `{"page":1,"page_size":10}`, cookie, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin 列表期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data []struct {
			Term string `json:"term"`
		} `json:"data"`
		Meta struct {
			Total int64 `json:"total"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析列表响应失败: %v", err)
	}
	if resp.Meta.Total != 2 || len(resp.Data) != 2 || resp.Data[0].Term != "钻石" {
		t.Fatalf("热门搜索应按 hits 降序：total=%d data=%v", resp.Meta.Total, resp.Data)
	}

	// kind=no_result：只返回有无结果记录的词
	rec = do(t, router, http.MethodPost, "/api/v1/admin/search-terms/list", `{"kind":"no_result"}`, cookie, csrf)
	if !strings.Contains(rec.Body.String(), "红石") || strings.Contains(rec.Body.String(), "钻石") {
		t.Fatalf("高频无结果词过滤异常：%s", rec.Body.String())
	}
}

// TestResourceReleasesAndCompatibility 覆盖 TODO §6：
// ① 无版本数据时兼容性结论为「未验证」；
// ② 发布版本可整体替换并幂等；
// ③ 公开详情返回 releases 与由发布版本推导的兼容性结论；
// ④ 依赖结构被规范化（缺省 type 补齐）。
func TestResourceReleasesAndCompatibility(t *testing.T) {
	_, router := integrationSetup(t)
	resetData(t, itDB)

	cookie, csrf := loginAs(t, router, itAdminUser)

	// 无 supported_versions / loader_types → 未验证
	createBody := `{"slug":"compat-it","name":"兼容性测试","summary":"摘要","resource_type":"mod",
		"source_url":"https://example.com/compat","status":"published",
		"dependencies":[{"name":"Forge"},{"name":"OptiFine","type":"conflict","note":"冲突"}]}`
	rec := do(t, router, http.MethodPost, "/api/v1/admin/resources/create", createBody, cookie, csrf)
	if rec.Code != http.StatusCreated {
		t.Fatalf("创建期望 201，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	id, _ := decodeData(t, rec)["id"].(string)

	// 管理端详情应带 compatibility.verified=false
	rec = do(t, router, http.MethodPost, "/api/v1/admin/resources/detail",
		fmt.Sprintf(`{"id":%q}`, id), cookie, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("详情期望 200，实际 %d", rec.Code)
	}
	detail := decodeData(t, rec)
	compat, _ := detail["compatibility"].(map[string]any)
	if compat == nil {
		t.Fatalf("详情应包含 compatibility：%s", rec.Body.String())
	}
	if verified, _ := compat["verified"].(bool); verified {
		t.Fatalf("无版本数据时不应给出已验证结论：%v", compat)
	}
	if hint, _ := compat["unverified_hint"].(string); hint == "" {
		t.Fatal("未验证时必须给出明确提示")
	}

	// 替换发布版本
	releasesBody := fmt.Sprintf(`{"id":%q,"releases":[
		{"version_label":"19.5.0","game_versions":["1.21.1"],"loader_types":["neoforge"],
		 "source_url":"https://example.com/compat/19","released_at":"2024-09-12","is_latest":true},
		{"version_label":"15.2.0","game_versions":["1.20.1"],"loader_types":["forge"],
		 "source_url":"https://example.com/compat/15","released_at":"2023-11-05"}
	]}`, id)
	rec = do(t, router, http.MethodPost, "/api/v1/admin/resources/releases/replace", releasesBody, cookie, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("替换发布版本期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 幂等重跑：数量保持一致
	rec = do(t, router, http.MethodPost, "/api/v1/admin/resources/releases/replace", releasesBody, cookie, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("幂等重跑期望 200，实际 %d", rec.Code)
	}
	rec = do(t, router, http.MethodPost, "/api/v1/admin/resources/releases/list",
		fmt.Sprintf(`{"id":%q}`, id), cookie, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("发布版本列表期望 200，实际 %d", rec.Code)
	}
	var listResp struct {
		Data []struct {
			VersionLabel string `json:"version_label"`
			IsLatest     bool   `json:"is_latest"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("解析发布版本失败: %v", err)
	}
	if len(listResp.Data) != 2 || !listResp.Data[0].IsLatest {
		t.Fatalf("发布版本应为 2 条且最新排首位：%+v", listResp.Data)
	}

	// 管理端详情同样返回 releases（后台编辑页依赖该字段）
	rec = do(t, router, http.MethodPost, "/api/v1/admin/resources/detail",
		fmt.Sprintf(`{"id":%q}`, id), cookie, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("管理端详情期望 200，实际 %d", rec.Code)
	}
	adminDetail := decodeData(t, rec)
	if rels, _ := adminDetail["releases"].([]any); len(rels) != 2 {
		t.Fatalf("管理端详情应返回 2 条发布版本，实际 %v", adminDetail["releases"])
	}

	// 公开详情：compatibility 由发布版本推导
	rec = do(t, router, http.MethodPost, "/api/v1/resources/compat-it", `{}`, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("公开详情期望 200，实际 %d", rec.Code)
	}
	pub := decodeData(t, rec)
	pubCompat, _ := pub["compatibility"].(map[string]any)
	if pubCompat == nil {
		t.Fatalf("公开详情应包含 compatibility：%s", rec.Body.String())
	}
	if verified, _ := pubCompat["verified"].(bool); !verified {
		t.Fatalf("发布版本提供数据后应给出已验证结论：%v", pubCompat)
	}
	versions, _ := pubCompat["game_versions"].([]any)
	if len(versions) != 2 {
		t.Fatalf("兼容版本应合并为 2 项，实际 %v", versions)
	}
	sources, _ := pubCompat["sources"].([]any)
	if len(sources) != 1 || sources[0] != "releases" {
		t.Fatalf("结论来源应为 releases，实际 %v", sources)
	}
	if releases, _ := pub["releases"].([]any); len(releases) != 2 {
		t.Fatalf("公开详情应返回 2 条发布版本，实际 %v", pub["releases"])
	}

	// 依赖缺省 type 应被补齐为 dependency
	deps, _ := pub["dependencies"].([]any)
	if len(deps) != 2 {
		t.Fatalf("依赖应为 2 条，实际 %v", deps)
	}
	first, _ := deps[0].(map[string]any)
	if first["type"] != "dependency" {
		t.Fatalf("依赖 type 缺省应补齐为 dependency，实际 %v", first)
	}
}

// TestResourceRejectsInvalidMetadata 覆盖 TODO §6：非法加载器 / 版本 / 依赖 / 发布版本
// 必须在写入前拦截为 422，而不是落库报错。
func TestResourceRejectsInvalidMetadata(t *testing.T) {
	_, router := integrationSetup(t)
	resetData(t, itDB)

	cookie, csrf := loginAs(t, router, itAdminUser)
	base := `{"slug":"meta-it","name":"元数据校验","summary":"摘要","resource_type":"mod",
		"source_url":"https://example.com/meta","status":"draft"`

	cases := []struct {
		name string
		body string
	}{
		{"非法加载器", base + `,"loader_types":["bukkit"]}`},
		{"非法版本", base + `,"supported_versions":["版本一"]}`},
		{"重复版本", base + `,"supported_versions":["1.20.1","1.20.1"]}`},
		{"非法依赖结构", base + `,"dependencies":[{"name":""}]}`},
		{"非法依赖类型", base + `,"dependencies":[{"name":"X","type":"suggests"}]}`},
		{"非法依赖链接", base + `,"dependencies":[{"name":"X","resource_url":"javascript:alert(1)"}]}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(t, router, http.MethodPost, "/api/v1/admin/resources/create", tc.body, cookie, csrf)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
			}
		})
	}

	// 发布版本校验：缺少来源 / 多条 is_latest
	rec := do(t, router, http.MethodPost, "/api/v1/admin/resources/create",
		base+`,"releases":[{"version_label":"1.0"}]}`, cookie, csrf)
	if rec.Code != http.StatusCreated {
		t.Fatalf("创建期望 201，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	id, _ := decodeData(t, rec)["id"].(string)

	badRelease := fmt.Sprintf(`{"id":%q,"releases":[{"version_label":"1.0"}]}`, id)
	if rec := do(t, router, http.MethodPost, "/api/v1/admin/resources/releases/replace", badRelease, cookie, csrf); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("缺少 source_url 期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	twoLatest := fmt.Sprintf(`{"id":%q,"releases":[
		{"version_label":"1.0","source_url":"https://e.com/1","is_latest":true},
		{"version_label":"2.0","source_url":"https://e.com/2","is_latest":true}]}`, id)
	if rec := do(t, router, http.MethodPost, "/api/v1/admin/resources/releases/replace", twoLatest, cookie, csrf); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("多条 is_latest 期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 未登录不得访问发布版本接口
	if rec := do(t, router, http.MethodPost, "/api/v1/admin/resources/releases/list",
		fmt.Sprintf(`{"id":%q}`, id), nil, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("未登录期望 401，实际 %d", rec.Code)
	}
}

// TestResourceAllVersionsWildcard 覆盖 TODO §6-C：
// ① 声明「所有版本」（all / *）视为有数据依据，并返回 all_versions；
// ② 通配必须独占，与具体版本并列返回 422；
// ③ 版本筛选：声明通配的资源对任何具体版本筛选都命中；
// ④ 「全部 MC 版本」筛选（game_version=all）只命中声明通配的资源；
// ⑤ 搜索同样遵循通配语义。
func TestResourceAllVersionsWildcard(t *testing.T) {
	_, router := integrationSetup(t)
	resetData(t, itDB)

	cookie, csrf := loginAs(t, router, itAdminUser)

	// ① 通配声明（用 * 验证归一化）
	createBody := `{"slug":"allver-it","name":"全版本资源","summary":"摘要","resource_type":"mod",
		"source_url":"https://example.com/allver","status":"published","supported_versions":["*"],
		"loader_types":["fabric"]}`
	rec := do(t, router, http.MethodPost, "/api/v1/admin/resources/create", createBody, cookie, csrf)
	if rec.Code != http.StatusCreated {
		t.Fatalf("创建期望 201，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	allID, _ := decodeData(t, rec)["id"].(string)

	rec = do(t, router, http.MethodPost, "/api/v1/resources/allver-it", `{}`, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("详情期望 200，实际 %d", rec.Code)
	}
	pub := decodeData(t, rec)
	compat, _ := pub["compatibility"].(map[string]any)
	if compat == nil {
		t.Fatalf("详情应包含 compatibility：%s", rec.Body.String())
	}
	if verified, _ := compat["verified"].(bool); !verified {
		t.Fatalf("通配声明应视为已验证：%v", compat)
	}
	if all, _ := compat["all_versions"].(bool); !all {
		t.Fatalf("应返回 all_versions=true：%v", compat)
	}
	versions, _ := compat["game_versions"].([]any)
	if len(versions) != 1 || versions[0] != "all" {
		t.Fatalf("通配应归一化为 all，实际 %v", versions)
	}

	// ② 通配与具体版本并列 → 422
	badBody := `{"slug":"allver-bad","name":"非法","summary":"摘要","resource_type":"mod",
		"source_url":"https://example.com/bad","status":"draft","supported_versions":["all","1.20.1"]}`
	if rec := do(t, router, http.MethodPost, "/api/v1/admin/resources/create", badBody, cookie, csrf); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("通配与具体版本并列期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 另建一条只声明具体版本的资源，用于对比筛选结果
	specificBody := `{"slug":"specver-it","name":"特定版本资源","summary":"摘要","resource_type":"mod",
		"source_url":"https://example.com/specver","status":"published","supported_versions":["1.20.1"]}`
	if rec := do(t, router, http.MethodPost, "/api/v1/admin/resources/create", specificBody, cookie, csrf); rec.Code != http.StatusCreated {
		t.Fatalf("创建具体版本资源期望 201，实际 %d", rec.Code)
	}

	// ③ 具体版本筛选应同时命中「声明该版本」与「声明通配」的资源
	rec = do(t, router, http.MethodPost, "/api/v1/resources", `{"game_version":"1.20.1"}`, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("列表期望 200，实际 %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "allver-it") || !strings.Contains(body, "specver-it") {
		t.Fatalf("具体版本筛选应命中通配与具体声明两条：%s", body)
	}

	// ④ game_version=all 只命中声明通配的资源
	rec = do(t, router, http.MethodPost, "/api/v1/resources", `{"game_version":"all"}`, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("列表期望 200，实际 %d", rec.Code)
	}
	body = rec.Body.String()
	if !strings.Contains(body, "allver-it") {
		t.Fatalf("all 筛选应命中通配资源：%s", body)
	}
	if strings.Contains(body, "specver-it") {
		t.Fatalf("all 筛选不应命中仅声明具体版本的资源：%s", body)
	}

	// ⑤ 搜索：通配资源对具体版本筛选同样命中
	rec = do(t, router, http.MethodPost, "/api/v1/search",
		`{"q":"全版本资源","type":"resource","game_version":"1.20.1"}`, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("搜索期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "allver-it") {
		t.Fatalf("搜索应命中通配资源：%s", rec.Body.String())
	}

	_ = allID
}

// TestUsageStatsAndAnalytics 覆盖 TODO §13：
// ① 匿名上报按白名单校验对象（内置工具放行、未知对象 422、非法入参 422）；
// ② 采集落库后 admin 运营看板可查询概览 / 明细 / 趋势，且不出现具体参数值；
// ③ 权限边界：未登录 401、editor 403。
func TestUsageStatsAndAnalytics(t *testing.T) {
	gdb, router := integrationSetup(t)
	resetData(t, itDB)

	// ① 上报内置工具的访问 / 使用 / 失败
	for _, body := range []string{
		`{"scope":"tool","id":"region-inspect","action":"visit"}`,
		`{"scope":"tool","id":"region-inspect","action":"use"}`,
		`{"scope":"tool","id":"region-inspect","action":"use"}`,
		`{"scope":"tool","id":"region-inspect","action":"fail"}`,
	} {
		rec := do(t, router, http.MethodPost, "/api/v1/stats/event", body, nil, "")
		if rec.Code != http.StatusNoContent {
			t.Fatalf("上报期望 204，实际 %d（%s）", rec.Code, rec.Body.String())
		}
	}

	// 未知工具（不在内置清单也不在工具表）应被拒绝，避免脏数据污染统计
	if rec := do(t, router, http.MethodPost, "/api/v1/stats/event",
		`{"scope":"tool","id":"not-a-real-tool","action":"visit"}`, nil, ""); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("未知工具期望 422，实际 %d", rec.Code)
	}
	// 非法 scope / action / id
	for _, body := range []string{
		`{"scope":"bogus","id":"x","action":"visit"}`,
		`{"scope":"tool","id":"region-inspect","action":"bogus"}`,
		`{"scope":"resource","id":"region-inspect","action":"use"}`,
		`{"scope":"tool","id":"bad id!","action":"visit"}`,
	} {
		if r := do(t, router, http.MethodPost, "/api/v1/stats/event", body, nil, ""); r.Code != http.StatusUnprocessableEntity {
			t.Fatalf("非法入参 %s 期望 422，实际 %d", body, r.Code)
		}
	}

	// 资源详情访问：公开详情接口自动累计，无需前端上报。
	if err := gdb.Create(&domain.Resource{
		Slug: "stats-res", Name: "统计资源", Summary: "s", ResourceType: "mod",
		SourceURL: "https://example.com/s", Status: domain.StatusPublished,
	}).Error; err != nil {
		t.Fatalf("写入资源失败: %v", err)
	}
	if rec := do(t, router, http.MethodPost, "/api/v1/resources/stats-res", `{}`, nil, ""); rec.Code != http.StatusOK {
		t.Fatalf("资源详情期望 200，实际 %d", rec.Code)
	}

	// 落库并直接校验聚合结果
	if err := itCollector.Flush(context.Background()); err != nil {
		t.Fatalf("统计落库失败: %v", err)
	}
	var toolRow domain.ToolStatDaily
	if err := gdb.Where("tool_slug = ?", "region-inspect").First(&toolRow).Error; err != nil {
		t.Fatalf("工具统计应已落库: %v", err)
	}
	if toolRow.Visits != 1 || toolRow.Uses != 2 || toolRow.Failures != 1 {
		t.Fatalf("工具统计异常: %+v", toolRow)
	}
	var resRow domain.ResourceStatDaily
	if err := gdb.Where("resource_slug = ?", "stats-res").First(&resRow).Error; err != nil {
		t.Fatalf("资源统计应已落库: %v", err)
	}
	if resRow.Views != 1 {
		t.Fatalf("资源访问应为 1，实际 %d", resRow.Views)
	}

	// ② 运营看板概览
	cookie, csrf := loginAs(t, router, itAdminUser)
	rec := do(t, router, http.MethodPost, "/api/v1/admin/analytics/overview", `{"days":7}`, cookie, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("概览期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	overview := decodeData(t, rec)
	if got, _ := overview["tool_uses"].(float64); got != 2 {
		t.Errorf("概览 tool_uses 期望 2，实际 %v", overview["tool_uses"])
	}
	if got, _ := overview["tool_failures"].(float64); got != 1 {
		t.Errorf("概览 tool_failures 期望 1，实际 %v", overview["tool_failures"])
	}
	if got, _ := overview["active_tools"].(float64); got != 1 {
		t.Errorf("概览 active_tools 期望 1，实际 %v", overview["active_tools"])
	}
	if got, _ := overview["resource_views"].(float64); got != 1 {
		t.Errorf("概览 resource_views 期望 1，实际 %v", overview["resource_views"])
	}
	// 失败率 = 1/2
	if got, _ := overview["tool_failure_rate"].(float64); got != 0.5 {
		t.Errorf("概览 tool_failure_rate 期望 0.5，实际 %v", overview["tool_failure_rate"])
	}

	// 工具明细
	rec = do(t, router, http.MethodPost, "/api/v1/admin/analytics/tools", `{"days":7}`, cookie, csrf)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "region-inspect") {
		t.Fatalf("工具明细异常：%d %s", rec.Code, rec.Body.String())
	}

	// 趋势：区间内每一天都存在（缺失补零）
	rec = do(t, router, http.MethodPost, "/api/v1/admin/analytics/trend", `{"days":3}`, cookie, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("趋势期望 200，实际 %d", rec.Code)
	}
	var trend struct {
		Data []struct {
			Date       string `json:"date"`
			ToolUses   int64  `json:"tool_uses"`
			ToolVisits int64  `json:"tool_visits"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &trend); err != nil {
		t.Fatalf("解析趋势失败: %v", err)
	}
	if len(trend.Data) != 3 {
		t.Fatalf("趋势应补零为 3 天，实际 %d", len(trend.Data))
	}
	if trend.Data[2].ToolUses != 2 || trend.Data[2].ToolVisits != 1 {
		t.Fatalf("最后一天趋势异常: %+v", trend.Data[2])
	}

	// API 指标只保留路由模板，不含具体参数值（隐私要求）
	rec = do(t, router, http.MethodPost, "/api/v1/admin/analytics/api", `{"days":7}`, cookie, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("API 指标期望 200，实际 %d", rec.Code)
	}
	apiBody := rec.Body.String()
	if !strings.Contains(apiBody, `/api/v1/resources/{slug}`) {
		t.Fatalf("API 指标应包含路由模板：%s", apiBody)
	}
	if strings.Contains(apiBody, "/api/v1/resources/stats-res") {
		t.Fatalf("API 指标不得包含具体参数值：%s", apiBody)
	}

	// ③ 权限边界
	if rec := do(t, router, http.MethodPost, "/api/v1/admin/analytics/overview", `{}`, nil, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("未登录期望 401，实际 %d", rec.Code)
	}
	editorCookie, editorCSRF := loginAs(t, router, itEditorUser)
	if rec := do(t, router, http.MethodPost, "/api/v1/admin/analytics/overview", `{}`, editorCookie, editorCSRF); rec.Code != http.StatusForbidden {
		t.Fatalf("editor 期望 403，实际 %d", rec.Code)
	}

	// 非法时间范围
	if rec := do(t, router, http.MethodPost, "/api/v1/admin/analytics/overview",
		`{"from":"2026-13-01","to":"2026-12-31"}`, cookie, csrf); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("非法日期期望 422，实际 %d", rec.Code)
	}
	if rec := do(t, router, http.MethodPost, "/api/v1/admin/analytics/overview",
		`{"from":"2026-12-31","to":"2026-01-01"}`, cookie, csrf); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("from 晚于 to 期望 422，实际 %d", rec.Code)
	}
}

// TestResourceSubmissionWorkflow 覆盖「用户提交资源 → 管理员审核 → 转为资源」闭环：
//   - 结构化投稿落库为 pending，字段归一化（游戏版本 / 加载器）；
//   - 同账号同来源链接重复提交被拒（部分唯一索引）；
//   - 非投稿作者看不到他人投稿（我的投稿按会话账号过滤）；
//   - 后台按状态筛选、审核通过生成草稿资源并回填 resource_id；
//   - 驳回不生成资源；
//   - 重复审核被拒；
//   - 权限边界：普通用户不能访问审核接口。
func TestResourceSubmissionWorkflow(t *testing.T) {
	gdb, router := integrationSetup(t)
	resetData(t, itDB)

	const userPass = "submission-pass-123456"

	// 注册并前台登录普通用户（登录返回统一会话 Cookie + CSRF）。
	rec := do(t, router, http.MethodPost, "/api/v1/user/register",
		fmt.Sprintf(`{"username":"it-submitter","nickname":"投稿人","email":"it-submitter@example.com","password":%q,"captcha_id":"it","captcha_answer":"135790"}`, userPass), nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("注册期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	rec = do(t, router, http.MethodPost, "/api/v1/user/login",
		fmt.Sprintf(`{"identifier":"it-submitter","password":%q}`, userPass), nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("前台登录期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	var loginResp struct {
		Data struct {
			CSRFToken string `json:"csrf_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &loginResp); err != nil {
		t.Fatalf("解析登录响应失败: %v", err)
	}
	var userCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == middleware.SessionCookieName() {
			userCookie = c
		}
	}
	if userCookie == nil {
		t.Fatal("前台登录缺少会话 Cookie")
	}
	userCSRF := loginResp.Data.CSRFToken

	// 未登录提交 → 401。
	if rec := do(t, router, http.MethodPost, "/api/v1/user/submissions/submit", `{}`, nil, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("未登录提交期望 401，实际 %d", rec.Code)
	}

	// 缺必填字段 → 422（带字段名）。
	rec = do(t, router, http.MethodPost, "/api/v1/user/submissions/submit",
		`{"summary":"缺少名称"}`, userCookie, userCSRF)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("缺名称期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 未接受授权条款 → 422（投稿须明确授权，TODO §17）。
	rec = do(t, router, http.MethodPost, "/api/v1/user/submissions/submit",
		`{"name":"未授权资源","summary":"未确认授权","resource_type":"mod","source_url":"https://example.com/no-license"}`,
		userCookie, userCSRF)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("未接受授权期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	submitBody := `{"name":"集成测试资源","summary":"用于集成测试","resource_type":"mod","source_url":"https://example.com/it-resource","game_edition":"java","supported_versions":["1.20.1"],"loader_types":["fabric"],"author_name":"作者甲","license":"MIT","license_version":"2026-10-19"}`
	rec = do(t, router, http.MethodPost, "/api/v1/user/submissions/submit", submitBody, userCookie, userCSRF)
	if rec.Code != http.StatusCreated {
		t.Fatalf("提交期望 201，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	sub := decodeData(t, rec)
	subID, _ := sub["id"].(string)
	if subID == "" {
		t.Fatalf("提交响应缺少 id：%+v", sub)
	}
	if sub["status"] != domain.SubmissionStatusPending {
		t.Fatalf("初始状态应为 pending，实际 %v", sub["status"])
	}

	// 同来源链接重复提交 → 409。
	rec = do(t, router, http.MethodPost, "/api/v1/user/submissions/submit", submitBody, userCookie, userCSRF)
	if rec.Code != http.StatusConflict {
		t.Fatalf("重复来源期望 409，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 我的投稿列表：1 条。
	rec = do(t, router, http.MethodPost, "/api/v1/user/submissions/list", `{}`, userCookie, userCSRF)
	if rec.Code != http.StatusOK {
		t.Fatalf("我的投稿期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	var mine struct {
		Data []map[string]any `json:"data"`
		Meta struct {
			Total int64 `json:"total"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &mine); err != nil {
		t.Fatalf("解析我的投稿失败: %v", err)
	}
	if mine.Meta.Total != 1 || len(mine.Data) != 1 {
		t.Fatalf("我的投稿应 1 条，实际 total=%d len=%d", mine.Meta.Total, len(mine.Data))
	}

	// 权限边界：普通用户不能访问后台审核接口。
	if rec := do(t, router, http.MethodPost, "/api/v1/admin/submissions/list", `{}`, userCookie, userCSRF); rec.Code != http.StatusForbidden {
		t.Fatalf("普通用户访问审核列表期望 403，实际 %d", rec.Code)
	}

	// 管理员：按 pending 筛选，审核通过。
	adminCookie, adminCSRF := loginAs(t, router, itAdminUser)
	rec = do(t, router, http.MethodPost, "/api/v1/admin/submissions/list",
		`{"status":"pending"}`, adminCookie, adminCSRF)
	if rec.Code != http.StatusOK {
		t.Fatalf("审核列表期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	var pending struct {
		Data []map[string]any `json:"data"`
		Meta struct {
			Total int64 `json:"total"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &pending); err != nil {
		t.Fatalf("解析审核列表失败: %v", err)
	}
	if pending.Meta.Total != 1 {
		t.Fatalf("待审核应 1 条，实际 %d", pending.Meta.Total)
	}

	rec = do(t, router, http.MethodPost, "/api/v1/admin/submissions/review",
		fmt.Sprintf(`{"id":%q,"status":"approved","note":"字段完整"}`, subID), adminCookie, adminCSRF)
	if rec.Code != http.StatusOK {
		t.Fatalf("审核通过期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	reviewed := decodeData(t, rec)
	resObj, _ := reviewed["resource"].(map[string]any)
	if resObj == nil {
		t.Fatalf("审核通过应返回生成的资源：%+v", reviewed)
	}
	resourceID, _ := resObj["id"].(string)
	if resourceID == "" {
		t.Fatalf("生成的资源缺少 id：%+v", resObj)
	}
	// 默认生成草稿（需人工核验后发布），且来源链接透传。
	if resObj["status"] != domain.StatusDraft {
		t.Fatalf("默认应生成草稿，实际 %v", resObj["status"])
	}
	if resObj["source_url"] != "https://example.com/it-resource" {
		t.Fatalf("来源链接应透传，实际 %v", resObj["source_url"])
	}
	subAfter, _ := reviewed["submission"].(map[string]any)
	if subAfter["status"] != domain.SubmissionStatusApproved || subAfter["resource_id"] != resourceID {
		t.Fatalf("投稿应回填 approved + resource_id：%+v", subAfter)
	}

	// 生成的资源可被资源管理读回。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/resources/detail",
		fmt.Sprintf(`{"id":%q}`, resourceID), adminCookie, adminCSRF)
	if rec.Code != http.StatusOK {
		t.Fatalf("资源详情期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 重复审核 → 409。
	rec = do(t, router, http.MethodPost, "/api/v1/admin/submissions/review",
		fmt.Sprintf(`{"id":%q,"status":"rejected"}`, subID), adminCookie, adminCSRF)
	if rec.Code != http.StatusConflict {
		t.Fatalf("重复审核期望 409，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 驳回路径：新投稿 → 驳回不生成资源。
	rec = do(t, router, http.MethodPost, "/api/v1/user/submissions/submit",
		`{"name":"待驳回资源","summary":"来源不可靠","resource_type":"modpack","source_url":"https://example.com/reject-me","license_version":"2026-10-19"}`, userCookie, userCSRF)
	if rec.Code != http.StatusCreated {
		t.Fatalf("第二次提交期望 201，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	sub2 := decodeData(t, rec)
	sub2ID, _ := sub2["id"].(string)
	rec = do(t, router, http.MethodPost, "/api/v1/admin/submissions/review",
		fmt.Sprintf(`{"id":%q,"status":"rejected","note":"来源不可靠"}`, sub2ID), adminCookie, adminCSRF)
	if rec.Code != http.StatusOK {
		t.Fatalf("驳回期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	rejected := decodeData(t, rec)
	if _, ok := rejected["resource"]; ok {
		t.Fatalf("驳回不应返回资源：%+v", rejected)
	}
	if s, _ := rejected["submission"].(map[string]any); s["status"] != domain.SubmissionStatusRejected {
		t.Fatalf("驳回后状态应为 rejected：%+v", rejected["submission"])
	}

	// 审计留痕（审核动作写入 audit_logs）。
	var auditCount int64
	if err := gdb.Model(&domain.AuditLog{}).
		Where("action = ?", "resource_submission.review").Count(&auditCount).Error; err != nil {
		t.Fatalf("查询审计失败: %v", err)
	}
	if auditCount != 2 {
		t.Fatalf("应有 2 条审核审计，实际 %d", auditCount)
	}
}
