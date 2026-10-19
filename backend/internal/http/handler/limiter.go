package handler

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// keyedLimiter 按业务键（IP / 管理员 ID 等）维护独立的令牌桶。
// 需求 §5.8（链接检查限流）与 §5.10（反馈反垃圾）均要求服务端限流，
// 且不能让 key 数量无界增长，因此这里内置惰性清理。
type keyedLimiter struct {
	mu      sync.Mutex
	limit   rate.Limit
	burst   int
	ttl     time.Duration
	entries map[string]*keyedEntry
}

type keyedEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// newKeyedLimiter 创建按键限流器。limit 为每秒补充令牌数，burst 为桶容量，
// ttl 为闲置多久后回收该键。
func newKeyedLimiter(limit rate.Limit, burst int, ttl time.Duration) *keyedLimiter {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &keyedLimiter{
		limit:   limit,
		burst:   burst,
		ttl:     ttl,
		entries: make(map[string]*keyedEntry),
	}
}

// allow 判断该键是否放行。首次出现时初始化满桶。
func (k *keyedLimiter) allow(key string) bool {
	if k == nil {
		return true
	}
	now := time.Now()

	k.mu.Lock()
	defer k.mu.Unlock()

	// 惰性清理：仅在键数量偏多时回收过期键，避免每次请求全表扫描。
	if len(k.entries) > 1024 {
		for key, e := range k.entries {
			if now.Sub(e.lastSeen) > k.ttl {
				delete(k.entries, key)
			}
		}
	}

	e, ok := k.entries[key]
	if !ok {
		e = &keyedEntry{limiter: rate.NewLimiter(k.limit, k.burst)}
		k.entries[key] = e
	}
	e.lastSeen = now
	return e.limiter.Allow()
}
