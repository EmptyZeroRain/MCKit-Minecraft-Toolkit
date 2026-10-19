// Package captcha 提供注册用图形验证码：服务端生成 PNG 图片与一次性校验码。
//
// 设计边界：
//   - 仅内存存储（与 MemorySessionStore 一致，适用于单实例部署），不落库、不落盘；
//   - 只保存校验码的 SHA-256 摘要，不保留明文；校验后立即失效（一次性）；
//   - 随机源使用 crypto/rand，比较使用常量时间实现，避免可预测与计时侧信道；
//   - 字符集剔除 0/O、1/I/L 等易混淆字符，降低人工识别成本。
package captcha

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/opentype"
)

const (
	// defaultLength 默认字符数。
	defaultLength = 4
	// defaultTTL 默认有效期。
	defaultTTL = 5 * time.Minute
	// defaultMaxStore 内存中同时保留的验证码上限。
	defaultMaxStore = 5000
	// charset 可选字符集：剔除 0/O、1/I/L 等易混淆字符。
	charset = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
	// fontSize 字号（pt）；DPI 取 72 时 1pt ≈ 1px。
	fontSize = 30
)

// Options 验证码参数；零值字段取默认值。
type Options struct {
	// Length 字符数（3-8，越界取默认 4）。
	Length int
	// TTL 有效期（<=0 取默认 5 分钟）。
	TTL time.Duration
	// MaxStore 内存中同时保留的验证码上限（<=0 取默认 5000）。
	MaxStore int
	// FixedCode 非空时始终使用该验证码，且校验忽略 ID；
	// 仅供自动化测试使用，生产环境必须为空，否则验证码形同虚设。
	FixedCode string
}

// Manager 图形验证码管理器（仅内存存储）。
type Manager struct {
	opts    Options
	face    font.Face
	mu      sync.Mutex
	entries map[string]entry
	// now 可替换的时间源，便于测试过期行为。
	now func() time.Time
}

// entry 单条验证码记录：只存摘要与到期时间。
type entry struct {
	sum       [sha256.Size]byte
	expiresAt time.Time
}

// IssueResult 一次签发结果。
type IssueResult struct {
	ID        string
	Image     []byte
	ExpiresIn time.Duration
}

// NewManager 创建验证码管理器。
//
// 字体优先使用内置 Go Mono（字号可调、抗锯齿）；解析异常时回退到基础位图字体，
// 不改变随机性与抗识别措施，也避免出现「已启用验证码却无法初始化」的失败开放路径。
func NewManager(opts Options) *Manager {
	if opts.Length < 3 || opts.Length > 8 {
		opts.Length = defaultLength
	}
	if opts.TTL <= 0 {
		opts.TTL = defaultTTL
	}
	if opts.MaxStore <= 0 {
		opts.MaxStore = defaultMaxStore
	}
	return &Manager{
		opts:    opts,
		face:    loadFace(),
		entries: make(map[string]entry),
		now:     time.Now,
	}
}

// loadFace 加载绘制字体，异常时回退基础位图字体。
func loadFace() font.Face {
	f, err := opentype.Parse(gomono.TTF)
	if err != nil {
		return basicfont.Face7x13
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{
		Size:    fontSize,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return basicfont.Face7x13
	}
	return face
}

// Issue 生成一张验证码图片并记录其校验码摘要。
func (m *Manager) Issue(_ context.Context) (IssueResult, error) {
	code, err := m.newCode()
	if err != nil {
		return IssueResult{}, err
	}
	id, err := newID()
	if err != nil {
		return IssueResult{}, err
	}
	img, err := m.render(code)
	if err != nil {
		return IssueResult{}, err
	}
	m.remember(id, sha256.Sum256([]byte(code)))
	return IssueResult{ID: id, Image: img, ExpiresIn: m.opts.TTL}, nil
}

// Verify 校验答案；无论对错该验证码都立即失效（一次性）。
func (m *Manager) Verify(id, answer string) bool {
	if m == nil {
		return false
	}
	answer = strings.ToUpper(strings.TrimSpace(answer))
	if m.opts.FixedCode != "" {
		// 测试钩子：只比较固定码，忽略 ID。
		return subtle.ConstantTimeCompare([]byte(answer), []byte(strings.ToUpper(m.opts.FixedCode))) == 1
	}
	if id == "" || answer == "" {
		return false
	}
	m.mu.Lock()
	e, ok := m.entries[id]
	if ok {
		delete(m.entries, id)
	}
	m.mu.Unlock()
	if !ok || m.now().After(e.expiresAt) {
		return false
	}
	sum := sha256.Sum256([]byte(answer))
	return subtle.ConstantTimeCompare(sum[:], e.sum[:]) == 1
}

// newCode 生成随机验证码；配置了 FixedCode 时直接返回该值。
func (m *Manager) newCode() (string, error) {
	if m.opts.FixedCode != "" {
		return strings.ToUpper(m.opts.FixedCode), nil
	}
	buf := make([]byte, m.opts.Length)
	limit := big.NewInt(int64(len(charset)))
	for i := range buf {
		n, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", fmt.Errorf("生成验证码失败: %w", err)
		}
		buf[i] = charset[n.Int64()]
	}
	return string(buf), nil
}

// remember 记录校验码摘要，并顺带清理过期项、限制总容量。
func (m *Manager) remember(id string, sum [sha256.Size]byte) {
	now := m.now()
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, e := range m.entries {
		if now.After(e.expiresAt) {
			delete(m.entries, k)
		}
	}
	// 容量兜底：被刷爆时随机淘汰，避免内存无界增长。
	for len(m.entries) >= m.opts.MaxStore {
		for k := range m.entries {
			delete(m.entries, k)
			break
		}
	}
	m.entries[id] = entry{sum: sum, expiresAt: now.Add(m.opts.TTL)}
}

// newID 生成不可预测的验证码 ID。
func newID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("生成验证码 ID 失败: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
