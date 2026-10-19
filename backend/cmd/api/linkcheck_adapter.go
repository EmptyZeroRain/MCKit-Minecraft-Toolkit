package main

import (
	"context"
	"errors"
	"time"

	"mckit/backend/internal/domain"
	"mckit/backend/internal/http/handler"
	lk "mckit/backend/internal/linkcheck"
)

// linkCheckAdapter 将 linkcheck.Checker 适配为 handler.linkChecker 接口。
type linkCheckAdapter struct {
	checker *lk.Checker
}

// CheckURL 检查 URL 可达性，返回带分类的结果。
// - 不安全 URL（SSRF 拦截）：Status = blocked
// - 网络/解析失败：Status = error
// - 正常：Status 为 ok / redirected / broken / error
func (a linkCheckAdapter) CheckURL(rawURL string) (handler.LinkCheckResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	res, err := a.checker.Check(ctx, rawURL)
	if err != nil {
		if errors.Is(err, lk.ErrUnsafeURL) {
			return handler.LinkCheckResult{Status: domain.LinkStatusBlocked}, err
		}
		if res != nil && res.Status != "" {
			return handler.LinkCheckResult{
				Status:     res.Status,
				StatusCode: res.StatusCode,
				FinalURL:   res.FinalURL,
			}, err
		}
		return handler.LinkCheckResult{Status: domain.LinkStatusError}, err
	}
	return handler.LinkCheckResult{
		Status:     res.Status,
		StatusCode: res.StatusCode,
		FinalURL:   res.FinalURL,
	}, nil
}
