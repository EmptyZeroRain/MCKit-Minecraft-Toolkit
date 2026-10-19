// Package linkcheck 提供资源链接可达性检查（需求 §5.8），带 SSRF 防护。
package linkcheck

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Result 链接检查结果。
type Result struct {
	StatusCode int
	Status     string // ok | redirected | blocked | broken | error
	FinalURL   string
}

// ErrUnsafeURL 链接不满足 SSRF 防护要求。
var ErrUnsafeURL = errors.New("链接不安全，已拒绝检查")

// DefaultMaxConcurrency 批量链接检查的默认并发上限。
const DefaultMaxConcurrency = 4

// MaxConcurrencyLimit 并发上限的硬顶，防止误配置把目标站打爆。
const MaxConcurrencyLimit = 16

// NormalizeConcurrency 归一化并发数：<=0 取默认值，超过硬顶则截断。
func NormalizeConcurrency(n int) int {
	if n <= 0 {
		return DefaultMaxConcurrency
	}
	if n > MaxConcurrencyLimit {
		return MaxConcurrencyLimit
	}
	return n
}

// BatchItem 批量检查中单条 URL 的结果。
type BatchItem struct {
	URL    string
	Result *Result
	Err    error
}

// CheckMany 以**受限并发**检查多个 URL，返回结果与输入顺序一一对应。
//
// 并发限制的意义：链接检查是出网请求，无限并发既可能被目标站限流 / 封禁，
// 也可能耗尽本机连接与内存。这里用固定大小的信号量把并发压到 concurrency
// （经 NormalizeConcurrency 归一），并通过派生 context 保证整体可取消。
func (c *Checker) CheckMany(ctx context.Context, urls []string, concurrency int) []BatchItem {
	items := make([]BatchItem, len(urls))
	if len(urls) == 0 {
		return items
	}
	limit := NormalizeConcurrency(concurrency)

	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for i, u := range urls {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, rawURL string) {
			defer wg.Done()
			defer func() { <-sem }()
			res, err := c.Check(ctx, rawURL)
			items[idx] = BatchItem{URL: rawURL, Result: res, Err: err}
		}(i, u)
	}
	wg.Wait()
	return items
}

// Checker 链接检查器。
type Checker struct {
	client       *http.Client
	maxRedirects int
	// AllowPrivateIP 仅用于单元测试（临时允许回环/私有地址）。
	// 生产环境必须保持 false。
	AllowPrivateIP bool
}

// NewChecker 创建检查器。timeout 为单次请求超时，maxRedirects 为重定向上限。
// 禁用了 http.Client 的自动重定向，由 Check 手动逐跳校验目标 IP（防 SSRF）。
func NewChecker(timeout time.Duration, maxRedirects int) *Checker {
	if maxRedirects < 0 {
		maxRedirects = 3
	}
	client := &http.Client{
		Timeout: timeout,
		// 不自动跟随，让每跳重定向在 Check 中重新做 SSRF 校验。
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return &Checker{
		client:       client,
		maxRedirects: maxRedirects,
	}
}

// Check 检查 URL 可达性。执行顺序：
//  1. 协议校验（仅 http/https）
//  2. 拒绝带用户信息的 URL
//  3. 首次请求前解析并校验目标主机 IP
//  4. 手动跟随重定向，每次跳转前重新校验
//  5. 限制响应体大小，不下载完整文件
func (c *Checker) Check(ctx context.Context, rawURL string) (*Result, error) {
	if err := c.validateURL(rawURL); err != nil {
		return nil, err
	}

	current := rawURL
	for redirect := 0; redirect <= c.maxRedirects; redirect++ {
		if err := c.validateHostname(current); err != nil {
			return nil, err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodHead, current, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "MCToolSite-LinkCheck/1.0 (+https://example.com)")

		resp, err := c.client.Do(req)
		if err != nil {
			// HEAD 不支持时尝试 GET（只读响应头）。
			req2, _ := http.NewRequestWithContext(ctx, http.MethodGet, current, nil)
			req2.Header.Set("User-Agent", req.Header.Get("User-Agent"))
			if resp2, err2 := c.client.Do(req2); err2 == nil {
				resp = resp2
				err = nil
			} else {
				return &Result{Status: "error", StatusCode: 0}, err
			}
		}

		// 限制读取响应体，防内存放大。
		_, readErr := io.CopyN(io.Discard, resp.Body, 1<<20) // 1 MiB
		resp.Body.Close()

		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			loc := resp.Header.Get("Location")
			next, uerr := c.resolveRedirect(current, loc)
			if uerr != nil {
				return &Result{Status: "error", StatusCode: resp.StatusCode, FinalURL: current}, uerr
			}
			if resp.StatusCode == http.StatusMovedPermanently ||
				resp.StatusCode == http.StatusFound ||
				resp.StatusCode == http.StatusSeeOther ||
				resp.StatusCode == http.StatusTemporaryRedirect ||
				resp.StatusCode == http.StatusPermanentRedirect {
				current = next
				continue
			}
		}

		if readErr != nil && readErr != io.EOF {
			return &Result{Status: "error", StatusCode: resp.StatusCode, FinalURL: current}, readErr
		}

		return classify(resp.StatusCode, current), nil
	}
	return &Result{Status: "blocked", StatusCode: 0, FinalURL: current}, errors.New("重定向次数超过限制")
}

// validateURL 校验协议并拒绝用户信息。
func (c *Checker) validateURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("URL 解析失败: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("仅允许 http/https 协议")
	}
	if u.User != nil {
		return errors.New("URL 不允许包含用户名或密码")
	}
	if u.Host == "" {
		return errors.New("URL 缺少主机")
	}
	return nil
}

// validateHostname 校验主机解析后的 IP 不在禁止范围。
func (c *Checker) validateHostname(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	host := u.Hostname()

	// 显式禁止 localhost 与常见内网域名。
	lower := strings.ToLower(host)
	for _, banned := range []string{"localhost", "metadata.google.internal", "169.254.169.254"} {
		if lower == banned {
			return ErrUnsafeURL
		}
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("DNS 解析失败: %w", err)
	}
	for _, ip := range ips {
		if !c.AllowPrivateIP && unsafeIP(ip) {
			return ErrUnsafeURL
		}
	}
	return nil
}

// unsafeIP 判断 IP 是否属于回环/私有/链路本地/保留/云元数据地址。
func unsafeIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}
	// 云元数据地址 169.254.169.254 归入链路本地，已覆盖。
	return false
}

// resolveRedirect 解析重定向地址并再次校验协议。
func (c *Checker) resolveRedirect(base, location string) (string, error) {
	ref, err := url.Parse(location)
	if err != nil {
		return "", fmt.Errorf("重定向地址解析失败: %w", err)
	}
	baseURL, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	resolved := baseURL.ResolveReference(ref)
	if resolved.Scheme != "http" && resolved.Scheme != "https" {
		return "", errors.New("拒绝非 http/https 重定向")
	}
	if resolved.User != nil {
		return "", errors.New("重定向地址不允许包含用户信息")
	}
	return resolved.String(), nil
}

// classify 将 HTTP 状态码映射为检查状态。
func classify(status int, finalURL string) *Result {
	statusStr := ""
	switch {
	case status >= 200 && status < 300:
		statusStr = "ok"
	case status >= 400 && status < 500:
		statusStr = "broken"
	case status >= 500:
		statusStr = "error"
	default:
		statusStr = "error"
	}
	return &Result{StatusCode: status, Status: statusStr, FinalURL: finalURL}
}
