package auth

import (
	"strings"
	"testing"
	"time"
)

func TestHashAndVerify(t *testing.T) {
	hash, err := HashPassword("correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("哈希失败: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Fatalf("哈希前缀异常: %s", hash)
	}
	if err := VerifyPassword(hash, "correct-horse-battery-staple"); err != nil {
		t.Fatalf("正确密码应通过: %v", err)
	}
	if err := VerifyPassword(hash, "wrong"); err != ErrInvalidPassword {
		t.Fatalf("错误密码应返回 ErrInvalidPassword，实际 %v", err)
	}
}

// 同一密码每次哈希结果应不同（随机盐）。
func TestHashIsSalted(t *testing.T) {
	a, _ := HashPassword("same-password")
	b, _ := HashPassword("same-password")
	if a == b {
		t.Fatal("同一密码两次哈希不应相同")
	}
}

func TestVerifyInvalidFormat(t *testing.T) {
	if err := VerifyPassword("not-a-valid-format", "x"); err == nil {
		t.Fatal("非法哈希格式应报错")
	}
}

func TestLoginThrottle(t *testing.T) {
	th := NewLoginThrottle(3, 3600*time.Second)
	for i := 0; i < 2; i++ {
		if th.IsLocked("admin", "1.2.3.4") {
			t.Fatalf("第 %d 次不应锁定", i+1)
		}
		th.RecordFailure("admin", "1.2.3.4")
	}
	// 第 3 次失败达到阈值
	th.RecordFailure("admin", "1.2.3.4")
	if !th.IsLocked("admin", "1.2.3.4") {
		t.Fatal("应已锁定")
	}
	// 相同用户不同 IP 不受影响
	if th.IsLocked("admin", "5.6.7.8") {
		t.Fatal("不同 IP 不应被锁定")
	}
	// 成功清零
	th.RecordSuccess("admin", "1.2.3.4")
	if th.IsLocked("admin", "1.2.3.4") {
		t.Fatal("成功后应解锁")
	}
}