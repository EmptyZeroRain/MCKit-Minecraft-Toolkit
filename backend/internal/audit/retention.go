// 审计日志保留策略：按天清理过期记录，避免只增不减。
//
// 与使用统计（internal/service/metrics）保持一致的语义：
// 默认保留 180 天，`AUDIT_RETENTION_DAYS <= 0` 表示关闭清理。
package audit

import (
	"context"
	"log/slog"
	"time"

	"mckit/backend/internal/domain"
)

// PurgeOlderThan 删除 created_at 早于 days 天前的审计记录，返回删除行数。
// days <= 0（关闭保留策略）时不做任何事。
func (s *Service) PurgeOlderThan(ctx context.Context, days int) (int64, error) {
	if s == nil || s.db == nil || days <= 0 {
		return 0, nil
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	res := s.db.WithContext(ctx).Where("created_at < ?", cutoff).Delete(&domain.AuditLog{})
	return res.RowsAffected, res.Error
}

// StartRetention 启动后台保留清理循环：启动即先清理一次，之后每 interval 执行一次。
// ctx 取消即停止；days <= 0 时不启动（保留策略关闭）。
func (s *Service) StartRetention(ctx context.Context, days int, interval time.Duration, log *slog.Logger) {
	if s == nil || s.db == nil || days <= 0 {
		return
	}
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	if log == nil {
		log = slog.Default()
	}

	go func() {
		prune := func() {
			// 单次清理限时，避免启动 / 关闭流程被大表删除卡住。
			cctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			n, err := s.PurgeOlderThan(cctx, days)
			if err != nil {
				log.Warn("审计日志清理失败", "err", err)
				return
			}
			if n > 0 {
				log.Info("审计日志清理完成", "deleted", n, "retention_days", days)
			}
		}
		prune()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				prune()
			}
		}
	}()
}
