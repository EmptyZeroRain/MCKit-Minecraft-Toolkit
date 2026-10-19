// Package mcversion 封装 Mojang piston-meta API 的访问，
// 包含版本清单（manifest）与单个版本详情的获取，全部走磁盘缓存。
// 移植自 CreeperCode 项目，接口层改为 mckit 的 POST + 统一响应。
package mcversion

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"mckit/backend/internal/cache"
)

// 默认 API 地址，可注入替换以便测试。
const (
	DefaultManifestURL = "https://launchermeta.mojang.com/mc/game/version_manifest.json"
	defaultTimeout     = 15 * time.Second
)

// Client Mojang 元数据客户端。
type Client struct {
	http        *http.Client
	cache       *cache.Cache
	manifestURL string
}

// New 创建客户端。cacheDir 为缓存目录，ttl 为缓存有效期。
func New(cacheDir string, ttl time.Duration) *Client {
	if ttl <= 0 {
		ttl = cache.DefaultTTL
	}
	return &Client{
		http:        &http.Client{Timeout: defaultTimeout},
		cache:       cache.New(cacheDir, ttl),
		manifestURL: DefaultManifestURL,
	}
}

// WithHTTP 注入自定义 HTTP 客户端（测试用）。
func (c *Client) WithHTTP(h *http.Client) *Client {
	c.http = h
	return c
}

// WithManifestURL 注入自定义 manifest 地址（测试用）。
func (c *Client) WithManifestURL(u string) *Client {
	c.manifestURL = u
	return c
}

// Latest 各类型最新版本 id。
type Latest struct {
	Release  string `json:"release"`
	Snapshot string `json:"snapshot"`
}

// Entry 版本清单中的一个条目。
type Entry struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	URL         string    `json:"url"`
	Time        time.Time `json:"time"`
	ReleaseTime time.Time `json:"releaseTime"`
	SHA1        string    `json:"sha1"`
}

// Manifest 版本清单。
type Manifest struct {
	Latest   Latest  `json:"latest"`
	Versions []Entry `json:"versions"`
}

// DownloadInfo 可下载文件的元信息。
type DownloadInfo struct {
	SHA1 string `json:"sha1"`
	Size int64  `json:"size"`
	URL  string `json:"url"`
}

// JavaVersion 版本要求的 Java 运行时信息。
type JavaVersion struct {
	Component    string `json:"component"`
	MajorVersion int    `json:"majorVersion"`
}

// LibraryDownloads 库的下载信息，artifact 为主 jar，
// classifiers 为各平台 natives 变体（仅部分库有）。
type LibraryDownloads struct {
	Artifact    *DownloadInfo            `json:"artifact,omitempty"`
	Classifiers map[string]*DownloadInfo `json:"classifiers,omitempty"`
}

// Library 版本依赖的运行库。
// 极旧版本清单里可能没有 downloads，只有 url 基址，
// 需调用方用 Maven 坐标自行拼接路径。
type Library struct {
	Name      string            `json:"name"`
	BaseURL   string            `json:"url,omitempty"`
	Downloads *LibraryDownloads `json:"downloads,omitempty"`
	Rules     []LibraryRule     `json:"rules,omitempty"`
}

// LibraryRule 规则（库与启动参数通用）：如仅 macOS 允许加载。
type LibraryRule struct {
	Action string `json:"action"`
	OS     *struct {
		Name string `json:"name"`
	} `json:"os,omitempty"`
}

// GameArgument 启动参数元素。
// 1.13+ 的 arguments 数组中元素可能是纯字符串，
// 也可能是带平台规则的对象 {rules, value}，value 又可能是字符串或数组，
// 故用自定义 UnmarshalJSON 归一化为 Values。
type GameArgument struct {
	Rules  []LibraryRule
	Values []string
}

