package auth

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
)

// 头像上传口径（管理员与普通用户共用）。
const (
	// MaxAvatarBytes 头像图片原始字节上限（512 KiB）。前端会在上传前压缩到 256px，
	// 通常远小于该上限；此处作为服务端硬约束，防止超大图片撑爆存储与响应体。
	MaxAvatarBytes = 512 << 10
	// MaxAvatarDataURI 头像 data URI 字符串上限（base64 膨胀约 4/3，留出余量）。
	MaxAvatarDataURI = 1 << 20
)

// ErrInvalidAvatar 头像不合法（非图片、格式不支持或超限）。
var ErrInvalidAvatar = errors.New("头像需为 PNG/JPEG/WebP/GIF 图片，且不超过 512KB")

// allowedAvatarTypes 允许的头像 MIME 类型（同时用于 data URI 声明与内容嗅探校验）。
var allowedAvatarTypes = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/webp": true,
	"image/gif":  true,
}

// NormalizeAvatar 校验并归一化头像。
//
// 头像只能上传图片：入参必须是 data:image/{png,jpeg,webp,gif};base64,<payload>
// 形式的数据 URI（由前端读图后压缩生成）。空串表示「不设置 / 清除头像」，原样返回。
// 仅信任内容嗅探结果，避免用声明头伪装成图片；解码后字节数需在 MaxAvatarBytes 内。
func NormalizeAvatar(dataURI string) (string, error) {
	dataURI = strings.TrimSpace(dataURI)
	if dataURI == "" {
		return "", nil
	}
	if len(dataURI) > MaxAvatarDataURI {
		return "", ErrInvalidAvatar
	}

	comma := strings.IndexByte(dataURI, ',')
	if comma < 0 {
		return "", ErrInvalidAvatar
	}
	meta, payload := dataURI[:comma], dataURI[comma+1:]

	// meta 形如 data:image/png;base64。
	if !strings.HasPrefix(meta, "data:") || !strings.HasSuffix(meta, ";base64") {
		return "", ErrInvalidAvatar
	}
	declared := strings.TrimSuffix(strings.TrimPrefix(meta, "data:"), ";base64")
	if !allowedAvatarTypes[declared] {
		return "", ErrInvalidAvatar
	}

	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil || len(raw) == 0 || len(raw) > MaxAvatarBytes {
		return "", ErrInvalidAvatar
	}
	detected := http.DetectContentType(raw)
	if !allowedAvatarTypes[detected] {
		return "", ErrInvalidAvatar
	}
	// 以嗅探到的真实类型回写，避免声明头与实际内容不一致。
	return "data:" + detected + ";base64," + payload, nil
}
