// Package mfa 实现后台账号的 TOTP 双因子认证基础能力。
//
// 设计边界：
//   - 本包只做纯算法与加解密，不碰数据库、不碰 HTTP，便于单元测试；
//   - TOTP 参数固定为 RFC 6238 的通用组合（SHA-1 / 6 位 / 30 秒），
//     校验窗口 ±1 步以容忍客户端时钟漂移；
//   - secret 落库前用 AES-256-GCM 加密，密钥由部署侧 MFA_ENCRYPTION_KEY 提供，
//     数据库泄露不等于 MFA 种子泄露；
//   - 恢复码只以 SHA-256 哈希落库，明文仅在生成时返回一次。
package mfa

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// 参数常量：改动会影响已绑定的账号，禁止随意调整。
const (
	// Period TOTP 时间步长（秒）。
	Period = 30
	// Digits 验证码位数。
	Digits = 6
	// Skew 允许前后偏移的时间步数（±1 步 = ±30 秒）。
	Skew = 1
	// SecretBytes 随机 secret 字节数（160 bit，RFC 4226 推荐）。
	SecretBytes = 20
	// RecoveryCodeCount 恢复码数量。
	RecoveryCodeCount = 10
	// RecoveryCodeLength 单个恢复码的字符数（不含分隔符）。
	RecoveryCodeLength = 10
)

// 恢复码字母表：去掉易混淆的 0/O、1/I/L，降低用户手抄错误。
const recoveryAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

var (
	// ErrInvalidSecret secret 格式不合法（非 base32）。
	ErrInvalidSecret = errors.New("MFA secret 格式不合法")
	// ErrInvalidKey 加密密钥长度不合法（必须是 16 / 24 / 32 字节）。
	ErrInvalidKey = errors.New("MFA 加密密钥长度必须为 16、24 或 32 字节")
	// ErrDecrypt 密文无法解密（密钥不匹配或数据损坏）。
	ErrDecrypt = errors.New("MFA 密文解密失败")
)

// b32 无填充 base32：otpauth URI 与大多数认证器都接受该形式。
var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// GenerateSecret 生成新的 TOTP secret（base32 无填充）。
func GenerateSecret() (string, error) {
	buf := make([]byte, SecretBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return b32.EncodeToString(buf), nil
}

// GenerateRecoveryCodes 生成 n 个一次性恢复码，格式为 `XXXXX-XXXXX`。
func GenerateRecoveryCodes(n int) ([]string, error) {
	if n <= 0 {
		n = RecoveryCodeCount
	}
	codes := make([]string, 0, n)
	for i := 0; i < n; i++ {
		raw := make([]byte, RecoveryCodeLength)
		if _, err := rand.Read(raw); err != nil {
			return nil, err
		}
		var sb strings.Builder
		for j, b := range raw {
			if j == RecoveryCodeLength/2 {
				sb.WriteByte('-')
			}
			sb.WriteByte(recoveryAlphabet[int(b)%len(recoveryAlphabet)])
		}
		codes = append(codes, sb.String())
	}
	return codes, nil
}

// NormalizeRecoveryCode 归一化用户输入：去空格 / 连字符并转大写。
func NormalizeRecoveryCode(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	code = strings.ReplaceAll(code, "-", "")
	code = strings.ReplaceAll(code, " ", "")
	return code
}

// HashRecoveryCode 计算恢复码的存储哈希（SHA-256 hex）。
//
// 恢复码本身是高熵随机串（≈49 bit），无需加盐 / 慢哈希；
// 加盐反而让「按账号查哈希」失效，失去一次性校验的索引能力。
func HashRecoveryCode(code string) string {
	sum := sha256.Sum256([]byte(NormalizeRecoveryCode(code)))
	return hex.EncodeToString(sum[:])
}

// ParseKey 解析部署侧提供的加密密钥。
//
// 支持 base64（标准 / URL 安全，含或不含填充）与 hex；解析后必须是 16/24/32 字节。
func ParseKey(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("MFA 加密密钥为空")
	}
	candidates := [][]byte{}
	if b, err := base64.StdEncoding.DecodeString(raw); err == nil {
		candidates = append(candidates, b)
	}
	if b, err := base64.RawStdEncoding.DecodeString(raw); err == nil {
		candidates = append(candidates, b)
	}
	if b, err := base64.RawURLEncoding.DecodeString(raw); err == nil {
		candidates = append(candidates, b)
	}
	if b, err := hex.DecodeString(raw); err == nil {
		candidates = append(candidates, b)
	}
	for _, b := range candidates {
		switch len(b) {
		case 16, 24, 32:
			return b, nil
		}
	}
	return nil, ErrInvalidKey
}

