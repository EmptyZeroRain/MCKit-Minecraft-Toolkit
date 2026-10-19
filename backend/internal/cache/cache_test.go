package cache

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSetGet(t *testing.T) {
	c := New(t.TempDir(), time.Hour)
	if err := c.Set("versions/25w07a.json", []byte(`{"id":"25w07a"}`)); err != nil {
		t.Fatalf("Set 失败: %v", err)
	}
	got, err := c.Get("versions/25w07a.json")
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if !strings.Contains(string(got), "25w07a") {
		t.Fatalf("内容不符: %s", got)
	}
}

func TestExpired(t *testing.T) {
	dir := t.TempDir()
	c := New(dir, time.Millisecond*50)
	if err := c.Set("manifest.json", []byte("data")); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get("manifest.json"); err != nil {
		t.Fatalf("未过期却读取失败: %v", err)
	}

	// 直接改 mtime 模拟过期，避免测试等待真实时间。
	path := filepath.Join(dir, "manifest.json")
	past := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get("manifest.json"); err == nil {
		t.Fatal("过期缓存应返回错误")
	}
	// 惰性删除应把过期文件清掉。
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("过期文件应被删除")
	}
}

func TestPathTraversal(t *testing.T) {
	c := New(t.TempDir(), time.Hour)
	malicious := []string{
		"../../etc/passwd",
		"/etc/passwd",
		"..\\..\\windows",
		"versions/../../secret.json",
		"bad key with space",
		"key<script>",
		".",
	}
	for _, key := range malicious {
		if err := c.Set(key, []byte("x")); err == nil {
			t.Errorf("key %q 应被拒绝", key)
		}
		if _, err := c.Get(key); err == nil {
			t.Errorf("key %q 读取应被拒绝", key)
		}
	}
}

func TestGetMissing(t *testing.T) {
	c := New(t.TempDir(), time.Hour)
	if _, err := c.Get("versions/none.json"); !os.IsNotExist(err) {
		t.Fatalf("不存在的 key 应返回 os.ErrNotExist，实际: %v", err)
	}
}
