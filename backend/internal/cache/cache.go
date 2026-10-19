// Package cache 提供带 TTL 的本地文件缓存（移植自 CreeperCode）。
// 设计决策：使用磁盘文件而非内存缓存，保证服务重启后缓存依然有效，
// 且大量版本 JSON 不占用进程内存。
package cache

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// DefaultTTL 缓存默认有效期 7 天。
const DefaultTTL = 7 * 24 * time.Hour

// ErrCacheExpired 表示缓存存在但已过期。
var ErrCacheExpired = errors.New("cache expired")

// ErrInvalidKey 表示缓存 key 含非法字符（防路径穿越）。
var ErrInvalidKey = errors.New("invalid cache key")

// keyRe 限制 key 每段只含字母数字与 . _ -，杜绝 ../ 穿越与任意文件读写。
var keyRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// Cache 磁盘文件缓存。
type Cache struct {
	root string
	ttl  time.Duration
}

// New 创建缓存实例，root 为缓存根目录，ttl 为有效期。
func New(root string, ttl time.Duration) *Cache {
	return &Cache{root: root, ttl: ttl}
}

// safePath 将 key 解析为 root 下的安全路径。
// key 形如 "manifest.json" 或 "versions/25w07a.json"，
// 任何包含 ..、绝对路径或非法字符的 key 一律拒绝。
func (c *Cache) safePath(key string) (string, error) {
	p := filepath.Clean(key)
	if p == "." || filepath.IsAbs(p) {
		return "", ErrInvalidKey
	}
	for _, seg := range splitPath(p) {
		// ".." 由点组成、能通过下方正则，必须显式拒绝。
		if seg == ".." {
			return "", ErrInvalidKey
		}
		if !keyRe.MatchString(seg) {
			return "", ErrInvalidKey
		}
	}
	return filepath.Join(c.root, p), nil
}

func splitPath(p string) []string {
	return strings.Split(p, string(filepath.Separator))
}

// Get 读取缓存，未命中或已过期返回 os.ErrNotExist。
// 过期文件会被顺带删除（惰性清理，避免后台任务）。
func (c *Cache) Get(key string) ([]byte, error) {
	path, err := c.safePath(key)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if time.Since(info.ModTime()) > c.ttl {
		_ = os.Remove(path)
		return nil, os.ErrNotExist
	}
	return os.ReadFile(path)
}

// Set 写入缓存，使用临时文件加 rename 保证原子性，
// 避免并发读写读到半截 JSON。
func (c *Cache) Set(key string, data []byte) error {
	path, err := c.safePath(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}
