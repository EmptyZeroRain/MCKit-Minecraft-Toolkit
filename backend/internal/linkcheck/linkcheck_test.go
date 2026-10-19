package linkcheck

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func newTestChecker() *Checker {
	c := NewChecker(3*time.Second, 3)
	c.AllowPrivateIP = true // 仅测试：httptest server 监听回环地址
	return c
}

// 严格检查器：生产同款配置，用于 SSRF 拦截断言。
func newStrictChecker() *Checker {
	return NewChecker(3*time.Second, 3)
}

func TestRejectNonHTTP(t *testing.T) {
	c := newStrictChecker()
	for _, u := range []string{"ftp://example.com/x", "file:///etc/passwd", "javascript:alert(1)", ""} {
		if _, err := c.Check(context.Background(), u); err == nil {
			t.Fatalf("URL %q 应被拒绝", u)
		}
	}
}

func TestRejectUserInfo(t *testing.T) {
	c := newTestChecker()
	_, err := c.Check(context.Background(), "http://user:pass@example.com/x")
	if err == nil || !errors.Is(err, ErrUnsafeURL) && !strings.Contains(err.Error(), "用户名") {
		t.Fatalf("带用户信息的 URL 应被拒绝，实际: %v", err)
	}
}

func TestRejectPrivateIP(t *testing.T) {
	c := newStrictChecker()
	for _, u := range []string{
		"http://127.0.0.1/x",
		"http://10.0.0.1/x",
		"http://192.168.1.1/x",
		"http://172.16.0.1/x",
		"http://169.254.169.254/latest/meta-data",
		"http://localhost/x",
	} {
		_, err := c.Check(context.Background(), u)
		if err == nil {
			t.Fatalf("URL %q 应触发 SSRF 拦截", u)
		}
	}
}

func TestCheckOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newTestChecker()
	res, err := c.Check(context.Background(), srv.URL+"/ok")
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if res.Status != "ok" {
		t.Fatalf("期望 ok，实际 %v", res.Status)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("期望 200，实际 %d", res.StatusCode)
	}
}

func TestCheckRedirect(t *testing.T) {
	var target *httptest.Server
	target = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/final", http.StatusFound)
	}))
	defer srv.Close()

	c := newTestChecker()
	res, err := c.Check(context.Background(), srv.URL+"/start")
	if err != nil {
		t.Fatalf("重定向后不应报错: %v", err)
	}
	// 重定向后最终应为 ok，且 FinalURL 指向 target
	if res.Status != "ok" {
		t.Fatalf("重定向后期望 ok，实际 %v", res.Status)
	}
	if !strings.HasPrefix(res.FinalURL, target.URL) {
		t.Fatalf("FinalURL 应为重定向目标，实际 %s", res.FinalURL)
	}
}

func TestRedirectToBlockedInternal(t *testing.T) {
	// 目标跳转到内网地址应被拦截
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/latest/meta-data", http.StatusFound)
	}))
	defer srv.Close()

	c := newTestChecker()
	_, err := c.Check(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("重定向到云元数据地址应被拦截")
	}
}

func TestBrokenLink(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := newTestChecker()
	res, err := c.Check(context.Background(), srv.URL+"/404")
	if err != nil {
		t.Fatalf("404 不应报网络错误: %v", err)
	}
	if res.Status != "broken" {
		t.Fatalf("期望 broken，实际 %v", res.Status)
	}
}
func TestNormalizeConcurrency(t *testing.T) {
	cases := []struct{ in, want int }{
		{0, DefaultMaxConcurrency},
		{-5, DefaultMaxConcurrency},
		{1, 1},
		{4, 4},
		{16, 16},
		{99, MaxConcurrencyLimit},
	}
	for _, tc := range cases {
		if got := NormalizeConcurrency(tc.in); got != tc.want {
			t.Fatalf("NormalizeConcurrency(%d)=%d，期望 %d", tc.in, got, tc.want)
		}
	}
}

// TestCheckManyConcurrencyBound 断言批量检查不会超过设定的并发上限。
func TestCheckManyConcurrencyBound(t *testing.T) {
	var mu sync.Mutex
	var inFlight, peak int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		inFlight++
		if inFlight > peak {
			peak = inFlight
		}
		mu.Unlock()

		time.Sleep(30 * time.Millisecond)

		mu.Lock()
		inFlight--
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	const limit = 2
	urls := make([]string, 8)
	for i := range urls {
		urls[i] = srv.URL + "/x"
	}

	c := newTestChecker()
	items := c.CheckMany(context.Background(), urls, limit)
	if len(items) != len(urls) {
		t.Fatalf("结果数应为 %d，实际 %d", len(urls), len(items))
	}
	for i, it := range items {
		if it.Err != nil {
			t.Fatalf("第 %d 条不应报错: %v", i, it.Err)
		}
		if it.Result == nil || it.Result.Status != "ok" {
			t.Fatalf("第 %d 条期望 ok，实际 %+v", i, it.Result)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if peak > limit {
		t.Fatalf("并发峰值 %d 超过上限 %d", peak, limit)
	}
	if peak < 2 {
		t.Fatalf("并发峰值 %d 过低，未真正并发", peak)
	}
}

// TestCheckManyEmpty 空输入返回空结果且不阻塞。
func TestCheckManyEmpty(t *testing.T) {
	c := newTestChecker()
	if items := c.CheckMany(context.Background(), nil, 4); len(items) != 0 {
		t.Fatalf("空输入应返回空结果，实际 %d", len(items))
	}
}
