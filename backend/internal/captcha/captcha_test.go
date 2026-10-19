package captcha

import (
	"bytes"
	"context"
	"crypto/sha256"
	"image/png"
	"strings"
	"testing"
	"time"
)

// newTestManager 构造用于测试的管理器。
func newTestManager(t *testing.T, opts Options) *Manager {
	t.Helper()
	return NewManager(opts)
}

func TestIssueReturnsPNG(t *testing.T) {
	m := newTestManager(t, Options{})

	res, err := m.Issue(context.Background())
	if err != nil {
		t.Fatalf("签发失败: %v", err)
	}
	if len(res.ID) != 32 {
		t.Errorf("ID 应为 32 位十六进制，实际 %q", res.ID)
	}
	if res.ExpiresIn <= 0 {
		t.Errorf("有效期应为正，实际 %v", res.ExpiresIn)
	}
	// 校验 PNG magic number，确保返回的是真实图片而非占位。
	if !bytes.HasPrefix(res.Image, []byte{0x89, 'P', 'N', 'G'}) {
		t.Fatalf("图片不是 PNG：% x", res.Image[:min(8, len(res.Image))])
	}
	img, err := png.Decode(bytes.NewReader(res.Image))
	if err != nil {
		t.Fatalf("PNG 解码失败: %v", err)
	}
	if img.Bounds().Dx() < 60 || img.Bounds().Dy() < 20 {
		t.Errorf("图片尺寸过小: %v", img.Bounds())
	}
	// 统计深色像素，确保确实绘制了字形（防止渲染成空白图）。
	dark := 0
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r>>8 < 128 && g>>8 < 128 && b>>8 < 128 {
				dark++
			}
		}
	}
	if dark < 50 {
		t.Fatalf("深色像素过少（%d），图像可能是空白", dark)
	}
}

func TestNewCodeCharsetAndLength(t *testing.T) {
	m := newTestManager(t, Options{Length: 6})
	for i := 0; i < 50; i++ {
		code, err := m.newCode()
		if err != nil {
			t.Fatalf("生成失败: %v", err)
		}
		if len(code) != 6 {
			t.Fatalf("长度应为 6，实际 %q", code)
		}
		for _, ch := range code {
			if !strings.ContainsRune(charset, ch) {
				t.Fatalf("出现字符集外字符 %q（%s）", ch, code)
			}
		}
	}
}

func TestVerifySingleUse(t *testing.T) {
	m := newTestManager(t, Options{})

	// 直接写入已知校验码，避免依赖随机生成值。
	id := "known-id"
	m.remember(id, sha256.Sum256([]byte("ABCD")))

	if !m.Verify(id, "abcd") {
		t.Fatal("正确校验码（大小写不敏感）应通过")
	}
	// 一次性：同一 ID 不能再次通过。
	if m.Verify(id, "ABCD") {
		t.Fatal("已使用过的验证码不应再次通过")
	}
}

func TestVerifyRejectsWrongAnswer(t *testing.T) {
	m := newTestManager(t, Options{})
	m.remember("id-1", sha256.Sum256([]byte("WXYZ")))

	if m.Verify("id-1", "WXYA") {
		t.Fatal("错误校验码不应通过")
	}
	if m.Verify("missing", "WXYZ") {
		t.Fatal("不存在的 ID 不应通过")
	}
	if m.Verify("", "WXYZ") {
		t.Fatal("空 ID 不应通过")
	}
}

func TestVerifyExpiry(t *testing.T) {
	m := newTestManager(t, Options{TTL: time.Minute})
	base := time.Now()
	m.now = func() time.Time { return base }
	m.remember("exp-1", sha256.Sum256([]byte("QRST")))

	m.now = func() time.Time { return base.Add(2 * time.Minute) }
	if m.Verify("exp-1", "QRST") {
		t.Fatal("过期验证码不应通过")
	}
}

func TestVerifyFixedCode(t *testing.T) {
	m := newTestManager(t, Options{FixedCode: "246810"})

	// 固定码模式下忽略 ID。
	if !m.Verify("any-id", "246810") {
		t.Fatal("固定码应通过")
	}
	if m.Verify("any-id", "000000") {
		t.Fatal("非固定码不应通过")
	}
	if !m.Verify("", "246810") {
		t.Fatal("固定码应不依赖 ID")
	}
}

func TestRememberCapsStore(t *testing.T) {
	m := newTestManager(t, Options{MaxStore: 10})
	for i := 0; i < 50; i++ {
		m.remember(strings.Repeat("x", 3)+string(rune('a'+i%26)), sha256.Sum256([]byte("AAAA")))
	}
	m.mu.Lock()
	n := len(m.entries)
	m.mu.Unlock()
	if n > 10 {
		t.Fatalf("内存中的验证码不应超过上限 10，实际 %d", n)
	}
}

func TestVerifyNilManager(t *testing.T) {
	var m *Manager
	if m.Verify("id", "code") {
		t.Fatal("nil 管理器应始终返回 false")
	}
}
