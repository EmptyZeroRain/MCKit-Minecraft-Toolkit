// main 程序：启动 MC 工具站 API 服务。
// 依据需求文档 §8：所有查询/搜索/列表接口统一使用 POST，参数放在 JSON body。
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"mckit/backend/internal/audit"
	"mckit/backend/internal/auth"
	"mckit/backend/internal/blockids"
	"mckit/backend/internal/captcha"
	"mckit/backend/internal/config"
	"mckit/backend/internal/domain"
	"mckit/backend/internal/envutil"
	"mckit/backend/internal/http/handler"
	"mckit/backend/internal/http/middleware"
	"mckit/backend/internal/http/response"
	lk "mckit/backend/internal/linkcheck"
	"mckit/backend/internal/mcsrc"
	"mckit/backend/internal/mcversion"
	mfacrypto "mckit/backend/internal/mfa"
	db "mckit/backend/internal/repository"
	"mckit/backend/internal/repository/types"
	"mckit/backend/internal/search"
	"mckit/backend/internal/service"
	"mckit/backend/internal/service/accountadmin"
	"mckit/backend/internal/service/adminaccount"
	"mckit/backend/internal/service/alert"
	"mckit/backend/internal/service/analytics"
	"mckit/backend/internal/service/catalog"
	communitysvc "mckit/backend/internal/service/community"
	"mckit/backend/internal/service/content"
	"mckit/backend/internal/service/contentfilter"
	"mckit/backend/internal/service/contenttask"
	"mckit/backend/internal/service/dataquality"
	"mckit/backend/internal/service/gift"
	"mckit/backend/internal/service/growth"
	"mckit/backend/internal/service/jobs"
	"mckit/backend/internal/service/linkhealth"
	"mckit/backend/internal/service/metrics"
	mfasvc "mckit/backend/internal/service/mfa"
	"mckit/backend/internal/service/resource"
	"mckit/backend/internal/service/revision"
	"mckit/backend/internal/service/role"
	"mckit/backend/internal/service/searchstats"
	"mckit/backend/internal/service/sitemap"
	"mckit/backend/internal/service/submission"
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

	// 启动时自动应用未执行的迁移：避免新迁移上线后忘记手动 migrate，
	// 导致后台调度、社区、MFA 等模块持续报「relation does not exist」。
	if cfg.AutoMigrate {
		migrationsDir, err := resolveMigrationsDir(cfg.MigrationsDir)
		if err != nil {
			slog.Error("定位数据库迁移目录失败", "err", err)
			os.Exit(1)
		}
		applied, err := db.Migrate(ctx, database, migrationsDir)
		if err != nil {
			slog.Error("数据库自动迁移失败", "err", err)
			os.Exit(1)
		}
		if len(applied) > 0 {
			slog.Info("数据库自动迁移完成", "count", len(applied), "versions", applied)
		} else {
			slog.Info("数据库结构已是最新")
		}
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
	jobSvc, alertSvc := registerRoutes(ctx, api, database, collector)
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

	// 数据库不可用时 registerRoutes 返回 nil（此时也不会注册任务接口）。
	if jobSvc != nil {
		jobSvc.Start(ctx, 5*time.Second)
		// 已完成任务的保留期限（默认 90 天）：admin_jobs 是纯运维记录，超期只增负担。
		jobSvc.StartRetention(ctx, cfgInstance.JobRetentionDays, 24*time.Hour)
		slog.Info("后台任务调度已启动", "interval", "5s")
	}
	if alertSvc != nil {
		alertSvc.Start(ctx, 5*time.Minute)
		slog.Info("运营告警评估已启动", "interval", "5m")
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
	// 先停调度：避免任务在数据库/客户端关闭后继续执行。
	if jobSvc != nil {
		jobSvc.Stop()
	}
	if alertSvc != nil {
		alertSvc.Stop()
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

// resolveMigrationsDir 定位迁移目录，兼容以下运行方式：
//   - backend 目录运行 go run ./cmd/api（默认 migrations）；
//   - 仓库根目录运行二进制（自动发现 backend/migrations）；
//   - Docker 镜像内 /app 运行（migrations 由镜像复制）。
func resolveMigrationsDir(configured string) (string, error) {
	if configured == "" {
		configured = "migrations"
	}
	if filepath.IsAbs(configured) {
		if isDir(configured) {
			return configured, nil
		}
		return "", fmt.Errorf("迁移目录不存在: %s", configured)
	}

	var candidates []string
	if wd, err := os.Getwd(); err == nil {
		for dir := wd; ; dir = filepath.Dir(dir) {
			candidates = append(candidates,
				filepath.Join(dir, configured),
				filepath.Join(dir, "backend", configured),
			)
			if filepath.Dir(dir) == dir {
				break
			}
		}
	}
	candidates = append(candidates, configured)
	for _, candidate := range candidates {
		if isDir(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("未找到迁移目录 %q，可通过 MIGRATIONS_DIR 指定", configured)
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// registerRoutes 注册 API 路由。
// 阶段 A/B：健康检查 + 管理员认证；后续阶段按文档 §8 追加。
// ctx 用于启动随进程生命周期结束的后台任务（如审计日志保留清理）。
func registerRoutes(ctx context.Context, r chi.Router, gormDB *gorm.DB, collector *metrics.Collector) (*jobs.Service, *alert.Service) {
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
		return nil, nil
	}

	// 认证模块
	adminRepo := service.NewAdminRepo(gormDB)
	// 会话存储（gj.md §3.4）：
	//   memory   —— 单实例部署；进程重启后全部会话失效（默认）；
	//   postgres —— 多实例部署；会话在所有实例间共享，创建/续期/撤销即时一致。
	sessionOpts := auth.SessionOptions{TTL: cfgInstance.SessionTTL, IdleTTL: cfgInstance.SessionIdleTTL}
	var sessionStore auth.SessionStore
	if cfgInstance.SessionStore == "postgres" {
		sessionStore = auth.NewPostgresSessionStore(gormDB, sessionOpts)
	} else {
		sessionStore = auth.NewMemorySessionStoreWithOptions(sessionOpts)
	}
	// 过期会话后台清理（两种存储都需要，避免过期数据长期驻留）。
	auth.StartSessionPurge(ctx, sessionStore, time.Hour, slog.Default())
	// 统一密码策略参数（gj.md §3.3）：后台与普通用户共用同一套校验实现。
	policy := auth.PasswordPolicy{MinLength: cfgInstance.PasswordMinLengthAdmin, MaxLength: cfgInstance.PasswordMaxLength}
	userPolicy := auth.PasswordPolicy{MinLength: cfgInstance.PasswordMinLengthUser, MaxLength: cfgInstance.PasswordMaxLength}

	// 双因子认证（gj.md §10.1）：
	// 密钥缺失时仍创建服务（key=nil），这样已绑定 MFA 的账号会被明确拒绝登录，
	// 而不是静默降级为单因子；未绑定的账号不受影响。
	var mfaKey []byte
	if cfgInstance.MFAEncryptionKey != "" {
		parsed, err := mfacrypto.ParseKey(cfgInstance.MFAEncryptionKey)
		if err != nil {
			slog.Error("MFA_ENCRYPTION_KEY 解析失败", "error", err)
			return nil, nil
		}
		mfaKey = parsed
	}
	mfaSvc := mfasvc.NewService(gormDB, mfaKey, cfgInstance.MFAIssuer)

	authSvc := auth.NewService(adminRepo, sessionStore, 5, 900).
		WithPasswordPolicy(policy).
		WithMFA(mfaSvc)

	// 审计日志服务
	auditSvc := audit.NewService(gormDB)
	// 审计日志保留清理（AUDIT_RETENTION_DAYS，默认 180 天；<=0 关闭）：
	// 审计表只增不减，随进程生命周期在后台按天清理。
	auditSvc.StartRetention(ctx, cfgInstance.AuditRetentionDays, 24*time.Hour, slog.Default())

	// 角色与权限点（后台「角色管理」底座）：
	// 角色权限存 roles 表，路由用 RequirePermission 校验，替换原硬编码 RequireRole(admin)；
	// 登录/会话接口一并返回权限点，供前端按权限显示入口（后端仍兜底）。
	roleSvc := role.NewService(gormDB)

	// 用户成长系统（jingyan.md）：经验 / 等级 / 每日任务 / 成就 / 成长审计。
	// 服务端判定 + 业务事件驱动；业务模块通过 GrowthEventSink 投递已确认事实，
	// 成长侧在事务内幂等处理（重复事件不重复发奖）。
	growthSvc := growth.NewService(gormDB, cfgInstance.GrowthTimezone)
	growthHandler := handler.NewGrowthHandler(growthSvc)

	// 礼物币钱包：用户查看本人余额 / 流水，后台通过独立 ops.gifts 权限受控增减。
	giftSvc := gift.NewService(gormDB).WithTimezone(cfgInstance.GrowthTimezone)
	giftHandler := handler.NewGiftHandler(giftSvc).WithAudit(auditSvc)

	// 社区（帖子 / 留言 / 点赞）：公开读匿名可用，写操作要求普通用户会话；
	// 点赞与留言在业务确认后投递 post.* 成长事件（幂等由成长侧保证）。
	// 内容关键字屏蔽在服务端同步校验，后台维护的规则启停立即生效。
	contentFilterSvc := contentfilter.NewService(gormDB)
	contentFilterHandler := handler.NewContentFilterHandler(contentFilterSvc).WithAudit(auditSvc)
	communitySvc := communitysvc.NewService(gormDB).WithContentFilter(contentFilterSvc)
	communityHandler := handler.NewCommunityHandler(communitySvc).
		WithGrowthSink(growthSvc).
		WithAudit(auditSvc)

	authHandler := handler.NewAuthHandler(authSvc, sessionStore, cfgInstance).
		WithAudit(auditSvc).
		WithPermissions(roleSvc).
		WithMFA(mfaSvc)
	sessionAdminHandler := handler.NewSessionAdminHandler(authSvc).WithAudit(auditSvc)

	r.Route("/admin/auth", func(ar chi.Router) {
		ar.Post("/login", authHandler.Login)
		// 第二因子校验：凭证是「登录票据 + 验证码」，不需要已有会话。
		ar.Post("/mfa/verify", authHandler.MFAVerify)
		ar.Group(func(protected chi.Router) {
			protected.Use(middleware.RequireAuth(authSvc))
			protected.Post("/logout", authHandler.Logout)
			protected.Post("/me", authHandler.Me)
			// 会话管理：列出当前账号全部有效会话（只读）。
			protected.Post("/sessions/list", authHandler.Sessions)
			// MFA 状态（只读，不含密钥材料）。
			protected.Post("/mfa/status", authHandler.MFAStatus)
			protected.Group(func(csrf chi.Router) {
				csrf.Use(middleware.CSRF)
				csrf.Post("/change-password", authHandler.ChangePassword)
				// 自助维护展示资料（昵称 / 头像）。
				csrf.Post("/profile", authHandler.UpdateProfile)
				// 撤销当前账号会话（其它设备 / 全部设备）。
				csrf.Post("/sessions/revoke", authHandler.RevokeSessions)
				// MFA 自助绑定 / 解绑。
				csrf.Post("/mfa/setup", authHandler.MFASetup)
				csrf.Post("/mfa/enable", authHandler.MFAEnable)
				csrf.Post("/mfa/disable", authHandler.MFADisable)
			})
		})
	})

	// 全站会话管理（系统管理）：查看普通用户与管理员的有效会话，按指纹踢下线。
	// 高敏感能力：独立 ops.sessions 权限 + CSRF + 审计；列表不回显 SessionID 原文。
	r.Route("/admin/sessions", func(sr chi.Router) {
		sr.Use(middleware.RequireAuth(authSvc))
		sr.Use(middleware.RequirePermission(roleSvc, domain.PermOpsSessions))
		sr.Use(middleware.CSRF)
		sr.Post("/list", sessionAdminHandler.List)
		sr.Post("/revoke", sessionAdminHandler.Revoke)
	})

	// 账号个人工作台（需求 §12）：
	// 统一账号（迁移 0022）后管理员与普通用户共用 accounts 与会话 Cookie，
	// 且后台账号同样拥有前台能力（收藏 / 最近使用 / 种子 / 投稿 / 成长），
	// 因此这里只要求登录（RequireAccount），不再按角色拦截；
	// 读接口仅需登录，写接口叠加 CSRF；登录按 (用户名, IP) 限流。
	userSvc := user.NewService(user.NewRepo(gormDB), sessionStore, 5, 900).
		WithPasswordPolicy(userPolicy).
		WithAdminPasswordPolicy(policy)

	// 注册图形验证码（迁移 0024 起注册需填邮箱与验证码）。
	// 仅内存存储，与 MemorySessionStore 一致，适用于单实例部署；
	// CAPTCHA_ENABLED=false 时不创建（注册跳过验证码校验）。
	var captchaMgr *captcha.Manager
	if cfgInstance.CaptchaEnabled {
		captchaMgr = captcha.NewManager(captcha.Options{
			TTL:       cfgInstance.CaptchaTTL,
			FixedCode: cfgInstance.CaptchaFixedCode,
		})
	}
	userHandler := handler.NewUserHandler(userSvc, captchaMgr, cfgInstance.SessionSecure).WithGrowthSink(growthSvc)

	// 资源投稿（用户提交结构化资源 → 人工审核 → 转为正式资源）。
	// 用户端与审核端共用同一服务；审核通过时复用 resource 服务生成资源，
	// 保证字段校验、标签关联与历史版本写入与「资源管理」完全一致。
	resSvc := resource.NewService(gormDB)
	submissionSvc := submission.NewService(gormDB, resSvc)
	submissionHandler := handler.NewSubmissionHandler(submissionSvc).WithGrowthSink(growthSvc)

	r.Route("/user", func(ur chi.Router) {
		ur.Post("/captcha", userHandler.Captcha)
		ur.Post("/register", userHandler.Register)
		ur.Post("/login", userHandler.Login)
		ur.Group(func(authed chi.Router) {
			authed.Use(middleware.RequireAuth(authSvc))
			authed.Use(middleware.RequireAccount())
			authed.Post("/logout", userHandler.Logout)
			authed.Post("/me", userHandler.Me)
			authed.Post("/favorites/list", userHandler.ListFavorites)
			authed.Post("/recents/list", userHandler.ListRecents)
			authed.Post("/seeds/list", userHandler.ListSeeds)
			// 我的投稿（含审核状态）。
			authed.Post("/submissions/list", submissionHandler.ListMine)
			authed.Group(func(csrf chi.Router) {
				csrf.Use(middleware.CSRF)
				// 处于「必须改密」状态时仍允许自助改密与资料维护（否则无法自救）。
				csrf.Post("/change-password", userHandler.ChangePassword)
				// 自助维护展示资料（昵称 / 头像）。
				csrf.Post("/profile", userHandler.UpdateProfile)
				// 其余写操作在完成改密前一律拒绝（gj.md §10.2）。
				csrf.Group(func(gated chi.Router) {
					gated.Use(middleware.RequirePasswordChanged())
					// 资源投稿（进入待审核队列，审核通过后由管理员转为资源）。
					gated.Post("/submissions/submit", submissionHandler.Submit)
					gated.Post("/delete", userHandler.DeleteAccount)
					gated.Post("/favorites/toggle", userHandler.ToggleFavorite)
					gated.Post("/favorites/remove", userHandler.RemoveFavorite)
					gated.Post("/recents/record", userHandler.RecordRecent)
					gated.Post("/recents/clear", userHandler.ClearRecents)
					gated.Post("/seeds/save", userHandler.SaveSeed)
					gated.Post("/seeds/delete", userHandler.DeleteSeed)
				})
			})
		})
	})

	// 社区（帖子 / 留言 / 点赞）：读接口匿名可用，写接口仅普通用户。
	// 公开读叠加 OptionalAuth，仅用于返回当前登录者的 liked 状态，不阻断匿名访问。
	r.Route("/community", func(cr chi.Router) {
		cr.Use(middleware.OptionalAuth(authSvc))
		cr.Post("/posts", communityHandler.ListPosts)
		cr.Post("/posts/detail", communityHandler.PostDetail)
		cr.Post("/comments", communityHandler.ListComments)
		cr.Group(func(authed chi.Router) {
			authed.Use(middleware.RequireAuth(authSvc))
			authed.Use(middleware.RequireAccount())
			authed.Group(func(csrf chi.Router) {
				csrf.Use(middleware.CSRF)
				csrf.Use(middleware.RequirePasswordChanged())
				csrf.Post("/posts/create", communityHandler.CreatePost)
				csrf.Post("/posts/delete", communityHandler.DeletePost)
				csrf.Post("/posts/like", communityHandler.ToggleLike)
				csrf.Post("/comments/create", communityHandler.CreateComment)
				csrf.Post("/comments/delete", communityHandler.DeleteComment)
			})
		})
	})

	// 用户成长中心（jingyan.md §9）：仅本人成长数据，等级仅展示用。
	// 读接口只要求登录（普通用户角色），不做任何前端可影响的判定。
	r.Route("/growth", func(gr chi.Router) {
		gr.Use(middleware.RequireAuth(authSvc))
		gr.Use(middleware.RequireAccount())
		gr.Post("/overview", growthHandler.Overview)
		gr.Post("/ledger", growthHandler.Ledger)
		gr.Post("/daily-tasks", growthHandler.DailyTasks)
		gr.Post("/daily-tasks/history", growthHandler.DailyTaskHistory)
		gr.Post("/achievements", growthHandler.Achievements)
	})

	// 礼物中心（本人）：余额 / 签到 / 礼物目录 / 兑换 / 流水。
	// 读接口不要求 CSRF；签到与兑换会改变余额，必须校验 CSRF。
	r.Route("/gifts", func(gr chi.Router) {
		gr.Use(middleware.RequireAuth(authSvc))
		gr.Use(middleware.RequireAccount())
		gr.Post("/overview", giftHandler.SelfOverview)
		gr.Post("/ledger/list", giftHandler.SelfLedger)
		gr.Post("/catalog", giftHandler.SelfCatalog)
		gr.Post("/redemptions/list", giftHandler.SelfRedemptions)
		gr.Group(func(wr chi.Router) {
			wr.Use(middleware.CSRF)
			wr.Post("/checkin", giftHandler.SelfCheckin)
			wr.Post("/redeem", giftHandler.SelfRedeem)
		})
	})

	// 公开个人主页成长摘要（不含经验流水与登录信息）。
	r.Post("/public/users/detail", growthHandler.PublicProfile)
	r.Post("/public/users/achievements", growthHandler.PublicAchievements)

	// 审计日志（admin 专属）
	auditHandler := handler.NewAuditHandler(auditSvc)

	// 角色与权限点（后台「角色管理」底座，需求 §6.2）：
	// 角色权限存 roles 表，路由用 RequirePermission 校验，替换原硬编码 RequireRole(admin)。
	roleHandler := handler.NewRoleHandler(roleSvc, auditSvc)

	// 目录服务（映射等）：mcsrc 的「映射重命名」需要按版本查映射，故提前构造。
	catalogSvc := catalog.NewService(gormDB)

	// MC 源码库（mcsrc）：公开只读状态与类检索；/prepare 任何访客可用
	// （前端在用户选中版本时自动触发下载 + 建索引），并叠加限流与并发上限
	// （单次可达数百 MB，禁止并发叠加）。
	// 提到块外：缓存重建任务需要把 jobSvc 注入到同一个 handler 实例。
	var mcsrcAdminHandler *handler.McsrcHandler
	if mcsrcStore != nil {
		mcsrcHandler := handler.NewMcsrcHandler(mcsrcStore, mcMetaClient).
			WithAutoPrepare(roleSvc, cfgInstance.MCSRCAutoPrepareMax).
			WithAudit(auditSvc).
			WithMappings(catalogSvc).
			WithCacheAlertMB(cfgInstance.MCSRCCacheAlertMB)
		mcsrcAdminHandler = mcsrcHandler
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
			// 字节码反汇编（ASM Textifier 风格文本）：同样为纯字节码解析。
			mr.Post("/classes/bytecode", mcsrcHandler.ClassBytecode)
			// 查找引用：扫描全 jar 常量池与指令建反向引用索引（首次较慢，之后缓存）。
			mr.Post("/classes/references", mcsrcHandler.ClassReferences)
			// 映射重命名：只读数据库中的映射数据（不读 jar、不建索引），
			// 未导入映射时返回 available=false。
			mr.Post("/classes/mapping", mcsrcHandler.ClassMapping)
			// jar 下发：公开只读，供前端在浏览器内做反编译（Vineflower WASM）。
			// 与项目约定一致走 POST（JSON 入参 + 二进制响应）；单次可达数十 MB，
			// 故叠加重接口限流。并发上限只给 /prepare，避免「取 jar + 建索引」互相阻塞。
			mr.With(mcsrcHeavy).Post("/jar", mcsrcHandler.Jar)
			// 自动准备：匿名可用，但受 MCSRC_AUTO_PREPARE_MAX 额度限制
			// （默认 12 个 version+side）；具备 ops.mcsrc 的管理员不受限。
			// OptionalAuth 只用于识别管理员，绝不返回 401。
			mr.Group(func(wr chi.Router) {
				wr.Use(middleware.OptionalAuth(authSvc))
				wr.Use(mcsrcHeavy)
				wr.Use(mcsrcConc)
				wr.Post("/prepare", mcsrcHandler.Prepare)
			})
		})

		// 缓存运维（后台「MC 源码缓存」）：列出磁盘占用 / 清理单个版本 / 清空全部。
		// jar 与索引均为可重建的派生数据，清理不影响业务数据；但属运维动作，
		// 故要求 ops.mcsrc 权限 + CSRF，并写审计（含释放字节数）。
		r.Route("/admin/mcsrc", func(ar chi.Router) {
			ar.Use(middleware.RequireAuth(authSvc))
			ar.Use(middleware.RequirePermission(roleSvc, domain.PermOpsMCsrc))
			ar.Use(middleware.CSRF)
			ar.Post("/cache/list", mcsrcHandler.CacheList)
			ar.Post("/cache/preview", mcsrcHandler.CachePreview)
			ar.Post("/cache/purge", mcsrcHandler.CachePurge)
			ar.Post("/cache/purge-all", mcsrcHandler.CachePurgeAll)
			// 缓存重建（gj.md 阶段三 / §11）：下载 + 建索引耗时数十秒，走后台任务队列。
			ar.Post("/cache/rebuild", mcsrcHandler.CacheRebuild)
		})
	}
	r.Route("/admin/roles", func(ar chi.Router) {
		ar.Use(middleware.RequireAuth(authSvc))
		ar.Use(middleware.RequirePermission(roleSvc, domain.PermRolesManage))
		ar.Use(middleware.CSRF)
		ar.Post("/list", roleHandler.List)
		ar.Post("/permissions", roleHandler.Permissions)
		ar.Post("/impact", roleHandler.Impact)
		ar.Post("/create", roleHandler.Create)
		ar.Post("/update", roleHandler.Update)
		ar.Post("/delete", roleHandler.Delete)
	})

	// 管理员账号管理（admin 专属，需求 §6.1 / §6.2）
	adminAccountSvc := adminaccount.NewService(gormDB).
		WithPasswordPolicy(policy).
		WithSessionRevoker(sessionStore)
	adminAccountHandler := handler.NewAdminAccountHandler(adminAccountSvc, authSvc, auditSvc)
	r.Route("/admin/admins", func(ar chi.Router) {
		ar.Use(middleware.RequireAuth(authSvc))
		ar.Use(middleware.RequirePermission(roleSvc, domain.PermAccountsAdmins))
		ar.Use(middleware.CSRF)
		ar.Post("/list", adminAccountHandler.List)
		ar.Post("/create", adminAccountHandler.Create)
		ar.Post("/update", adminAccountHandler.Update)
		ar.Post("/reset-password", adminAccountHandler.ResetPassword)
		ar.Post("/delete-preview", adminAccountHandler.DeletePreview)
		// 账号恢复：解绑目标账号的 MFA（换手机且恢复码丢失时使用）。
		ar.Post("/mfa/reset", adminAccountHandler.ResetMFA)
		ar.Post("/delete", adminAccountHandler.Delete)
	})

	// 普通用户管理（需 accounts.users 权限）：列表 / 详情 / 启用停用 / 重置密码 / 删除 / 导出。
	userAdminSvc := useradmin.NewService(gormDB).
		WithPasswordPolicy(userPolicy).
		WithSessionRevoker(sessionStore)
	userAdminHandler := handler.NewUserAdminHandler(userAdminSvc, authSvc, auditSvc)
	r.Route("/admin/users", func(ar chi.Router) {
		ar.Use(middleware.RequireAuth(authSvc))
		ar.Use(middleware.RequirePermission(roleSvc, domain.PermAccountsUsers))
		ar.Use(middleware.CSRF)
		ar.Post("/list", userAdminHandler.List)
		ar.Post("/create", userAdminHandler.Create)
		ar.Post("/detail", userAdminHandler.Detail)
		ar.Post("/update", userAdminHandler.Update)
		ar.Post("/reset-password", userAdminHandler.ResetPassword)
		ar.Post("/delete-preview", userAdminHandler.DeletePreview)
		ar.Post("/delete", userAdminHandler.Delete)
		ar.Post("/export", userAdminHandler.Export)
	})

	// 统一账号管理（后台「账号管理」单列表）：
	// 列表 / 详情 / 启停 / 改角色 / 重置密码 / 删除 / 导出 / 解绑 MFA。
	// 路由只要求 accounts.admins 或 accounts.users 之一；处理器再按目标账号
	// 角色做二次判定（后台账号需 accounts.admins，普通用户需 accounts.users），
	// 避免仅持有 accounts.users 的运营看到或操作管理员账号。
	accountAdminSvc := accountadmin.NewService(gormDB).
		WithPasswordPolicies(policy, userPolicy).
		WithSessionRevoker(sessionStore)
	accountAdminHandler := handler.NewAccountAdminHandler(accountAdminSvc, authSvc, auditSvc).
		WithPermissions(roleSvc)
	r.Route("/admin/accounts", func(ar chi.Router) {
		ar.Use(middleware.RequireAuth(authSvc))
		ar.Use(middleware.RequireAnyPermission(roleSvc, domain.PermAccountsAdmins, domain.PermAccountsUsers))
		ar.Use(middleware.CSRF)
		ar.Post("/list", accountAdminHandler.List)
		ar.Post("/create", accountAdminHandler.Create)
		ar.Post("/detail", accountAdminHandler.Detail)
		ar.Post("/update", accountAdminHandler.Update)
		ar.Post("/reset-password", accountAdminHandler.ResetPassword)
		ar.Post("/delete-preview", accountAdminHandler.DeletePreview)
		ar.Post("/delete", accountAdminHandler.Delete)
		ar.Post("/mfa/reset", accountAdminHandler.ResetMFA)
		ar.Post("/export", accountAdminHandler.Export)
	})

	// 用户成长管理（后台，需 ops.growth 权限）：概览 / 规则 / 等级 / 全局流水 /
	// 人工调整与冲正 / 审计；定义类操作与经验变更均写成长审计日志。
	r.Route("/admin/growth", func(ar chi.Router) {
		ar.Use(middleware.RequireAuth(authSvc))
		ar.Use(middleware.RequirePermission(roleSvc, domain.PermOpsGrowth))
		ar.Use(middleware.CSRF)
		ar.Post("/overview", growthHandler.AdminOverview)
		ar.Post("/users/list", growthHandler.AdminUsersList)
		ar.Post("/users/detail", growthHandler.AdminUserDetail)
		ar.Post("/ledger/list", growthHandler.AdminLedger)
		ar.Post("/adjust", growthHandler.Adjust)
		ar.Post("/reverse", growthHandler.Reverse)
		ar.Post("/audit/list", growthHandler.AdminAudit)
		ar.Post("/rules/list", growthHandler.RulesList)
		ar.Post("/rules/create", growthHandler.RulesCreate)
		ar.Post("/rules/update", growthHandler.RulesUpdate)
		ar.Post("/rules/toggle", growthHandler.RulesToggle)
		ar.Post("/rules/delete", growthHandler.RulesDelete)
		ar.Post("/levels/list", growthHandler.LevelsList)
		ar.Post("/levels/create", growthHandler.LevelsCreate)
		ar.Post("/levels/update", growthHandler.LevelsUpdate)
		ar.Post("/levels/delete", growthHandler.LevelsDelete)
		ar.Post("/levels/recalculate", growthHandler.LevelsRecalculate)
		// 单用户等级调整：只改等级偏移，不修改等级定义。
		ar.Post("/levels/adjust", growthHandler.LevelsAdjust)
	})

	// 每日任务定义管理（后台，需 ops.growth 权限）。
	r.Route("/admin/daily-tasks", func(ar chi.Router) {
		ar.Use(middleware.RequireAuth(authSvc))
		ar.Use(middleware.RequirePermission(roleSvc, domain.PermOpsGrowth))
		ar.Use(middleware.CSRF)
		ar.Post("/list", growthHandler.TasksList)
		ar.Post("/create", growthHandler.TasksCreate)
		ar.Post("/update", growthHandler.TasksUpdate)
		ar.Post("/toggle", growthHandler.TasksToggle)
		ar.Post("/delete", growthHandler.TasksDelete)
	})

	// 成就定义管理（后台，需 ops.growth 权限）：含历史补算。
	r.Route("/admin/achievements", func(ar chi.Router) {
		ar.Use(middleware.RequireAuth(authSvc))
		ar.Use(middleware.RequirePermission(roleSvc, domain.PermOpsGrowth))
		ar.Use(middleware.CSRF)
		ar.Post("/list", growthHandler.AchievementsList)
		ar.Post("/create", growthHandler.AchievementsCreate)
		ar.Post("/update", growthHandler.AchievementsUpdate)
		ar.Post("/toggle", growthHandler.AchievementsToggle)
		ar.Post("/recalculate", growthHandler.AchievementsRecalculate)
		// 单用户成就：查询 / 手动授予 / 手动撤销。
		ar.Post("/user/list", growthHandler.AchievementsUserList)
		ar.Post("/grant", growthHandler.AchievementsGrant)
		ar.Post("/revoke", growthHandler.AchievementsRevoke)
	})

	// 礼物运营管理（后台，需独立 ops.gifts 权限）：钱包 / 目录 / 签到设置 / 排行榜。
	r.Route("/admin/gifts", func(ar chi.Router) {
		ar.Use(middleware.RequireAuth(authSvc))
		ar.Use(middleware.RequirePermission(roleSvc, domain.PermOpsGifts))
		ar.Use(middleware.CSRF)
		ar.Post("/wallet/get", giftHandler.AdminWallet)
		ar.Post("/wallet/adjust", giftHandler.AdminAdjust)
		ar.Post("/ledger/list", giftHandler.AdminLedger)
		ar.Post("/definitions/list", giftHandler.AdminDefinitions)
		ar.Post("/definitions/create", giftHandler.AdminDefinitionCreate)
		ar.Post("/definitions/update", giftHandler.AdminDefinitionUpdate)
		ar.Post("/definitions/delete", giftHandler.AdminDefinitionDelete)
		ar.Post("/settings/get", giftHandler.AdminSettingsGet)
		ar.Post("/settings/update", giftHandler.AdminSettingsUpdate)
		ar.Post("/rankings/checkin", giftHandler.AdminCheckinRanking)
		ar.Post("/rankings/gifts", giftHandler.AdminGiftRanking)
		ar.Post("/rankings/coins", giftHandler.AdminCoinRanking)
	})

	// 数据质量与导入任务（admin 专属，TODO §2 / §4）
	dataQualitySvc := dataquality.NewService(gormDB)
	dataQualityHandler := handler.NewDataQualityHandler(dataQualitySvc, auditSvc)

	// 运营告警（gj.md 阶段三）：规则定时评估 + 人工确认 / 关闭，历史保留。
	alertSvc := alert.NewService(gormDB)
	alertSvc.RegisterRule(alert.Rule{
		Key:      "dataquality.error_checks",
		Severity: domain.AlertSeverityWarning,
		Title:    "存在未修复的严重数据质量问题",
		Check: func(jobCtx context.Context) (bool, string, map[string]any, error) {
			ov, err := dataQualitySvc.Overview(jobCtx)
			if err != nil {
				return false, "", nil, err
			}
			var n int64
			for _, c := range ov.Checks {
				if c.Severity == "error" && c.Count > 0 {
					n++
				}
			}
			metric := map[string]any{"error_checks": n}
			if n == 0 {
				return false, "", metric, nil
			}
			return true, fmt.Sprintf("%d 项严重数据质量问题仍未修复", n), metric, nil
		},
	})
	// 缓存容量告警：占用超过阈值时提示运营清理（jar 可重建，清理无业务风险）。
	if mcsrcStore != nil {
		alertSvc.RegisterRule(alert.Rule{
			Key:      "mcsrc.capacity",
			Severity: domain.AlertSeverityWarning,
			Title:    "MC 源码缓存占用超过阈值",
			Check: func(jobCtx context.Context) (bool, string, map[string]any, error) {
				_, usage, err := mcsrcStore.ListCache()
				if err != nil {
					return false, "", nil, err
				}
				limit := int64(cfgInstance.MCSRCCacheAlertMB) << 20
				metric := map[string]any{
					"total_bytes": usage.TotalBytes,
					"limit_bytes": limit,
					"entries":     usage.Entries,
				}
				if limit <= 0 || usage.TotalBytes < limit {
					return false, "", metric, nil
				}
				return true, fmt.Sprintf("缓存占用 %d MB，超过阈值 %d MB",
					usage.TotalBytes>>20, limit>>20), metric, nil
			},
		})
	}
	alertsHandler := handler.NewAlertsHandler(alertSvc, auditSvc)
	r.Route("/admin/data-quality", func(ar chi.Router) {
		ar.Use(middleware.RequireAuth(authSvc))
		ar.Use(middleware.RequirePermission(roleSvc, domain.PermOpsDataQuality))
		ar.Use(middleware.CSRF)
		ar.Post("/overview", dataQualityHandler.Overview)
		ar.Post("/assignees", dataQualityHandler.Assignees)
		ar.Post("/issues/update", dataQualityHandler.UpdateIssue)
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
	r.With(middleware.OptionalAuth(authSvc)).
		Post("/stats/event", handler.NewStatsHandler(statsRecorder, analyticsSvc).WithGrowthSink(growthSvc).Event)

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
	contentSvc := content.NewService(gormDB)

	// 管理端 CRUD
	adminHandler := handler.NewAdminHandler(resSvc, contentSvc, auditSvc)

	// 资源投稿审核（审核通过会生成 resources 记录，与资源管理同权能）。
	submissionAdminHandler := handler.NewSubmissionAdminHandler(submissionSvc, auditSvc).WithGrowthSink(growthSvc)

	// 链接检查器（SSRF 防护）
	linkChecker := lk.NewChecker(cfgInstance.LinkCheckTimeout, cfgInstance.LinkCheckMaxRedirect)
	adminHandler.SetLinkChecker(linkCheckAdapter{checker: linkChecker})

	// 内容历史版本（gj.md §15 / §16.2）：记录 + 差异 + 回滚。
	// 还原器在 main 装配，避免 revision 与内容服务之间的循环依赖。
	revisionSvc := revision.NewService(gormDB)
	revisionSvc.Register(domain.RevisionContentResource, resSvc)
	revisionSvc.Register(domain.RevisionContentTool, contentSvc)
	revisionSvc.Register(domain.RevisionContentWiki, contentSvc)
	revisionHandler := handler.NewRevisionHandler(revisionSvc, auditSvc)

	// 链接健康与人工确认（TODO §6：后台复查 + 并发限制）
	linkHealthSvc := linkhealth.NewService(gormDB, linkChecker, cfgInstance.LinkCheckConcurrency)
	linkHealthHandler := handler.NewLinkHealthHandler(linkHealthSvc, auditSvc)

	// 通用后台任务（gj.md 阶段二：任务重试与失败详情）。
	// 耗时 / 可能失败的运维动作统一走队列，避免同步请求超时，并支持自动与人工重试。
	jobSvc := jobs.NewService(gormDB)
	jobSvc.Register("linkhealth.run", "链接健康巡检", func(jobCtx context.Context, payload types.JSONB) (types.JSONB, error) {
		var p struct {
			OnlyPending bool `json:"only_pending"`
			Limit       int  `json:"limit"`
		}
		if len(payload) > 0 {
			if err := json.Unmarshal(payload, &p); err != nil {
				return nil, fmt.Errorf("任务参数格式错误")
			}
		}
		if p.Limit < 0 {
			p.Limit = 0
		}
		// 单次上限保护：全量巡检数据量大时应分批，而不是让一个任务跑几小时。
		if p.Limit > 2000 {
			p.Limit = 2000
		}
		sum, err := linkHealthSvc.RunCheck(jobCtx, linkhealth.RunOptions{
			OnlyPending:   p.OnlyPending,
			Limit:         p.Limit,
			TriggerSource: domain.LinkTriggerManual,
		})
		if err != nil {
			// 原始错误可能含 SQL / 连接串，只写日志，返回给运营的是通用文案。
			slog.Error("链接巡检任务失败", "err", err)
			return nil, fmt.Errorf("链接巡检执行失败，请查看服务日志")
		}
		out, _ := json.Marshal(sum)
		return types.JSONB(out), nil
	})
	// MC 源码缓存重建：清理 → 拉取版本详情 → 重新下载并建索引。
	// 失败原因面向运营脱敏，原始错误只进日志。
	jobSvc.Register("mcsrc.rebuild", "MC 源码缓存重建", func(jobCtx context.Context, payload types.JSONB) (types.JSONB, error) {
		var p struct {
			Version string `json:"version"`
			Side    string `json:"side"`
		}
		if len(payload) > 0 {
			if err := json.Unmarshal(payload, &p); err != nil {
				return nil, fmt.Errorf("任务参数格式错误")
			}
		}
		version := strings.TrimSpace(p.Version)
		if version == "" {
			return nil, fmt.Errorf("version 必填")
		}
		side := strings.ToLower(strings.TrimSpace(p.Side))
		if side == "" {
			side = mcsrc.SideClient
		}
		if side != mcsrc.SideClient && side != mcsrc.SideServer {
			return nil, fmt.Errorf("side 仅支持 client / server")
		}
		if mcsrcStore == nil || mcMetaClient == nil {
			return nil, fmt.Errorf("MC 源码缓存未启用")
		}
		// 先清理旧缓存；未缓存时 Purge 幂等返回 nil。
		if err := mcsrcStore.Purge(version, side); err != nil {
			if errors.Is(err, mcsrc.ErrCacheBusy) {
				return nil, fmt.Errorf("该版本正在下载 / 建索引，请稍后重试")
			}
			return nil, fmt.Errorf("清理旧缓存失败，请查看服务日志")
		}
		detail, err := mcMetaClient.FetchVersion(jobCtx, version)
		if err != nil {
			slog.Warn("缓存重建获取版本详情失败", "version", version, "err", err)
			return nil, fmt.Errorf("获取版本详情失败，请检查版本号与网络")
		}
		dl, ok := detail.Downloads[side]
		if !ok || strings.TrimSpace(dl.URL) == "" {
			return nil, fmt.Errorf("该版本缺少 %s 端下载地址", side)
		}
		st, err := mcsrcStore.Prepare(jobCtx, version, side, dl)
		if err != nil {
			slog.Error("缓存重建失败", "version", version, "side", side, "err", err)
			return nil, fmt.Errorf("下载或建索引失败，请查看服务日志")
		}
		out, _ := json.Marshal(map[string]any{
			"version":     st.Version,
			"side":        st.Side,
			"size":        st.Size,
			"class_count": st.ClassCount,
			"indexed":     st.Indexed,
		})
		return types.JSONB(out), nil
	})
	if mcsrcAdminHandler != nil {
		mcsrcAdminHandler.WithJobs(jobSvc)
	}
	jobsHandler := handler.NewJobsHandler(jobSvc, auditSvc)

	// 认证保护的管理接口
	r.Route("/admin", func(ar chi.Router) {
		ar.Use(middleware.RequireAuth(authSvc))
		ar.Use(middleware.CSRF)
		// 内容历史版本：按内容类型细分权限点，保持最小权限边界（§15 / §16.2）。
		ar.Route("/revisions", func(rv chi.Router) {
			for _, spec := range []struct{ path, ctype, perm string }{
				{"resources", domain.RevisionContentResource, domain.PermContentResources},
				{"tools", domain.RevisionContentTool, domain.PermContentTools},
				{"wiki", domain.RevisionContentWiki, domain.PermContentWiki},
			} {
				spec := spec
				rv.Route("/"+spec.path, func(x chi.Router) {
					x.Use(middleware.RequirePermission(roleSvc, spec.perm))
					x.Post("/list", revisionHandler.List(spec.ctype))
					x.Post("/diff", revisionHandler.Diff(spec.ctype))
					x.Post("/rollback", revisionHandler.Rollback(spec.ctype))
				})
			}
		})
		// 运营告警：汇总 / 列表 / 手动评估 / 确认 / 关闭（gj.md 阶段三）。
		ar.Route("/alerts", func(alr chi.Router) {
			alr.Use(middleware.RequirePermission(roleSvc, domain.PermOpsAlerts))
			alr.Post("/summary", alertsHandler.Summary)
			alr.Post("/list", alertsHandler.List)
			alr.Post("/evaluate", alertsHandler.Evaluate)
			alr.Post("/ack", alertsHandler.Ack)
			alr.Post("/resolve", alertsHandler.Resolve)
		})
		// 后台任务：入队 / 列表 / 详情 / 重试 / 取消（gj.md 阶段二）。
		ar.Route("/jobs", func(jr chi.Router) {
			jr.Use(middleware.RequirePermission(roleSvc, domain.PermOpsJobs))
			jr.Post("/types", jobsHandler.Types)
			jr.Post("/list", jobsHandler.List)
			jr.Post("/detail", jobsHandler.Detail)
			jr.Post("/enqueue", jobsHandler.Enqueue)
			jr.Post("/retry", jobsHandler.Retry)
			jr.Post("/cancel", jobsHandler.Cancel)
		})
		// 链接健康与人工确认（TODO §6）
		ar.Route("/link-health", func(lr chi.Router) {
			lr.Use(middleware.RequirePermission(roleSvc, domain.PermOpsLinkHealth))
			lr.Post("/run", linkHealthHandler.Run)
			lr.Post("/list", linkHealthHandler.List)
			lr.Post("/confirm", linkHealthHandler.Confirm)
			lr.Post("/{id}/confirm", linkHealthHandler.Confirm)
		})
		// 社区内容审核（隐藏 / 删除帖子与留言），权限点 content.community。
		ar.Route("/community", func(cr chi.Router) {
			cr.Use(middleware.RequirePermission(roleSvc, domain.PermContentCommunity))
			cr.Post("/posts/list", communityHandler.AdminListPosts)
			cr.Post("/posts/status", communityHandler.AdminSetPostStatus)
			cr.Post("/posts/delete", communityHandler.AdminDeletePost)
			cr.Post("/comments/list", communityHandler.AdminListComments)
			cr.Post("/comments/all", communityHandler.AdminListAllComments)
			cr.Post("/comments/status", communityHandler.AdminSetCommentStatus)
			cr.Post("/comments/delete", communityHandler.AdminDeleteComment)
		})
		// 内容健康：关键字屏蔽规则维护（ops.dataquality）。
		ar.Route("/content-health", func(kr chi.Router) {
			kr.Use(middleware.RequirePermission(roleSvc, domain.PermOpsDataQuality))
			kr.Post("/keywords/list", contentFilterHandler.List)
			kr.Post("/keywords/create", contentFilterHandler.Create)
			kr.Post("/keywords/update", contentFilterHandler.Update)
			kr.Post("/keywords/delete", contentFilterHandler.Delete)
		})
		ar.Route("/resources", func(rr chi.Router) {
			rr.Use(middleware.RequirePermission(roleSvc, domain.PermContentResources))
			// 前端契约：动作子路径 + 全 POST
			rr.Post("/list", adminHandler.ListResources)
			rr.Post("/export", adminHandler.ExportResources)
			rr.Post("/create", adminHandler.CreateResource)
			rr.Post("/detail", handler.WithBodyID(adminHandler.ResourceDetail))
			rr.Post("/update", handler.WithBodyID(adminHandler.UpdateResource))
			rr.Post("/unpublish", handler.WithBodyID(adminHandler.UnpublishResource))
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
			rr.Post("/{id}/unpublish", adminHandler.UnpublishResource)
			rr.Delete("/{id}", adminHandler.RemoveResource)
			rr.Post("/{id}/check-link", adminHandler.CheckResourceLink)
		})
		// 资源投稿审核（用户提交 → 通过则生成资源）：与资源管理同权限点。
		ar.Route("/submissions", func(sr chi.Router) {
			sr.Use(middleware.RequirePermission(roleSvc, domain.PermContentResources))
			sr.Post("/list", submissionAdminHandler.List)
			sr.Post("/detail", submissionAdminHandler.Detail)
			sr.Post("/review", submissionAdminHandler.Review)
			// 批量审核：逐条独立事务，返回逐条结果（部分成功是正常情况）。
			sr.Post("/batch-review", submissionAdminHandler.BatchReview)
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
			rr.Post("/usage", adminHandler.TagUsage)
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
			protected.Post("/export", auditHandler.Export)
		})
	})

	// 公开 API
	searchSvc := search.NewService(gormDB)

	// 内容建设任务（gj.md §8.3 搜索词闭环）：搜索缺口 → 可指派/可跟踪的运营任务。
	contentTaskSvc := contenttask.NewService(gormDB, searchSvc)
	contentTaskHandler := handler.NewContentTaskHandler(contentTaskSvc, auditSvc)
	r.Route("/admin/content-tasks", func(ar chi.Router) {
		ar.Use(middleware.RequireAuth(authSvc))
		ar.Use(middleware.RequirePermission(roleSvc, domain.PermOpsContentTasks))
		ar.Use(middleware.CSRF)
		ar.Post("/list", contentTaskHandler.List)
		ar.Post("/candidates", contentTaskHandler.Candidates)
		ar.Post("/create", contentTaskHandler.Create)
		ar.Post("/update", contentTaskHandler.Update)
		ar.Post("/recheck", contentTaskHandler.Recheck)
		ar.Post("/delete", contentTaskHandler.Delete)
	})

	// 红石机器设计审核（TODO §15，admin 专属）：按审核状态查询 + 审核留痕。
	redstoneAdminHandler := handler.NewRedstoneAdminHandler(catalogSvc, auditSvc)
	r.Route("/admin/redstone", func(ar chi.Router) {
		ar.Use(middleware.RequireAuth(authSvc))
		ar.Use(middleware.RequirePermission(roleSvc, domain.PermReviewRedstone))
		ar.Use(middleware.CSRF)
		ar.Post("/designs/list", redstoneAdminHandler.ListDesigns)
		ar.Post("/designs/review", redstoneAdminHandler.ReviewDesign)
		ar.Post("/designs/batch-review", redstoneAdminHandler.BatchReviewDesigns)
	})

	// Mod 与整合包审核（TODO §16，admin 专属）：按审核状态查询 + 审核留痕。
	modAdminHandler := handler.NewModAdminHandler(catalogSvc, auditSvc)
	r.Route("/admin/mods", func(ar chi.Router) {
		ar.Use(middleware.RequireAuth(authSvc))
		ar.Use(middleware.RequirePermission(roleSvc, domain.PermReviewMods))
		ar.Use(middleware.CSRF)
		ar.Post("/list", modAdminHandler.AdminMods)
		ar.Post("/review", modAdminHandler.ReviewMod)
		ar.Post("/batch-review", modAdminHandler.BatchReviewMods)
	})
	r.Route("/admin/modpacks", func(ar chi.Router) {
		ar.Use(middleware.RequireAuth(authSvc))
		ar.Use(middleware.RequirePermission(roleSvc, domain.PermReviewMods))
		ar.Use(middleware.CSRF)
		ar.Post("/list", modAdminHandler.AdminModpacks)
		ar.Post("/review", modAdminHandler.ReviewModpack)
		ar.Post("/batch-review", modAdminHandler.BatchReviewModpacks)
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

	return jobSvc, alertSvc
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
