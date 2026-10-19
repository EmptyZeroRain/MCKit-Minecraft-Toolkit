package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestValidatePassword(t *testing.T) {
	admin := DefaultAdminPolicy()
	user := DefaultUserPolicy()

	cases := []struct {
		name     string
		password string
		policy   PasswordPolicy
		username string
		wantErr  bool
	}{
		{"后台账号过短", "short123456", admin, "root", true},
		{"后台账号达标", "short12345678", admin, "root", false},
		{"普通用户达标", "password123", user, "alice", false},
		{"普通用户过短", "abc123", user, "alice", true},
		{"单一重复字符", "aaaaaaaaaaaa", admin, "root", true},
		{"包含用户名", "root-password-1", admin, "root", true},
		{"包含用户名大小写不敏感", "MyRootPass123", admin, "ROOT", true},
		{"超长", strings.Repeat("a1", 100), admin, "root", true},
		{"空用户名跳过包含校验", "password12345", admin, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidatePassword(c.password, c.policy, c.username)
			if c.wantErr {
				if err == nil {
					t.Fatalf("期望校验失败，实际通过")
				}
				if !errors.Is(err, ErrWeakPassword) {
					t.Fatalf("错误应包裹 ErrWeakPassword，实际 %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("期望通过，实际 %v", err)
			}
		})
	}
}

func TestPasswordPolicyNormalize(t *testing.T) {
	// 非法参数应被补齐为可用策略（不会放行弱口令）。
	p := PasswordPolicy{MinLength: 0, MaxLength: 0}.normalize()
	if p.MinLength < 8 || p.MaxLength < p.MinLength {
		t.Fatalf("策略未补齐: %+v", p)
	}
}
