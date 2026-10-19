// Package config 负责从环境变量加载并校验应用配置。
// 缺少关键配置时必须在启动阶段明确失败，不静默使用弱默认值。
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config 应用运行所需的全部配置。
type Config struct {
	AppEnv     string
	AppAddr    string
	AppBaseURL string

	DatabaseURL string

	SessionSecret string
	// SessionCookieName 统一会话 Cookie 名（迁移 0022 后管理员与普通用户共用一个会话）。
	SessionCookieName string
	SessionSecure     bool

	CORSAllowedOrigins []string
	CSRFExemptOrigins  []string

	LogLevel string

	RateLimitRPS   float64
	RateLimitBurst int

	// MCFindRPS/MCFindBurst 为 /mc/seeds/find 单独限流（该接口单次限时 5s，
	// 与全局限流共用会长期占用 worker，故单独设置更小配额）。
	MCFindRPS   float64
	MCFindBurst int

	// MCHeavyRPS/MCHeavyBurst 为结构候选（/mc/structures）、史莱姆区块
	// （/mc/slime-chunks）等遍历型接口，以及上传解析类接口
	// （/mc/region/inspect、/mc/level/parse）单独限流：它们单次可能遍历数千个点
	// 或解压放大，与普通查询共用配额会挤占 worker，故单独设置更小配额。
	MCHeavyRPS   float64
	MCHeavyBurst int
	// MCHeavyConcurrency 限制上述重接口同时执行的请求数，避免并发叠加占用
	// CPU / 内存；超出返回 429。<=0 表示不限制。
	MCHeavyConcurrency int

	// RegionCacheMaxMB/RegionCacheTTL 为 /mc/region/inspect 返回的 region_token
	// 内存缓存上限与有效期（仅内存、不落盘）。
	RegionCacheMaxMB int
	RegionCacheTTL   time.Duration

	LinkCheckTimeout     time.Duration
	LinkCheckMaxRedirect int
	LinkCheckConcurrency int

	// MCMetaCacheDir Mojang 版本元数据磁盘缓存目录。
	MCMetaCacheDir string
	// MCMetaCacheTTL 缓存有效期（默认 7 天）。
	MCMetaCacheTTL time.Duration
	// BlockIDFile 物品对照表（物品id.md）路径。
	BlockIDFile string

	// MC 源码库（mcsrc）后端支持：服务端下载并缓存 Minecraft jar，构建类索引，
	// 供「源码浏览器」按版本浏览与搜索，避免每个访客各自下载数百 MB。
	// MCSRCEnabled 是否启用（默认 true；关闭后相关接口返回 503）。
	MCSRCEnabled bool
	// MCSRCCacheDir jar 与索引的磁盘缓存目录（默认 <MCMetaCacheDir>/mcsrc）。
	MCSRCCacheDir string
	// MCSRCJarMaxMB 单个 jar 体积上限（MB，默认 512），防止异常文件耗尽磁盘。
	MCSRCJarMaxMB int
	// MCSRCIndexMaxClasses 单个 jar 建索引的类数量上限（默认 60000），防止异常膨胀。
	MCSRCIndexMaxClasses int
	// MCSRCAutoPrepareMax 匿名访客可自动准备的版本数上限（默认 12）。
	// 前端在用户选中版本时会自动触发下载 + 建索引；该额度约束匿名触发者，
	// 避免任意访客逐个版本把磁盘与带宽吃满（0 表示关闭自动准备，仅管理员可准备）。
	// 具备 ops.mcsrc 的管理员不受此额度限制。
	MCSRCAutoPrepareMax int

	// 使用统计与运营看板（TODO §13）。
	// StatsEnabled 是否启用使用统计采集（默认 true）。
	StatsEnabled bool
	// StatsFlushSeconds 内存聚合落库周期（秒，默认 60）。
	StatsFlushSeconds int
	// StatsRetentionDays 日聚合保留天数（含当天，默认 180）；<=0 表示不清理。
	StatsRetentionDays int
}

