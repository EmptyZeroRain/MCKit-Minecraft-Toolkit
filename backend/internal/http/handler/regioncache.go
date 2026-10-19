package handler

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// regionCacheEntry 单个区域文件缓存项。
type regionCacheEntry struct {
	data      []byte
	regionX   int
	regionZ   int
	expiresAt time.Time
	lastSeen  time.Time
}

// regionCache 区域文件短期内存缓存。
//
// 目的：/mc/region/inspect 解析后暂存原始字节，供 /mc/region/chunk 复用，
// 避免用户每查看一个区块都要重传整个 .mca（可达 64MiB）。
//
// 安全与资源边界（刻意的取舍）：
//   - 仅内存、不落盘，进程退出即释放；
//   - 有总量上限与条目上限，超限按最久未使用淘汰；
//   - 条目带 TTL，过期惰性回收；
//   - token 为 16 字节加密随机数，单实例有效（多实例部署需替换为共享存储）。
type regionCache struct {
	mu       sync.Mutex
	entries  map[string]*regionCacheEntry
	maxBytes int64
	curBytes int64
	ttl      time.Duration
	maxItems int
}

// newRegionCache 创建缓存。maxBytes<=0 时默认 256MiB；ttl<=0 时默认 10 分钟。
func newRegionCache(maxBytes int64, ttl time.Duration) *regionCache {
	if maxBytes <= 0 {
		maxBytes = 256 << 20
	}
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &regionCache{
		entries:  make(map[string]*regionCacheEntry),
		maxBytes: maxBytes,
		ttl:      ttl,
		maxItems: 16,
	}
}

// put 缓存区域文件字节，返回一次性 token；数据超上限时返回空串（不缓存）。
func (c *regionCache) put(data []byte, regionX, regionZ int) string {
	if c == nil || len(data) == 0 || int64(len(data)) > c.maxBytes {
		return ""
	}
	tok := newRegionToken()
	if tok == "" {
		return ""
	}
	now := time.Now()

	c.mu.Lock()
	defer c.mu.Unlock()
	c.evictLocked(now)
	c.entries[tok] = &regionCacheEntry{
		data:      data,
		regionX:   regionX,
		regionZ:   regionZ,
		expiresAt: now.Add(c.ttl),
		lastSeen:  now,
	}
	c.curBytes += int64(len(data))
	c.evictLocked(now)
	return tok
}

// get 按 token 取回缓存项；过期或不存在返回 false。
func (c *regionCache) get(tok string) (*regionCacheEntry, bool) {
	if c == nil || tok == "" {
		return nil, false
	}
	now := time.Now()

	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[tok]
	if !ok {
		return nil, false
	}
	if now.After(e.expiresAt) {
		delete(c.entries, tok)
		c.curBytes -= int64(len(e.data))
		return nil, false
	}
	e.lastSeen = now
	return e, true
}

// evictLocked 回收过期项，并在超限时按最久未使用淘汰。调用方须持锁。
func (c *regionCache) evictLocked(now time.Time) {
	for k, e := range c.entries {
		if now.After(e.expiresAt) {
			delete(c.entries, k)
			c.curBytes -= int64(len(e.data))
		}
	}
	for (c.curBytes > c.maxBytes || len(c.entries) > c.maxItems) && len(c.entries) > 0 {
		var oldestKey string
		var oldest time.Time
		first := true
		for k, e := range c.entries {
			if first || e.lastSeen.Before(oldest) {
				oldest, oldestKey, first = e.lastSeen, k, false
			}
		}
		c.curBytes -= int64(len(c.entries[oldestKey].data))
		delete(c.entries, oldestKey)
	}
}

// newRegionToken 生成 16 字节加密随机 token（32 位十六进制）。
func newRegionToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}
