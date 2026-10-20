package config

import (
	"strings"
	"testing"
	"time"
)

// setBase 设置一组满足最低要求的配置。
func setBase(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost:5432/db?sslmode=disable")
	t.Setenv("SESSION_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("CORS_ALLOWED_ORIGINS", "http://localhost:5173")
	t.Setenv("RATE_LIMIT_RPS", "10")
	t.Setenv("RATE_LIMIT_BURST", "20")
}

// TestLoadValid 合法配置应加载成功并填充默认值。
func TestLoadValid(t *testing.T) {
	setBase(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("合法配置不应报错: %v", err)
	}
	if cfg.AppEnv != "development" {
		t.Errorf("默认 APP_ENV 应为 development，实际 %q", cfg.AppEnv)
	}
	if cfg.AppAddr != ":8080" {
		t.Errorf("默认 APP_ADDR 应为 :8080，实际 %q", cfg.AppAddr)
	}
	// 统一账号（迁移 0022）：管理员与普通用户共用一个会话 Cookie。
	if cfg.SessionCookieName != "mc_session" {
		t.Errorf("默认统一会话 Cookie 名异常: %q", cfg.SessionCookieName)
	}
	if cfg.LinkCheckTimeout != 8*time.Second {
		t.Errorf("默认链接检查超时应为 8s，实际 %v", cfg.LinkCheckTimeout)
	}
	if !cfg.AutoMigrate {
		t.Error("AUTO_MIGRATE 默认应为 true")
	}
	if cfg.MigrationsDir != "migrations" {
		t.Errorf("默认迁移目录应为 migrations，实际 %q", cfg.MigrationsDir)
	}
}

// TestLoadMissingRequired 覆盖需求 §12：缺少关键配置必须启动失败。
func TestLoadMissingRequired(t *testing.T) {
	cases := []struct {
		name    string
		key     string
		value   string
		wantSub string
	}{
		{"缺少 DATABASE_URL", "DATABASE_URL", "", "DATABASE_URL"},
		{"缺少 SESSION_SECRET", "SESSION_SECRET", "", "SESSION_SECRET"},
		{"SESSION_SECRET 过短", "SESSION_SECRET", "short", "过短"},
		{"缺少 CORS 来源", "CORS_ALLOWED_ORIGINS", "", "CORS_ALLOWED_ORIGINS"},
		{"CORS 禁止通配符", "CORS_ALLOWED_ORIGINS", "*", "通配符"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setBase(t)
			t.Setenv(tc.key, tc.value)
			_, err := Load()
			if err == nil {
				t.Fatalf("期望加载失败，实际通过")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("错误信息应包含 %q，实际 %q", tc.wantSub, err.Error())
			}
		})
	}
}

// TestLoadRateLimitInvalid 限流参数必须为正。
func TestLoadRateLimitInvalid(t *testing.T) {
	setBase(t)
	t.Setenv("RATE_LIMIT_RPS", "0")
	if _, err := Load(); err == nil {
		t.Fatal("RPS 为 0 应启动失败")
	}
}

// TestLoadCaptchaDefaults 验证码默认启用、默认有效期 5 分钟、固定码默认空。
func TestLoadCaptchaDefaults(t *testing.T) {
	setBase(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("合法配置不应报错: %v", err)
	}
	if !cfg.CaptchaEnabled {
		t.Error("CAPTCHA_ENABLED 默认应为 true")
	}
	if cfg.CaptchaTTL != 5*time.Minute {
		t.Errorf("默认验证码有效期应为 5m，实际 %v", cfg.CaptchaTTL)
	}
	if cfg.CaptchaFixedCode != "" {
		t.Errorf("默认固定码应为空，实际 %q", cfg.CaptchaFixedCode)
	}
}

// TestLoadCaptchaFixedCodeProductionRejected 生产环境禁止固定码（失败关闭）。
func TestLoadCaptchaFixedCodeProductionRejected(t *testing.T) {
	setBase(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("CAPTCHA_FIXED_CODE", "135790")
	_, err := Load()
	if err == nil {
		t.Fatal("生产环境设置 CAPTCHA_FIXED_CODE 应启动失败")
	}
	if !strings.Contains(err.Error(), "CAPTCHA_FIXED_CODE") {
		t.Fatalf("错误信息应包含 CAPTCHA_FIXED_CODE，实际 %q", err.Error())
	}

	// 非生产环境允许（自动化测试需要）。
	setBase(t)
	t.Setenv("APP_ENV", "test")
	t.Setenv("CAPTCHA_FIXED_CODE", "135790")
	if _, err := Load(); err != nil {
		t.Fatalf("测试环境允许固定码，实际 %v", err)
	}
}