// Load 从环境变量加载配置并提供默认值。
// 当安全敏感配置缺失或非法时返回错误，阻止启动。
func Load() (*Config, error) {
	cfg := &Config{
		AppEnv:     os.Getenv("APP_ENV"),
		AppAddr:    os.Getenv("APP_ADDR"),
		AppBaseURL: os.Getenv("APP_BASE_URL"),

		DatabaseURL: os.Getenv("DATABASE_URL"),

		SessionSecret:     os.Getenv("SESSION_SECRET"),
		SessionCookieName: os.Getenv("SESSION_COOKIE_NAME"),
		SessionSecure:     parseBool(os.Getenv("SESSION_SECURE_COOKIE"), false),

		CORSAllowedOrigins: splitList(os.Getenv("CORS_ALLOWED_ORIGINS")),
		CSRFExemptOrigins:  splitList(os.Getenv("CSRF_EXEMPT_ORIGINS")),

		LogLevel: os.Getenv("LOG_LEVEL"),

		RateLimitRPS:   parseFloat(os.Getenv("RATE_LIMIT_RPS"), 10),
		RateLimitBurst: parseInt(os.Getenv("RATE_LIMIT_BURST"), 20),

		MCFindRPS:   parseFloat(os.Getenv("MC_FIND_RPS"), 1),
		MCFindBurst: parseInt(os.Getenv("MC_FIND_BURST"), 2),

		MCHeavyRPS:         parseFloat(os.Getenv("MC_HEAVY_RPS"), 3),
		MCHeavyBurst:       parseInt(os.Getenv("MC_HEAVY_BURST"), 6),
		MCHeavyConcurrency: parseInt(os.Getenv("MC_HEAVY_CONCURRENCY"), 2),

		RegionCacheMaxMB: parseInt(os.Getenv("REGION_CACHE_MAX_MB"), 256),
		RegionCacheTTL:   time.Duration(parseInt(os.Getenv("REGION_CACHE_TTL_SECONDS"), 600)) * time.Second,

		LinkCheckTimeout:     parseDuration(os.Getenv("LINK_CHECK_TIMEOUT_SECONDS"), 8),
		LinkCheckMaxRedirect: parseInt(os.Getenv("LINK_CHECK_MAX_REDIRECTS"), 3),
		LinkCheckConcurrency: parseInt(os.Getenv("LINK_CHECK_MAX_CONCURRENCY"), 4),

		MCMetaCacheDir: os.Getenv("MCMETA_CACHE_DIR"),
		MCMetaCacheTTL: time.Duration(parseInt(os.Getenv("MCMETA_CACHE_TTL_HOURS"), 168)) * time.Hour,
		BlockIDFile:    os.Getenv("BLOCK_ID_FILE"),

		MCSRCEnabled:         parseBool(os.Getenv("MCSRC_ENABLED"), true),
		MCSRCCacheDir:        os.Getenv("MCSRC_CACHE_DIR"),
		MCSRCJarMaxMB:        parseInt(os.Getenv("MCSRC_JAR_MAX_MB"), 512),
		MCSRCIndexMaxClasses: parseInt(os.Getenv("MCSRC_INDEX_MAX_CLASSES"), 60000),
		MCSRCAutoPrepareMax:  parseInt(os.Getenv("MCSRC_AUTO_PREPARE_MAX"), 12),

		StatsEnabled:       parseBool(os.Getenv("STATS_ENABLED"), true),
		StatsFlushSeconds:  parseInt(os.Getenv("STATS_FLUSH_SECONDS"), 60),
		StatsRetentionDays: parseInt(os.Getenv("STATS_RETENTION_DAYS"), 180),
	}

	if cfg.AppEnv == "" {
		cfg.AppEnv = "development"
	}
	if cfg.AppAddr == "" {
		cfg.AppAddr = ":8080"
	}
	if cfg.AppBaseURL == "" {
		cfg.AppBaseURL = "http://localhost:5173"
	}
	if cfg.SessionCookieName == "" {
		cfg.SessionCookieName = "mc_session"
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}
	if cfg.LinkCheckTimeout <= 0 {
		cfg.LinkCheckTimeout = 8 * time.Second
	}
	if cfg.LinkCheckMaxRedirect < 0 {
		cfg.LinkCheckMaxRedirect = 3
	}
	if cfg.LinkCheckConcurrency <= 0 {
		cfg.LinkCheckConcurrency = 4
	}
	if cfg.LinkCheckConcurrency > 16 {
		cfg.LinkCheckConcurrency = 16
	}
	if cfg.MCMetaCacheDir == "" {
		cfg.MCMetaCacheDir = "cache"
	}
	if cfg.MCMetaCacheTTL <= 0 {
		cfg.MCMetaCacheTTL = 7 * 24 * time.Hour
	}
	if cfg.BlockIDFile == "" {
		cfg.BlockIDFile = "data/物品id.md"
	}
	if cfg.MCSRCCacheDir == "" {
		cfg.MCSRCCacheDir = filepath.Join(cfg.MCMetaCacheDir, "mcsrc")
	}
	if cfg.MCSRCJarMaxMB <= 0 {
		cfg.MCSRCJarMaxMB = 512
	}
	if cfg.MCSRCIndexMaxClasses <= 0 {
		cfg.MCSRCIndexMaxClasses = 60000
	}
	if cfg.RegionCacheMaxMB <= 0 {
		cfg.RegionCacheMaxMB = 256
	}
	if cfg.RegionCacheTTL <= 0 {
		cfg.RegionCacheTTL = 10 * time.Minute
	}
	if cfg.StatsFlushSeconds <= 0 {
		cfg.StatsFlushSeconds = 60
	}
	if cfg.StatsRetentionDays < 0 {
		cfg.StatsRetentionDays = 180
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	if c.DatabaseURL == "" {
		return fmt.Errorf("缺少必需配置 DATABASE_URL")
	}
	if c.SessionSecret == "" {
		return fmt.Errorf("缺少必需配置 SESSION_SECRET，生产环境必须使用强随机值")
	}
	if len(c.SessionSecret) < 16 {
		return fmt.Errorf("SESSION_SECRET 过短（小于 16 字符），存在会话伪造风险")
	}
	if len(c.CORSAllowedOrigins) == 0 {
		return fmt.Errorf("缺少必需配置 CORS_ALLOWED_ORIGINS，禁止开放任意跨域来源")
	}
	for _, o := range c.CORSAllowedOrigins {
		if o == "*" {
			return fmt.Errorf("CORS_ALLOWED_ORIGINS 禁止使用通配符 * 作为来源")
		}
	}
	if c.RateLimitRPS <= 0 || c.RateLimitBurst <= 0 {
		return fmt.Errorf("RATE_LIMIT_RPS / RATE_LIMIT_BURST 必须为正数")
	}
	return nil
}

func splitList(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseBool(s string, def bool) bool {
	v, err := strconv.ParseBool(s)
	if err != nil {
		return def
	}
	return v
}

func parseFloat(s string, def float64) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return def
	}
	return v
}

func parseInt(s string, def int) int {
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}

func parseDuration(s string, def int) time.Duration {
	v, err := strconv.Atoi(s)
	if err != nil || v < 0 {
		return time.Duration(def) * time.Second
	}
	return time.Duration(v) * time.Second
}
