// main 程序：启动 MC 工具站 API 服务。
// 依据需求文档 §8：所有查询/搜索/列表接口统一使用 POST，参数放在 JSON body。
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"mckit/backend/internal/audit"
	"mckit/backend/internal/auth"
	"mckit/backend/internal/blockids"
	"mckit/backend/internal/config"
	"mckit/backend/internal/domain"
	"mckit/backend/internal/envutil"
	"mckit/backend/internal/http/handler"
	"mckit/backend/internal/http/middleware"
	"mckit/backend/internal/http/response"
	lk "mckit/backend/internal/linkcheck"
	"mckit/backend/internal/mcsrc"
	"mckit/backend/internal/mcversion"
	db "mckit/backend/internal/repository"
	"mckit/backend/internal/search"
	"mckit/backend/internal/service"
	"mckit/backend/internal/service/adminaccount"
	"mckit/backend/internal/service/analytics"
	"mckit/backend/internal/service/catalog"
	"mckit/backend/internal/service/content"
	"mckit/backend/internal/service/dataquality"
	"mckit/backend/internal/service/linkhealth"
	"mckit/backend/internal/service/metrics"
	"mckit/backend/internal/service/resource"
	"mckit/backend/internal/service/role"
	"mckit/backend/internal/service/searchstats"
	"mckit/backend/internal/service/sitemap"
	"mckit/backend/internal/service/user"
	"mckit/backend/internal/service/useradmin"
)

func main() {
	_ = envutil.LoadDotEnv()
	cfg, err := config.Load()
	if err != nil {
		slog.Error("配置加载失败", "err", err)
		os.Exit(1)
	}
	cfgInstance = cfg
	middleware.SetCookieName(cfg.SessionCookieName)

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: levelFromString(cfg.LogLevel),
	}))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(),
		syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	database, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		slog.Error("数据库初始化失败", "err", err)
		os.Exit(1)
	}
	sqlDB, err := database.DB()
	if err != nil {
		slog.Error("获取底层连接失败", "err", err)
		os.Exit(1)
	}
	defer sqlDB.Close()

	// 使用统计采集（TODO §13）：内存聚合 + 定时落库，只记录路由模板与公开 slug。
	collector := newStatsCollector(cfg, database)
	collector.Start(ctx)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recover)
	r.Use(middleware.AccessLog(logger))
	r.Use(middleware.Metrics(collector))
	r.Use(middleware.SecurityHeaders)
	r.Use(middleware.CORS(cfg.CORSAllowedOrigins))
	r.Use(middleware.RateLimit(cfg.RateLimitRPS, cfg.RateLimitBurst))

	api := chi.NewRouter()
	registerRoutes(api, database, collector)
	api.NotFound(func(w http.ResponseWriter, r *http.Request) {
		response.NotFound(w, r)
	})
	api.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		response.Error(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "请求方法不允许", nil)
	})

	r.Mount("/api/v1", api)

	// SEO：robots.txt 与 sitemap.xml 挂在根路径。
	// sitemap 动态收录数据库中的资源/百科/工具详情页。
	seoHandler := handler.NewSEOHandler(cfg.AppBaseURL, sitemap.NewService(database))
	r.Get("/robots.txt", seoHandler.RobotsTxt)
	r.Get("/sitemap.xml", seoHandler.SitemapIndex)

	srv := &http.Server{
		Addr:              cfg.AppAddr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		slog.Info("API 服务启动", "addr", cfg.AppAddr, "env", cfg.AppEnv)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("服务异常退出", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	slog.Info("收到退出信号，正在优雅关闭")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("优雅关闭失败", "err", err)
	}
	// 最终落库使用统计（Stop 为空操作时为 nil）。
	collector.Stop()
	slog.Info("服务已退出")
}