// UnmarshalJSON 兼容字符串与 {rules, value} 两种形态。
func (a *GameArgument) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		a.Values = []string{s}
		return nil
	}
	var obj struct {
		Rules []LibraryRule   `json:"rules"`
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	a.Rules = obj.Rules
	a.Values = parseArgValue(obj.Value)
	return nil
}

// parseArgValue 解析 value 字段的 string | []string 两种形态。
func parseArgValue(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return []string{s}
	}
	var arr []string
	if json.Unmarshal(raw, &arr) == nil {
		return arr
	}
	return nil
}

// VersionArguments 1.13+ 版本的启动参数。
// 除标准的 game/jvm 外，新版快照还携带 default-user-jvm
// （官方启动器的默认用户 JVM 参数，如内存与 GC 调优），结构与 jvm 相同。
type VersionArguments struct {
	Game           []GameArgument `json:"game"`
	JVM            []GameArgument `json:"jvm"`
	DefaultUserJVM []GameArgument `json:"default-user-jvm,omitempty"`
}

// VersionDetail 是 piston-meta 版本 JSON 中我们关心的字段子集。
// 旧版本（old_beta/old_alpha）可能缺少 mappings 与 server 下载，
// 故 downloads 用 map 且指针/存在性检查兜底。
type VersionDetail struct {
	ID          string                  `json:"id"`
	Type        string                  `json:"type"`
	Time        time.Time               `json:"time"`
	ReleaseTime time.Time               `json:"releaseTime"`
	MainClass   string                  `json:"mainClass"`
	JavaVersion *JavaVersion            `json:"javaVersion,omitempty"`
	Downloads   map[string]DownloadInfo `json:"downloads"`
	Libraries   []Library               `json:"libraries,omitempty"`
	// MinecraftArguments 1.13 之前的启动参数字符串（空格分隔）。
	MinecraftArguments string `json:"minecraftArguments,omitempty"`
	// Arguments 1.13+ 的结构化启动参数。
	Arguments *VersionArguments `json:"arguments,omitempty"`
}

// FetchManifest 获取版本清单，优先读缓存。
func (c *Client) FetchManifest(ctx context.Context) (*Manifest, error) {
	if data, err := c.cache.Get("manifest.json"); err == nil {
		var m Manifest
		if json.Unmarshal(data, &m) == nil {
			return &m, nil
		}
		// 缓存内容损坏时降级为网络获取。
	}
	data, err := c.fetch(ctx, c.manifestURL)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("解析 manifest 失败: %w", err)
	}
	_ = c.cache.Set("manifest.json", data)
	return &m, nil
}

// FetchVersion 获取单个版本详情，优先读缓存。
func (c *Client) FetchVersion(ctx context.Context, id string) (*VersionDetail, error) {
	m, err := c.FetchManifest(ctx)
	if err != nil {
		return nil, err
	}
	var entry *Entry
	for i := range m.Versions {
		if m.Versions[i].ID == id {
			entry = &m.Versions[i]
			break
		}
	}
	if entry == nil {
		return nil, fmt.Errorf("版本 %s 不存在", id)
	}
	if entry.URL == "" {
		return nil, fmt.Errorf("版本 %s 缺少详情地址", id)
	}

	key := "versions/" + id + ".json"
	if data, err := c.cache.Get(key); err == nil {
		var d VersionDetail
		if json.Unmarshal(data, &d) == nil {
			return &d, nil
		}
	}
	data, err := c.fetch(ctx, entry.URL)
	if err != nil {
		return nil, err
	}
	var d VersionDetail
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("解析版本 %s 详情失败: %w", id, err)
	}
	_ = c.cache.Set(key, data)
	return &d, nil
}

// fetch 发起 GET 请求并读取响应体。
func (c *Client) fetch(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 Mojang API 失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Mojang API 返回 %d: %s", resp.StatusCode, url)
	}
	// 限制响应体大小，防御异常响应撑爆内存（manifest 约 1MB，详情约 500KB）。
	return io.ReadAll(io.LimitReader(resp.Body, 64<<20))
}
