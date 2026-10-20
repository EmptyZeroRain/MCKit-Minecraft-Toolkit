package auth

import (
	"crypto/sha256"
	"encoding/hex"
)

// sessionFingerprintHexLength 会话展示指纹长度（SHA-256 前 16 位十六进制，64 bit）。
//
// 会话 ID 是 Cookie 中的 bearer 凭据，后台列表不得回显原文；指纹仅用于辨认与
// 撤销定位。16 位在百万级会话下碰撞概率约 2.7e-8，远低于 12 位的 0.18%。
const sessionFingerprintHexLength = 16

// SessionFingerprint 返回会话 ID 的不可逆展示指纹。
func SessionFingerprint(sessionID string) string {
	sum := sha256.Sum256([]byte(sessionID))
	return hex.EncodeToString(sum[:])[:sessionFingerprintHexLength]
}
