// Package accountid 生成账号对外展示 ID（accounts.user_no）。
//
// 设计说明：
//   - 内部主键仍是 UUID（accounts.id），用于外键与鉴权，不对外暴露；
//   - user_no 是面向用户展示的随机短 ID，形如 U7K3M2QX9F；
//   - 使用 crypto/rand 生成，字符集排除 I / L / O / U 等易混淆字符，
//     避免口头或截图传播时误读；
//   - 一旦生成不再变更（迁移与更新语句均不触碰该列）。
package accountid

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
)

const (
	// Prefix 对外 ID 固定前缀。
	Prefix = "U"
	// BodyLength 随机部分长度；30^10 ≈ 5.9e14，足够避免碰撞。
	BodyLength = 10
	// MaxLength 与 accounts.user_no 列宽（VARCHAR(12)）一致。
	MaxLength = 12
	// alphabet 字符集：2-9（8 个）+ A-Z 去掉 I / L / O / U（22 个）= 30 个，
	// 排除易混淆字符，便于口头 / 截图传播。
	alphabet = "23456789ABCDEFGHJKMNPQRSTVWXYZ"
)

// Generate 生成一个新的随机对外用户 ID。
func Generate() (string, error) {
	var b strings.Builder
	b.Grow(1 + BodyLength)
	b.WriteString(Prefix)
	limit := big.NewInt(int64(len(alphabet)))
	for i := 0; i < BodyLength; i++ {
		n, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", fmt.Errorf("生成用户 ID 失败: %w", err)
		}
		b.WriteByte(alphabet[n.Int64()])
	}
	return b.String(), nil
}

// Valid 判断字符串是否符合对外用户 ID 格式。
func Valid(s string) bool {
	if len(s) != 1+BodyLength || !strings.HasPrefix(s, Prefix) {
		return false
	}
	for i := 1; i < len(s); i++ {
		if !strings.ContainsRune(alphabet, rune(s[i])) {
			return false
		}
	}
	return true
}
