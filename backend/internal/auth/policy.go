package auth

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// PasswordPolicy 统一密码策略（gj.md §3.3 / §10）。
//
// 后台账号与普通用户共用同一套校验实现，仅参数不同（后台最小长度更高），
// 避免「文档写一套、实现另一套」。长度按 Unicode 字符计。
type PasswordPolicy struct {
	// MinLength 最小长度。
	MinLength int
	// MaxLength 最大长度（防止超长输入拖慢 Argon2 哈希）。
	MaxLength int
}

// DefaultAdminPolicy 后台账号默认策略（最小 12 位）。
func DefaultAdminPolicy() PasswordPolicy { return PasswordPolicy{MinLength: 12, MaxLength: 128} }

// DefaultUserPolicy 普通用户默认策略（最小 8 位）。
func DefaultUserPolicy() PasswordPolicy { return PasswordPolicy{MinLength: 8, MaxLength: 128} }

// normalize 补齐非法参数，保证策略始终可用（配置缺省时不会放行弱口令）。
func (p PasswordPolicy) normalize() PasswordPolicy {
	if p.MinLength < 6 {
		p.MinLength = 8
	}
	if p.MaxLength < p.MinLength {
		p.MaxLength = 128
	}
	return p
}

// ErrWeakPassword 密码不满足策略。具体原因在错误信息中，可直接展示给用户。
var ErrWeakPassword = errors.New("密码强度不足")

// ValidatePassword 校验密码是否符合策略。
//
// 校验项：长度区间、不得为单一重复字符、不得包含用户名（大小写不敏感）。
// 说明：这里刻意不引入「常见弱口令字典」——字典需要持续维护且容易误伤，
// 本项目的策略以「长度 + 形态」为基线；如需字典可作为后续增强单独评估。
// 返回的错误均包裹 ErrWeakPassword，调用方可用 errors.Is 判定。
func ValidatePassword(password string, policy PasswordPolicy, username string) error {
	p := policy.normalize()
	n := utf8.RuneCountInString(password)
	if n < p.MinLength {
		return fmt.Errorf("%w：长度至少 %d 位", ErrWeakPassword, p.MinLength)
	}
	if n > p.MaxLength {
		return fmt.Errorf("%w：长度不能超过 %d 位", ErrWeakPassword, p.MaxLength)
	}
	if isAllSameRune(password) {
		return fmt.Errorf("%w：不能由单一重复字符组成", ErrWeakPassword)
	}
	if u := strings.ToLower(strings.TrimSpace(username)); u != "" &&
		strings.Contains(strings.ToLower(password), u) {
		return fmt.Errorf("%w：不能包含用户名", ErrWeakPassword)
	}
	return nil
}

// isAllSameRune 判断字符串是否由同一个字符重复组成（如 "aaaaaaaa"）。
func isAllSameRune(s string) bool {
	first, size := utf8.DecodeRuneInString(s)
	if size == 0 {
		return false
	}
	for _, r := range s[size:] {
		if r != first {
			return false
		}
	}
	return true
}