// newStatsCollector 创建使用统计采集器（TODO §13）。
// STATS_ENABLED=false 或数据库不可用时不采集（返回 nil，各处按「统计关闭」处理）。
func newStatsCollector(cfg *config.Config, database *gorm.DB) *metrics.Collector {
	if cfg == nil || !cfg.StatsEnabled || database == nil {
		return nil
	}
	return metrics.New(metrics.NewGormStore(database), metrics.Options{
		FlushInterval: time.Duration(cfg.StatsFlushSeconds) * time.Second,
		RetentionDays: cfg.StatsRetentionDays,
		Logger:        slog.Default(),
	})
}

// registerRoutes 注册 API 路由。
// 阶段 A/B：健康检查 + 管理员认证；后续阶段按文档 §8 追加。
func registerRoutes(r chi.Router, gormDB *gorm.DB, collector *metrics.Collector) {
	r.Post("/health", func(w http.ResponseWriter, req *http.Request) {
		// 健康检查不暴露内部配置；返回常量信息。
		response.OK(w, map[string]string{"status": "ok", "service": "mckit-api"})
	})

	// MC 元数据客户端：版本信息与源码库共用（源码库需按版本取 jar 下载地址）。
	var mcMetaClient *mcversion.Client
	// MC 源码库（mcsrc）缓存：服务端下载并缓存 jar、构建类索引。
	var mcsrcStore *mcsrc.Store
	if cfgInstance != nil {
		mcMetaClient = mcversion.New(cfgInstance.MCMetaCacheDir, cfgInstance.MCMetaCacheTTL)
		if cfgInstance.MCSRCEnabled {
			mcsrcStore = mcsrc.NewStore(
				cfgInstance.MCSRCCacheDir,
				int64(cfgInstance.MCSRCJarMaxMB)<<20,
				cfgInstance.MCSRCIndexMaxClasses,
			)
		}
	}

	// MC 元数据：版本信息与物品 ID 对照表（移植自 CreeperCode，无需数据库）。
	if cfgInstance != nil {
		mcMetaHandler := handler.NewMcMetaHandler(mcMetaClient, loadBlockIDs(cfgInstance.BlockIDFile, cfgInstance.MCMetaCacheDir))
		r.Post("/versions", mcMetaHandler.ListVersions)
		r.Post("/versions/detail", mcMetaHandler.VersionDetail)
		r.Post("/block-ids", mcMetaHandler.BlockIDs)

		// 区块读取器：解析上传的 .mca 区域文件（仅内存、不落盘）。
		regionHandler := handler.NewRegionHandler(int64(cfgInstance.RegionCacheMaxMB)<<20, cfgInstance.RegionCacheTTL)
		levelHandler := handler.NewLevelHandler()

		// 上传解析类接口：限制速率与同时执行数，避免解压放大/CPU 被长期占用。
		regionHeavy := middleware.RateLimit(cfgInstance.MCHeavyRPS, cfgInstance.MCHeavyBurst)
		regionConc := middleware.ConcurrencyLimit(cfgInstance.MCHeavyConcurrency)

		r.With(regionHeavy, regionConc).Post("/mc/region/inspect", regionHandler.Inspect)
		// chunk 详情常被连续点击（复用 token 时很轻），只限并发不限速率。
		r.With(regionConc).Post("/mc/region/chunk", regionHandler.ChunkDetail)
		r.With(regionHeavy, regionConc).Post("/mc/level/parse", levelHandler.Parse)
	}

	if gormDB == nil {
		return
	}

	// 认证模块
	adminRepo := service.NewAdminRepo(gormDB)
	sessionStore := auth.NewMemorySessionStore()
	authSvc := auth.NewService(adminRepo, sessionStore, 5, 900)

	// 审计日志服务
	auditSvc := audit.NewService(gormDB)

	// 角色与权限点（后台「角色管理」底座）：
	// 角色权限存 roles 表，路由用 RequirePermission 校验，替换原硬编码 RequireRole(admin)；
	// 登录/会话接口一并返回权限点，供前端按权限显示入口（后端仍兜底）。
	roleSvc := role.NewService(gormDB)

	authHandler := handler.NewAuthHandler(authSvc, sessionStore, cfgInstance).
		WithAudit(auditSvc).
		WithPermissions(roleSvc)

	r.Route("/admin/auth", func(ar chi.Router) {
		ar.Post("/login", authHandler.Login)
		ar.Group(func(protected chi.Router) {
			protected.Use(middleware.RequireAuth(authSvc))
			protected.Post("/logout", authHandler.Logout)
			protected.Post("/me", authHandler.Me)
			protected.Group(func(csrf chi.Router) {
				csrf.Use(middleware.CSRF)
				csrf.Post("/change-password", authHandler.ChangePassword)
			})
		})
	})

	// 普通用户账号与个人工作台（需求 §12）：
	// 统一账号（迁移 0022）后与后台共用 accounts 与会话 Cookie，
	// 但接口仍只接受 role = 'user' 的会话（RequireUserRole），后台账号访问返回 401；
	// 读接口仅需登录，写接口叠加 CSRF；登录按 (用户名, IP) 限流。
	userSvc := user.NewService(user.NewRepo(gormDB), sessionStore, 5, 900)
	userHandler := handler.NewUserHandler(userSvc, cfgInstance.SessionSecure)
	r.Route("/user", func(ur chi.Router) {
		ur.Post("/register", userHandler.Register)
		ur.Post("/login", userHandler.Login)
		ur.Group(func(authed chi.Router) {
			authed.Use(middleware.RequireAuth(authSvc))
			authed.Use(middleware.RequireUserRole())
			authed.Post("/logout", userHandler.Logout)
			authed.Post("/me", userHandler.Me)
			authed.Post("/favorites/list", userHandler.ListFavorites)
			authed.Post("/recents/list", userHandler.ListRecents)
			authed.Post("/seeds/list", userHandler.ListSeeds)
			authed.Group(func(csrf chi.Router) {
				csrf.Use(middleware.CSRF)
				csrf.Post("/change-password", userHandler.ChangePassword)
				csrf.Post("/delete", userHandler.DeleteAccount)
				csrf.Post("/favorites/toggle", userHandler.ToggleFavorite)
				csrf.Post("/favorites/remove", userHandler.RemoveFavorite)
				csrf.Post("/recents/record", userHandler.RecordRecent)
				csrf.Post("/recents/clear", userHandler.ClearRecents)
				csrf.Post("/seeds/save", userHandler.SaveSeed)
				csrf.Post("/seeds/delete", userHandler.DeleteSeed)
			})
		})
	})

	// 审计日志（admin 专属）
	auditHandler := handler.NewAuditHandler(auditSvc)

	// 角色与权限点（后台「角色管理」底座，需求 §6.2）：
	// 角色权限存 roles 表，路由用 RequirePermission 校验，替换原硬编码 RequireRole(admin)。
	roleHandler := handler.NewRoleHandler(roleSvc, auditSvc)

	// MC 源码库（mcsrc）：公开只读状态与类检索；触发下载 / 建索引为管理员专属，
	// 并叠加限流与并发上限（单次可达数百 MB，禁止并发叠加）。
	if mcsrcStore != nil {
		mcsrcHandler := handler.NewMcsrcHandler(mcsrcStore, mcMetaClient)
		mcsrcHeavy := middleware.RateLimit(cfgInstance.MCHeavyRPS, cfgInstance.MCHeavyBurst)
		mcsrcConc := middleware.ConcurrencyLimit(1)
		r.Route("/mcsrc", func(mr chi.Router) {
			mr.Post("/status", mcsrcHandler.Status)
			mr.Post("/prepared", mcsrcHandler.Prepared)
			mr.Post("/classes/list", mcsrcHandler.Classes)
			// 类结构视图与继承关系：纯字节码解析，不反编译、不依赖 JVM，
			// 与检索同属公开只读（不触发下载）。
			mr.Post("/classes/detail", mcsrcHandler.ClassDetail)
			mr.Post("/classes/hierarchy", mcsrcHandler.ClassHierarchy)
			// jar 下载：公开只读，供前端在浏览器内做反编译（Vineflower WASM）。
			// 唯一的 GET 接口，需要 Range / ETag 语义（见 handler 注释）。
			mr.Get("/jar", mcsrcHandler.Jar)
			mr.Group(func(wr chi.Router) {
				wr.Use(middleware.RequireAuth(authSvc))
				wr.Use(middleware.RequirePermission(roleSvc, domain.PermOpsMCsrc))
				wr.Use(middleware.CSRF)
				wr.Use(mcsrcHeavy)
				wr.Use(mcsrcConc)
				wr.Post("/prepare", mcsrcHandler.Prepare)
			})
		})
	}
	r.Route("/admin/roles", func(ar chi.Router) {
		ar.Use(middleware.RequireAuth(authSvc))
		ar.Use(middleware.RequirePermission(roleSvc, domain.PermRolesManage))
		ar.Use(middleware.CSRF)
		ar.Post("/list", roleHandler.List)
		ar.Post("/permissions", roleHandler.Permissions)
		ar.Post("/create", roleHandler.Create)
		ar.Post("/update", roleHandler.Update)
		ar.Post("/delete", roleHandler.Delete)
	})

	// 管理员账号管理（admin 专属，需求 §6.1 / §6.2）
	adminAccountSvc := adminaccount.NewService(gormDB)
	adminAccountHandler := handler.NewAdminAccountHandler(adminAccountSvc, authSvc, auditSvc)
	r.Route("/admin/admins", func(ar chi.Router) {
		ar.Use(middleware.RequireAuth(authSvc))
		ar.Use(middleware.RequirePermission(roleSvc, domain.PermAccountsAdmins))
		ar.Use(middleware.CSRF)
		ar.Post("/list", adminAccountHandler.List)
		ar.Post("/create", adminAccountHandler.Create)
		ar.Post("/update", adminAccountHandler.Update)
		ar.Post("/reset-password", adminAccountHandler.ResetPassword)
		ar.Post("/delete", adminAccountHandler.Delete)
	})

	// 普通用户管理（admin 专属）：账号列表 + 个人数据计数（后台「普通用户管理」）。
	userAdminSvc := useradmin.NewService(gormDB)
	userAdminHandler := handler.NewUserAdminHandler(userAdminSvc)
	r.Route("/admin/users", func(ar chi.Router) {
		ar.Use(middleware.RequireAuth(authSvc))
		ar.Use(middleware.RequirePermission(roleSvc, domain.PermAccountsUsers))
		ar.Use(middleware.CSRF)
		ar.Post("/list", userAdminHandler.List)
	})

	// 数据质量与导入任务（admin 专属，TODO §2 / §4）
	dataQualitySvc := dataquality.NewService(gormDB)
	dataQualityHandler := handler.NewDataQualityHandler(dataQualitySvc)
	r.Route("/admin/data-quality", func(ar chi.Router) {
		ar.Use(middleware.RequireAuth(authSvc))
		ar.Use(middleware.RequirePermission(roleSvc, domain.PermOpsDataQuality))
		ar.Use(middleware.CSRF)
		ar.Post("/overview", dataQualityHandler.Overview)
	})
	r.Route("/admin/data-import-jobs", func(ar chi.Router) {
		ar.Use(middleware.RequireAuth(authSvc))
		ar.Use(middleware.RequirePermission(roleSvc, domain.PermOpsDataQuality))
		ar.Use(middleware.CSRF)
		ar.Post("/list", dataQualityHandler.ListJobs)
	})

	// 搜索词统计（admin 专属，TODO §5；仅匿名化聚合数据）
	searchStatsSvc := searchstats.NewService(gormDB)
	// TODO §13：搜索日聚合（词级明细仍由 search_terms 保存）。
	if collector != nil {
		searchStatsSvc.WithDailyRecorder(collector)
	}
	searchTermsHandler := handler.NewSearchTermsHandler(searchStatsSvc)
	r.Route("/admin/search-terms", func(ar chi.Router) {
		ar.Use(middleware.RequireAuth(authSvc))
		ar.Use(middleware.RequirePermission(roleSvc, domain.PermOpsSearchStats))
		ar.Use(middleware.CSRF)
		ar.Post("/list", searchTermsHandler.List)
	})

	// 使用统计与运营看板（TODO §13）。
	analyticsSvc := analytics.NewService(gormDB)
	// 公开上报：匿名、best-effort；对象须通过白名单校验（内置工具 / 工具表 / 资源表）。
	var statsRecorder handler.StatsRecorder
	if collector != nil {
		statsRecorder = collector
	}
	r.Post("/stats/event", handler.NewStatsHandler(statsRecorder, analyticsSvc).Event)

	analyticsHandler := handler.NewAnalyticsHandler(analyticsSvc)
	r.Route("/admin/analytics", func(ar chi.Router) {
		ar.Use(middleware.RequireAuth(authSvc))
		ar.Use(middleware.RequirePermission(roleSvc, domain.PermOpsAnalytics))
		ar.Use(middleware.CSRF)
		ar.Post("/overview", analyticsHandler.Overview)
		ar.Post("/tools", analyticsHandler.Tools)
		ar.Post("/resources", analyticsHandler.Resources)
		ar.Post("/api", analyticsHandler.API)
		ar.Post("/trend", analyticsHandler.Trend)
	})

	// 内容服务
	resSvc := resource.NewService(gormDB)
	contentSvc := content.NewService(gormDB)

	// 管理端 CRUD
	adminHandler := handler.NewAdminHandler(resSvc, contentSvc, auditSvc)

	// 链接检查器（SSRF 防护）
	linkChecker := lk.NewChecker(cfgInstance.LinkCheckTimeout, cfgInstance.LinkCheckMaxRedirect)
	adminHandler.SetLinkChecker(linkCheckAdapter{checker: linkChecker})

	// 链接健康与人工确认（TODO §6：后台复查 + 并发限制）
	linkHealthSvc := linkhealth.NewService(gormDB, linkChecker, cfgInstance.LinkCheckConcurrency)
	linkHealthHandler := handler.NewLinkHealthHandler(linkHealthSvc, auditSvc)

	// 认证保护的管理接口
	r.Route("/admin", func(ar chi.Router) {
		ar.Use(middleware.RequireAuth(authSvc))
		ar.Use(middleware.CSRF)
		// 链接健康与人工确认（TODO §6）
		ar.Route("/link-health", func(lr chi.Router) {
			lr.Use(middleware.RequirePermission(roleSvc, domain.PermOpsLinkHealth))
			lr.Post("/run", linkHealthHandler.Run)
			lr.Post("/list", linkHealthHandler.List)
			lr.Post("/confirm", linkHealthHandler.Confirm)
			lr.Post("/{id}/confirm", linkHealthHandler.Confirm)
		})
		ar.Route("/resources", func(rr chi.Router) {
			rr.Use(middleware.RequirePermission(roleSvc, domain.PermContentResources))
			// 前端契约：动作子路径 + 全 POST
			rr.Post("/list", adminHandler.ListResources)
			rr.Post("/create", adminHandler.CreateResource)
			rr.Post("/detail", handler.WithBodyID(adminHandler.ResourceDetail))
			rr.Post("/update", handler.WithBodyID(adminHandler.UpdateResource))
			rr.Post("/delete", handler.WithBodyID(adminHandler.RemoveResource))
			rr.Post("/check-link", handler.WithBodyID(adminHandler.CheckResourceLink))
			// 发布版本（TODO §6）：同一资源不同发布版本的关联
			rr.Post("/releases/list", handler.WithBodyID(adminHandler.ListResourceReleases))
			rr.Post("/releases/replace", handler.WithBodyID(adminHandler.ReplaceResourceReleases))
			// 兼容 REST 风格
			rr.Post("/", adminHandler.ListResources)
			rr.Post("/new", adminHandler.CreateResource)
			rr.Post("/{id}", adminHandler.ResourceDetail)
			rr.Patch("/{id}", adminHandler.UpdateResource)
			rr.Delete("/{id}", adminHandler.RemoveResource)
			rr.Post("/{id}/check-link", adminHandler.CheckResourceLink)
		})
		ar.Route("/tools", func(rr chi.Router) {
			rr.Use(middleware.RequirePermission(roleSvc, domain.PermContentTools))
			rr.Post("/list", adminHandler.ListTools)
			rr.Post("/create", adminHandler.CreateTool)
			rr.Post("/detail", handler.WithBodyID(adminHandler.ToolDetail))
			rr.Post("/update", handler.WithBodyID(adminHandler.UpdateTool))
			rr.Post("/delete", handler.WithBodyID(adminHandler.RemoveTool))
			rr.Post("/", adminHandler.ListTools)
			rr.Post("/new", adminHandler.CreateTool)
			rr.Post("/{id}", adminHandler.ToolDetail)
			rr.Patch("/{id}", adminHandler.UpdateTool)
			rr.Delete("/{id}", adminHandler.RemoveTool)
		})
		ar.Route("/wiki", func(rr chi.Router) {
			rr.Use(middleware.RequirePermission(roleSvc, domain.PermContentWiki))
			rr.Post("/list", adminHandler.ListWiki)
			rr.Post("/create", adminHandler.CreateWiki)
			rr.Post("/detail", handler.WithBodyID(adminHandler.WikiDetail))
			rr.Post("/update", handler.WithBodyID(adminHandler.UpdateWiki))
			rr.Post("/delete", handler.WithBodyID(adminHandler.RemoveWiki))
			rr.Post("/", adminHandler.ListWiki)
			rr.Post("/new", adminHandler.CreateWiki)
			rr.Post("/{id}", adminHandler.WikiDetail)
			rr.Patch("/{id}", adminHandler.UpdateWiki)
			rr.Delete("/{id}", adminHandler.RemoveWiki)
		})
		ar.Route("/categories", func(rr chi.Router) {
			rr.Use(middleware.RequirePermission(roleSvc, domain.PermContentCategories))
			rr.Post("/list", adminHandler.ListCategories)
			rr.Post("/create", adminHandler.CreateCategory)
			rr.Post("/update", adminHandler.UpdateCategory)
			rr.Post("/delete", adminHandler.DeleteCategory)
			rr.Post("/", adminHandler.ListCategories)
			rr.Post("/new", adminHandler.CreateCategory)
		})
		ar.Route("/tags", func(rr chi.Router) {
			rr.Use(middleware.RequirePermission(roleSvc, domain.PermContentTags))
			rr.Post("/list", adminHandler.ListTags)
			rr.Post("/create", adminHandler.CreateTag)
			rr.Post("/update", adminHandler.UpdateTag)
			rr.Post("/delete", adminHandler.DeleteTag)
			rr.Post("/", adminHandler.ListTags)
			rr.Post("/new", adminHandler.CreateTag)
		})
		ar.Route("/feedback", func(rr chi.Router) {
			rr.Use(middleware.RequirePermission(roleSvc, domain.PermContentFeedback))
			rr.Post("/list", adminHandler.ListFeedback)
			rr.Post("/update", handler.WithBodyID(adminHandler.UpdateFeedback))
			rr.Post("/reviews", handler.WithBodyID(adminHandler.ListFeedbackReviews))
			rr.Post("/", adminHandler.ListFeedback)
			rr.Patch("/{id}", adminHandler.UpdateFeedback)
			rr.Post("/{id}/reviews", adminHandler.ListFeedbackReviews)
		})
	})

	r.Route("/admin/audit-logs", func(ar chi.Router) {
		ar.Group(func(protected chi.Router) {
			protected.Use(middleware.RequireAuth(authSvc))
			protected.Use(middleware.RequirePermission(roleSvc, domain.PermOpsAudit))
			protected.Use(middleware.CSRF)
			protected.Post("/", auditHandler.List)
			protected.Post("/list", auditHandler.List)
		})
	})

	// 公开 API
	catalogSvc := catalog.NewService(gormDB)
	searchSvc := search.NewService(gormDB)

	// 红石机器设计审核（TODO §15，admin 专属）：按审核状态查询 + 审核留痕。
	redstoneAdminHandler := handler.NewRedstoneAdminHandler(catalogSvc, auditSvc)
	r.Route("/admin/redstone", func(ar chi.Router) {
		ar.Use(middleware.RequireAuth(authSvc))
		ar.Use(middleware.RequirePermission(roleSvc, domain.PermReviewRedstone))
		ar.Use(middleware.CSRF)
		ar.Post("/designs/list", redstoneAdminHandler.ListDesigns)
		ar.Post("/designs/review", redstoneAdminHandler.ReviewDesign)
	})

	// Mod 与整合包审核（TODO §16，admin 专属）：按审核状态查询 + 审核留痕。
	modAdminHandler := handler.NewModAdminHandler(catalogSvc, auditSvc)
	r.Route("/admin/mods", func(ar chi.Router) {
		ar.Use(middleware.RequireAuth(authSvc))
		ar.Use(middleware.RequirePermission(roleSvc, domain.PermReviewMods))
		ar.Use(middleware.CSRF)
		ar.Post("/list", modAdminHandler.AdminMods)
		ar.Post("/review", modAdminHandler.ReviewMod)
	})
	r.Route("/admin/modpacks", func(ar chi.Router) {
		ar.Use(middleware.RequireAuth(authSvc))
		ar.Use(middleware.RequirePermission(roleSvc, domain.PermReviewMods))
		ar.Use(middleware.CSRF)
		ar.Post("/list", modAdminHandler.AdminModpacks)
		ar.Post("/review", modAdminHandler.ReviewModpack)
	})

	publicHandler := handler.NewPublicHandler(resSvc, contentSvc, catalogSvc, searchSvc).WithSearchStats(searchStatsSvc)
	// TODO §13：资源详情访问量统计。
	if collector != nil {
		publicHandler = publicHandler.WithResourceViewRecorder(collector)
	}
	r.Post("/search", publicHandler.Search)
	r.Route("/resources", func(pr chi.Router) {
		pr.Post("/", publicHandler.ListResources)
		pr.Post("/{slug}", publicHandler.ResourceDetail)
	})
	r.Route("/tools", func(pr chi.Router) {
		pr.Post("/", publicHandler.ListTools)
		pr.Post("/{slug}", publicHandler.ToolDetail)
	})
	r.Route("/items", func(pr chi.Router) {
		pr.Post("/", publicHandler.Items)
		pr.Post("/{item_id}", publicHandler.ItemDetail)
	})
	r.Post("/recipes", publicHandler.Recipes)
	r.Route("/potions", func(pr chi.Router) {
		pr.Post("/", publicHandler.Potions)
		// 静态段优先于参数段（chi 路由树保证），故 rules 不会被 {slug} 匹配。
		pr.Post("/rules", publicHandler.BrewingRules)
		pr.Post("/{slug}", publicHandler.PotionDetail)
	})
	// 村民交易资料（TODO §9B）：职业 / 交易等级 / 交易表。
	r.Route("/villagers", func(pr chi.Router) {
		pr.Post("/", publicHandler.Villagers)
		// 静态段优先于参数段，故 levels 不会被 {slug} 匹配。
		pr.Post("/levels", publicHandler.VillagerTradeLevels)
		pr.Post("/{slug}", publicHandler.VillagerDetail)
	})
	// 生物资料（TODO §9C）：生物 / 掉落 / 生成与驯服繁殖说明。
	r.Route("/mobs", func(pr chi.Router) {
		pr.Post("/", publicHandler.Mobs)
		pr.Post("/{slug}", publicHandler.MobDetail)
	})
	// 红石与机器设计资料库（TODO §15）：元件 / 设计（仅已发布）。
	r.Route("/redstone", func(pr chi.Router) {
		pr.Route("/components", func(cr chi.Router) {
			cr.Post("/", publicHandler.RedstoneComponents)
			cr.Post("/{slug}", publicHandler.RedstoneComponentDetail)
		})
		pr.Route("/designs", func(dr chi.Router) {
			dr.Post("/", publicHandler.RedstoneDesigns)
			dr.Post("/{slug}", publicHandler.RedstoneDesignDetail)
		})
	})
	// Mod 与整合包兼容性查询（TODO §16）：Mod / 整合包（仅已发布）。
	r.Route("/mods", func(pr chi.Router) {
		pr.Post("/", publicHandler.Mods)
		pr.Post("/{slug}", publicHandler.ModDetail)
	})
	r.Route("/modpacks", func(pr chi.Router) {
		pr.Post("/", publicHandler.Modpacks)
		pr.Post("/{slug}", publicHandler.ModpackDetail)
	})
	r.Route("/commands", func(pr chi.Router) {
		pr.Post("/", publicHandler.Commands)
		pr.Post("/{slug}", publicHandler.CommandDetail)
	})
	// 我的世界开发栏目（TODO §21）：开发文档知识库 + 版本映射查询。
	r.Route("/dev-docs", func(pr chi.Router) {
		pr.Post("/", publicHandler.DevDocs)
		// 静态段优先于参数段（chi 路由树保证），故 facets 不会被 {slug} 匹配。
		pr.Post("/facets", publicHandler.DevDocFacets)
		pr.Post("/{slug}", publicHandler.DevDocDetail)
	})
	r.Route("/mappings", func(pr chi.Router) {
		pr.Post("/sets", publicHandler.MappingSets)
		pr.Post("/search", publicHandler.MappingSearch)
		pr.Post("/class/{id}", publicHandler.MappingClassDetail)
	})
	r.Post("/wiki/{slug}", publicHandler.WikiDetail)
	r.Post("/categories", publicHandler.Categories)
	r.Post("/feedback", adminHandler.SubmitFeedback)

	// Minecraft 种子/结构工具（公开）
	mcHandler := handler.NewMCHandler()
	r.Post("/seed/analyze", mcHandler.SeedAnalyze)
	r.Route("/mc", func(mr chi.Router) {
		// structures 与 slime-chunks 单次遍历数千点：既限制速率，也限制同时执行的请求数。
		heavy := middleware.RateLimit(cfgInstance.MCHeavyRPS, cfgInstance.MCHeavyBurst)
		conc := middleware.ConcurrencyLimit(cfgInstance.MCHeavyConcurrency)
		mr.With(heavy, conc).Post("/structures", mcHandler.Structures)
		mr.Post("/structures/types", mcHandler.ListStructures)
		mr.Post("/strongholds", mcHandler.Strongholds)
		mr.With(heavy, conc).Post("/slime-chunks", mcHandler.SlimeChunks)
		mr.Post("/capabilities", mcHandler.Capabilities)
		mr.Post("/biome", mcHandler.Biome)
		mr.Post("/seeds/generate", mcHandler.GenerateSeeds)
		// find 单次限时 5s，单独使用更小的限流配额，避免长期占用 worker。
		mr.With(middleware.RateLimit(cfgInstance.MCFindRPS, cfgInstance.MCFindBurst)).
			Post("/seeds/find", mcHandler.FindSeeds)
		mr.Post("/seeds/parse", mcHandler.ParseSeed)
	})
}

// cfgInstance 由 main 在启动时注入（避免全局配置散落）。
var cfgInstance *config.Config

// loadBlockIDs 加载物品对照表：以 md 为权威源解析并刷新缓存 item.json；
// md 缺失时降级读取上次解析的 item.json；两者皆无返回 nil（接口将返回 503）。
func loadBlockIDs(mdPath, cacheDir string) []blockids.Entry {
	jsonPath := filepath.Join(cacheDir, "item.json")
	if _, err := os.Stat(mdPath); err == nil {
		entries, err := blockids.Load(mdPath)
		if err != nil {
			slog.Warn("物品对照表解析失败，/block-ids 将不可用", "err", err)
			return nil
		}
		if err := blockids.SaveJSON(entries, jsonPath, mdPath); err != nil {
			slog.Warn("写入 item.json 失败", "err", err)
		}
		return entries
	}
	if entries, err := blockids.LoadJSON(jsonPath); err == nil {
		slog.Info("物品对照表源文件缺失，已从缓存恢复", "path", jsonPath, "count", len(entries))
		return entries
	}
	slog.Warn("找不到物品对照表文件，/block-ids 将不可用", "md", mdPath, "cache", jsonPath)
	return nil
}

func levelFromString(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
