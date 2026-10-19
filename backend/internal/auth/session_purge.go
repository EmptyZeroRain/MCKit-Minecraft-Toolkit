package auth

import (
	"context"
	"log/slog"
	"time"
)

// StartSessionPurge 周期清理过期会话，直到 ctx 结束。
//
// 内存实现下避免过期会话长期驻留 map；共享存储下避免 sessions 表无限增长。
// 与 audit.StartRetention 保持一致的「后台循环 + 可关闭」形态。
func StartSessionPurge(ctx context.Context, store SessionStore, interval time.Duration, log *slog.Logger) {
	if store == nil || interval <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				n, err := store.PurgeExpired(ctx)
				if err != nil {
					if log != nil {
						log.Warn("会话过期清理失败", "err", err)
					}
					continue
				}
				if n > 0 && log != nil {
					log.Info("会话过期清理完成", "purged", n)
				}
			}
		}
	}()
}
