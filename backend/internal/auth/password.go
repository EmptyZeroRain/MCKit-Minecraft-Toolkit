// Package auth 提供管理员认证：Argon2id 密码哈希、Cookie 会话与 CSRF。
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// 密码哈希参数（OWASP 推荐基线；生产可按硬件上调）。
const (
	argonTime    = 3
	argonMemory  = 64 * 1024 // 64 MiB
	argonThreads = 2
	argonKeyLen  = 32
	saltLen      = 16
)

// ErrInvalidPassword 密码不匹配。
var ErrInvalidPassword = errors.New("密码不正确")

// HashPassword 使用 Argon2id 生成密码哈希。
// 格式：$argon2id$v=19$m=<MiB>,t=<time>,p=<threads>$<salt>$<hash>
func HashPassword(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("生成盐值失败: %w", err)
	}
	hash := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)

	encoded := base64.RawStdEncoding.EncodeToString(hash)
	encodedSalt := base64.RawStdEncoding.EncodeToString(salt)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argonMemory, argonTime, argonThreads, encodedSalt, encoded), nil
}

// VerifyPassword 校验密码是否与存储哈希匹配。
// 不匹配返回 ErrInvalidPassword；存储格式非法返回错误。
func VerifyPassword(storedHash, password string) error {
	parts := strings.Split(storedHash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return errors.New("存储的密码哈希格式非法")
	}

	var m, t, p, keyLen uint32
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return errors.New("存储的密码哈希参数非法")
	}
	keyLen = argonKeyLen

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return errors.New("存储的密码盐值非法")
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return errors.New("存储的密码哈希非法")
	}

	got := argon2.IDKey([]byte(password), salt, t, m, uint8(p), keyLen)
	if subtle.ConstantTimeCompare(got, want) != 1 {
		return ErrInvalidPassword
	}
	return nil
}

// RandomToken 生成 URL 安全的随机令牌（用于会话 ID / CSRF token）。
func RandomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
