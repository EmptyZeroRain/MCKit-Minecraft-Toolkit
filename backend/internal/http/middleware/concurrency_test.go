package middleware

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// TestConcurrencyLimit 并发上限：占满后新请求返回 429，释放后可再次通过。
func TestConcurrencyLimit(t *testing.T) {
	release := make(chan struct{})
	entered := make(chan struct{}, 4)
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		entered <- struct{}{}
		<-release
		w.WriteHeader(http.StatusOK)
	})
	h := ConcurrencyLimit(1)(next)

	// 第一个请求占用信号量。
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/mc/structures", nil))
	}()
	<-entered

	// 第二个请求应被拒绝。
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mc/structures", nil))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("并发占满时应返回 429，实际 %d", rec.Code)
	}

	// 释放后再次请求应通过。
	close(release)
	wg.Wait()
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, httptest.NewRequest(http.MethodPost, "/mc/structures", nil))
	if rec2.Code != http.StatusOK {
		t.Fatalf("释放后应返回 200，实际 %d", rec2.Code)
	}
}

// TestConcurrencyLimitDisabled max<=0 时不限制。
func TestConcurrencyLimitDisabled(t *testing.T) {
	h := ConcurrencyLimit(0)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mc/structures", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("禁用限流应返回 200，实际 %d", rec.Code)
	}
}
