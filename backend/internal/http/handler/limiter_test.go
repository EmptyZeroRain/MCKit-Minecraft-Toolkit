package handler

import (
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// TestKeyedLimiterBurst 覆盖需求 §5.8 / §5.10：按键限流在突发额度用尽后拒绝。
func TestKeyedLimiterBurst(t *testing.T) {
	l := newKeyedLimiter(rate.Every(time.Hour), 5, time.Minute)

	for i := 0; i < 5; i++ {
		if !l.allow("ip-1") {
			t.Fatalf("第 %d 次应放行", i+1)
		}
	}
	if l.allow("ip-1") {
		t.Fatal("超出突发额度应拒绝")
	}
	// 不同键互不影响
	if !l.allow("ip-2") {
		t.Fatal("不同键应独立计数")
	}
}

// TestKeyedLimiterNilSafe nil 限流器应全部放行（未配置时的降级）。
func TestKeyedLimiterNilSafe(t *testing.T) {
	var l *keyedLimiter
	if !l.allow("any") {
		t.Fatal("nil 限流器应放行")
	}
}