// Encrypt 用 AES-GCM 加密 secret，输出 base64(nonce || ciphertext)。
func Encrypt(key []byte, plaintext string) (string, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	out := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(out), nil
}

// Decrypt 解密 Encrypt 的输出。
func Decrypt(key []byte, encoded string) (string, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(raw) < gcm.NonceSize() {
		return "", ErrDecrypt
	}
	nonce, ciphertext := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", ErrDecrypt
	}
	return string(plain), nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	switch len(key) {
	case 16, 24, 32:
	default:
		return nil, ErrInvalidKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// CodeAt 计算指定时刻的 TOTP 验证码（供测试与服务端校验使用）。
func CodeAt(secret string, at time.Time) (string, error) {
	key, err := decodeSecret(secret)
	if err != nil {
		return "", err
	}
	counter := uint64(at.Unix() / Period)
	return hotp(key, counter), nil
}

// Validate 校验用户输入的 TOTP，允许 ±Skew 个时间步的时钟漂移。
//
// 返回命中的时间步计数器（用于防重放：同一计数器只能成功一次时为非零），
// 校验失败时返回 0 与 false。
func Validate(secret, code string, at time.Time) (uint64, bool) {
	code = strings.TrimSpace(code)
	if len(code) != Digits {
		return 0, false
	}
	key, err := decodeSecret(secret)
	if err != nil {
		return 0, false
	}
	base := at.Unix() / Period
	for delta := int64(-Skew); delta <= Skew; delta++ {
		step := base + delta
		if step < 0 {
			continue
		}
		want := hotp(key, uint64(step))
		// 常量时间比较，避免时序侧信道。
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return uint64(step), true
		}
	}
	return 0, false
}

func decodeSecret(secret string) ([]byte, error) {
	secret = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(secret), " ", ""))
	// 兼容用户手抄时补上的填充。
	if pad := len(secret) % 8; pad != 0 {
		secret += strings.Repeat("=", 8-pad)
	}
	key, err := base32.StdEncoding.DecodeString(secret)
	if err != nil || len(key) == 0 {
		return nil, ErrInvalidSecret
	}
	return key, nil
}

// hotp 实现 RFC 4226；TOTP = HOTP(counter = unix / Period)。
func hotp(key []byte, counter uint64) string {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, counter)
	mac := hmac.New(sha1.New, key)
	mac.Write(buf)
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := (uint32(sum[offset])&0x7f)<<24 |
		uint32(sum[offset+1])<<16 |
		uint32(sum[offset+2])<<8 |
		uint32(sum[offset+3])
	mod := uint32(1)
	for i := 0; i < Digits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", Digits, value%mod)
}

// ProvisioningURI 生成 otpauth:// URI，供前端渲染二维码。
func ProvisioningURI(issuer, account, secret string) string {
	label := issuer + ":" + account
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", fmt.Sprintf("%d", Digits))
	q.Set("period", fmt.Sprintf("%d", Period))
	// 二维码内容固定为标准 otpauth 形式，冒号在 Path 中合法但需转义非 ASCII。
	return "otpauth://totp/" + url.PathEscape(label) + "?" + q.Encode()
}
